package agentdriver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
	"github.com/m0n0x41d/haft/internal/agentserver"
	"github.com/m0n0x41d/haft/internal/agentstore"
)

// scriptedProvider returns a fresh channel per Generate call, replaying
// the same scripted events. Used for end-to-end tests where the same
// session may run several turns.
type scriptedProvider struct {
	events []ProviderEvent
}

func (p *scriptedProvider) Generate(ctx context.Context, _ agentcore.ModelChoice, _ []agentcore.Turn, _ string) (<-chan ProviderEvent, error) {
	ch := make(chan ProviderEvent, len(p.events))
	go func() {
		defer close(ch)
		for _, ev := range p.events {
			select {
			case <-ctx.Done():
				return
			case ch <- ev:
				time.Sleep(time.Millisecond)
			}
		}
	}()
	return ch, nil
}

type passthroughTools struct{}

func (passthroughTools) Authorize(context.Context, string, []byte) AuthorisationVerdict {
	return AuthorisationGranted
}

func (passthroughTools) Run(context.Context, string, []byte) (string, bool, error) {
	return "ok", false, nil
}

func bootIntegrationServer(t *testing.T, provider Provider) (string, *agentstore.Store, func()) {
	t.Helper()
	store, err := agentstore.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	hub := agentserver.NewHub()
	gate := NewPermissionGate()

	var counter atomic.Int64
	driver := &Driver{
		Provider: provider,
		Tools:    passthroughTools{},
		IDGen: func(kind string) string {
			n := counter.Add(1)
			return fmt.Sprintf("%s-%d", kind, n)
		},
		Now: time.Now,
	}

	dispatcher := NewDispatcher(store, hub, driver, gate)
	dispatcher.IDGen = func() agentcore.SessionID {
		n := counter.Add(1)
		return agentcore.SessionID(fmt.Sprintf("s-%d", n))
	}
	dispatcher.Now = time.Now

	srv := agentserver.NewServer("127.0.0.1:0", store, dispatcher, hub)
	addr, errCh, err := srv.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = store.Close()
		select {
		case e := <-errCh:
			if e != nil {
				t.Logf("server exit error: %v", e)
			}
		default:
		}
	}
	return "http://" + addr, store, cleanup
}

// TestIntegration_FullTurnRoundTrip is the M2 acceptance test:
//
//  1. Spawn server + driver
//  2. Subscribe to /event/global SSE
//  3. POST /session — receive session_id
//  4. POST /session/:id/turn — fire and forget
//  5. Observe live SSE events: turn.started, deltas, tool_use, turn.completed
//  6. GET /session/:id — replayed history matches
//
// This verifies the M2 dogfood bar: real HTTP, real SSE, real driver
// orchestration on real journal.
func TestIntegration_FullTurnRoundTrip(t *testing.T) {
	provider := &scriptedProvider{events: []ProviderEvent{
		ProviderTextDelta{Delta: "Hello "},
		ProviderTextDelta{Delta: "from agent"},
		ProviderToolCall{CallID: "tc1", Name: "ls", Args: []byte(`{}`)},
		ProviderTurnDone{},
	}}
	base, store, cleanup := bootIntegrationServer(t, provider)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sseReq, _ := http.NewRequestWithContext(ctx, "GET", base+"/event/global", nil)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("subscribe SSE: %v", err)
	}
	defer sseResp.Body.Close()
	if sseResp.StatusCode != http.StatusOK {
		t.Fatalf("SSE status %d", sseResp.StatusCode)
	}

	// Drain the connect comment frame so subsequent reads receive real events.
	reader := bufio.NewReader(sseResp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("drain SSE: %v", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	// 1. POST /session.
	createBody, _ := json.Marshal(agentproto.SessionCreateCmd{ProjectID: "proj1", Title: "Smoke"})
	createResp, err := http.Post(base+"/session", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /session: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		dump, _ := io.ReadAll(createResp.Body)
		t.Fatalf("create status %d: %s", createResp.StatusCode, dump)
	}
	var created map[string]any
	_ = json.NewDecoder(createResp.Body).Decode(&created)
	sessionID := agentcore.SessionID(created["session_id"].(string))
	if sessionID == "" {
		t.Fatalf("no session id in response: %+v", created)
	}

	// 2. POST /session/:id/turn.
	turnBody, _ := json.Marshal(map[string]any{"text": "do the thing"})
	turnResp, err := http.Post(base+"/session/"+string(sessionID)+"/turn", "application/json", bytes.NewReader(turnBody))
	if err != nil {
		t.Fatalf("POST /turn: %v", err)
	}
	_ = turnResp.Body.Close()
	if turnResp.StatusCode != http.StatusOK {
		t.Fatalf("turn submit status %d", turnResp.StatusCode)
	}

	// 3. Read SSE events on a goroutine, collect kinds via channel.
	kindCh := make(chan agentproto.EventKind, 32)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				close(kindCh)
				return
			}
			line = strings.TrimRight(line, "\n")
			if !strings.HasPrefix(line, "event: ") {
				continue
			}
			kindCh <- agentproto.EventKind(strings.TrimPrefix(line, "event: "))
		}
	}()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	seen := map[agentproto.EventKind]int{}
	var lastEvent agentproto.EventKind
