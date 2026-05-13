package agentstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
)

func freshStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func sampleModel() agentcore.ModelChoice {
	return agentcore.ModelChoice{Provider: agentcore.ProviderCodex, Model: "gpt-5.4", CredentialKey: "codex"}
}

func TestStore_CreateAppendLoadRoundTrip(t *testing.T) {
	store := freshStore(t)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	created, err := store.Create("s1", "proj", "Hello", sampleModel(), now)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	textPart := agentcore.NewTextPart("p1", now.Add(time.Second), "hi there")
	firstPayload, err := agentproto.EncodePart(textPart)
	if err != nil {
		t.Fatalf("encode part: %v", err)
	}
	turnStarted := agentproto.TurnStartedEvent{}
	turnStarted.SessionID = created.ID
	turnStarted.At = now.Add(time.Second)
	turnStarted.TurnID = "t1"
	turnStarted.Role = agentcore.TurnRoleUser
	turnStarted.FirstPart = firstPayload
	if err := store.Append(created.ID, turnStarted); err != nil {
		t.Fatalf("Append turn.started: %v", err)
	}

	turnComplete := agentproto.TurnCompletedEvent{}
	turnComplete.SessionID = created.ID
	turnComplete.At = now.Add(2 * time.Second)
	turnComplete.TurnID = "t1"
	if err := store.Append(created.ID, turnComplete); err != nil {
		t.Fatalf("Append turn.completed: %v", err)
	}

	loaded, err := store.Load(created.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != created.ID {
		t.Fatalf("session id mismatch: %s != %s", loaded.ID, created.ID)
	}
	if len(loaded.History) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(loaded.History))
	}
	if loaded.History[0].State != agentcore.TurnStateCompleted {
		t.Fatalf("turn not completed: %+v", loaded.History[0])
	}
	if len(loaded.History[0].Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(loaded.History[0].Parts))
	}
	tp, ok := loaded.History[0].Parts[0].(agentcore.TextPart)
	if !ok {
		t.Fatalf("expected TextPart, got %T", loaded.History[0].Parts[0])
	}
	if tp.Text != "hi there" {
		t.Fatalf("text drift: %q", tp.Text)
	}
}

func TestStore_RejectsDoubleSessionCreated(t *testing.T) {
	store := freshStore(t)
	now := time.Now()
	if _, err := store.Create("s1", "proj", "x", sampleModel(), now); err != nil {
		t.Fatal(err)
	}
	bogus := agentproto.SessionCreatedEvent{}
	bogus.SessionID = "s1"
	bogus.At = now
	err := store.Append("s1", bogus)
	if err == nil {
		t.Fatal("appending second session.created should error")
	}
}

func TestStore_RejectsCreateExistingSession(t *testing.T) {
	store := freshStore(t)
	now := time.Now()
	if _, err := store.Create("s1", "proj", "x", sampleModel(), now); err != nil {
		t.Fatal(err)
	}
	_, err := store.Create("s1", "proj", "x2", sampleModel(), now)
	if err == nil {
		t.Fatal("re-creating session should error")
	}
}

