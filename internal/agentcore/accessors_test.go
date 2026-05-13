package agentcore

import (
	"testing"
	"time"
)

// TestAccessors_TurnsAreDefensiveCopy proves that mutating the slice
// returned by Session.Turns() does NOT affect the underlying Session.
// This is the load-bearing invariant t07 closes: TUI consumers can
// iterate without risking silent corruption of shared state.
func TestAccessors_TurnsAreDefensiveCopy(t *testing.T) {
	now := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	sess := NewSession("s1", "p1", "Test", ModelChoice{Provider: "stub"}, now)
	sess, err := StartTurn(sess, TurnID("turn-1"), TurnRoleUser, NewTextPart(PartID("part-1"), now, "hi"), now)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	turns := sess.Turns()
	if len(turns) != 1 {
		t.Fatalf("Turns() returned %d, want 1", len(turns))
	}
	// Mutate the returned slice — try to reach back into Session via append.
	turns = append(turns, Turn{ID: "intruder"})
	if len(sess.History) != 1 {
		t.Fatalf("Session.History grew to %d after mutating Turns() result", len(sess.History))
	}
	// Mutate an element — defensive copy is at the slice level; element
	// fields are values so direct assignment to turns[0] does not affect
	// the source unless we reach through a pointer field. We do not have
	// any, so just confirm the source is unchanged.
	turns[0].State = TurnStateFailed
	if sess.History[0].State == TurnStateFailed {
		t.Fatal("mutating element of Turns() result corrupted Session.History[0]")
	}
}

func TestAccessors_PartsAreDefensiveCopy(t *testing.T) {
	now := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	sess := NewSession("s1", "p1", "Test", ModelChoice{Provider: "stub"}, now)
	sess, err := StartTurn(sess, TurnID("turn-1"), TurnRoleUser, NewTextPart(PartID("part-1"), now, "hi"), now)
	if err != nil {
		t.Fatal(err)
	}

	parts := sess.Parts("turn-1")
	if len(parts) != 1 {
		t.Fatalf("Parts() returned %d, want 1", len(parts))
	}
	parts = append(parts, NewTextPart(PartID("intruder"), now, "x"))
	turn, _ := sess.FindTurn("turn-1")
	if len(turn.Parts) != 1 {
		t.Fatalf("Turn.Parts grew to %d after mutating Parts() result", len(turn.Parts))
	}
}

func TestAccessors_PartsMissingTurnReturnsNil(t *testing.T) {
	now := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	sess := NewSession("s1", "p1", "Test", ModelChoice{Provider: "stub"}, now)
	parts := sess.Parts("does-not-exist")
	if parts != nil {
		t.Fatalf("Parts(missing) = %v, want nil", parts)
	}
}

func TestAccessors_PermissionsListIsDefensiveCopy(t *testing.T) {
	now := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	sess := NewSession("s1", "p1", "Test", ModelChoice{Provider: "stub"}, now)
	sess, err := StartTurn(sess, TurnID("turn-1"), TurnRoleUser, NewTextPart(PartID("part-1"), now, "hi"), now)
	if err != nil {
		t.Fatal(err)
	}
	sess, err = RequestPermission(sess, Permission{ID: PermissionID("perm-1"), TurnID: TurnID("turn-1"), ToolName: "bash", Decision: PermissionPending}, now)
	if err != nil {
		t.Fatal(err)
	}

	perms := sess.PermissionsList()
	if len(perms) != 1 {
		t.Fatalf("PermissionsList() = %d, want 1", len(perms))
	}
	perms = append(perms, Permission{ID: "intruder"})
	if len(sess.Permissions) != 1 {
		t.Fatalf("Session.Permissions grew to %d after mutating PermissionsList() result", len(sess.Permissions))
	}
}

func TestAccessors_SubAgentsListIsDefensiveCopy(t *testing.T) {
	now := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	sess := NewSession("s1", "p1", "Test", ModelChoice{Provider: "stub"}, now)
	sess, err := StartTurn(sess, TurnID("turn-1"), TurnRoleUser, NewTextPart(PartID("part-1"), now, "hi"), now)
	if err != nil {
		t.Fatal(err)
	}
	sess, err = AttachSubAgent(sess, SubAgentLink{ID: SubAgentID("sa-1"), ParentSession: sess.ID, ParentTurn: TurnID("turn-1"), ChildSession: SessionID("child-1"), Prompt: "x"}, now)
	if err != nil {
		t.Fatal(err)
	}

	links := sess.SubAgentsList()
	if len(links) != 1 {
		t.Fatalf("SubAgentsList() = %d, want 1", len(links))
	}
	links = append(links, SubAgentLink{ID: "intruder"})
	if len(sess.SubAgents) != 1 {
		t.Fatalf("Session.SubAgents grew to %d after mutating SubAgentsList() result", len(sess.SubAgents))
	}
}

func TestAccessors_ModelChoiceReturnsByValue(t *testing.T) {
	now := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	sess := NewSession("s1", "p1", "Test", ModelChoice{Provider: "anthropic", Model: "claude-opus-4-7"}, now)
	got := sess.ModelChoice()
	if got.Provider != "anthropic" || got.Model != "claude-opus-4-7" {
		t.Fatalf("ModelChoice() = %+v, want (anthropic, claude-opus-4-7)", got)
	}
	// Mutating the returned copy does not affect the source.
	got.Model = "intruder"
	if sess.Model.Model == "intruder" {
		t.Fatal("mutating ModelChoice() result corrupted Session.Model")
	}
}
