// L1: Part — discriminated sum over every concrete part kind a Turn can
// hold. Sealed at compile time: a new variant requires updating every
// switch site, and TypeScript's exhaustiveness check via `never` makes
// missed arms a compile error.
//
// FPF artifact kinds (DecisionRecord / ProblemCard / SolutionPortfolio /
// Note / EvidenceItem / WorkCommission) are first-class siblings of the
// chat kinds, not opaque tool_use payloads. This is the haft-unique
// addition that opencode does not have — the TUI renders them with
// dedicated views in L7.

import { type PartID, type ToolCallID } from "./ids.js";

export type PartKind =
  | "text"
  | "reasoning"
  | "tool_use_started"
  | "tool_use_completed"
  | "file_ref"
  | "step_boundary"
  | "decision_record"
  | "problem_card"
  | "solution_portfolio"
  | "note"
  | "evidence_item"
  | "work_commission";

type PartBase = {
  readonly id: PartID;
  readonly createdAt: Date;
};

export type PartText = PartBase & {
  readonly kind: "text";
  readonly text: string;
};

export type PartReasoning = PartBase & {
  readonly kind: "reasoning";
  readonly text: string;
};

export type PartToolUseStarted = PartBase & {
  readonly kind: "tool_use_started";
  readonly toolCallId: ToolCallID;
  readonly toolName: string;
  readonly args: unknown;
  readonly requiresApproval: boolean;
};

export type PartToolUseCompleted = PartBase & {
  readonly kind: "tool_use_completed";
  readonly toolCallId: ToolCallID;
  readonly toolName: string;
  readonly content: string;
  readonly isError: boolean;
};

export type PartFileRef = PartBase & {
  readonly kind: "file_ref";
  readonly path: string;
  readonly mime: string;
  readonly bytes: number;
};

export type PartStepBoundary = PartBase & {
  readonly kind: "step_boundary";
  readonly label: string;
};

// --- FPF artifact kinds ----------------------------------------------------

export type PartDecisionRecord = PartBase & {
  readonly kind: "decision_record";
  readonly artifactId: string;
  readonly title: string;
  readonly selected: string;
  readonly selectionPolicy: string;
  readonly whySelected: string;
  readonly counterargument: string;
  readonly weakestLink: string;
  readonly rejected: ReadonlyArray<{ readonly variant: string; readonly reason: string }>;
  readonly invariants: ReadonlyArray<string>;
  readonly predictions: ReadonlyArray<{
    readonly claim: string;
    readonly observable: string;
    readonly threshold: string;
  }>;
  readonly refreshTriggers: ReadonlyArray<string>;
  readonly rollback: {
    readonly triggers: ReadonlyArray<string>;
    readonly steps: ReadonlyArray<string>;
    readonly blastRadius: string;
  };
};

export type PartProblemCard = PartBase & {
  readonly kind: "problem_card";
  readonly artifactId: string;
  readonly title: string;
  readonly signal: string;
  readonly constraints: ReadonlyArray<string>;
  readonly acceptance: string;
  readonly problemType: "optimization" | "diagnosis" | "search" | "synthesis";
};

export type PartSolutionPortfolio = PartBase & {
  readonly kind: "solution_portfolio";
  readonly artifactId: string;
  readonly variants: ReadonlyArray<{
    readonly id: string;
    readonly title: string;
    readonly weakestLink: string;
    readonly noveltyMarker: string;
    readonly steppingStone: boolean;
  }>;
  readonly paretoFront: ReadonlyArray<string>;
};

export type PartNote = PartBase & {
  readonly kind: "note";
  readonly artifactId: string;
  readonly title: string;
  readonly rationale: string;
};

export type PartEvidenceItem = PartBase & {
  readonly kind: "evidence_item";
  readonly artifactId: string;
  readonly type: "measurement" | "test" | "research" | "benchmark" | "audit";
  readonly verdict: "supports" | "weakens" | "refutes";
  readonly content: string;
  readonly congruenceLevel: 0 | 1 | 2 | 3;
};

export type PartWorkCommission = PartBase & {
  readonly kind: "work_commission";
  readonly artifactId: string;
  readonly decisionRef: string;
  readonly status: "queued" | "preflighting" | "running" | "completed" | "failed" | "blocked" | "cancelled" | "expired";
  readonly allowedPaths: ReadonlyArray<string>;
  readonly lockset: ReadonlyArray<string>;
};

export type Part =
  | PartText
  | PartReasoning
  | PartToolUseStarted
  | PartToolUseCompleted
  | PartFileRef
  | PartStepBoundary
  | PartDecisionRecord
  | PartProblemCard
  | PartSolutionPortfolio
  | PartNote
  | PartEvidenceItem
  | PartWorkCommission;

// Exhaustiveness witness — call from default branches of switches over
// part.kind so a missing arm is a TypeScript compile error.
export function assertNeverPart(p: never): never {
  throw new Error(`unhandled part kind: ${JSON.stringify(p)}`);
}
