// L7: DiffView — colored unified-diff renderer with line numbers.
//
// Claude-Code-style layout:
//
//   12  │ const x = 1                     (context, dim)
//   13  │ - const y = 2                   (removal, red)
//      │ + const y = 3                    (addition, green)
//   14  │ const z = 4                     (context, dim)
//   ──  │ @@ -23,5 +23,5 @@ (hunk header, cyan)
//
// Line numbers are extracted from `@@ -old,+new @@` headers and
// tracked through context/+/- lines. Synthetic non-diff inputs
// (tool output that just has +/- prefixes) fall back to a simpler
// gutter-less render so the legacy looksLikeDiff path stays useful.

import { type JSX, For } from "solid-js";
import { useTheme } from "../theme-context.js";

export interface DiffViewProps {
  readonly diff: string;
}

type LineKind = "context" | "add" | "remove" | "hunk" | "header" | "blank";

interface LineRow {
  readonly kind: LineKind;
  readonly oldNo: number | null;
  readonly newNo: number | null;
  readonly text: string;
}

function parseDiff(src: string): readonly LineRow[] {
  const lines = src.split("\n");
  const out: LineRow[] = [];
  let oldNo: number | null = null;
  let newNo: number | null = null;

  for (const ln of lines) {
    if (ln.startsWith("@@")) {
      const m = ln.match(/^@@\s+-([0-9]+)(?:,([0-9]+))?\s+\+([0-9]+)(?:,([0-9]+))?\s+@@/);
      if (m) {
        oldNo = parseInt(m[1] ?? "0", 10);
        newNo = parseInt(m[3] ?? "0", 10);
      }
      out.push({ kind: "hunk", oldNo: null, newNo: null, text: ln });
      continue;
    }
    if (ln.startsWith("+++") || ln.startsWith("---")) {
      out.push({ kind: "header", oldNo: null, newNo: null, text: ln });
      continue;
    }
    if (ln === "") {
      out.push({ kind: "blank", oldNo: null, newNo: null, text: "" });
      continue;
    }
    if (ln.startsWith("+")) {
      const row: LineRow = { kind: "add", oldNo: null, newNo, text: ln.slice(1) };
      if (newNo !== null) newNo += 1;
      out.push(row);
      continue;
    }
    if (ln.startsWith("-")) {
      const row: LineRow = { kind: "remove", oldNo, newNo: null, text: ln.slice(1) };
      if (oldNo !== null) oldNo += 1;
      out.push(row);
      continue;
    }
    // Context: advance both counters.
    const row: LineRow = {
      kind: "context",
      oldNo,
      newNo,
      text: ln.startsWith(" ") ? ln.slice(1) : ln,
    };
    if (oldNo !== null) oldNo += 1;
    if (newNo !== null) newNo += 1;
    out.push(row);
  }
  return out;
}

export function DiffView(props: DiffViewProps): JSX.Element {
  const theme = useTheme();
  const rows = (): readonly LineRow[] => parseDiff(props.diff);
  const c = (token: string): string => {
    const t = theme();
    const v = t[token as keyof typeof t];
    return typeof v === "string" ? (v as string) : t.fg;
  };
  return (
    <box flexDirection="column">
      <For each={rows()}>{(r) => <DiffLine row={r} color={c} />}</For>
    </box>
  );
}

function DiffLine(props: { row: LineRow; color: (t: string) => string }): JSX.Element {
  const r = props.row;

  if (r.kind === "hunk") {
    return (
      <text fg={props.color("toolName")}>
        <span style={{ fg: props.color("fgDim") }}>{"     │ "}</span>
        {r.text}
      </text>
    );
  }
  if (r.kind === "header") {
    return <text fg={props.color("fgDim")}>{r.text}</text>;
  }
  if (r.kind === "blank") {
    return <text>{""}</text>;
  }

  const gutter = formatGutter(r.oldNo, r.newNo, r.kind);
  if (r.kind === "add") {
    return (
      <text>
        <span style={{ fg: props.color("fgDim") }}>{gutter}</span>
        <span style={{ fg: props.color("success") }}>+ </span>
        <span style={{ fg: props.color("success") }}>{r.text}</span>
      </text>
    );
  }
  if (r.kind === "remove") {
    return (
      <text>
        <span style={{ fg: props.color("fgDim") }}>{gutter}</span>
        <span style={{ fg: props.color("danger") }}>- </span>
        <span style={{ fg: props.color("danger") }}>{r.text}</span>
      </text>
    );
  }
  return (
    <text>
      <span style={{ fg: props.color("fgDim") }}>{gutter}</span>
      <span style={{ fg: props.color("fgDim") }}>  </span>
      <span style={{ fg: props.color("fgMuted") }}>{r.text}</span>
    </text>
  );
}

function formatGutter(
  oldNo: number | null,
  newNo: number | null,
  kind: LineKind,
): string {
  // 4-char numeric pad + " │ " separator. Removed lines show their
  // OLD line number; added show their NEW; context shows the new
  // (current) number.
  const n = kind === "remove" ? oldNo : newNo;
  const s = n === null ? "    " : String(n).padStart(4, " ");
  return `${s} │ `;
}

export function looksLikeDiff(content: string): boolean {
  if (content.length < 4) return false;
  if (content.includes("\n@@ ") || content.startsWith("@@ ")) return true;
  if (content.includes("\n--- ") && content.includes("\n+++ ")) return true;
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
