// L7: DiffView — colored unified-diff renderer.
//
// Ported from tui-react/src/components/DiffView.tsx. Color rules:
//   @@ hunk header     → cyan
//   --- / +++ header   → cyan dim
//   +line              → green bold "+" prefix + green body
//   -line              → red bold "-" prefix + red body
//   anything else      → dim
//
// Used by tool_use_completed parts whose content matches a unified
// diff shape (write / edit / multiedit results). Detection lives in
// the part view — DiffView itself is a pure renderer over a string.

import { type JSX, For } from "solid-js";
import { useTheme } from "../theme-context.js";

export interface DiffViewProps {
  readonly diff: string;
}

export function DiffView(props: DiffViewProps): JSX.Element {
  const theme = useTheme();
  const lines = (): readonly string[] => props.diff.split("\n");
  const c = (token: string): string => {
    const t = theme();
    const v = t[token as keyof typeof t];
    return typeof v === "string" ? (v as string) : t.fg;
  };

  return (
    <box flexDirection="column">
      <For each={lines()}>{(line) => <DiffLine line={line} color={c} />}</For>
    </box>
  );
}

function DiffLine(props: { line: string; color: (t: string) => string }): JSX.Element {
  const l = props.line;
  if (l.startsWith("@@")) {
    return <text fg={props.color("toolName")}>{l}</text>;
  }
  if (l.startsWith("+++") || l.startsWith("---")) {
    return <text fg={props.color("fgDim")}>{l}</text>;
  }
  if (l.startsWith("+")) {
    return (
      <text>
        <b style={{ fg: props.color("success") }}>+</b>
        <span style={{ fg: props.color("success") }}>{l.slice(1)}</span>
      </text>
    );
  }
  if (l.startsWith("-")) {
    return (
      <text>
        <b style={{ fg: props.color("danger") }}>-</b>
        <span style={{ fg: props.color("danger") }}>{l.slice(1)}</span>
      </text>
    );
  }
  return <text fg={props.color("fgDim")}>{l}</text>;
}

// looksLikeDiff sniffs whether a tool_use_completed content payload
// is worth handing to DiffView. We don't try to be exhaustive —
// false positives just downgrade rendering to plain text. True
// positives produce the colored output.
export function looksLikeDiff(content: string): boolean {
  if (content.length < 4) return false;
  // Unified diff headers
  if (content.includes("\n@@ ") || content.startsWith("@@ ")) return true;
  if (content.includes("\n--- ") && content.includes("\n+++ ")) return true;
  // Tool output that just lists "+line" / "-line" without headers
  // (the haft edit tool's compact summary). Heuristic: at least 2
  // lines start with "+" or "-" and majority of non-blank lines do.
  const lines = content.split("\n");
  let signed = 0;
  let nonBlank = 0;
  for (const l of lines) {
    if (l.trim() === "") continue;
    nonBlank += 1;
    if (l.startsWith("+") || l.startsWith("-")) signed += 1;
  }
  return signed >= 2 && signed * 2 >= nonBlank;
}
