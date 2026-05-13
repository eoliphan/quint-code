package agentserver

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
	"github.com/m0n0x41d/haft/internal/agentstore"
)

func newTestServer(t *testing.T) (*Server, string, *agentstore.Store) {
	t.Helper()
	store, err := agentstore.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var idCounter atomic.Int64
	dispatcher := &StoreDispatcher{
		Store: store,
		IDGen: func() agentcore.SessionID {
			n := idCounter.Add(1)
			return agentcore.SessionID(fmt.Sprintf("s-%03d", n))
		},
		Now: func() time.Time { return time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC) },
	}

	srv := NewServer("127.0.0.1:0", store, dispatcher, nil)
	addr, errCh, err := srv.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		select {
		case e := <-errCh:
			if e != nil {
				t.Logf("server exit error: %v", e)
			}
		default:
		}
	})
	return srv, "http://" + addr, store
}

func TestServer_Healthz(t *testing.T) {
	_, base, _ := newTestServer(t)
	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestServer_SessionCreate_PersistsAndReturnsID(t *testing.T) {
	_, base, store := newTestServer(t)
	body, _ := json.Marshal(agentproto.SessionCreateCmd{
		ProjectID: "proj1",
		Title:     "Hello",
		Model:     agentcore.ModelChoice{Provider: agentcore.ProviderCodex, Model: "gpt-5.4"},
	})
	resp, err := http.Post(base+"/session", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		dump, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s", resp.StatusCode, dump)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	id, ok := got["session_id"].(string)
	if !ok || !strings.HasPrefix(id, "s-") {
		t.Fatalf("unexpected response: %+v", got)
	}
	if _, err := store.Load(agentcore.SessionID(id)); err != nil {
		t.Fatalf("session not persisted: %v", err)
	}
}

func TestServer_SessionList_AfterCreate(t *testing.T) {
	_, base, _ := newTestServer(t)
	// Create two sessions in the same project.
	for range 2 {
		body, _ := json.Marshal(agentproto.SessionCreateCmd{ProjectID: "proj1", Title: "x"})
		resp, err := http.Post(base+"/session", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST /session: %v", err)
		}
		_ = resp.Body.Close()
	}
	resp, err := http.Get(base + "/session?project_id=proj1")
	if err != nil {
		t.Fatalf("GET /session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var list []agentstore.SessionMeta
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}
}

func TestServer_SessionGet_UnknownReturns404(t *testing.T) {
	_, base, _ := newTestServer(t)
	resp, err := http.Get(base + "/session/nope")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServer_SessionRename_Broadcasts(t *testing.T) {
	srv, base, _ := newTestServer(t)
	id := createSession(t, base, "proj1", "Original")

	// Subscribe to SSE BEFORE renaming.
	ch, cancel := srv.Hub.Subscribe()
	defer cancel()

	body, _ := json.Marshal(map[string]any{"title": "Renamed"})
	resp, err := http.Post(base+"/session/"+string(id)+"/rename", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST rename: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	select {
	case ev := <-ch:
		if ev.Kind() != agentproto.EventSessionUpdated {
			t.Fatalf("expected session.updated, got %s", ev.Kind())
		}
		updated, ok := ev.(agentproto.SessionUpdatedEvent)
		if !ok {
			t.Fatalf("unexpected event type %T", ev)
		}
		if updated.Title != "Renamed" {
			t.Fatalf("title not propagated: %q", updated.Title)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received within 1s")
	}
}

func TestServer_SSEStream_DeliversCreatedEvent(t *testing.T) {
	srv, base, _ := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/event/global", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("subscribe SSE: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	// Wait for hub to register the subscriber. The handler sends the
	// initial comment frame, which proves the subscribe call has hit the
	// Hub mutex.
	deadline := time.Now().Add(time.Second)
	for srv.Hub.Count() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("hub never registered subscriber")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Trigger an event from another goroutine — POST a session create.
	go func() {
		body, _ := json.Marshal(agentproto.SessionCreateCmd{ProjectID: "proj1", Title: "Streamed"})
		_, _ = http.Post(base+"/session", "application/json", bytes.NewReader(body))
	}()

	reader := bufio.NewReader(resp.Body)
	deadline = time.Now().Add(2 * time.Second)
	type sseFrame struct {
		event string
		data  string
	}
	var frame sseFrame

	for time.Now().Before(deadline) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case strings.HasPrefix(line, "event: "):
			frame.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			frame.data = strings.TrimPrefix(line, "data: ")
		case line == "":
			if frame.event == string(agentproto.EventSessionCreated) {
				goto matched
			}
			frame = sseFrame{}
		}
	}
	t.Fatalf("session.created not seen on SSE stream within 2s")

matched:
	if frame.data == "" {
		t.Fatal("matched frame had empty data")
	}
	ev, err := agentproto.DecodeEvent([]byte(frame.data))
	if err != nil {
		t.Fatalf("decode SSE event: %v", err)
	}
	created, ok := ev.(agentproto.SessionCreatedEvent)
	if !ok {
		t.Fatalf("unexpected event type %T", ev)
	}
	if created.Title != "Streamed" {
		t.Fatalf("title drift: %q", created.Title)
	}
}

func TestServer_SessionCreate_RejectsMalformedJSON(t *testing.T) {
	_, base, _ := newTestServer(t)
	resp, err := http.Post(base+"/session", "application/json", strings.NewReader(`{not json`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestServer_TurnSubmit_RoutesToDispatcher(t *testing.T) {
	// StoreDispatcher does not handle turn.submit and should return 501.
	_, base, _ := newTestServer(t)
	id := createSession(t, base, "proj1", "x")
	body, _ := json.Marshal(map[string]any{"text": "hello"})
	resp, err := http.Post(base+"/session/"+string(id)+"/turn", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST turn: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}
}

func TestStoreDispatcher_ResumeReturnsSession(t *testing.T) {
	store, err := agentstore.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.Create("s1", "proj", "x", agentcore.ModelChoice{}, now); err != nil {
		t.Fatal(err)
	}
	d := &StoreDispatcher{
		Store: store,
		IDGen: func() agentcore.SessionID { return "ignored" },
		Now:   func() time.Time { return now },
	}
	res, err := d.Dispatch(context.Background(), agentproto.SessionResumeCmd{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := res.Response.(agentproto.SessionPayload)
	if !ok {
		t.Fatalf("unexpected response type %T", res.Response)
	}
	if payload.ID != "s1" {
		t.Fatalf("session id drift: %s", payload.ID)
	}
}

func TestStoreDispatcher_RejectsUnsupported(t *testing.T) {
	store, _ := agentstore.NewStore(t.TempDir())
	defer store.Close()
	d := &StoreDispatcher{
		Store: store,
		IDGen: func() agentcore.SessionID { return "x" },
		Now:   func() time.Time { return time.Now() },
	}
	_, err := d.Dispatch(context.Background(), agentproto.TurnSubmitCmd{SessionID: "s1", Text: "hi"})
	if err == nil {
		t.Fatal("expected error for unsupported command")
	}
}

func createSession(t *testing.T, base, projectID, title string) agentcore.SessionID {
	t.Helper()
	body, _ := json.Marshal(agentproto.SessionCreateCmd{ProjectID: projectID, Title: title})
	resp, err := http.Post(base+"/session", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		dump, _ := io.ReadAll(resp.Body)
		t.Fatalf("create session status %d: %s", resp.StatusCode, dump)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	id, _ := got["session_id"].(string)
	return agentcore.SessionID(id)
}
