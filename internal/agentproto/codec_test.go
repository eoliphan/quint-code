package agentproto

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

func fixedTime() time.Time {
	return time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
}

func sampleEvents(t *testing.T) []AgentEvent {
	t.Helper()
	at := fixedTime()
	model := agentcore.ModelChoice{Provider: agentcore.ProviderCodex, Model: "gpt-5.4", CredentialKey: "codex"}
	firstPart := agentcore.NewTextPart("p1", at, "hello")
	firstPayload, err := EncodePart(firstPart)
	if err != nil {
		t.Fatalf("encode first part: %v", err)
	}
	return []AgentEvent{
		SessionCreatedEvent{eventBase: eventBase{SessionID: "s1", At: at}, Title: "Demo", Model: model},
		SessionUpdatedEvent{eventBase: eventBase{SessionID: "s1", At: at}, Title: "Demo Renamed"},
		SessionArchivedEvent{eventBase: eventBase{SessionID: "s1", At: at}, Reason: "user-archived"},
		TurnStartedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, Role: agentcore.TurnRoleUser, FirstPart: firstPayload},
		TurnCompletedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}},
		TurnFailedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, Verdict: agentcore.VerdictError, Message: "provider 500"},
		PartTextDeltaEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PartID: "p2", Delta: "world"},
		PartTextCompletedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PartID: "p2", Text: "hello world"},
		PartReasoningDeltaEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PartID: "r1", Delta: "thinking"},
		PartReasoningCompletedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PartID: "r1", Text: "thinking..."},
		PartToolUseStartedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PartID: "u1", ToolCallID: "tc1", ToolName: "bash", Args: []byte(`{"cmd":"ls"}`)},
		PartToolUseCompletedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PartID: "u1", ToolCallID: "tc1", ToolName: "bash", Content: "file.go", IsError: false},
		PartFileRefEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PartID: "f1", Path: "main.go", MIMEType: "text/x-go", Bytes: 1024},
		PartStepBoundaryEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PartID: "sb1", Label: "step 1"},
		SubAgentSpawnedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, SubAgentID: "sa1", ChildSession: "child1", Prompt: "investigate"},
		SubAgentCompletedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, SubAgentID: "sa1", Verdict: agentcore.VerdictPass},
		PermissionRequestedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PermissionID: "pe1", ToolCallID: "tc1", ToolName: "bash", Args: []byte(`{"cmd":"ls"}`)},
		PermissionResolvedEvent{turnEventBase: turnEventBase{eventBase: eventBase{SessionID: "s1", At: at}, TurnID: "t1"}, PermissionID: "pe1", Decision: agentcore.PermissionApproved},
		ModelSwitchedEvent{eventBase: eventBase{SessionID: "s1", At: at}, Model: model},
		AuthExpiredEvent{eventBase: eventBase{SessionID: "s1", At: at}, Provider: agentcore.ProviderCodex, Detail: "token expired"},
	}
}

func TestEvent_RoundTripStable(t *testing.T) {
	for _, ev := range sampleEvents(t) {
		bytes, err := EncodeEvent(ev)
		if err != nil {
			t.Fatalf("encode %T: %v", ev, err)
		}
		got, err := DecodeEvent(bytes)
		if err != nil {
			t.Fatalf("decode %T: %v", ev, err)
		}
		if got.Kind() != ev.Kind() {
			t.Fatalf("kind drift: %s != %s", got.Kind(), ev.Kind())
		}
		if got.Session() != ev.Session() {
			t.Fatalf("session drift on %s: %s != %s", ev.Kind(), got.Session(), ev.Session())
		}
		if !got.Timestamp().Equal(ev.Timestamp()) {
			t.Fatalf("timestamp drift on %s: %v != %v", ev.Kind(), got.Timestamp(), ev.Timestamp())
		}
	}
}

func TestEvent_DecodeRejectsUnknownKind(t *testing.T) {
	bytes := []byte(`{"kind":"event.bogus","body":{}}`)
	_, err := DecodeEvent(bytes)
	if err == nil {
		t.Fatal("decode of unknown kind should error")
	}
}

func TestEvent_DecodeRejectsMalformedEnvelope(t *testing.T) {
	bytes := []byte(`{not json`)
	_, err := DecodeEvent(bytes)
	if err == nil {
		t.Fatal("decode of malformed JSON should error")
	}
}

