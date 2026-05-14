// L4: Pure Reducer — event → Session transition.
//
// Total function: every AgentEvent variant is handled, exhaustiveness
// witnessed by `assertNeverEvent` in the default branch. Mutation-free —
// always returns a new Session value. Errors (turn-ID mismatch, missing
// permission, etc.) flow through Result<Session, ReduceError>.
//
// The reducer is the bridge between the wire layer (L2) and the domain
// algebra (L1). Wire events arrive decoded; reducer maps each to the
// appropriate L1 transition and threads the new Session through.

import { type AgentEventWire } from "../wire/events.js";
import { Result, NonEmpty } from "../algebra/index.js";
import {
  type Session,
  type SessionIdle,
  type SessionWithLiveTurn,
  hasLiveTurn,
  isIdle,
} from "../domain/session.js";
import {
  type Part,
  type PartText,
  type PartReasoning,
  type PartToolUseStarted,
  type PartToolUseCompleted,
  type PartFileRef,
  type PartStepBoundary,
} from "../domain/part.js";
import {
  type SessionID,
  type TurnID,
  type PartID,
  type PermissionID,
  type SubAgentID,
  type ToolCallID,
} from "../domain/ids.js";
import { type ModelChoice } from "../domain/model-choice.js";
import { type Verdict } from "../domain/verdict.js";
import { type PermissionPending } from "../domain/permission.js";
import { type SubAgentLive } from "../domain/subagent.js";
import { unsafeBrand } from "../algebra/brand.js";
import {
  startTurn,
  appendPart,
  completeTurn,
  failTurn,
  requestPermission,
  resolvePermission,
  attachSubAgent,
  resolveSubAgent,
  type DomainError,
} from "../domain/transitions.js";

export type ReduceError =
  | { readonly kind: "domain"; readonly cause: DomainError }
  | { readonly kind: "session_not_initialized"; readonly eventKind: string }
  | { readonly kind: "wrong_session_state"; readonly expected: "idle" | "hasLiveTurn"; readonly eventKind: string }
  | { readonly kind: "session_id_mismatch"; readonly expected: SessionID; readonly got: string }
  | { readonly kind: "unknown_event_kind"; readonly raw: AgentEventWire };

// reduce takes a wire event and the current session (or undefined if no
// session has been created yet — session.created bootstraps). Returns a
// new Session<S'> where S' is the post-transition state.
export function reduce(
  session: Session | undefined,
  event: AgentEventWire,
  now: Date,
): Result.Result<Session, ReduceError> {
  switch (event.kind) {
    case "session.created":
      return Result.ok(initSession(event, now));

    case "session.updated":
      if (session === undefined) {
        return Result.err({ kind: "session_not_initialized", eventKind: event.kind });
      }
      return Result.ok({
        ...session,
        title: event.title ?? session.title,
        updatedAt: now,
      });

    case "session.archived":
      // No structural change at the session level for archived — the
      // RouteStore would navigate away. Reducer is a no-op.
      if (session === undefined) {
        return Result.err({ kind: "session_not_initialized", eventKind: event.kind });
      }
      return Result.ok({ ...session, updatedAt: now });

    case "model.switched":
      if (session === undefined) {
        return Result.err({ kind: "session_not_initialized", eventKind: event.kind });
      }
      return Result.ok({
        ...session,
        model: wireModel(event.model),
        updatedAt: now,
      });

    case "auth.expired":
      // Auth expiry is a notification event; session shape unchanged.
      if (session === undefined) {
        return Result.err({ kind: "session_not_initialized", eventKind: event.kind });
      }
      return Result.ok({ ...session, updatedAt: now });

    case "turn.started":
      if (session === undefined) {
        return Result.err({ kind: "session_not_initialized", eventKind: event.kind });
      }
      if (!isIdle(session)) {
        return Result.err({ kind: "wrong_session_state", expected: "idle", eventKind: event.kind });
      }
      return Result.ok(
        startTurn(
          session,
          unsafeBrand<string, "TurnID">(event.turn_id) as TurnID,
          event.role,
          firstPartFromEvent(event.first_part, now),
          now,
        ),
      );

    case "turn.completed":
      if (session === undefined || !hasLiveTurn(session)) {
        return Result.err({ kind: "wrong_session_state", expected: "hasLiveTurn", eventKind: event.kind });
      }
      return mapDomain(completeTurn(session, unsafeBrand<string, "TurnID">(event.turn_id) as TurnID, now));

    case "turn.failed":
      if (session === undefined || !hasLiveTurn(session)) {
        return Result.err({ kind: "wrong_session_state", expected: "hasLiveTurn", eventKind: event.kind });
      }
      return mapDomain(
        failTurn(
          session,
          unsafeBrand<string, "TurnID">(event.turn_id) as TurnID,
          event.verdict as Exclude<Verdict, "pass">,
          event.error,
          now,
        ),
      );

    case "part.text.delta":
    case "part.reasoning.delta":
      // Deltas are wire-only — the journal carries materialized
      // completed parts. Reducer drops deltas; UI may render them via
      // a separate streaming buffer outside the Session state.
      return Result.ok(session ?? noSession(event.kind));

    case "part.text.completed":
      return appendKind(session, event.turn_id, event.kind, makeTextPart(event.part_id, event.text, now));

    case "part.reasoning.completed":
      return appendKind(session, event.turn_id, event.kind, makeReasoningPart(event.part_id, event.text, now));

    case "part.tool_use.started":
      return appendKind(
        session,
        event.turn_id,
        event.kind,
        makeToolUseStartedPart(event.part_id, event.tool_call_id, event.tool_name, event.args, now),
      );

    case "part.tool_use.completed":
      return appendKind(
        session,
        event.turn_id,
        event.kind,
        makeToolUseCompletedPart(
          event.part_id,
          event.tool_call_id,
          event.tool_name,
          event.content,
          event.is_error,
          now,
        ),
      );

    case "part.file_ref":
      return appendKind(
        session,
        event.turn_id,
        event.kind,
        makeFileRefPart(event.part_id, event.path, event.mime, event.bytes, now),
      );

    case "part.step.boundary":
      return appendKind(
        session,
        event.turn_id,
        event.kind,
        makeStepBoundaryPart(event.part_id, event.label, now),
      );

    case "subagent.spawned":
      if (session === undefined) {
        return Result.err({ kind: "session_not_initialized", eventKind: event.kind });
      }
      return Result.ok(
        attachSubAgent(session, makeSubAgentLive(event, now), now),
      );

    case "subagent.completed":
      if (session === undefined) {
        return Result.err({ kind: "session_not_initialized", eventKind: event.kind });
      }
      return mapDomain(
        resolveSubAgent(
          session,
          unsafeBrand<string, "SubAgentID">(event.subagent_id) as SubAgentID,
          event.verdict,
          now,
        ),
      );

    case "permission.requested":
      if (session === undefined || !hasLiveTurn(session)) {
        return Result.err({ kind: "wrong_session_state", expected: "hasLiveTurn", eventKind: event.kind });
      }
      return Result.ok(
        requestPermission(session, makePermissionPending(event, now), now),
      );

    case "permission.resolved":
      if (session === undefined) {
        return Result.err({ kind: "session_not_initialized", eventKind: event.kind });
      }
      if (event.decision === "pending") {
        // Wire shape allows "pending" as a non-resolution announcement
        // — treat as no-op.
        return Result.ok(session);
      }
      return mapDomain(
        resolvePermission(
          session,
          unsafeBrand<string, "PermissionID">(event.permission_id) as PermissionID,
          event.decision,
          event.reason,
          now,
        ),
      );

    default:
      return assertNeverEvent(event);
  }
}

