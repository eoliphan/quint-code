// L1: Pure Session transitions.
//
// Every function is total (returns Result, never throws) and pure (no
// I/O, no clock — `now` is injected). The signatures encode the
// inexpressibility contract:
//
//   startTurn(s: Session<'idle'>, ...)            → Session<'hasLiveTurn'>
//   appendPart(s: Session<'hasLiveTurn'>, ...)    → Session<'hasLiveTurn'>
//   completeTurn(s: Session<'hasLiveTurn'>, ...)  → Session<'idle'>
//   failTurn(s: Session<'hasLiveTurn'>, ...)      → Session<'idle'>
//
// You cannot call startTurn on a Session<'hasLiveTurn'> — TypeScript
// refuses to type-check. Concurrent turns become a compile error.

import { type Result, ok, err } from "../algebra/result.js";
import { type NonEmpty, append as neAppend } from "../algebra/non-empty.js";
import {
  type Session,
  type SessionIdle,
  type SessionWithLiveTurn,
} from "./session.js";
import {
  type TurnRunning,
  type TurnCompleted,
  type TurnFailed,
  type TurnRole,
} from "./turn.js";
import { type Part } from "./part.js";
import {
  type Permission,
  type PermissionPending,
  type PermissionResolved,
} from "./permission.js";
import {
  type SubAgentLink,
  type SubAgentLive,
  type SubAgentResolved,
} from "./subagent.js";
import { type TurnID, type PermissionID, type SubAgentID } from "./ids.js";
import { type Verdict } from "./verdict.js";
import { type UserDecision } from "../user-action/user-decision.js";

export type DomainError =
  | { readonly kind: "turn_not_found"; readonly turnId: TurnID }
  | { readonly kind: "permission_not_found"; readonly id: PermissionID }
  | { readonly kind: "permission_already_resolved"; readonly id: PermissionID }
  | { readonly kind: "subagent_not_found"; readonly id: SubAgentID }
  | { readonly kind: "subagent_already_resolved"; readonly id: SubAgentID }
  | { readonly kind: "turn_id_mismatch"; readonly expected: TurnID; readonly got: TurnID };

// --- Turn lifecycle --------------------------------------------------------

export function startTurn(
  s: SessionIdle,
  turnId: TurnID,
  role: TurnRole,
  firstPart: Part,
  now: Date,
  subAgentId?: SubAgentID,
): SessionWithLiveTurn {
  const liveTurn: TurnRunning = {
    id: turnId,
    role,
    parts: [firstPart] as NonEmpty<Part>,
    startedAt: now,
    state: "running",
    ...(subAgentId !== undefined ? { subAgentId } : {}),
  };
  return {
    ...s,
    state: "hasLiveTurn",
    updatedAt: now,
    liveTurn,
  };
}

export function appendPart(
  s: SessionWithLiveTurn,
  turnId: TurnID,
  part: Part,
  now: Date,
): Result<SessionWithLiveTurn, DomainError> {
  if (s.liveTurn.id !== turnId) {
    return err({ kind: "turn_id_mismatch", expected: s.liveTurn.id, got: turnId });
  }
  return ok({
    ...s,
    updatedAt: now,
    liveTurn: {
      ...s.liveTurn,
      parts: neAppend(s.liveTurn.parts, part),
    },
  });
}

export function completeTurn(
  s: SessionWithLiveTurn,
  turnId: TurnID,
  now: Date,
): Result<SessionIdle, DomainError> {
  if (s.liveTurn.id !== turnId) {
    return err({ kind: "turn_id_mismatch", expected: s.liveTurn.id, got: turnId });
  }
  const completed: TurnCompleted = {
    ...s.liveTurn,
    state: "completed",
    verdict: "pass",
    endedAt: now,
  };
  // history now includes the just-completed turn
  // intentionally omit liveTurn from the result — type narrowing
  // requires the field to be absent on SessionIdle.
  const { liveTurn: _omit, ...base } = s;
  void _omit;
  return ok({
    ...base,
    state: "idle",
    updatedAt: now,
    history: [...s.history, completed],
  });
}

export function failTurn(
  s: SessionWithLiveTurn,
  turnId: TurnID,
  verdict: Exclude<Verdict, "pass">,
  errorMessage: string,
  now: Date,
): Result<SessionIdle, DomainError> {
  if (s.liveTurn.id !== turnId) {
    return err({ kind: "turn_id_mismatch", expected: s.liveTurn.id, got: turnId });
  }
  const failed: TurnFailed = {
    ...s.liveTurn,
    state: "failed",
    verdict,
    endedAt: now,
    errorMessage,
  };
  const { liveTurn: _omit, ...base } = s;
  void _omit;
  return ok({
    ...base,
    state: "idle",
    updatedAt: now,
    history: [...s.history, failed],
  });
}

// --- Permissions -----------------------------------------------------------

export function requestPermission(
  s: SessionWithLiveTurn,
  perm: PermissionPending,
  now: Date,
): SessionWithLiveTurn {
  const next = new Map<string, Permission>(s.permissions);
  next.set(perm.id, perm);
  return { ...s, updatedAt: now, permissions: next };
}

// resolvePermission REQUIRES a UserDecision capability token. The token
// can only be produced by L8 keyboard input handlers — auto-approval
// from code paths is a compile error because UserDecision's constructor
// is module-private to user-action/.
export function resolvePermission<S extends SessionWithLiveTurn | SessionIdle>(
  s: S,
  id: PermissionID,
  user: UserDecision,
  now: Date,
): Result<S, DomainError> {
  const existing = s.permissions.get(id);
  if (existing === undefined) return err({ kind: "permission_not_found", id });
  if (existing.state === "resolved") return err({ kind: "permission_already_resolved", id });
  const resolved: PermissionResolved = {
    ...existing,
    state: "resolved",
    decision: user.decision,
    reason: user.reason,
    resolvedAt: now,
  };
  const next = new Map<string, Permission>(s.permissions);
  next.set(id, resolved);
  return ok({ ...s, updatedAt: now, permissions: next });
}

// --- SubAgents -------------------------------------------------------------

export function attachSubAgent<S extends Session>(
  s: S,
  link: SubAgentLive,
  now: Date,
): S {
  const next = new Map<string, SubAgentLink>(s.subAgents);
  next.set(link.id, link);
  return { ...s, updatedAt: now, subAgents: next };
}

export function resolveSubAgent<S extends Session>(
  s: S,
  id: SubAgentID,
  verdict: Verdict,
  now: Date,
): Result<S, DomainError> {
  const existing = s.subAgents.get(id);
  if (existing === undefined) return err({ kind: "subagent_not_found", id });
  if (existing.state === "resolved") return err({ kind: "subagent_already_resolved", id });
  const resolved: SubAgentResolved = {
    ...existing,
    state: "resolved",
    verdict,
    resolvedAt: now,
  };
  const next = new Map<string, SubAgentLink>(s.subAgents);
  next.set(id, resolved);
  return ok({ ...s, updatedAt: now, subAgents: next });
}