collectLoop:
	for {
		select {
		case <-deadline.C:
			break collectLoop
		case kind, ok := <-kindCh:
			if !ok {
				break collectLoop
			}
			seen[kind]++
			lastEvent = kind
			if kind == agentproto.EventTurnCompleted {
				break collectLoop
			}
		}
	}

	if seen[agentproto.EventSessionCreated] == 0 {
		t.Errorf("session.created not observed")
	}
	if seen[agentproto.EventTurnStarted] == 0 {
		t.Errorf("turn.started not observed")
	}
	if seen[agentproto.EventPartTextDelta] == 0 {
		t.Errorf("text deltas not observed")
	}
	if seen[agentproto.EventPartToolUseStarted] == 0 {
		t.Errorf("tool_use.started not observed")
	}
	if seen[agentproto.EventPartToolUseCompleted] == 0 {
		t.Errorf("tool_use.completed not observed")
	}
	if seen[agentproto.EventTurnCompleted] == 0 {
		t.Errorf("turn.completed not observed (last: %s)", lastEvent)
	}

	// 4. Verify journal persisted: load via store, expect identical state.
	loaded, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.History) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(loaded.History))
	}
	if loaded.History[0].State != agentcore.TurnStateCompleted {
		t.Fatalf("turn not completed in store: %s", loaded.History[0].State)
	}
	// 1 user text + 1 assistant text (flushed from deltas) + 1 tool_use +
	// 1 tool_result = 4 parts. The assistant TextPart is journaled via
	// part.text.completed so reload reproduces what Drive returned.
	if len(loaded.History[0].Parts) != 4 {
		t.Fatalf("expected 4 journaled parts, got %d", len(loaded.History[0].Parts))
	}
}

func TestIntegration_TurnCancel(t *testing.T) {
	// Provider that hangs forever.
	provider := hangingProvider{}
	base, _, cleanup := bootIntegrationServer(t, provider)
	defer cleanup()

	createBody, _ := json.Marshal(agentproto.SessionCreateCmd{ProjectID: "proj1", Title: "Cancel"})
	createResp, err := http.Post(base+"/session", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /session: %v", err)
	}
	defer createResp.Body.Close()
	var created map[string]any
	_ = json.NewDecoder(createResp.Body).Decode(&created)
	sessionID := agentcore.SessionID(created["session_id"].(string))

	turnBody, _ := json.Marshal(map[string]any{"text": "hang"})
	turnResp, _ := http.Post(base+"/session/"+string(sessionID)+"/turn", "application/json", bytes.NewReader(turnBody))
	var submitted map[string]any
	_ = json.NewDecoder(turnResp.Body).Decode(&submitted)
	_ = turnResp.Body.Close()
	turnID, _ := submitted["turn_id"].(string)
	if turnID == "" {
		t.Fatalf("turn submit response missing turn_id: %v", submitted)
	}

	// Give the driver a moment to hit the hang.
	time.Sleep(20 * time.Millisecond)

	cancelBody, _ := json.Marshal(map[string]any{"turn_id": turnID})
	cancelResp, err := http.Post(base+"/session/"+string(sessionID)+"/cancel", "application/json", bytes.NewReader(cancelBody))
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	defer cancelResp.Body.Close()
	if cancelResp.StatusCode >= 400 {
		dump, _ := io.ReadAll(cancelResp.Body)
		t.Fatalf("cancel status %d: %s", cancelResp.StatusCode, dump)
	}
}

