// L0: Brand<T, Tag> — nominal subtype carrier.
//
// Brand makes IDs nominal: TurnID and PartID are both strings at runtime
// but distinct types at compile time, so a function expecting a TurnID
// rejects a PartID argument. The brand is structurally invisible (the
// __brand field is a phantom type), so JSON serialisation roundtrips
// preserve the value verbatim.
//
// Construction is package-private — only the domain layer creates brands
// via smart constructors that validate the inner value. Calling
// `unsafeBrand` from outside L0/L1 is forbidden by convention; the
// linter rule against named imports of `unsafeBrand` from `algebra/`
// outside `core/domain/` enforces this.

export type Brand<T, Tag extends string> = T & { readonly __brand: Tag };

export function unsafeBrand<T, Tag extends string>(v: T): Brand<T, Tag> {
  return v as Brand<T, Tag>;
}
