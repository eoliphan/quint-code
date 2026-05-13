package agentdriver

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
)

// fakeProvider streams a pre-baked sequence of ProviderEvents.
type fakeProvider struct {
	events []ProviderEvent
}

func (p *fakeProvider) Generate(ctx context.Context, model agentcore.ModelChoice, history []agentcore.Turn, userInput string) (<-chan ProviderEvent, error) {
	ch := make(chan ProviderEvent, len(p.events))
	go func() {
		defer close(ch)
		for _, ev := range p.events {
			select {
			case <-ctx.Done():
				return
			case ch <- ev:
			}
		}
	}()
	return ch, nil
}

// fakeTools dispatches based on a static authorisation map and returns a
// canned content per call.
type fakeTools struct {
	authorise map[string]AuthorisationVerdict
	results   map[string]string
	runErr    error

	mu    sync.Mutex
	calls []ProviderToolCall
}

func (t *fakeTools) Authorize(ctx context.Context, name string, args []byte) AuthorisationVerdict {
	if v, ok := t.authorise[name]; ok {
		return v
	}
	return AuthorisationGranted
}

func (t *fakeTools) Run(ctx context.Context, name string, args []byte) (string, bool, error) {
	t.mu.Lock()
	t.calls = append(t.calls, ProviderToolCall{Name: name, Args: args})
	t.mu.Unlock()
	if t.runErr != nil {
		return "", true, t.runErr
	}
	if r, ok := t.results[name]; ok {
		return r, false, nil
	}
	return "ok", false, nil
}

// Calls returns a snapshot of recorded tool invocations. Safe to call
// while the driver runs in another goroutine (integration tests).
func (t *fakeTools) Calls() []ProviderToolCall {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]ProviderToolCall, len(t.calls))
	copy(out, t.calls)
	return out
}

func newFixedClock() (func() time.Time, *time.Time) {
	t := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	return func() time.Time {
		t = t.Add(time.Second)
		return t
	}, &t
}

func newCounterIDGen() func(string) string {
	var n atomic.Int64
	return func(kind string) string {
		return fmt.Sprintf("%s-%d", kind, n.Add(1))
	}
}

func freshSession() agentcore.Session {
	return agentcore.NewSession(
		"s1",
		"proj1",
		"Test",
		agentcore.ModelChoice{Provider: agentcore.ProviderCodex, Model: "gpt-5.4"},
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	)
}

func TestDriver_TextOnlyTurn_EmitsExpectedEvents(t *testing.T) {
	provider := &fakeProvider{events: []ProviderEvent{
		ProviderTextDelta{Delta: "Hello, "},
		ProviderTextDelta{Delta: "world"},
		ProviderTurnDone{},
	}}
	tools := &fakeTools{}
	sink := &CollectingSink{}
	now, _ := newFixedClock()

	d := &Driver{
		Provider: provider,
		Tools:    tools,
		Sink:     sink,
		IDGen:    newCounterIDGen(),
		Now:      now,
	}
	updated, err := d.Drive(context.Background(), freshSession(), "do thing")
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}

	wantKinds := []agentproto.EventKind{
		agentproto.EventTurnStarted,
		agentproto.EventPartTextDelta,
		agentproto.EventPartTextDelta,
		agentproto.EventPartTextCompleted,
		agentproto.EventTurnCompleted,
	}
	if len(sink.Events) != len(wantKinds) {
		t.Fatalf("unexpected event count: got %d, want %d\n%+v", len(sink.Events), len(wantKinds), eventKinds(sink.Events))
	}
	for i, want := range wantKinds {
		if got := sink.Events[i].Kind(); got != want {
			t.Errorf("event %d: got %s, want %s", i, got, want)
		}
	}

	if len(updated.History) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(updated.History))
	}
	turn := updated.History[0]
	if turn.State != agentcore.TurnStateCompleted {
		t.Fatalf("turn not completed: %s", turn.State)
	}
	// Expect: user text part (1) + flushed assistant text part (1) = 2 parts.
	if len(turn.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(turn.Parts))
	}
	tp, ok := turn.Parts[1].(agentcore.TextPart)
	if !ok {
		t.Fatalf("second part should be TextPart, got %T", turn.Parts[1])
	}
	if tp.Text != "Hello, world" {
		t.Fatalf("text accumulation: %q", tp.Text)
	}

	// All deltas in one unbroken text part must carry the SAME part_id so
	// clients can accumulate by id. The finalized TextPart must also reuse
	// that id.
	var deltaIDs []agentcore.PartID
	for _, ev := range sink.Events {
		if td, ok := ev.(agentproto.PartTextDeltaEvent); ok {
			deltaIDs = append(deltaIDs, td.PartID)
		}
	}
	if len(deltaIDs) < 2 || deltaIDs[0] == "" {
		t.Fatalf("expected at least 2 non-empty delta part ids, got %v", deltaIDs)
	}
	for i := 1; i < len(deltaIDs); i++ {
		if deltaIDs[i] != deltaIDs[0] {
			t.Fatalf("delta part ids diverge: %v", deltaIDs)
		}
	}
	if tp.ID() != deltaIDs[0] {
		t.Fatalf("materialized text part id %q does not match delta id %q", tp.ID(), deltaIDs[0])
	}
}

