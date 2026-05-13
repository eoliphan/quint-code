package agentcore

import "time"

// PartKind discriminates Part variants in the wire format.
// The set is closed — adding a new kind is a breaking change to Layer P.
type PartKind string

const (
	PartKindText         PartKind = "text"
	PartKindReasoning    PartKind = "reasoning"
	PartKindToolUse      PartKind = "tool_use"
	PartKindToolResult   PartKind = "tool_result"
	PartKindFileRef      PartKind = "file_ref"
	PartKindStepBoundary PartKind = "step_boundary"
)

// Part is a sealed sum type: only the variants in this file implement it.
// Each Part is an immutable value carrying its kind, ID, and the moment it
// was attached to its Turn.
type Part interface {
	Kind() PartKind
	ID() PartID
	CreatedAt() time.Time
	partSeal()
}

type partBase struct {
	id        PartID
	createdAt time.Time
}

func (b partBase) ID() PartID           { return b.id }
func (b partBase) CreatedAt() time.Time { return b.createdAt }
func (partBase) partSeal()              {}

// TextPart carries assistant or user text. Append-only — deltas materialize
// as additional TextParts; the runtime never mutates an existing TextPart's
// Text field. Compaction logic lives at G4 and produces NEW Parts.
type TextPart struct {
	partBase
	Text string
}

func (TextPart) Kind() PartKind { return PartKindText }

// ReasoningPart carries hidden chain-of-thought text streamed from providers
// that surface it (e.g. OpenAI o1, Anthropic thinking blocks). Rendered
// dimly in the TUI; not folded into LLM input by default.
type ReasoningPart struct {
	partBase
	Text string
}

func (ReasoningPart) Kind() PartKind { return PartKindReasoning }

// ToolUsePart marks the start of a tool invocation. Args is the raw
// JSON-encoded argument blob the LLM produced. ToolCallID is the
// provider-assigned identifier the matching ToolResultPart will reference.
type ToolUsePart struct {
	partBase
	ToolCallID string
	ToolName   string
	Args       []byte
}

func (ToolUsePart) Kind() PartKind { return PartKindToolUse }

// ToolResultPart carries the typed outcome of a tool invocation. IsError is
// distinct from a non-zero exit code — it signals that the tool layer
// itself failed (timeout, cancelled, dispatch error) and the LLM should
// reason about the failure.
type ToolResultPart struct {
	partBase
	ToolCallID string
	ToolName   string
	Content    string
	IsError    bool
}

func (ToolResultPart) Kind() PartKind { return PartKindToolResult }

// FileRefPart records that a tool attached a file to the Turn. Path is
// project-relative when possible. The TUI uses this to render an inline
// chip and offers a viewer command.
type FileRefPart struct {
	partBase
	Path     string
	MIMEType string
	Bytes    int64
}

func (FileRefPart) Kind() PartKind { return PartKindFileRef }

// StepBoundaryPart marks a logical step inside a Turn — used by providers
// that batch tool calls into agentic "steps". Lets the TUI fold sections.
// Carries no payload beyond its own identity and timestamp.
type StepBoundaryPart struct {
	partBase
	Label string
}

func (StepBoundaryPart) Kind() PartKind { return PartKindStepBoundary }

// NewTextPart, NewReasoningPart, ... are the only constructors. They stamp
// CreatedAt at call time. Tests override via the now closure passed to
// transition functions; production calls use time.Now.

func newPartBase(id PartID, now time.Time) partBase {
	return partBase{id: id, createdAt: now}
}

func NewTextPart(id PartID, now time.Time, text string) TextPart {
	return TextPart{partBase: newPartBase(id, now), Text: text}
}

func NewReasoningPart(id PartID, now time.Time, text string) ReasoningPart {
	return ReasoningPart{partBase: newPartBase(id, now), Text: text}
}

func NewToolUsePart(id PartID, now time.Time, callID, name string, args []byte) ToolUsePart {
	// Args carries the raw JSON the LLM produced. Downstream wire formats
	// (PartToolUseStartedEvent.Args, toolUseBody.Args) hold it as a
	// json.RawMessage, and a non-nil empty []byte marshals as invalid JSON
	// ("unexpected end of JSON input") which aborts event encoding. A
	// no-argument tool call is normal — collapse empty/nil inputs to a nil
	// Args so RawMessage encodes as JSON null instead.
	var cloned []byte
	if len(args) > 0 {
		cloned = make([]byte, len(args))
		copy(cloned, args)
	}
	return ToolUsePart{
		partBase:   newPartBase(id, now),
		ToolCallID: callID,
		ToolName:   name,
		Args:       cloned,
	}
}

func NewToolResultPart(id PartID, now time.Time, callID, name, content string, isError bool) ToolResultPart {
	return ToolResultPart{
		partBase:   newPartBase(id, now),
		ToolCallID: callID,
		ToolName:   name,
		Content:    content,
		IsError:    isError,
	}
}

func NewFileRefPart(id PartID, now time.Time, path, mime string, bytes int64) FileRefPart {
	return FileRefPart{
		partBase: newPartBase(id, now),
		Path:     path,
		MIMEType: mime,
		Bytes:    bytes,
	}
}

func NewStepBoundaryPart(id PartID, now time.Time, label string) StepBoundaryPart {
	return StepBoundaryPart{
		partBase: newPartBase(id, now),
		Label:    label,
	}
}
