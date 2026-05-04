package agentdriver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
	"github.com/m0n0x41d/haft/internal/agentserver"
	"github.com/m0n0x41d/haft/internal/agentstore"
)

// Dispatcher implements agentserver.Dispatcher with a real agent driver.
// It handles the session lifecycle commands the StoreDispatcher already
// covers AND also turn.submit, permission.respond, model.set, turn.cancel
// — the commands that need the LLM in the loop.
//
// Concurrency: TurnSubmit launches a turn on a goroutine and returns
// immediately with an "accepted" response; the actual events stream over
// SSE as the driver progresses. One in-flight turn per session is enforced
// by agentcore.StartTurn, which rejects ErrTurnAlreadyRunning.
type Dispatcher struct {
	Store *agentstore.Store
	Hub   *agentserver.Hub

	Driver      *Driver
	Permissions *PermissionGate

	IDGen func() agentcore.SessionID
	Now   func() time.Time

	mu      sync.Mutex
	running map[agentcore.SessionID]context.CancelFunc
}

// NewDispatcher constructs a Dispatcher with the given dependencies.
// Driver.Sink is overwritten to use this dispatcher's CombinedSink so
// events flow through one canonical path.
func NewDispatcher(store *agentstore.Store, hub *agentserver.Hub, driver *Driver, perms *PermissionGate) *Dispatcher {
	d := &Dispatcher{
		Store:       store,
		Hub:         hub,
		Driver:      driver,
		Permissions: perms,
		IDGen:       func() agentcore.SessionID { return "" },
		Now:         time.Now,
		running:     map[agentcore.SessionID]context.CancelFunc{},
	}
	d.Driver.Sink = &CombinedSink{
		Store: store,
		Broadcast: func(ev agentproto.AgentEvent) {
			hub.Publish(ev)
		},
	}
	d.Driver.Permissions = perms
	return d
}

// Dispatch routes the typed command. Session lifecycle commands fall back
// to the embedded StoreDispatcher behavior; turn-driving commands invoke
// the driver.
func (d *Dispatcher) Dispatch(ctx context.Context, cmd agentproto.RPCCommand) (agentserver.DispatchResult, error) {
	if d.Store == nil || d.Hub == nil || d.Driver == nil {
		return agentserver.DispatchResult{}, errors.New("agentdriver: Dispatcher missing fields")
	}
	switch c := cmd.(type) {
	case agentproto.SessionCreateCmd:
		return d.handleSessionCreate(c)
	case agentproto.SessionListCmd:
		return d.handleSessionList(c)
	case agentproto.SessionResumeCmd:
		return d.handleSessionResume(c)
	case agentproto.SessionRenameCmd:
		return d.handleSessionRename(c)
	case agentproto.TurnSubmitCmd:
		return d.handleTurnSubmit(ctx, c)
	case agentproto.TurnCancelCmd:
		return d.handleTurnCancel(c)
	case agentproto.PermissionRespondCmd:
		return d.handlePermissionRespond(c)
	case agentproto.ModelSetCmd:
		return d.handleModelSet(c)
	}
	return agentserver.DispatchResult{}, fmt.Errorf("%w: %s", agentserver.ErrUnsupportedCommand, cmd.Kind())
}

func (d *Dispatcher) handleSessionCreate(c agentproto.SessionCreateCmd) (agentserver.DispatchResult, error) {
	id := d.IDGen()
	if id == "" {
		return agentserver.DispatchResult{}, errors.New("agentdriver: IDGen returned empty session id")
	}
	now := d.Now()
	if _, err := d.Store.Create(id, c.ProjectID, c.Title, c.Model, now); err != nil {
		return agentserver.DispatchResult{}, err
	}
	created := agentproto.SessionCreatedEvent{}
	created.SessionID = id
	created.At = now
	created.Title = c.Title
	created.Model = c.Model
	return agentserver.DispatchResult{
		SessionID: id,
		Events:    []agentproto.AgentEvent{created},
		Response:  map[string]any{"session_id": id},
	}, nil
}

