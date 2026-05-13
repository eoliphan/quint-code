import { type Brand } from "../core/types.js";

export type SessionID = Brand<string, "SessionID">;
export type TurnID = Brand<string, "TurnID">;
export type PartID = Brand<string, "PartID">;
export type PermissionID = Brand<string, "PermissionID">;
export type SubAgentID = Brand<string, "SubAgentID">;

export const sessionID = (raw: string): SessionID => raw as SessionID;
export const turnID = (raw: string): TurnID => raw as TurnID;
export const partID = (raw: string): PartID => raw as PartID;
export const permissionID = (raw: string): PermissionID => raw as PermissionID;
export const subAgentID = (raw: string): SubAgentID => raw as SubAgentID;

export type ModelChoice = {
  provider: string;
  model: string;
  credential_key?: string;
};

export type AuthStatus = {
  provider: string;
  model: string;
  has_credentials: boolean;
  expires_at?: string;
};

export type Verdict = "pass" | "canceled" | "error";

export type TurnState = "running" | "completed" | "failed";

export type PartKind =
  | "text"
  | "reasoning"
  | "tool_use"
  | "tool_result"
  | "file_ref"
  | "step_boundary";

/** Tagged-envelope kind discriminator, matches Go EventKind constants. */
export type EventKind =
  | "session.created"
  | "session.updated"
  | "session.archived"
  | "turn.started"
  | "turn.completed"
  | "turn.failed"
  | "part.text.delta"
  | "part.text.completed"
  | "part.reasoning.delta"
  | "part.reasoning.completed"
  | "part.tool_use.started"
  | "part.tool_use.completed"
  | "part.file_ref"
  | "part.step.boundary"
  | "subagent.spawned"
  | "subagent.completed"
  | "permission.requested"
  | "permission.resolved"
  | "model.switched"
  | "auth.expired";

type EventBase = {
  session_id: SessionID;
  at: string; // RFC3339Nano UTC per agentproto.timeStamp wrapper
};

type TurnEventBase = EventBase & {
  turn_id: TurnID;
};

export type SessionCreatedEvent = EventBase & {
  kind: "session.created";
  project_id: string;
  title: string;
  model: ModelChoice;
};

export type SessionUpdatedEvent = EventBase & {
  kind: "session.updated";
  title?: string;
};

export type SessionArchivedEvent = EventBase & {
  kind: "session.archived";
};

export type TurnStartedEvent = TurnEventBase & {
  kind: "turn.started";
  role: "user" | "sub_agent";
  first_part: unknown; // PartPayload tagged-envelope — decoded by route layer
};

export type TurnCompletedEvent = TurnEventBase & {
  kind: "turn.completed";
  verdict: Verdict;
};

export type TurnFailedEvent = TurnEventBase & {
  kind: "turn.failed";
  verdict: Verdict;
  error: string;
};

export type PartTextDeltaEvent = TurnEventBase & {
  kind: "part.text.delta";
  part_id: PartID;
  delta: string;
};

export type PartTextCompletedEvent = TurnEventBase & {
  kind: "part.text.completed";
  part_id: PartID;
  text: string;
};

export type PartReasoningDeltaEvent = TurnEventBase & {
  kind: "part.reasoning.delta";
  part_id: PartID;
  delta: string;
};

export type PartReasoningCompletedEvent = TurnEventBase & {
  kind: "part.reasoning.completed";
  part_id: PartID;
  text: string;
};

export type PartToolUseStartedEvent = TurnEventBase & {
  kind: "part.tool_use.started";
  part_id: PartID;
  tool_call_id: string;
  tool_name: string;
  args: unknown; // raw JSON the LLM produced
};

export type PartToolUseCompletedEvent = TurnEventBase & {
  kind: "part.tool_use.completed";
  part_id: PartID;
  tool_call_id: string;
  tool_name: string;
  content: string;
  is_error: boolean;
};

export type PartFileRefEvent = TurnEventBase & {
  kind: "part.file_ref";
  part_id: PartID;
  path: string;
  mime: string;
  bytes: number;
};

export type PartStepBoundaryEvent = TurnEventBase & {
  kind: "part.step.boundary";
  part_id: PartID;
  label: string;
};

export type SubAgentSpawnedEvent = TurnEventBase & {
  kind: "subagent.spawned";
  subagent_id: SubAgentID;
  child_session: SessionID;
  prompt: string;
};

export type SubAgentCompletedEvent = TurnEventBase & {
  kind: "subagent.completed";
  subagent_id: SubAgentID;
  verdict: Verdict;
};

export type PermissionRequestedEvent = TurnEventBase & {
  kind: "permission.requested";
  permission_id: PermissionID;
  tool_call_id: string;
  tool_name: string;
  args: unknown;
};

export type PermissionResolvedEvent = TurnEventBase & {
  kind: "permission.resolved";
  permission_id: PermissionID;
  decision: "approved" | "denied" | "pending";
  reason: string;
};

export type ModelSwitchedEvent = EventBase & {
  kind: "model.switched";
  model: ModelChoice;
};

export type AuthExpiredEvent = EventBase & {
  kind: "auth.expired";
  provider: string;
};

/** Closed sum over every event variant on the agent wire. */
export type AgentEvent =
  | SessionCreatedEvent
  | SessionUpdatedEvent
  | SessionArchivedEvent
  | TurnStartedEvent
  | TurnCompletedEvent
  | TurnFailedEvent
  | PartTextDeltaEvent
  | PartTextCompletedEvent
  | PartReasoningDeltaEvent
  | PartReasoningCompletedEvent
  | PartToolUseStartedEvent
  | PartToolUseCompletedEvent
  | PartFileRefEvent
  | PartStepBoundaryEvent
  | SubAgentSpawnedEvent
  | SubAgentCompletedEvent
  | PermissionRequestedEvent
  | PermissionResolvedEvent
  | ModelSwitchedEvent
  | AuthExpiredEvent;
