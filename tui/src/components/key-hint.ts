export type KeyHint = {
  key: string;
  action: string;
};

/** renderKeyHint formats one chip. Single-key chips render verbatim;
 *  modifier chords render with " + " between segments. */
export function renderKeyHint(hint: KeyHint): string {
  return `${hint.key} ${hint.action}`;
}

/** renderHintRow joins multiple key hints with " · " separators. */
export function renderHintRow(hints: readonly KeyHint[]): string {
  return hints.map(renderKeyHint).join(" · ");
}
