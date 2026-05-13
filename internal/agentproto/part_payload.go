package agentproto

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

// PartPayload is the wire form of an agentcore.Part. It is a tagged
// envelope that round-trips through JSON without ever instantiating the
// zero value of an unknown variant. Decoding rejects unknown kinds.
//
// PartPayload is NOT used to incrementally stream deltas — those go via
// PartTextDeltaEvent / PartReasoningDeltaEvent / etc. PartPayload is for
// transport of materialized parts (e.g. the FirstPart on TurnStarted).
type PartPayload struct {
	Kind agentcore.PartKind `json:"kind"`
	ID   agentcore.PartID   `json:"id"`
	Body json.RawMessage    `json:"body"`
}

// EncodePart produces a PartPayload for the given Part.
func EncodePart(p agentcore.Part) (PartPayload, error) {
	body, err := encodePartBody(p)
	if err != nil {
		return PartPayload{}, err
	}
	return PartPayload{Kind: p.Kind(), ID: p.ID(), Body: body}, nil
}

// DecodePart reconstructs an agentcore.Part from its wire payload. Errors
// on unknown kinds and on body shapes that do not match the declared kind.
func DecodePart(payload PartPayload) (agentcore.Part, error) {
	switch payload.Kind {
	case agentcore.PartKindText:
		var b textBody
		if err := json.Unmarshal(payload.Body, &b); err != nil {
			return nil, fmt.Errorf("decode text part: %w", err)
		}
		return agentcore.NewTextPart(payload.ID, b.At.Time(), b.Text), nil
	case agentcore.PartKindReasoning:
		var b reasoningBody
		if err := json.Unmarshal(payload.Body, &b); err != nil {
			return nil, fmt.Errorf("decode reasoning part: %w", err)
		}
		return agentcore.NewReasoningPart(payload.ID, b.At.Time(), b.Text), nil
	case agentcore.PartKindToolUse:
		var b toolUseBody
		if err := json.Unmarshal(payload.Body, &b); err != nil {
			return nil, fmt.Errorf("decode tool_use part: %w", err)
		}
		return agentcore.NewToolUsePart(payload.ID, b.At.Time(), b.ToolCallID, b.ToolName, b.Args), nil
	case agentcore.PartKindToolResult:
		var b toolResultBody
		if err := json.Unmarshal(payload.Body, &b); err != nil {
			return nil, fmt.Errorf("decode tool_result part: %w", err)
		}
		return agentcore.NewToolResultPart(payload.ID, b.At.Time(), b.ToolCallID, b.ToolName, b.Content, b.IsError), nil
	case agentcore.PartKindFileRef:
		var b fileRefBody
		if err := json.Unmarshal(payload.Body, &b); err != nil {
			return nil, fmt.Errorf("decode file_ref part: %w", err)
		}
		return agentcore.NewFileRefPart(payload.ID, b.At.Time(), b.Path, b.MIMEType, b.Bytes), nil
	case agentcore.PartKindStepBoundary:
		var b stepBoundaryBody
		if err := json.Unmarshal(payload.Body, &b); err != nil {
			return nil, fmt.Errorf("decode step_boundary part: %w", err)
		}
		return agentcore.NewStepBoundaryPart(payload.ID, b.At.Time(), b.Label), nil
	}
	return nil, fmt.Errorf("agentproto: unknown part kind %q", payload.Kind)
}

func encodePartBody(p agentcore.Part) (json.RawMessage, error) {
	switch v := p.(type) {
	case agentcore.TextPart:
		return json.Marshal(textBody{At: timeStamp(v.CreatedAt()), Text: v.Text})
	case agentcore.ReasoningPart:
		return json.Marshal(reasoningBody{At: timeStamp(v.CreatedAt()), Text: v.Text})
	case agentcore.ToolUsePart:
		return json.Marshal(toolUseBody{At: timeStamp(v.CreatedAt()), ToolCallID: v.ToolCallID, ToolName: v.ToolName, Args: v.Args})
	case agentcore.ToolResultPart:
		return json.Marshal(toolResultBody{At: timeStamp(v.CreatedAt()), ToolCallID: v.ToolCallID, ToolName: v.ToolName, Content: v.Content, IsError: v.IsError})
	case agentcore.FileRefPart:
		return json.Marshal(fileRefBody{At: timeStamp(v.CreatedAt()), Path: v.Path, MIMEType: v.MIMEType, Bytes: v.Bytes})
	case agentcore.StepBoundaryPart:
		return json.Marshal(stepBoundaryBody{At: timeStamp(v.CreatedAt()), Label: v.Label})
	}
	return nil, errors.New("agentproto: cannot encode unknown Part variant")
}

type textBody struct {
	At   timeStamp `json:"at"`
	Text string    `json:"text"`
}

type reasoningBody struct {
	At   timeStamp `json:"at"`
	Text string    `json:"text"`
}

type toolUseBody struct {
	At         timeStamp       `json:"at"`
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	Args       json.RawMessage `json:"args"`
}

type toolResultBody struct {
	At         timeStamp `json:"at"`
	ToolCallID string    `json:"tool_call_id"`
	ToolName   string    `json:"tool_name"`
	Content    string    `json:"content"`
	IsError    bool      `json:"is_error"`
}

type fileRefBody struct {
	At       timeStamp `json:"at"`
	Path     string    `json:"path"`
	MIMEType string    `json:"mime_type"`
	Bytes    int64     `json:"bytes"`
}

type stepBoundaryBody struct {
	At    timeStamp `json:"at"`
	Label string    `json:"label"`
}
