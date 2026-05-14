// L0 barrel — algebraic foundation.
export * from "./brand.js";
export * as Result from "./result.js";
export * as Option from "./option.js";
export * as NonEmpty from "./non-empty.js";

// Re-export the type aliases for convenience (callers usually want the type
// without the namespace prefix).
export type { Result as ResultType } from "./result.js";
export type { Option as OptionType } from "./option.js";
export type { NonEmpty as NonEmptyType } from "./non-empty.js";