func TestStore_LoadMissingSession(t *testing.T) {
	store := freshStore(t)
	_, err := store.Load("nonexistent")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestReplay_RejectsEmptyJournal(t *testing.T) {
	_, err := Replay(nil)
	if !errors.Is(err, ErrEmptyJournal) {
		t.Fatalf("expected ErrEmptyJournal, got %v", err)
	}
}

func TestReplay_RejectsNonHeaderFirstEvent(t *testing.T) {
	turn := agentproto.TurnStartedEvent{}
	turn.SessionID = "s1"
	_, err := Replay([]agentproto.AgentEvent{turn})
	if !errors.Is(err, ErrJournalMissingHeader) {
		t.Fatalf("expected ErrJournalMissingHeader, got %v", err)
	}
}

func TestApply_RejectsNonJournalEvent(t *testing.T) {
	delta := agentproto.PartTextDeltaEvent{}
	delta.SessionID = "s1"
	_, err := Apply(agentcore.Session{}, delta)
	if !errors.Is(err, ErrNotJournalEvent) {
		t.Fatalf("expected ErrNotJournalEvent, got %v", err)
	}
}

func TestIsJournalEvent_Coverage(t *testing.T) {
	// Concrete enumeration: every Layer-P EventKind constant must have a
	// deliberate boolean answer here. If a new kind is added without
	// updating IsJournalEvent, this test fails by including the new kind
	// in the unknown-kinds list.
	knownJournal := map[agentproto.EventKind]bool{
		agentproto.EventSessionCreated:         true,
		agentproto.EventSessionUpdated:         true,
		agentproto.EventSessionArchived:        true,
		agentproto.EventTurnStarted:            true,
		agentproto.EventTurnCompleted:          true,
		agentproto.EventTurnFailed:             true,
		agentproto.EventPartToolUseStarted:     true,
		agentproto.EventPartToolUseCompleted:   true,
		agentproto.EventPartTextCompleted:      true,
		agentproto.EventPartReasoningCompleted: true,
		agentproto.EventPartFileRef:            true,
		agentproto.EventPartStepBoundary:       true,
		agentproto.EventSubAgentSpawned:        true,
		agentproto.EventSubAgentCompleted:      true,
		agentproto.EventPermissionRequested:    true,
		agentproto.EventPermissionResolved:     true,
		agentproto.EventModelSwitched:          true,
		agentproto.EventPartTextDelta:          false,
		agentproto.EventPartReasoningDelta:     false,
		agentproto.EventAuthExpired:            false,
	}
	for kind, want := range knownJournal {
		got := IsJournalEvent(kind)
		if got != want {
			t.Errorf("IsJournalEvent(%s) = %v, want %v", kind, got, want)
		}
	}
}

// generate1000Events produces a deterministic mix of journal events that
// exercise every G2 transition. The total is exactly 1000 to match the
// M1 acceptance bar.
func generate1000Events(sessionID agentcore.SessionID) []agentproto.AgentEvent {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	out := make([]agentproto.AgentEvent, 0, 1000)

	// Header.
	created := agentproto.SessionCreatedEvent{}
	created.SessionID = sessionID
	created.At = now
	created.ProjectID = "proj"
	created.Title = "Replay test"
	created.Model = sampleModel()
	out = append(out, created)

	// 100 turns, each: start (1) + 7 parts (7) + complete (1) = 9 events.
	// Plus a few permission/model/subagent variants sprinkled in.
	for turnIdx := 0; turnIdx < 100; turnIdx++ {
		turnID := agentcore.TurnID(fmt.Sprintf("t%d", turnIdx))
		turnTime := now.Add(time.Duration(turnIdx+1) * time.Minute)

		firstPart := agentcore.NewTextPart(
			agentcore.PartID(fmt.Sprintf("p%d-0", turnIdx)),
			turnTime,
			fmt.Sprintf("user input %d", turnIdx),
		)
		firstPayload, _ := agentproto.EncodePart(firstPart)

		started := agentproto.TurnStartedEvent{}
		started.SessionID = sessionID
		started.At = turnTime
		started.TurnID = turnID
		started.Role = agentcore.TurnRoleUser
		started.FirstPart = firstPayload
		out = append(out, started)

		// 7 part-append events per turn: tool_use_started, tool_use_completed,
		// file_ref, step_boundary, then 3 more tool cycles.
		for pi := 0; pi < 7; pi++ {
			partID := agentcore.PartID(fmt.Sprintf("p%d-%d", turnIdx, pi+1))
			at := turnTime.Add(time.Duration(pi+1) * time.Second)
			switch pi % 4 {
			case 0:
				tu := agentproto.PartToolUseStartedEvent{}
				tu.SessionID = sessionID
				tu.At = at
				tu.TurnID = turnID
				tu.PartID = partID
				tu.ToolCallID = fmt.Sprintf("tc%d-%d", turnIdx, pi)
				tu.ToolName = "bash"
				tu.Args = []byte(`{"cmd":"ls"}`)
				out = append(out, tu)
			case 1:
				tr := agentproto.PartToolUseCompletedEvent{}
				tr.SessionID = sessionID
				tr.At = at
				tr.TurnID = turnID
				tr.PartID = partID
				tr.ToolCallID = fmt.Sprintf("tc%d-%d", turnIdx, pi-1)
				tr.ToolName = "bash"
				tr.Content = "out"
				out = append(out, tr)
			case 2:
				fr := agentproto.PartFileRefEvent{}
				fr.SessionID = sessionID
				fr.At = at
				fr.TurnID = turnID
				fr.PartID = partID
				fr.Path = "main.go"
				fr.MIMEType = "text/x-go"
				fr.Bytes = 1024
				out = append(out, fr)
			case 3:
				sb := agentproto.PartStepBoundaryEvent{}
				sb.SessionID = sessionID
				sb.At = at
				sb.TurnID = turnID
				sb.PartID = partID
				sb.Label = "step"
				out = append(out, sb)
			}
		}

		// Complete the turn so the next turn can start.
		complete := agentproto.TurnCompletedEvent{}
		complete.SessionID = sessionID
		complete.At = turnTime.Add(10 * time.Second)
		complete.TurnID = turnID
		out = append(out, complete)
	}

	// We're now at 1 + 100*(1+7+1) = 901 events. Pad to 1000 with renames
	// and model switches (both apply when no turn is running).
	for pad := 0; pad < 99; pad++ {
		at := now.Add(time.Duration(200+pad) * time.Minute)
		if pad%3 == 0 {
			rename := agentproto.SessionUpdatedEvent{}
			rename.SessionID = sessionID
			rename.At = at
			rename.Title = fmt.Sprintf("Renamed %d", pad)
			out = append(out, rename)
		} else {
			ms := agentproto.ModelSwitchedEvent{}
			ms.SessionID = sessionID
			ms.At = at
			ms.Model = agentcore.ModelChoice{Provider: agentcore.ProviderAnthropic, Model: "claude-sonnet-4-6", CredentialKey: "anthropic"}
			out = append(out, ms)
		}
	}
	return out
}

func TestReplay_1000Events_DeterministicState(t *testing.T) {
	events := generate1000Events("replay-bench")
	if len(events) != 1000 {
		t.Fatalf("expected exactly 1000 events, got %d", len(events))
	}

	first, err := Replay(events)
	if err != nil {
		t.Fatalf("first replay: %v", err)
	}
	second, err := Replay(events)
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}

	// Determinism: same input → byte-identical Session value (modulo map
	// iteration which Go randomises but reflect.DeepEqual handles).
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("replay non-deterministic")
	}

	// Sanity: 100 turns, all completed.
	if len(first.History) != 100 {
		t.Fatalf("expected 100 turns, got %d", len(first.History))
	}
	for i, turn := range first.History {
		if turn.State != agentcore.TurnStateCompleted {
			t.Fatalf("turn %d not completed: %s", i, turn.State)
		}
		if len(turn.Parts) != 8 { // 1 first + 7 appended
			t.Fatalf("turn %d: expected 8 parts, got %d", i, len(turn.Parts))
		}
	}
}

