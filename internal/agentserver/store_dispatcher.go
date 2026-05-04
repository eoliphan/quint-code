package agentserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
	"github.com/m0n0x41d/haft/internal/agentstore"
)

// StoreDispatcher is the minimal Dispatcher: it understands session
// lifecycle commands (create/list/resume/rename) and rejects everything
// that requires a real agent driver. Useful for tests and as a fallback
// when the LLM-backed driver is unavailable (e.g. expired credentials).
//
// IDGen is the only injectable side-effect: tests override it for
// determinism, production wires it to a uuid generator. Same for Now.
type StoreDispatcher struct {
	Store *agentstore.Store
	IDGen func() agentcore.SessionID
	Now   func() time.Time
}

// ErrUnsupportedCommand is returned by Dispatchers that recognize the
// envelope but cannot handle the operation themselves.
var ErrUnsupportedCommand = errors.New("agentserver: dispatcher does not support this command")

// Dispatch routes the typed command. Errors are returned as-is; the
// transport maps them to HTTP status codes.
func (d *StoreDispatcher) Dispatch(ctx context.Context, cmd agentproto.RPCCommand) (DispatchResult, error) {
	if d.IDGen == nil || d.Now == nil || d.Store == nil {
		return DispatchResult{}, errors.New("agentserver: StoreDispatcher missing required fields")
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
	}
	return DispatchResult{}, fmt.Errorf("%w: %s", ErrUnsupportedCommand, cmd.Kind())
}

func (d *StoreDispatcher) handleSessionCreate(c agentproto.SessionCreateCmd) (DispatchResult, error) {
	id := d.IDGen()
	now := d.Now()
	if _, err := d.Store.Create(id, c.ProjectID, c.Title, c.Model, now); err != nil {
		return DispatchResult{}, err
	}
	// Create() already wrote SessionCreated to the journal. The transport
	// layer uses the event we return here ONLY for broadcast — it must
	// NOT re-journal it. We mark this with the broadcast-only flag below.
	created := agentproto.SessionCreatedEvent{}
	created.SessionID = id
	created.At = now
	created.Title = c.Title
	created.Model = c.Model
	return DispatchResult{
		SessionID: id,
		Events:    []agentproto.AgentEvent{created},
		Response: map[string]any{
			"session_id": id,
		},
	}, nil
}

func (d *StoreDispatcher) handleSessionList(c agentproto.SessionListCmd) (DispatchResult, error) {
	metas, err := d.Store.List(c.ProjectID)
	if err != nil {
		return DispatchResult{}, err
	}
	return DispatchResult{Response: metas}, nil
}

func (d *StoreDispatcher) handleSessionResume(c agentproto.SessionResumeCmd) (DispatchResult, error) {
	session, err := d.Store.Load(c.SessionID)
	if err != nil {
		return DispatchResult{}, err
	}
	return DispatchResult{Response: session, SessionID: c.SessionID}, nil
}

func (d *StoreDispatcher) handleSessionRename(c agentproto.SessionRenameCmd) (DispatchResult, error) {
	now := d.Now()
	rename := agentproto.SessionUpdatedEvent{}
	rename.SessionID = c.SessionID
	rename.At = now
	rename.Title = c.Title
	if err := d.Store.Append(c.SessionID, rename); err != nil {
		return DispatchResult{}, err
	}
	return DispatchResult{
		SessionID: c.SessionID,
		Events:    []agentproto.AgentEvent{rename},
	}, nil
}
