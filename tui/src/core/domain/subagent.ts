// L1: SubAgentLink with typestate.
//
// Mirrors the agentcore SubAgentLink: a live link is one whose child
// Session has not yet finished. Once resolved, the verdict is attached
// immutably.

import { type SessionID, type SubAgentID, type TurnID } from "./ids.js";
import { type Verdict } from "./verdict.js";

export type SubAgentState = "live" | "resolved";

type SubAgentLinkBase<S extends SubAgentState> = {
  readonly id: SubAgentID;
  readonly parentSession: SessionID;
  readonly parentTurn: TurnID;
  readonly childSession: SessionID;
  readonly prompt: string;
  readonly spawnedAt: Date;
  readonly state: S;
};

export type SubAgentLive = SubAgentLinkBase<"live">;

export type SubAgentResolved = SubAgentLinkBase<"resolved"> & {
  readonly verdict: Verdict;
  readonly resolvedAt: Date;
};

export type SubAgentLink<S extends SubAgentState = SubAgentState> =
  S extends "live" ? SubAgentLive :
  S extends "resolved" ? SubAgentResolved :
  SubAgentLive | SubAgentResolved;

export function isLive(l: SubAgentLink): l is SubAgentLive {
  return l.state === "live";
}

export function isResolvedSubAgent(l: SubAgentLink): l is SubAgentResolved {
  return l.state === "resolved";
}
