// L1: Turn with typestate.
//
// Turn<'running'> has Parts being appended; verdict + endedAt are absent
// at the type level. Turn<'completed'> and Turn<'failed'> have verdict
// and endedAt as required fields. Trying to read `verdict` from a Turn
// you haven't narrowed to terminal state is a compile error.

import { type NonEmpty } from "../algebra/non-empty.js";
import { type Part } from "./part.js";
import { type TurnID, type SubAgentID } from "./ids.js";
import { type Verdict } from "./verdict.js";

export type TurnState = "running" | "completed" | "failed";
export type TurnRole = "user" | "sub_agent";

type TurnBase<S extends TurnState> = {
  readonly id: TurnID;
  readonly role: TurnRole;
  readonly parts: NonEmpty<Part>; // turn always has firstPart at creation
  readonly startedAt: Date;
  readonly state: S;
  // SubAgentID is non-empty only for role=sub_agent; identifies the
  // SubAgentLink on the parent Session.
  readonly subAgentId?: SubAgentID;
};

export type TurnRunning = TurnBase<"running">;

export type TurnCompleted = TurnBase<"completed"> & {
  readonly verdict: Extract<Verdict, "pass">;
  readonly endedAt: Date;
};

export type TurnFailed = TurnBase<"failed"> & {
  readonly verdict: Exclude<Verdict, "pass">;
  readonly endedAt: Date;
  readonly errorMessage: string;
};

export type Turn<S extends TurnState = TurnState> =
  S extends "running" ? TurnRunning :
  S extends "completed" ? TurnCompleted :
  S extends "failed" ? TurnFailed :
  TurnRunning | TurnCompleted | TurnFailed;

export type AnyTurn = TurnRunning | TurnCompleted | TurnFailed;

export function isRunning(t: AnyTurn): t is TurnRunning {
  return t.state === "running";
}

export function isCompleted(t: AnyTurn): t is TurnCompleted {
  return t.state === "completed";
}

export function isFailed(t: AnyTurn): t is TurnFailed {
  return t.state === "failed";
}

export function isTerminal(t: AnyTurn): t is TurnCompleted | TurnFailed {
  return t.state !== "running";
}
