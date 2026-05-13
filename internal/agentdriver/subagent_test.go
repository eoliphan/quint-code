package agentdriver

import (
	"context"
	"strings"
	"testing"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
)

// fakeSubAgentRunner is a deterministic SubAgentRunner that records the
// spawn invocation and returns the configured result.
type fakeSubAgentRunner struct {
	calls  []SubAgentSpawnArgs
	result SubAgentResult
	err    error
}

func (f *fakeSubAgentRunner) Spawn(_ context.Context, parent agentcore.Session, _ agentcore.TurnID, args SubAgentSpawnArgs) (SubAgentResult, error) {
	f.calls = append(f.calls, args)
	if f.err != nil {
		return SubAgentResult{}, f.err
	}
	result := f.result
	if result.Session.ID == "" {
		result.Session = parent
	}
	return result, nil
}

// runSpawnDrive boots a Driver with a scripted provider that emits one
// spawn_subagent tool call followed by a done event.
func runSpawnDrive(t *testing.T, runner SubAgentRunner, isSub bool, args []byte) ([]agentproto.AgentEvent, *fakeSubAgentRunner) {
	t.Helper()
	provider := &fakeProvider{
		events: []ProviderEvent{
			ProviderToolCall{CallID: "tc-1", Name: SubAgentSpawnToolName, Args: args},
			ProviderTurnDone{},
		},
	}
	tools := &fakeTools{}
	sink := &CollectingSink{}
	clock, _ := newFixedClock()
	d := &Driver{
		Provider:   provider,
		Tools:      tools,
		Sink:       sink,
		SubAgent:   runner,
		IsSubAgent: isSub,
		IDGen:      newCounterIDGen(),
		Now:        clock,
	}
	sess := freshSession()
	if _, err := d.Drive(context.Background(), sess, "do the thing"); err != nil {
		t.Fatalf("Drive: %v", err)
	}
	rec, _ := runner.(*fakeSubAgentRunner)
	return sink.Events, rec
}

func TestDriver_SpawnSubAgent_HappyPath(t *testing.T) {
	runner := &fakeSubAgentRunner{
		result: SubAgentResult{Content: "child finished: 12 files indexed", Verdict: agentcore.VerdictPass},
	}
	events, rec := runSpawnDrive(t, runner, false, []byte(`{"name":"explore","prompt":"map the codebase"}`))

	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 spawn call, got %d", len(rec.calls))
	}
	if rec.calls[0].Name != "explore" || rec.calls[0].Prompt != "map the codebase" {
		t.Fatalf("spawn args round-trip wrong: %+v", rec.calls[0])
	}
	if !hasToolUseStarted(events, SubAgentSpawnToolName) {
		t.Fatal("expected tool_use.started for spawn_subagent")
	}
	completed := findCompleted(events, SubAgentSpawnToolName)
	if completed == nil {
		t.Fatal("expected tool_use.completed for spawn_subagent")
	}
	if completed.IsError {
		t.Fatalf("happy-path IsError=true, content=%q", completed.Content)
	}
	if !strings.Contains(completed.Content, "child finished") {
		t.Fatalf("content=%q, expected runner output", completed.Content)
	}
}

func TestDriver_SpawnSubAgent_NestedRejected(t *testing.T) {
	runner := &fakeSubAgentRunner{
		result: SubAgentResult{Content: "should not be called", Verdict: agentcore.VerdictPass},
	}
	events, rec := runSpawnDrive(t, runner, true, []byte(`{"name":"explore","prompt":"x"}`))

	if len(rec.calls) != 0 {
		t.Fatalf("nested spawn must NOT invoke runner; got %d", len(rec.calls))
	}
	completed := findCompleted(events, SubAgentSpawnToolName)
	if completed == nil {
		t.Fatal("expected synthetic tool_use.completed on nested rejection")
	}
	if !completed.IsError {
		t.Fatal("nested rejection must mark IsError=true")
	}
	if !strings.Contains(completed.Content, "nested subagent spawn rejected") {
		t.Fatalf("content=%q, expected nested-rejection marker", completed.Content)
	}
}

func TestDriver_SpawnSubAgent_NoRunnerConfigured(t *testing.T) {
	events, _ := runSpawnDrive(t, nil, false, []byte(`{"name":"explore","prompt":"x"}`))
	completed := findCompleted(events, SubAgentSpawnToolName)
	if completed == nil {
		t.Fatal("expected synthetic tool_use.completed when SubAgent is nil")
	}
	if !completed.IsError {
		t.Fatal("missing runner must mark IsError=true")
	}
	if !strings.Contains(completed.Content, "Driver.SubAgent is nil") {
		t.Fatalf("content=%q, expected missing-runner marker", completed.Content)
	}
}

func TestDriver_SpawnSubAgent_ArgsDecodeError(t *testing.T) {
	runner := &fakeSubAgentRunner{}
	events, rec := runSpawnDrive(t, runner, false, []byte(`not json`))
	if len(rec.calls) != 0 {
		t.Fatal("runner must NOT be called when args fail to decode")
	}
	completed := findCompleted(events, SubAgentSpawnToolName)
	if completed == nil || !completed.IsError || !strings.Contains(completed.Content, "args decode") {
		t.Fatalf("expected decode-error tool_use.completed, got %+v", completed)
	}
}

// --- helpers ---

func hasToolUseStarted(events []agentproto.AgentEvent, toolName string) bool {
	for _, ev := range events {
		if started, ok := ev.(agentproto.PartToolUseStartedEvent); ok && started.ToolName == toolName {
			return true
		}
	}
	return false
}

func findCompleted(events []agentproto.AgentEvent, toolName string) *agentproto.PartToolUseCompletedEvent {
	for _, ev := range events {
		if c, ok := ev.(agentproto.PartToolUseCompletedEvent); ok && c.ToolName == toolName {
			return &c
		}
	}
	return nil
}
