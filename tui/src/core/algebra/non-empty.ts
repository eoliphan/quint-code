// L0: NonEmpty<T> — readonly tuple with proven head.
//
// `readonly [T, ...T[]]` carries the invariant at compile time: the head
// element always exists, so consumers can read it without an Option
// dance. Construction via `nonEmpty(head, ...tail)` is the only entry.

export type NonEmpty<T> = readonly [T, ...T[]];

export function nonEmpty<T>(head: T, ...tail: readonly T[]): NonEmpty<T> {
  return [head, ...tail];
}

export function head<T>(ne: NonEmpty<T>): T {
  return ne[0];
}

export function tail<T>(ne: NonEmpty<T>): readonly T[] {
  return ne.slice(1);
}

export function append<T>(ne: NonEmpty<T>, item: T): NonEmpty<T> {
  return [ne[0], ...ne.slice(1), item];
}

export function fromArray<T>(arr: readonly T[]): NonEmpty<T> | null {
  if (arr.length === 0) return null;
  return arr as unknown as NonEmpty<T>;
}
