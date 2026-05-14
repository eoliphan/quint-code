// L1: Session with typestate.
//
// Session<'idle'> has no running turn — startTurn is the only transition
// that takes one. Session<'hasLiveTurn'> has exactly one running turn at
// the head of the History; the type-level invariant is enforced by the
// fact that only completeTurn / failTurn produce Session<'idle'> again.
//
// This is the load-bearing invariant: passing a Session<'hasLiveTurn'>
// into startTurn is a compile error → concurrent turns are unrepresentable.

import { type SessionID } from "./ids.js";
import { type ModelChoice } from "./model-choice.js";
import { type AnyTurn, type TurnRunning } from "./turn.js";
import { type Permission } from "./permission.js";
import { type SubAgentLink } from "./subagent.js";

export type SessionState = "idle" | "hasLiveTurn";

type SessionBase = {
  readonly id: SessionID;
  readonly projectId: string;
  readonly title: string;
  readonly model: ModelChoice;
  readonly createdAt: Date;
  readonly updatedAt: Date;
  readonly parent?: SessionID;
  readonly permissions: ReadonlyMap<string, Permission>;
  readonly subAgents: ReadonlyMap<string, SubAgentLink>;
};

export type SessionIdle = SessionBase & {
  readonly state: "idle";
  // Idle session has a history of terminal turns only — the type
  // discriminator marks every entry as completed | failed.
  readonly history: ReadonlyArray<AnyTurn>; // all terminal
  // No live turn at the type level.
};

export type SessionWithLiveTurn = SessionBase & {
  readonly state: "hasLiveTurn";
  // History head is the running turn; everything before it is terminal.
  readonly history: ReadonlyArray<AnyTurn>;
  readonly liveTurn: TurnRunning;
};

export type Session<S extends SessionState = SessionState> =
  S extends "idle" ? SessionIdle :
  S extends "hasLiveTurn" ? SessionWithLiveTurn :
  SessionIdle | SessionWithLiveTurn;

export function isIdle(s: Session): s is SessionIdle {
  return s.state === "idle";
}

export function hasLiveTurn(s: Session): s is SessionWithLiveTurn {
  return s.state === "hasLiveTurn";
}

// All turns in display order — for an idle Session this is exactly the
// history; for a Session<'hasLiveTurn'> the running turn appears at the
// end after the historical entries.
export function turns(s: Session): ReadonlyArray<AnyTurn> {
  return s.state === "idle"
    ? s.history
    : [...s.history, s.liveTurn];
}
