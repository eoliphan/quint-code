import { type DecodeResult } from "../core/types.js";
import { type AgentEvent, type EventKind } from "./types.js";

const KNOWN_KINDS: ReadonlySet<EventKind> = new Set<EventKind>([
  "session.created",
  "session.updated",
  "session.archived",
  "turn.started",
  "turn.completed",
  "turn.failed",
  "part.text.delta",
  "part.text.completed",
  "part.reasoning.delta",
  "part.reasoning.completed",
  "part.tool_use.started",
  "part.tool_use.completed",
  "part.file_ref",
  "part.step.boundary",
  "subagent.spawned",
  "subagent.completed",
  "permission.requested",
  "permission.resolved",
  "model.switched",
  "auth.expired",
]);

/** decodeAgentEvent normalises a raw JSON object into an AgentEvent.
 *  Returns ok=false for unknown kinds OR malformed envelopes — the
 *  caller decides whether to ignore, log, or surface to the user. */
export function decodeAgentEvent(raw: unknown): DecodeResult<AgentEvent> {
  if (raw === null || typeof raw !== "object") {
    return { ok: false, reason: "malformed", error: "event is not an object", raw };
  }
  const obj = raw as { kind?: unknown };
  if (typeof obj.kind !== "string") {
    return { ok: false, reason: "malformed", error: "missing or non-string kind", raw };
  }
  if (!KNOWN_KINDS.has(obj.kind as EventKind)) {
    return { ok: false, reason: "unknown_kind", kind: obj.kind, raw };
  }
  // Trust the shape past discriminator validation. agentproto Go side
  // is the schema authority; if a field is missing here it is a Go
  // bug, not a decode bug. The route layer will surface render errors
  // honestly.
  return { ok: true, event: raw as AgentEvent };
}