func TestDriver_ToolCall_GrantedRunsTool(t *testing.T) {
	provider := &fakeProvider{events: []ProviderEvent{
		ProviderToolCall{CallID: "tc1", Name: "ls", Args: []byte(`{}`)},
		ProviderTurnDone{},
	}}
	tools := &fakeTools{
		authorise: map[string]AuthorisationVerdict{"ls": AuthorisationGranted},
		results:   map[string]string{"ls": "file.go\n"},
	}
	sink := &CollectingSink{}
	now, _ := newFixedClock()

	d := &Driver{
		Provider: provider,
		Tools:    tools,
		Sink:     sink,
		IDGen:    newCounterIDGen(),
		Now:      now,
	}
	updated, err := d.Drive(context.Background(), freshSession(), "list files")
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}

	wantKinds := []agentproto.EventKind{
		agentproto.EventTurnStarted,
		agentproto.EventPartToolUseStarted,
		agentproto.EventPartToolUseCompleted,
		agentproto.EventTurnCompleted,
	}
	if got := eventKinds(sink.Events); !equalKinds(got, wantKinds) {
		t.Fatalf("event kinds: got %v want %v", got, wantKinds)
	}
	if len(tools.calls) != 1 || tools.calls[0].Name != "ls" {
		t.Fatalf("tool not invoked: %+v", tools.calls)
	}
	turn := updated.History[0]
	// 1 user part + 1 tool_use + 1 tool_result = 3
	if len(turn.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(turn.Parts))
	}
}

// TestDriver_ToolCall_EmptyArgsEncodes guards against a provider emitting a
// no-argument tool call as a non-nil empty []byte. json.RawMessage of an
// empty byte slice marshals as invalid JSON, which would fail event
// encoding on the journal's CombinedSink and abort the turn before the
// tool runs. The driver must normalise empty Args before assigning them
// to the wire event so the round-trip succeeds.
func TestDriver_ToolCall_EmptyArgsEncodes(t *testing.T) {
	provider := &fakeProvider{events: []ProviderEvent{
		ProviderToolCall{CallID: "tc1", Name: "ping", Args: []byte{}},
		ProviderTurnDone{},
	}}
	tools := &fakeTools{
		authorise: map[string]AuthorisationVerdict{"ping": AuthorisationGranted},
		results:   map[string]string{"ping": "pong"},
	}
	sink := &CollectingSink{}
	now, _ := newFixedClock()

	d := &Driver{
		Provider: provider,
		Tools:    tools,
		Sink:     sink,
		IDGen:    newCounterIDGen(),
		Now:      now,
	}
	if _, err := d.Drive(context.Background(), freshSession(), "ping"); err != nil {
		t.Fatalf("Drive: %v", err)
	}

	var started agentproto.PartToolUseStartedEvent
	for _, ev := range sink.Events {
		if s, ok := ev.(agentproto.PartToolUseStartedEvent); ok {
			started = s
			break
		}
	}
	if started.ToolName != "ping" {
		t.Fatalf("no tool_use.started event captured: %+v", sink.Events)
	}
	// Encode the event the way the journal would; an empty non-nil
	// json.RawMessage would crash here with "unexpected end of JSON input".
	if _, err := agentproto.EncodeEvent(started); err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
}

