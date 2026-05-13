import { type ErrorMapper, RPCError } from "../core/types.js";

/** Domain-specific RPCError kinds the agent surface emits. The base
 *  RPCError.kind union covers transport-level categories; this set
 *  expands the message to include the specific agent invariant. */
export type AgentErrorKind =
  | "turn_already_running" // 409 — replayed running turn
  | "turn_mismatch"        // 409 — cancel names a different turn
  | "turn_not_running"     // 404 — cancel for a session with no live turn
  | "permission_unknown"   // 404 — POST /permission/<id> for a stale id
  | "unsupported_command"  // 400 — StoreDispatcher rejecting turn submit
  | "validation"           // 400 — generic body validation failure
  | "session_not_found";   // 404

/** Lookup of well-known agentserver error markers (matches the strings
 *  the Go side returns via errors.Is mapping). */
const MARKERS: Array<[AgentErrorKind, RegExp]> = [
  ["turn_already_running", /turn already running/i],
  ["turn_mismatch", /turn id does not match running turn/i],
  ["turn_not_running", /no running turn on session/i],
  ["permission_unknown", /permission not found|unknown permission/i],
  ["unsupported_command", /dispatcher does not support/i],
  ["session_not_found", /session not found/i],
];

/** AgentErrorMapper wraps the default transport mapper and tags the
 *  resulting RPCError message with the matched agent-domain marker
 *  while preserving the HTTP status semantics in the transport-level
 *  kind. Callers branch on (kind, message-prefix) — kind for HTTP-layer
 *  routing, prefix for surface-specific recovery. The original Go
 *  error string remains in body for evidence. */
export const agentErrorMapper: ErrorMapper = (status, body) => {
  const transportKind =
    status === 400 ? "bad_request" :
    status === 404 ? "not_found" :
    status === 409 ? "conflict" :
    status >= 500 ? "server_error" : "server_error";
  for (const [marker, re] of MARKERS) {
    if (re.test(body)) {
      return new RPCError(transportKind, status, body, `${marker}: ${body}`);
    }
  }
  if (status === 400) return new RPCError("bad_request", status, body, `validation: ${body}`);
  return new RPCError(transportKind, status, body, body);
};