// replay folds a stream of events into a final Session, short-circuiting
// on the first reducer error. Used by tests + recovery paths.
export function replay(
  events: ReadonlyArray<AgentEventWire>,
  now: () => Date,
): Result.Result<Session | undefined, ReduceError> {
  let session: Session | undefined = undefined;
  for (const ev of events) {
    const next = reduce(session, ev, now());
    if (!next.ok) return next;
    session = next.value;
  }
  return Result.ok(session);
}

// ---- helpers ----

function initSession(
  ev: Extract<AgentEventWire, { kind: "session.created" }>,
  now: Date,
): SessionIdle {
  return {
    id: unsafeBrand<string, "SessionID">(ev.session_id) as SessionID,
    projectId: ev.project_id,
    title: ev.title,
    model: wireModel(ev.model),
    createdAt: now,
    updatedAt: now,
    state: "idle",
    history: [],
    permissions: new Map(),
    subAgents: new Map(),
  };
}

function wireModel(m: { provider: string; model: string; credential_key?: string }): ModelChoice {
  return m.credential_key !== undefined
    ? { provider: m.provider, model: m.model, credentialKey: m.credential_key }
    : { provider: m.provider, model: m.model };
}

function appendKind(
  session: Session | undefined,
  turnIdRaw: string,
  eventKind: string,
  part: Part,
): Result.Result<SessionWithLiveTurn, ReduceError> {
  if (session === undefined || !hasLiveTurn(session)) {
    return Result.err({ kind: "wrong_session_state", expected: "hasLiveTurn", eventKind });
  }
  return mapDomain(appendPart(session, unsafeBrand<string, "TurnID">(turnIdRaw) as TurnID, part, part.createdAt));
}

function mapDomain<T>(r: Result.Result<T, DomainError>): Result.Result<T, ReduceError> {
  return r.ok ? r : Result.err({ kind: "domain", cause: r.error });
}

function noSession<S>(eventKind: string): S {
  // Used for non-failing no-op paths where session may legitimately be
  // undefined (delta events arriving before session.created — should be
  // impossible in practice but we don't want to crash).
  throw new Error(`reducer: ${eventKind} received before session.created`);
}

function assertNeverEvent(_: never): never {
  throw new Error(`reducer: unhandled AgentEvent kind`);
}

// ---- part constructors ----

function makeTextPart(idRaw: string, text: string, now: Date): PartText {
  return {
    id: unsafeBrand<string, "PartID">(idRaw) as PartID,
    kind: "text",
    text,
    createdAt: now,
  };
}

