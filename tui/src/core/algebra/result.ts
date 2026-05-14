// L0: Result<T, E> — total error channel.
//
// Every fallible function returns Result instead of throwing. Pattern
// matching on `.ok` is exhaustive — TypeScript's discriminated union
// rejects code paths that ignore either side.
//
// Composition primitives: `map` lifts a pure transformation; `flatMap`
// sequences fallible computations short-circuiting on the first error.
// Use `pipe` (Effect's `pipe`) at call sites to chain these without
// nesting parentheses.

export type Result<T, E> =
  | { readonly ok: true; readonly value: T }
  | { readonly ok: false; readonly error: E };

export function ok<T>(value: T): Result<T, never> {
  return { ok: true, value };
}

export function err<E>(error: E): Result<never, E> {
  return { ok: false, error };
}

export function isOk<T, E>(r: Result<T, E>): r is { ok: true; value: T } {
  return r.ok;
}

export function isErr<T, E>(r: Result<T, E>): r is { ok: false; error: E } {
  return !r.ok;
}

export function map<T, U, E>(r: Result<T, E>, f: (t: T) => U): Result<U, E> {
  return r.ok ? ok(f(r.value)) : r;
}

export function flatMap<T, U, E, F>(
  r: Result<T, E>,
  f: (t: T) => Result<U, F>,
): Result<U, E | F> {
  return r.ok ? f(r.value) : r;
}

export function mapErr<T, E, F>(r: Result<T, E>, f: (e: E) => F): Result<T, F> {
  return r.ok ? r : err(f(r.error));
}

// unwrap is the escape hatch — only L10 (process entry) is permitted to
// call it. Anywhere else the linter should flag it; for now, convention.
export function unwrap<T, E>(r: Result<T, E>): T {
  if (r.ok) return r.value;
  throw new Error(`unwrap on err: ${String(r.error)}`);
}