func TestDriver_ToolCall_RequiresPromptApproved(t *testing.T) {
	provider := &fakeProvider{events: []ProviderEvent{
		ProviderToolCall{CallID: "tc1", Name: "bash", Args: []byte(`{"cmd":"ls"}`)},
		ProviderTurnDone{},
	}}
	tools := &fakeTools{
		authorise: map[string]AuthorisationVerdict{"bash": AuthorisationRequiresPrompt},
		results:   map[string]string{"bash": "file.go"},
	}
	sink := &CollectingSink{}
	now, _ := newFixedClock()
	gate := NewPermissionGate()
	idgen := newCounterIDGen()

	d := &Driver{
		Provider:    provider,
		Tools:       tools,
		Sink:        sink,
		Permissions: gate,
		IDGen:       idgen,
		Now:         now,
	}

	// Resolve the permission asynchronously once the driver opens it.
	doneCh := make(chan error, 1)
	go func() {
		// Poll until the gate has a pending entry, then resolve.
		deadline := time.Now().Add(time.Second)
		for {
			gate.mu.Lock()
			n := len(gate.pending)
			var firstID agentcore.PermissionID
			for id := range gate.pending {
				firstID = id
				break
			}
			gate.mu.Unlock()
			if n > 0 {
				doneCh <- gate.Resolve(firstID, agentcore.PermissionApproved, "")
				return
			}
			if time.Now().After(deadline) {
				doneCh <- errors.New("gate never opened")
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	updated, err := d.Drive(context.Background(), freshSession(), "run bash")
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if err := <-doneCh; err != nil {
		t.Fatalf("permission goroutine: %v", err)
	}

	wantKinds := []agentproto.EventKind{
		agentproto.EventTurnStarted,
		agentproto.EventPartToolUseStarted,
		agentproto.EventPermissionRequested,
		agentproto.EventPermissionResolved,
		agentproto.EventPartToolUseCompleted,
		agentproto.EventTurnCompleted,
	}
	if got := eventKinds(sink.Events); !equalKinds(got, wantKinds) {
		t.Fatalf("event kinds: got %v want %v", got, wantKinds)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("tool should run after approval: %+v", tools.calls)
	}
	if updated.History[0].State != agentcore.TurnStateCompleted {
		t.Fatalf("turn not completed: %s", updated.History[0].State)
	}
}

func TestDriver_ToolCall_RequiresPromptDenied(t *testing.T) {
	provider := &fakeProvider{events: []ProviderEvent{
		ProviderToolCall{CallID: "tc1", Name: "bash", Args: []byte(`{}`)},
		ProviderTurnDone{},
	}}
	tools := &fakeTools{
		authorise: map[string]AuthorisationVerdict{"bash": AuthorisationRequiresPrompt},
	}
	sink := &CollectingSink{}
	now, _ := newFixedClock()
	gate := NewPermissionGate()

	d := &Driver{
		Provider:    provider,
		Tools:       tools,
		Sink:        sink,
		Permissions: gate,
		IDGen:       newCounterIDGen(),
		Now:         now,
	}

	go func() {
		// Resolve as denied.
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			gate.mu.Lock()
			var firstID agentcore.PermissionID
			for id := range gate.pending {
				firstID = id
				break
			}
			gate.mu.Unlock()
			if firstID != "" {
				_ = gate.Resolve(firstID, agentcore.PermissionDenied, "blocked")
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	_, err := d.Drive(context.Background(), freshSession(), "danger")
	if err != nil {
		t.Fatalf("Drive: %v", err)
	}
	if len(tools.calls) != 0 {
		t.Fatalf("tool should NOT run after denial: %+v", tools.calls)
	}
	// Last non-completed event must be a tool_use.completed flagged isError.
	var foundDenied bool
	for _, ev := range sink.Events {
		if r, ok := ev.(agentproto.PartToolUseCompletedEvent); ok && r.IsError {
			foundDenied = true
		}
	}
	if !foundDenied {
		t.Fatal("denied tool result not emitted")
	}
}

func TestDriver_ToolCall_CancelDuringPermissionWait_EmitsTurnFailed(t *testing.T) {
	// If the operator cancels ctx while gatePermission is waiting on the
	// PermissionGate, handleToolCall returns context.Canceled. The journal
	// would replay to a Running turn — blocking the next submit — unless
	// the driver routes that error through failTurn.
	provider := &fakeProvider{events: []ProviderEvent{
		ProviderToolCall{CallID: "tc1", Name: "bash", Args: []byte(`{}`)},
		ProviderTurnDone{},
	}}
	tools := &fakeTools{
		authorise: map[string]AuthorisationVerdict{"bash": AuthorisationRequiresPrompt},
	}
	sink := &CollectingSink{}
	now, _ := newFixedClock()
	gate := NewPermissionGate()

	d := &Driver{
		Provider:    provider,
		Tools:       tools,
		Sink:        sink,
		Permissions: gate,
		IDGen:       newCounterIDGen(),
		Now:         now,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Wait until the gate has a pending entry, then cancel ctx (without
		// resolving). The driver should fail the turn.
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			gate.mu.Lock()
			n := len(gate.pending)
			gate.mu.Unlock()
			if n > 0 {
				cancel()
				return
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	_, err := d.Drive(ctx, freshSession(), "destructive")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if len(sink.Events) == 0 {
		t.Fatal("no events emitted")
	}
	last := sink.Events[len(sink.Events)-1]
	if last.Kind() != agentproto.EventTurnFailed {
		t.Fatalf("last event must be turn.failed, got %s", last.Kind())
	}
}

func TestDriver_ProviderError_FailsTurn(t *testing.T) {
	provider := &fakeProvider{events: []ProviderEvent{
		ProviderTextDelta{Delta: "hi "},
		ProviderError{Err: errors.New("provider 500")},
	}}
	sink := &CollectingSink{}
	now, _ := newFixedClock()
	d := &Driver{
		Provider: provider,
		Tools:    &fakeTools{},
		Sink:     sink,
		IDGen:    newCounterIDGen(),
		Now:      now,
	}
	_, err := d.Drive(context.Background(), freshSession(), "x")
	if err == nil || err.Error() != "provider 500" {
		t.Fatalf("expected provider error, got %v", err)
	}
	last := sink.Events[len(sink.Events)-1]
	if last.Kind() != agentproto.EventTurnFailed {
		t.Fatalf("last event should be turn.failed, got %s", last.Kind())
	}
	// Pending text must be flushed before turn.failed so replay matches the
	// stream operators saw over SSE — provider contract in provider.go.
	var sawCompletedText bool
	for _, ev := range sink.Events {
		if ev.Kind() == agentproto.EventPartTextCompleted {
			sawCompletedText = true
		}
	}
	if !sawCompletedText {
		t.Fatalf("ProviderError must flush pending text before failing; events=%v", sink.Events)
	}
}

func TestDriver_ContextCancel_FailsTurnAsCanceled(t *testing.T) {
	// Provider that never emits — exercises the ctx.Done() branch.
	provider := &fakeProvider{events: nil}
	// Override Generate to return a channel we never fill.
	d := &Driver{
		Provider: hangingProvider{},
		Tools:    &fakeTools{},
		Sink:     &CollectingSink{},
		IDGen:    newCounterIDGen(),
		Now:      func() time.Time { return time.Now() },
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	_, err := d.Drive(ctx, freshSession(), "x")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
	_ = provider // suppress unused
}

type hangingProvider struct{}

func (hangingProvider) Generate(ctx context.Context, model agentcore.ModelChoice, history []agentcore.Turn, userInput string) (<-chan ProviderEvent, error) {
	ch := make(chan ProviderEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

// closedProvider returns an already-closed channel and tracks whether
// ctx was canceled before Generate ran. Used to assert that the driver
// classifies a closed stream as canceled (not completed) when ctx is
// already done.
type closedProvider struct{}

func (closedProvider) Generate(ctx context.Context, model agentcore.ModelChoice, history []agentcore.Turn, userInput string) (<-chan ProviderEvent, error) {
	ch := make(chan ProviderEvent)
	close(ch)
	return ch, nil
}

func TestDriver_ClosedStreamWithCanceledCtx_ReportsCanceled(t *testing.T) {
	// Reproduces the select-race: when the provider closes its stream as a
	// reaction to ctx cancellation, both ctx.Done() and the !open receive
	// can be ready. If the !open branch wins we used to journal
	// turn.completed for a turn that was actually canceled.
	d := &Driver{
		Provider: closedProvider{},
		Tools:    &fakeTools{},
		Sink:     &CollectingSink{},
		IDGen:    newCounterIDGen(),
		Now:      func() time.Time { return time.Now() },
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d.Drive(ctx, freshSession(), "x")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
	sink := d.Sink.(*CollectingSink)
	last := sink.Events[len(sink.Events)-1]
	if last.Kind() != agentproto.EventTurnFailed {
		t.Fatalf("expected last event turn.failed, got %s", last.Kind())
	}
}

func TestDriver_RejectsSubmitWithLiveTurn_DoesNotEmitTurnStarted(t *testing.T) {
	// If a session loaded from the journal already has a Running turn
	// (e.g. after a server restart while the dispatcher's in-memory
	// running map is empty), Drive must reject the new submit BEFORE
	// publishing turn.started — otherwise the journal grows a second
	// turn.started for an already-live turn and replay breaks.
	clock, _ := newFixedClock()
	session := freshSession()
	first := agentcore.NewTextPart("p-first", clock(), "earlier")
	live, err := agentcore.StartTurn(session, "t-live", agentcore.TurnRoleUser, first, clock())
	if err != nil {
		t.Fatalf("seed StartTurn: %v", err)
	}
	sink := &CollectingSink{}
	d := &Driver{
		Provider: &fakeProvider{events: []ProviderEvent{ProviderTurnDone{}}},
		Tools:    &fakeTools{},
		Sink:     sink,
		IDGen:    newCounterIDGen(),
		Now:      clock,
	}
	_, err = d.Drive(context.Background(), live, "second submit")
	if err == nil {
		t.Fatal("expected error when submitting on a session with a live turn")
	}
	for _, ev := range sink.Events {
		if ev.Kind() == agentproto.EventTurnStarted {
			t.Fatalf("Drive must not publish turn.started before validating; got events %v", eventKinds(sink.Events))
		}
	}
}

func TestDriver_ValidateMissingDeps(t *testing.T) {
	d := &Driver{}
	_, err := d.Drive(context.Background(), freshSession(), "x")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestPermissionGate_UnknownResolveErrors(t *testing.T) {
	g := NewPermissionGate()
	err := g.Resolve("ghost", agentcore.PermissionApproved, "")
	if !errors.Is(err, ErrUnknownPermission) {
		t.Fatalf("expected ErrUnknownPermission, got %v", err)
	}
}

func TestPermissionGate_ContextCanceledCleansPending(t *testing.T) {
	g := NewPermissionGate()
	ctx, cancel := context.WithCancel(context.Background())
	ch := g.Open("p1")
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	_, _, err := g.Wait(ctx, "p1", ch)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, present := g.pending["p1"]; present {
		t.Fatal("pending entry should be cleaned after cancellation")
	}
}

func eventKinds(evs []agentproto.AgentEvent) []agentproto.EventKind {
	out := make([]agentproto.EventKind, len(evs))
	for i, ev := range evs {
		out[i] = ev.Kind()
	}
	return out
}

func equalKinds(a, b []agentproto.EventKind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