func (d *Dispatcher) handleSessionList(c agentproto.SessionListCmd) (agentserver.DispatchResult, error) {
	metas, err := d.Store.List(c.ProjectID)
	if err != nil {
		return agentserver.DispatchResult{}, err
	}
	return agentserver.DispatchResult{Response: metas}, nil
}

func (d *Dispatcher) handleSessionResume(c agentproto.SessionResumeCmd) (agentserver.DispatchResult, error) {
	session, err := d.Store.Load(c.SessionID)
	if err != nil {
		return agentserver.DispatchResult{}, err
	}
	return agentserver.DispatchResult{Response: session, SessionID: c.SessionID}, nil
}

func (d *Dispatcher) handleSessionRename(c agentproto.SessionRenameCmd) (agentserver.DispatchResult, error) {
	now := d.Now()
	rename := agentproto.SessionUpdatedEvent{}
	rename.SessionID = c.SessionID
	rename.At = now
	rename.Title = c.Title
	if err := d.Store.Append(c.SessionID, rename); err != nil {
		return agentserver.DispatchResult{}, err
	}
	return agentserver.DispatchResult{
		SessionID: c.SessionID,
		Events:    []agentproto.AgentEvent{rename},
	}, nil
}

func (d *Dispatcher) handleTurnSubmit(ctx context.Context, c agentproto.TurnSubmitCmd) (agentserver.DispatchResult, error) {
	session, err := d.Store.Load(c.SessionID)
	if err != nil {
		return agentserver.DispatchResult{}, err
	}

	turnCtx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	if existing, ok := d.running[c.SessionID]; ok {
		// Cancel the previous turn before starting a new one. agentcore
		// also rejects concurrent turns at append time, but we cancel
		// proactively to free the goroutine.
		existing()
	}
	d.running[c.SessionID] = cancel
	d.mu.Unlock()

	go func() {
		defer func() {
			d.mu.Lock()
			delete(d.running, c.SessionID)
			d.mu.Unlock()
			cancel()
		}()
		_, _ = d.Driver.Drive(turnCtx, session, c.Text)
	}()

	return agentserver.DispatchResult{
		SessionID: c.SessionID,
		Response: map[string]any{
			"session_id": c.SessionID,
			"accepted":   true,
		},
	}, nil
}

func (d *Dispatcher) handleTurnCancel(c agentproto.TurnCancelCmd) (agentserver.DispatchResult, error) {
	d.mu.Lock()
	cancel, ok := d.running[c.SessionID]
	d.mu.Unlock()
	if !ok {
		return agentserver.DispatchResult{}, fmt.Errorf("no running turn on session %s", c.SessionID)
	}
	cancel()
	return agentserver.DispatchResult{SessionID: c.SessionID}, nil
}

func (d *Dispatcher) handlePermissionRespond(c agentproto.PermissionRespondCmd) (agentserver.DispatchResult, error) {
	if d.Permissions == nil {
		return agentserver.DispatchResult{}, errors.New("agentdriver: PermissionGate not configured")
	}
	if err := d.Permissions.Resolve(c.PermissionID, c.Decision, c.Reason); err != nil {
		return agentserver.DispatchResult{}, err
	}
	return agentserver.DispatchResult{SessionID: c.SessionID}, nil
}

func (d *Dispatcher) handleModelSet(c agentproto.ModelSetCmd) (agentserver.DispatchResult, error) {
	now := d.Now()
	switched := agentproto.ModelSwitchedEvent{}
	switched.SessionID = c.SessionID
	switched.At = now
	switched.Model = c.Model
	if err := d.Store.Append(c.SessionID, switched); err != nil {
		return agentserver.DispatchResult{}, err
	}
	return agentserver.DispatchResult{
		SessionID: c.SessionID,
		Events:    []agentproto.AgentEvent{switched},
	}, nil
}
