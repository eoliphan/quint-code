package agentcore

import (
	"errors"
	"testing"
	"time"
)

// fixedClock returns timestamps in 1-second increments starting from a
// deterministic epoch. Tests use it to assert that transitions stamp the
// CORRECT timestamp without depending on wall clock.
type fixedClock struct {
	now time.Time
}

func newClock() *fixedClock {
	return &fixedClock{now: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fixedClock) tick() time.Time {
	c.now = c.now.Add(time.Second)
	return c.now
}

func freshSession(t *testing.T) (Session, *fixedClock) {
	t.Helper()
	clock := newClock()
	model := ModelChoice{Provider: ProviderOpenAI, Model: "gpt-5.4", CredentialKey: "test"}
	return NewSession("s1", "proj", "Test session", model, clock.tick()), clock
}

func TestNewSession_ZeroValueCollections(t *testing.T) {
	s, _ := freshSession(t)
	if len(s.History) != 0 {
		t.Fatalf("History should be empty, got %d", len(s.History))
	}
	if s.Permissions == nil {
		t.Fatal("Permissions must be a non-nil map")
	}
	if s.SubAgents == nil {
		t.Fatal("SubAgents must be a non-nil map")
	}
}

func TestStartTurn_RejectsConcurrentTurns(t *testing.T) {
	s, clock := freshSession(t)
	s, err := StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "hello"), clock.tick())
	if err != nil {
		t.Fatalf("first StartTurn failed: %v", err)
	}
	_, err = StartTurn(s, "t2", TurnRoleUser, NewTextPart("p2", clock.tick(), "again"), clock.tick())
	if !errors.Is(err, ErrTurnAlreadyRunning) {
		t.Fatalf("expected ErrTurnAlreadyRunning, got %v", err)
	}
}

func TestStartTurn_PreservesReceiverHistory(t *testing.T) {
	original, clock := freshSession(t)
	updated, err := StartTurn(original, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "hi"), clock.tick())
	if err != nil {
		t.Fatal(err)
	}
	if len(original.History) != 0 {
		t.Fatalf("original Session History was mutated: %d entries", len(original.History))
	}
	if len(updated.History) != 1 {
		t.Fatalf("updated Session History should have 1 turn, got %d", len(updated.History))
	}
}

func TestAppendPart_DoesNotShareUnderlyingSlice(t *testing.T) {
	s, clock := freshSession(t)
	s, err := StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "hi"), clock.tick())
	if err != nil {
		t.Fatal(err)
	}
	prev := s
	s, err = AppendPart(s, "t1", NewTextPart("p2", clock.tick(), "more"), clock.tick())
	if err != nil {
		t.Fatal(err)
	}
	if len(prev.History[0].Parts) != 1 {
		t.Fatalf("previous Turn Parts mutated: %d entries", len(prev.History[0].Parts))
	}
	if len(s.History[0].Parts) != 2 {
		t.Fatalf("new Turn Parts should have 2, got %d", len(s.History[0].Parts))
	}
}

func TestAppendPart_RejectsTerminalTurn(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "hi"), clock.tick())
	s, _ = CompleteTurn(s, "t1", clock.tick())
	_, err := AppendPart(s, "t1", NewTextPart("p2", clock.tick(), "late"), clock.tick())
	if !errors.Is(err, ErrTurnAlreadyTerminal) {
		t.Fatalf("expected ErrTurnAlreadyTerminal, got %v", err)
	}
}

func TestAppendPart_RejectsUnknownTurn(t *testing.T) {
	s, clock := freshSession(t)
	_, err := AppendPart(s, "ghost", NewTextPart("p1", clock.tick(), "x"), clock.tick())
	if !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("expected ErrTurnNotFound, got %v", err)
	}
}

func TestCompleteTurn_TerminalThenAnotherStartAllowed(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "first"), clock.tick())
	s, _ = CompleteTurn(s, "t1", clock.tick())
	if s.History[0].State != TurnStateCompleted {
		t.Fatalf("expected Completed, got %s", s.History[0].State)
	}
	s2, err := StartTurn(s, "t2", TurnRoleUser, NewTextPart("p2", clock.tick(), "second"), clock.tick())
	if err != nil {
		t.Fatalf("second StartTurn after complete failed: %v", err)
	}
	if len(s2.History) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(s2.History))
	}
}

func TestFailTurn_RequiresCanceledOrError(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "first"), clock.tick())
	_, err := FailTurn(s, "t1", VerdictPass, "oops", clock.tick())
	if err == nil {
		t.Fatal("FailTurn with VerdictPass should error")
	}
	s, err = FailTurn(s, "t1", VerdictError, "provider 500", clock.tick())
	if err != nil {
		t.Fatalf("FailTurn legit failure rejected: %v", err)
	}
	if s.History[0].State != TurnStateFailed || s.History[0].Verdict != VerdictError {
		t.Fatalf("FailTurn did not record terminal state: %+v", s.History[0])
	}
}

func TestRequestPermission_AttachesPending(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "go"), clock.tick())
	perm := Permission{ID: "perm1", TurnID: "t1", ToolCallID: "tc1", ToolName: "bash", Args: []byte(`{"cmd":"ls"}`)}
	s, err := RequestPermission(s, perm, clock.tick())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s.Permissions["perm1"]
	if !ok {
		t.Fatal("permission not attached")
	}
	if got.Decision != PermissionPending {
		t.Fatalf("expected pending, got %s", got.Decision)
	}
}