func sampleCommands() []RPCCommand {
	model := agentcore.ModelChoice{Provider: agentcore.ProviderAnthropic, Model: "claude-sonnet-4-6", CredentialKey: "anthropic"}
	return []RPCCommand{
		SessionCreateCmd{ProjectID: "proj1", Title: "Test", Model: model},
		SessionListCmd{ProjectID: "proj1"},
		SessionResumeCmd{SessionID: "s1"},
		SessionRenameCmd{SessionID: "s1", Title: "New"},
		TurnSubmitCmd{SessionID: "s1", Text: "hello"},
		TurnCancelCmd{SessionID: "s1", TurnID: "t1"},
		PermissionRespondCmd{SessionID: "s1", PermissionID: "pe1", Decision: agentcore.PermissionApproved, Reason: "ok"},
		ModelSetCmd{SessionID: "s1", Model: model},
		SubAgentAttachCmd{ParentSession: "s1", ParentTurn: "t1", ChildSession: "child1", SubAgentID: "sa1", Prompt: "go forth"},
	}
}

func TestRPC_RoundTripStable(t *testing.T) {
	for _, cmd := range sampleCommands() {
		bytes, err := EncodeCommand(cmd)
		if err != nil {
			t.Fatalf("encode %T: %v", cmd, err)
		}
		got, err := DecodeCommand(bytes)
		if err != nil {
			t.Fatalf("decode %T: %v", cmd, err)
		}
		if got.Kind() != cmd.Kind() {
			t.Fatalf("kind drift: %s != %s", got.Kind(), cmd.Kind())
		}
		if !reflect.DeepEqual(got, cmd) {
			t.Fatalf("payload drift on %s\n got: %+v\nwant: %+v", cmd.Kind(), got, cmd)
		}
	}
}

func TestRPC_DecodeRejectsUnknownKind(t *testing.T) {
	bytes := []byte(`{"kind":"rpc.bogus","body":{}}`)
	_, err := DecodeCommand(bytes)
	if err == nil {
		t.Fatal("decode of unknown command should error")
	}
}

func TestPartPayload_RoundTripAllVariants(t *testing.T) {
	at := fixedTime()
	parts := []agentcore.Part{
		agentcore.NewTextPart("p1", at, "hello"),
		agentcore.NewReasoningPart("p2", at, "thinking"),
		agentcore.NewToolUsePart("p3", at, "tc1", "bash", []byte(`{"cmd":"ls"}`)),
		agentcore.NewToolResultPart("p4", at, "tc1", "bash", "file.go\n", false),
		agentcore.NewFileRefPart("p5", at, "main.go", "text/x-go", 1024),
		agentcore.NewStepBoundaryPart("p6", at, "step 1"),
	}
	for _, p := range parts {
		payload, err := EncodePart(p)
		if err != nil {
			t.Fatalf("encode %T: %v", p, err)
		}
		decoded, err := DecodePart(payload)
		if err != nil {
			t.Fatalf("decode %T: %v", p, err)
		}
		if decoded.Kind() != p.Kind() {
			t.Fatalf("kind drift: %s != %s", decoded.Kind(), p.Kind())
		}
		if decoded.ID() != p.ID() {
			t.Fatalf("id drift: %s != %s", decoded.ID(), p.ID())
		}
		if !decoded.CreatedAt().Equal(p.CreatedAt()) {
			t.Fatalf("timestamp drift on %s: %v != %v", p.Kind(), decoded.CreatedAt(), p.CreatedAt())
		}
	}
}

func TestPartPayload_RejectsUnknownKind(t *testing.T) {
	payload := PartPayload{Kind: "bogus", ID: "x", Body: []byte(`{}`)}
	_, err := DecodePart(payload)
	if err == nil {
		t.Fatal("decode of unknown part kind should error")
	}
}

func TestErrUnknownKind_Sentinel(t *testing.T) {
	// Sanity check the sentinel is exported and comparable with errors.Is.
	wrapped := errors.New("agentproto: unknown kind: surprise")
	if errors.Is(wrapped, ErrUnknownKind) {
		t.Fatal("errors.Is should not match unrelated error text")
	}
}
