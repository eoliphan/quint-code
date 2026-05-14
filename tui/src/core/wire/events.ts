// L2: AgentEvent wire schemas.
//
// Tagged-envelope discriminated union over `kind`. Each variant decodes
// to a typed event. Unknown kinds fail the decode with a typed ParseError
// rather than silently dropping — L3 transport surfaces them to the
// caller, who may log or ignore.

import { Schema } from "effect";
import { ModelChoiceSchema } from "./model-choice.js";

const TimestampSchema = Schema.String; // RFC3339Nano UTC

const SessionEventBase = Schema.Struct({
  session_id: Schema.String,
  at: TimestampSchema,
});

const TurnEventBase = Schema.extend(
  SessionEventBase,
  Schema.Struct({ turn_id: Schema.String }),
);

const VerdictSchema = Schema.Literal("pass", "canceled", "error");

// ---- Session lifecycle ----

export const SessionCreatedSchema = Schema.extend(
  SessionEventBase,
  Schema.Struct({
    kind: Schema.Literal("session.created"),
    project_id: Schema.String,
    title: Schema.String,
    model: ModelChoiceSchema,
  }),
);

export const SessionUpdatedSchema = Schema.extend(
  SessionEventBase,
  Schema.Struct({
    kind: Schema.Literal("session.updated"),
    title: Schema.optional(Schema.String),
  }),
);

export const SessionArchivedSchema = Schema.extend(
  SessionEventBase,
  Schema.Struct({
    kind: Schema.Literal("session.archived"),
  }),
);

// ---- Turn lifecycle ----

export const TurnStartedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("turn.started"),
    role: Schema.Literal("user", "sub_agent"),
    first_part: Schema.Unknown,
  }),
);

export const TurnCompletedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("turn.completed"),
    verdict: VerdictSchema,
  }),
);

export const TurnFailedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("turn.failed"),
    verdict: VerdictSchema,
    error: Schema.String,
  }),
);

// ---- Part deltas + completions ----

export const PartTextDeltaSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("part.text.delta"),
    part_id: Schema.String,
    delta: Schema.String,
  }),
);

export const PartTextCompletedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("part.text.completed"),
    part_id: Schema.String,
    text: Schema.String,
  }),
);

export const PartReasoningDeltaSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("part.reasoning.delta"),
    part_id: Schema.String,
    delta: Schema.String,
  }),
);

export const PartReasoningCompletedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("part.reasoning.completed"),
    part_id: Schema.String,
    text: Schema.String,
  }),
);

export const PartToolUseStartedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("part.tool_use.started"),
    part_id: Schema.String,
    tool_call_id: Schema.String,
    tool_name: Schema.String,
    args: Schema.Unknown,
  }),
);

export const PartToolUseCompletedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("part.tool_use.completed"),
    part_id: Schema.String,
    tool_call_id: Schema.String,
    tool_name: Schema.String,
    content: Schema.String,
    is_error: Schema.Boolean,
  }),
);

export const PartFileRefSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("part.file_ref"),
    part_id: Schema.String,
    path: Schema.String,
    mime: Schema.String,
    bytes: Schema.Number,
  }),
);

export const PartStepBoundarySchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("part.step.boundary"),
    part_id: Schema.String,
    label: Schema.String,
  }),
);

// ---- SubAgents ----

export const SubAgentSpawnedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("subagent.spawned"),
    subagent_id: Schema.String,
    child_session: Schema.String,
    prompt: Schema.String,
  }),
);

export const SubAgentCompletedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("subagent.completed"),
    subagent_id: Schema.String,
    verdict: VerdictSchema,
  }),
);

// ---- Permissions ----

export const PermissionRequestedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("permission.requested"),
    permission_id: Schema.String,
    tool_call_id: Schema.String,
    tool_name: Schema.String,
    args: Schema.Unknown,
  }),
);

export const PermissionResolvedSchema = Schema.extend(
  TurnEventBase,
  Schema.Struct({
    kind: Schema.Literal("permission.resolved"),
    permission_id: Schema.String,
    decision: Schema.Literal("approved", "denied", "pending"),
    reason: Schema.String,
  }),
);

// ---- Model + auth ----

export const ModelSwitchedSchema = Schema.extend(
  SessionEventBase,
  Schema.Struct({
    kind: Schema.Literal("model.switched"),
    model: ModelChoiceSchema,
  }),
);

export const AuthExpiredSchema = Schema.extend(
  SessionEventBase,
  Schema.Struct({
    kind: Schema.Literal("auth.expired"),
    provider: Schema.String,
  }),
);

// ---- Closed sum ----

export const AgentEventSchema = Schema.Union(
  SessionCreatedSchema,
  SessionUpdatedSchema,
  SessionArchivedSchema,
  TurnStartedSchema,
  TurnCompletedSchema,
  TurnFailedSchema,
  PartTextDeltaSchema,
  PartTextCompletedSchema,
  PartReasoningDeltaSchema,
  PartReasoningCompletedSchema,
  PartToolUseStartedSchema,
  PartToolUseCompletedSchema,
  PartFileRefSchema,
  PartStepBoundarySchema,
  SubAgentSpawnedSchema,
  SubAgentCompletedSchema,
  PermissionRequestedSchema,
  PermissionResolvedSchema,
  ModelSwitchedSchema,
  AuthExpiredSchema,
);

export type AgentEventWire = Schema.Schema.Type<typeof AgentEventSchema>;

export const decodeAgentEvent = Schema.decodeUnknownEither(AgentEventSchema);
