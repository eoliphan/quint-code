// L1: Verdict — closed sum.
//
// Mirrors agentcore.Verdict on the Go side; the wire schema (L2)
// validates that a raw string from the network resolves to exactly one of
// these tags.

export type Verdict = "pass" | "canceled" | "error";

export const VERDICTS = ["pass", "canceled", "error"] as const satisfies readonly Verdict[];