func TestRequestPermission_RejectsTerminalTurn(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "go"), clock.tick())
	s, _ = CompleteTurn(s, "t1", clock.tick())
	perm := Permission{ID: "perm1", TurnID: "t1", ToolCallID: "tc1", ToolName: "bash", Args: []byte(`{}`)}
	_, err := RequestPermission(s, perm, clock.tick())
	if !errors.Is(err, ErrTurnAlreadyTerminal) {
		t.Fatalf("expected ErrTurnAlreadyTerminal, got %v", err)
	}
}

func TestResolvePermission_DoubleResolveRejected(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "go"), clock.tick())
	s, _ = RequestPermission(s, Permission{ID: "perm1", TurnID: "t1", ToolCallID: "tc1", ToolName: "bash"}, clock.tick())
	s, err := ResolvePermission(s, "perm1", PermissionApproved, "", clock.tick())
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	if !s.Permissions["perm1"].IsResolved() {
		t.Fatal("permission did not flip to resolved")
	}
	_, err = ResolvePermission(s, "perm1", PermissionDenied, "changed mind", clock.tick())
	if !errors.Is(err, ErrPermissionResolved) {
		t.Fatalf("expected ErrPermissionResolved, got %v", err)
	}
}

func TestResolvePermission_PendingRejected(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "go"), clock.tick())
	s, _ = RequestPermission(s, Permission{ID: "perm1", TurnID: "t1", ToolCallID: "tc1", ToolName: "bash"}, clock.tick())
	_, err := ResolvePermission(s, "perm1", PermissionPending, "", clock.tick())
	if err == nil {
		t.Fatal("resolving to pending should error")
	}
}

func TestAttachSubAgent_RequiresExistingTurn(t *testing.T) {
	s, clock := freshSession(t)
	link := SubAgentLink{ID: "sa1", ParentSession: s.ID, ParentTurn: "ghost", ChildSession: "child1", Prompt: "go forth"}
	_, err := AttachSubAgent(s, link, clock.tick())
	if !errors.Is(err, ErrTurnNotFound) {
		t.Fatalf("expected ErrTurnNotFound, got %v", err)
	}
}

func TestAttachSubAgent_LiveByDefault(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "spawn"), clock.tick())
	link := SubAgentLink{ID: "sa1", ParentSession: s.ID, ParentTurn: "t1", ChildSession: "child1", Prompt: "investigate"}
	s, err := AttachSubAgent(s, link, clock.tick())
	if err != nil {
		t.Fatal(err)
	}
	got := s.SubAgents["sa1"]
	if got.State != SubAgentLive {
		t.Fatalf("expected Live, got %s", got.State)
	}
	if got.Verdict != "" {
		t.Fatalf("expected empty verdict, got %s", got.Verdict)
	}
}

func TestResolveSubAgent_DoubleRejected(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "go"), clock.tick())
	s, _ = AttachSubAgent(s, SubAgentLink{ID: "sa1", ParentSession: s.ID, ParentTurn: "t1", ChildSession: "child1"}, clock.tick())
	s, _ = ResolveSubAgent(s, "sa1", VerdictPass, clock.tick())
	_, err := ResolveSubAgent(s, "sa1", VerdictPass, clock.tick())
	if !errors.Is(err, ErrSubAgentResolved) {
		t.Fatalf("expected ErrSubAgentResolved, got %v", err)
	}
}

func TestSwitchModel_RejectedDuringRunningTurn(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "live"), clock.tick())
	_, err := SwitchModel(s, ModelChoice{Provider: ProviderAnthropic, Model: "claude-sonnet-4-6"}, clock.tick())
	if !errors.Is(err, ErrTurnAlreadyRunning) {
		t.Fatalf("expected ErrTurnAlreadyRunning, got %v", err)
	}
}

func TestSwitchModel_AppliesWhenIdle(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "first"), clock.tick())
	s, _ = CompleteTurn(s, "t1", clock.tick())
	choice := ModelChoice{Provider: ProviderAnthropic, Model: "claude-sonnet-4-6"}
	s, err := SwitchModel(s, choice, clock.tick())
	if err != nil {
		t.Fatal(err)
	}
	if s.Model.Model != "claude-sonnet-4-6" {
		t.Fatalf("model not switched: %+v", s.Model)
	}
}

func TestImmutability_AppendPartCopiesPartsSlice(t *testing.T) {
	// Capacity-driven aliasing trap: starting with 1 element then appending
	// would normally re-use underlying array if cap > len. Verify withPart
	// uses a fresh allocation regardless of underlying capacity.
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "x"), clock.tick())
	original := s
	s, _ = AppendPart(s, "t1", NewTextPart("p2", clock.tick(), "y"), clock.tick())

	// Tamper-test: mutate the new Session's Parts slice and ensure the
	// original is unaffected.
	s.History[0].Parts[0] = NewTextPart("hijack", clock.tick(), "tamper")
	if tp, ok := original.History[0].Parts[0].(TextPart); !ok || tp.Text != "x" {
		t.Fatalf("original Session aliased the new one's Parts: %+v", original.History[0].Parts[0])
	}
}

func TestImmutability_RequestPermissionFreshMap(t *testing.T) {
	s, clock := freshSession(t)
	s, _ = StartTurn(s, "t1", TurnRoleUser, NewTextPart("p1", clock.tick(), "go"), clock.tick())
	original := s
	s, _ = RequestPermission(s, Permission{ID: "perm1", TurnID: "t1"}, clock.tick())
	if _, exists := original.Permissions["perm1"]; exists {
		t.Fatal("original Session's Permissions map was shared with new Session")
	}
}