func TestStore_Replay1000ViaDisk(t *testing.T) {
	store := freshStore(t)
	events := generate1000Events("disk-replay")

	// First event is SessionCreated — feed via Create.
	header := events[0].(agentproto.SessionCreatedEvent)
	if _, err := store.Create("disk-replay", "proj", header.Title, header.Model, header.At); err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i, ev := range events[1:] {
		if err := store.Append("disk-replay", ev); err != nil {
			t.Fatalf("Append event %d (%s): %v", i+1, ev.Kind(), err)
		}
	}

	loaded, err := store.Load("disk-replay")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Independent in-memory replay for comparison.
	pure, err := Replay(events)
	if err != nil {
		t.Fatalf("pure Replay: %v", err)
	}
	if !reflect.DeepEqual(loaded, pure) {
		t.Fatal("disk replay diverges from pure replay")
	}
	if len(loaded.History) != 100 {
		t.Fatalf("expected 100 turns, got %d", len(loaded.History))
	}
}

func TestStore_List_OrdersByUpdatedAt(t *testing.T) {
	store := freshStore(t)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	for i, id := range []agentcore.SessionID{"a", "b", "c"} {
		if _, err := store.Create(id, "proj", string(id), sampleModel(), now.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	list, err := store.List("proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(list))
	}
	if list[0].ID != "c" || list[1].ID != "b" || list[2].ID != "a" {
		t.Fatalf("unexpected order: %+v", list)
	}
}

func TestStore_List_FiltersByProject(t *testing.T) {
	store := freshStore(t)
	now := time.Now()
	if _, err := store.Create("s1", "alpha", "x", sampleModel(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("s2", "beta", "x", sampleModel(), now); err != nil {
		t.Fatal(err)
	}
	alpha, err := store.List("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(alpha) != 1 || alpha[0].ID != "s1" {
		t.Fatalf("unexpected alpha list: %+v", alpha)
	}
	beta, err := store.List("beta")
	if err != nil {
		t.Fatal(err)
	}
	if len(beta) != 1 || beta[0].ID != "s2" {
		t.Fatalf("unexpected beta list: %+v", beta)
	}
}

func TestStore_PreservesJournalOnDisk(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	created, err := store.Create("s1", "proj", "x", sampleModel(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Re-open the store; Load should reproduce the same session.
	store2, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	loaded, err := store2.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != created.ID || loaded.Title != created.Title {
		t.Fatalf("loaded session diverges from created: %+v vs %+v", loaded, created)
	}
}

func TestStore_AppendRejectsCrossSessionEvent(t *testing.T) {
	store := freshStore(t)
	now := time.Now()
	if _, err := store.Create("s1", "proj", "x", sampleModel(), now); err != nil {
		t.Fatal(err)
	}
	rename := agentproto.SessionUpdatedEvent{}
	rename.SessionID = "s_other"
	rename.At = now
	rename.Title = "wrong"
	err := store.Append("s1", rename)
	if err == nil {
		t.Fatal("Append should reject mismatched session id")
	}
}

func TestStore_AppendOnMissingSessionReturnsNotFound(t *testing.T) {
	store := freshStore(t)
	rename := agentproto.SessionUpdatedEvent{}
	rename.SessionID = "ghost"
	rename.At = time.Now()
	rename.Title = "should not land"
	err := store.Append("ghost", rename)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.root, "ghost")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Append must not materialize a session directory; stat err: %v", err)
	}
}

func TestStore_AppendValidatesBeforeJournaling(t *testing.T) {
	// Switching the model while a turn is in flight must not leave an
	// unreplayable event behind: Apply rejects the transition, so Append
	// must refuse to write to disk.
	store := freshStore(t)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.Create("s1", "proj", "x", sampleModel(), now); err != nil {
		t.Fatal(err)
	}
	firstPart := agentcore.NewTextPart("p1", now.Add(time.Second), "live")
	firstPayload, err := agentproto.EncodePart(firstPart)
	if err != nil {
		t.Fatal(err)
	}
	started := agentproto.TurnStartedEvent{}
	started.SessionID = "s1"
	started.At = now.Add(time.Second)
	started.TurnID = "t1"
	started.Role = agentcore.TurnRoleUser
	started.FirstPart = firstPayload
	if err := store.Append("s1", started); err != nil {
		t.Fatal(err)
	}
	switched := agentproto.ModelSwitchedEvent{}
	switched.SessionID = "s1"
	switched.At = now.Add(2 * time.Second)
	switched.Model = agentcore.ModelChoice{Provider: agentcore.ProviderAnthropic, Model: "claude-sonnet-4-6"}
	if err := store.Append("s1", switched); !errors.Is(err, agentcore.ErrTurnAlreadyRunning) {
		t.Fatalf("expected ErrTurnAlreadyRunning, got %v", err)
	}
	// Journal must still replay cleanly.
	if _, err := store.Load("s1"); err != nil {
		t.Fatalf("journal poisoned by rejected append: %v", err)
	}
}

// TestStore_AppendSucceedsWhenMetaWriteFails locks the session directory
// read-only after Create so writeMeta cannot create its tempfile, while the
// already-open journal FD keeps accepting Appends. Append must still
// succeed (the journal is authoritative); otherwise the driver would see a
// turn.started publish failure and skip emitting turn.failed for a turn the
// journal already records as Running.
func TestStore_AppendSucceedsWhenMetaWriteFails(t *testing.T) {
	store := freshStore(t)
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	created, err := store.Create("s1", "proj", "x", sampleModel(), now)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dir := filepath.Join(store.root, "s1")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod ro: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	firstPart := agentcore.NewTextPart("p1", now.Add(time.Second), "hi")
	firstPayload, err := agentproto.EncodePart(firstPart)
	if err != nil {
		t.Fatalf("encode part: %v", err)
	}
	started := agentproto.TurnStartedEvent{}
	started.SessionID = created.ID
	started.At = now.Add(time.Second)
	started.TurnID = "t1"
	started.Role = agentcore.TurnRoleUser
	started.FirstPart = firstPayload
	if err := store.Append(created.ID, started); err != nil {
		t.Fatalf("Append must not fail on post-commit meta failure: %v", err)
	}

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod rw: %v", err)
	}
	loaded, err := store.Load(created.ID)
	if err != nil {
		t.Fatalf("Load after partial commit: %v", err)
	}
	if len(loaded.History) != 1 || loaded.History[0].State != agentcore.TurnStateRunning {
		t.Fatalf("journal must replay the committed turn: %+v", loaded.History)
	}
}

func TestStore_JournalFilePresent(t *testing.T) {
	store := freshStore(t)
	now := time.Now()
	if _, err := store.Create("s1", "proj", "x", sampleModel(), now); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(store.root, "s1", journalFile)
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("journal file missing: %v", err)
	}
}