func TestIntegration_PermissionRoundTrip(t *testing.T) {
	provider := &scriptedProvider{events: []ProviderEvent{
		ProviderToolCall{CallID: "tc1", Name: "bash", Args: []byte(`{"cmd":"rm"}`)},
		ProviderTurnDone{},
	}}
	store, err := agentstore.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	hub := agentserver.NewHub()
	gate := NewPermissionGate()

	var counter atomic.Int64
	tools := &fakeTools{
		authorise: map[string]AuthorisationVerdict{"bash": AuthorisationRequiresPrompt},
		results:   map[string]string{"bash": "removed"},
	}
	driver := &Driver{
		Provider: provider,
		Tools:    tools,
		IDGen: func(kind string) string {
			return fmt.Sprintf("%s-%d", kind, counter.Add(1))
		},
		Now: time.Now,
	}

	dispatcher := NewDispatcher(store, hub, driver, gate)
	dispatcher.IDGen = func() agentcore.SessionID {
		return agentcore.SessionID(fmt.Sprintf("s-%d", counter.Add(1)))
	}

	srv := agentserver.NewServer("127.0.0.1:0", store, dispatcher, hub)
	addr, errCh, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		select {
		case <-errCh:
		default:
		}
	}()
	base := "http://" + addr

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sseReq, _ := http.NewRequestWithContext(ctx, "GET", base+"/event/global", nil)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	reader := bufio.NewReader(sseResp.Body)
	for {
		line, _ := reader.ReadString('\n')
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	createBody, _ := json.Marshal(agentproto.SessionCreateCmd{ProjectID: "proj1", Title: "Perm"})
	createResp, err := http.Post(base+"/session", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /session: %v", err)
	}
	defer createResp.Body.Close()
	var created map[string]any
	_ = json.NewDecoder(createResp.Body).Decode(&created)
	sessionID := agentcore.SessionID(created["session_id"].(string))

	turnBody, _ := json.Marshal(map[string]any{"text": "destructive"})
	turnResp, _ := http.Post(base+"/session/"+string(sessionID)+"/turn", "application/json", bytes.NewReader(turnBody))
	_ = turnResp.Body.Close()

	// Read events until we hit permission.requested.
	var permID agentcore.PermissionID
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && permID == "" {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "permission_id") {
			data := strings.TrimPrefix(line, "data: ")
			ev, err := agentproto.DecodeEvent([]byte(data))
			if err == nil {
				if pr, ok := ev.(agentproto.PermissionRequestedEvent); ok {
					permID = pr.PermissionID
				}
			}
		}
	}
	if permID == "" {
		t.Fatal("permission.requested never observed")
	}

	// Approve.
	approveBody, _ := json.Marshal(map[string]any{
		"session_id": sessionID,
		"decision":   string(agentcore.PermissionApproved),
	})
	approveResp, err := http.Post(base+"/permission/"+string(permID), "application/json", bytes.NewReader(approveBody))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	defer approveResp.Body.Close()
	if approveResp.StatusCode >= 400 {
		dump, _ := io.ReadAll(approveResp.Body)
		t.Fatalf("approve status %d: %s", approveResp.StatusCode, dump)
	}

	// Wait for the tool to actually run by checking dispatcher's tools record.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(tools.calls) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(tools.calls) == 0 {
		t.Fatal("tool was not invoked after approval")
	}
}