function makeReasoningPart(idRaw: string, text: string, now: Date): PartReasoning {
  return {
    id: unsafeBrand<string, "PartID">(idRaw) as PartID,
    kind: "reasoning",
    text,
    createdAt: now,
  };
}

function makeToolUseStartedPart(
  idRaw: string,
  callIdRaw: string,
  toolName: string,
  args: unknown,
  now: Date,
): PartToolUseStarted {
  return {
    id: unsafeBrand<string, "PartID">(idRaw) as PartID,
    kind: "tool_use_started",
    toolCallId: unsafeBrand<string, "ToolCallID">(callIdRaw) as ToolCallID,
    toolName,
    args,
    // requiresApproval is decided by the L3 tool registry side; the
    // reducer doesn't know — default false here, the route reads the
    // companion permission.requested event to gate execution.
    requiresApproval: false,
    createdAt: now,
  };
}

function makeToolUseCompletedPart(
  idRaw: string,
  callIdRaw: string,
  toolName: string,
  content: string,
  isError: boolean,
  now: Date,
): PartToolUseCompleted {
  return {
    id: unsafeBrand<string, "PartID">(idRaw) as PartID,
    kind: "tool_use_completed",
    toolCallId: unsafeBrand<string, "ToolCallID">(callIdRaw) as ToolCallID,
    toolName,
    content,
    isError,
    createdAt: now,
  };
}

function makeFileRefPart(
  idRaw: string,
  path: string,
  mime: string,
  bytes: number,
  now: Date,
): PartFileRef {
  return {
    id: unsafeBrand<string, "PartID">(idRaw) as PartID,
    kind: "file_ref",
    path,
    mime,
    bytes,
    createdAt: now,
  };
}

function makeStepBoundaryPart(idRaw: string, label: string, now: Date): PartStepBoundary {
  return {
    id: unsafeBrand<string, "PartID">(idRaw) as PartID,
    kind: "step_boundary",
    label,
    createdAt: now,
  };
}

function firstPartFromEvent(raw: unknown, now: Date): Part {
  // turn.started carries `first_part` in the tagged-envelope shape
  // {kind, id, body:{at, text, ...}}. Extract the inner body and use
  // the part's own id when it is present so journal replays produce
  // identical part values. Fall through to step_boundary only when
  // the envelope is malformed — every well-formed text turn produces
  // a real text part here.
  if (typeof raw === "string") {
    return {
      id: unsafeBrand<string, "PartID">(`part-firstof-${now.getTime()}`) as PartID,
      kind: "text",
      text: raw,
      createdAt: now,
    };
  }
  if (raw !== null && typeof raw === "object") {
    const obj = raw as Record<string, unknown>;
    const kind = obj["kind"];
    const id = typeof obj["id"] === "string" ? (obj["id"] as string) : `part-firstof-${now.getTime()}`;
    const body = obj["body"];
    if (kind === "text" && body !== null && typeof body === "object") {
      const text = (body as Record<string, unknown>)["text"];
      if (typeof text === "string") {
        return {
          id: unsafeBrand<string, "PartID">(id) as PartID,
          kind: "text",
          text,
          createdAt: now,
        };
      }
    }
    // Legacy/flat shape: {text:"..."} with no envelope. Kept so a
    // future Go-side simplification doesn't silently break the
    // first-part rendering path.
    if (typeof obj["text"] === "string") {
      return {
        id: unsafeBrand<string, "PartID">(id) as PartID,
        kind: "text",
        text: obj["text"] as string,
        createdAt: now,
      };
    }
  }
  return {
    id: unsafeBrand<string, "PartID">(`part-firstof-${now.getTime()}`) as PartID,
    kind: "step_boundary",
    label: "turn started",
    createdAt: now,
  };
}

function makeSubAgentLive(
  ev: Extract<AgentEventWire, { kind: "subagent.spawned" }>,
  now: Date,
): SubAgentLive {
  return {
    id: unsafeBrand<string, "SubAgentID">(ev.subagent_id) as SubAgentID,
    parentSession: unsafeBrand<string, "SessionID">(ev.session_id) as SessionID,
    parentTurn: unsafeBrand<string, "TurnID">(ev.turn_id) as TurnID,
    childSession: unsafeBrand<string, "SessionID">(ev.child_session) as SessionID,
    prompt: ev.prompt,
    spawnedAt: now,
    state: "live",
  };
}

function makePermissionPending(
  ev: Extract<AgentEventWire, { kind: "permission.requested" }>,
  now: Date,
): PermissionPending {
  return {
    id: unsafeBrand<string, "PermissionID">(ev.permission_id) as PermissionID,
    turnId: unsafeBrand<string, "TurnID">(ev.turn_id) as TurnID,
    toolCallId: unsafeBrand<string, "ToolCallID">(ev.tool_call_id) as ToolCallID,
    toolName: ev.tool_name,
    args: ev.args,
    requestedAt: now,
    state: "pending",
  };
}

// NonEmpty import is required indirectly through Part — silence
// "unused import" by referencing.
void NonEmpty;
