// L0: Option<T> — explicit absence.
//
// We deliberately do NOT use `T | undefined` or `T | null` for absence:
// those leak as field-set vs field-missing ambiguities (object literal
// with `key: undefined` is structurally different from object without
// the key). Option<T> is a tagged sum with a discriminator field, so
// pattern-match exhaustiveness is enforced.

export type Option<T> =
  | { readonly some: true; readonly value: T }
  | { readonly some: false };

const NONE: Option<never> = { some: false };

export function some<T>(value: T): Option<T> {
  return { some: true, value };
}

export function none<T = never>(): Option<T> {
  return NONE as Option<T>;
}

export function isSome<T>(o: Option<T>): o is { some: true; value: T } {
  return o.some;
}

export function isNone<T>(o: Option<T>): o is { some: false } {
  return !o.some;
}

export function map<T, U>(o: Option<T>, f: (t: T) => U): Option<U> {
  return o.some ? some(f(o.value)) : o;
}

export function flatMap<T, U>(o: Option<T>, f: (t: T) => Option<U>): Option<U> {
  return o.some ? f(o.value) : o;
}

export function getOrElse<T>(o: Option<T>, fallback: () => T): T {
  return o.some ? o.value : fallback();
}

export function fromNullable<T>(v: T | null | undefined): Option<T> {
  return v === null || v === undefined ? none() : some(v);
}
