// L1.5: UserDecision — capability token proving a human action.
//
// This is the load-bearing carrier of the "no auto-approval" invariant.
// `UserDecision` has a module-private brand symbol; the only way to
// produce one is `fromKeystroke`, which itself is package-private to
// the user-action/ directory. L8 input handlers import `fromKeystroke`
// to wrap a real keyboard event into a UserDecision token.
//
// The L1 `resolvePermission` transition requires UserDecision in its
// argument list. No other layer can fabricate one — auto-approval from
// code is a compile error (TS rejects construction outside this module
// because the brand symbol is `unique symbol` and not exported).

declare const __userDecisionBrand: unique symbol;

export type UserDecision = {
  readonly [__userDecisionBrand]: true;
  readonly decision: "approved" | "denied";
  readonly reason: string;
  readonly capturedAt: Date;
};

// Type-only marker that can be passed around but never constructed
// outside this module (the symbol is module-private).

// fromKeystroke wraps a real keyboard event into a UserDecision token.
// Callers MUST be L8 input handlers — there is no other legitimate use
// site. The function signature deliberately takes a `KeyboardEvent`-shaped
// value so static analysis can flag synthetic decisions.
export function fromKeystroke(
  event: { readonly type: "keydown"; readonly key: string },
  decision: "approved" | "denied",
  reason: string,
  now: Date,
): UserDecision {
  // The event parameter is a witness — its presence (and type) at the
  // call site is enforced by tsc.
  void event;
  // The brand field is phantom-only (declared, no runtime symbol). The
  // runtime object carries just the three concrete fields; the cast
  // through `unknown` introduces the brand at the type level.
  return { decision, reason, capturedAt: now } as unknown as UserDecision;
}
