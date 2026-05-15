// L7: MarkdownView — assistant-text markdown renderer.
//
// Hand-rolled line parser (same shape as the legacy Ink-based haft
// TUI MarkdownView): headings, lists, ordered lists, blockquotes,
// horizontal rules, fenced code blocks, inline bold/italic/code.
//
// We don't use OpenTUI's <markdown> renderable because it pulls in
// tree-sitter for syntax highlighting and requires a SyntaxStyle
// instance the chat surface doesn't need. The hand-rolled approach
// composes from the same inline text primitives (span/b/i) the rest
// of the TUI uses, so colours and theme tokens flow through cleanly.

import { type JSX, For } from "solid-js";
import { useTheme } from "../theme-context.js";

export interface MarkdownViewProps {
  readonly text: string;
}

type Block =
  | { kind: "h1"; text: string }
  | { kind: "h2"; text: string }
  | { kind: "h3"; text: string }
  | { kind: "li"; indent: string; text: string; marker: string }
  | { kind: "ol"; indent: string; num: string; text: string }
  | { kind: "blockquote"; text: string }
  | { kind: "hr" }
  | { kind: "empty" }
  | { kind: "p"; text: string }
  | { kind: "code"; lang: string; lines: readonly string[] };

function parseMarkdown(src: string): readonly Block[] {
  const lines = src.split("\n");
  const out: Block[] = [];
  let inCode = false;
  let codeLang = "";
  let codeBuf: string[] = [];

  for (const line of lines) {
    if (line.trimStart().startsWith("```")) {
      if (inCode) {
        out.push({ kind: "code", lang: codeLang, lines: codeBuf.slice() });
        inCode = false;
        codeBuf = [];
        codeLang = "";
      } else {
        inCode = true;
        codeLang = line.trimStart().slice(3).trim();
      }
      continue;
    }
    if (inCode) {
      codeBuf.push(line);
      continue;
    }
    if (line.startsWith("# ")) {
      out.push({ kind: "h1", text: line.slice(2) });
      continue;
    }
    if (line.startsWith("## ")) {
      out.push({ kind: "h2", text: line.slice(3) });
      continue;
    }
    if (line.startsWith("### ")) {
      out.push({ kind: "h3", text: line.slice(4) });
      continue;
    }
    const liMatch = line.match(/^(\s*)([-*])\s(.*)/);
    if (liMatch) {
      out.push({
        kind: "li",
        indent: liMatch[1] ?? "",
        marker: "•",
        text: liMatch[3] ?? "",
      });
      continue;
    }
    const olMatch = line.match(/^(\s*)(\d+\.)\s(.*)/);
    if (olMatch) {
      out.push({
        kind: "ol",
        indent: olMatch[1] ?? "",
        num: olMatch[2] ?? "",
        text: olMatch[3] ?? "",
      });
      continue;
    }
    if (line.startsWith("> ")) {
      out.push({ kind: "blockquote", text: line.slice(2) });
      continue;
    }
    if (/^-{3,}$/.test(line) || /^\*{3,}$/.test(line)) {
      out.push({ kind: "hr" });
      continue;
    }
    if (line.trim() === "") {
      out.push({ kind: "empty" });
      continue;
    }
    out.push({ kind: "p", text: line });
  }
  // Flush an unclosed code block (truncated response)
  if (inCode && codeBuf.length > 0) {
    out.push({ kind: "code", lang: codeLang, lines: codeBuf });
  }
  return out;
}

export function MarkdownView(props: MarkdownViewProps): JSX.Element {
  const theme = useTheme();
  const c = (token: string): string => {
    const t = theme();
    const v = t[token as keyof typeof t];
    return typeof v === "string" ? (v as string) : t.fg;
  };

  const blocks = (): readonly Block[] => parseMarkdown(props.text);

  return (
    <box flexDirection="column">
      <For each={blocks()}>{(b) => renderBlock(b, c)}</For>
    </box>
  );
}

function renderBlock(b: Block, c: (t: string) => string): JSX.Element {
  switch (b.kind) {
    case "h1":
      return (
        <text>
          <b style={{ fg: c("fg") }}>{b.text}</b>
        </text>
      );
    case "h2":
      return (
        <text>
          <b style={{ fg: c("toolName") }}>{b.text}</b>
        </text>
      );
    case "h3":
      return (
        <text>
          <b style={{ fg: c("fgMuted") }}>{b.text}</b>
        </text>
      );
    case "li":
      return (
        <text>
          <span style={{ fg: c("fgDim") }}>{b.indent}{b.marker} </span>
          {inlineFragments(b.text, c)}
        </text>
      );
    case "ol":
      return (
        <text>
          <span style={{ fg: c("fgDim") }}>{b.indent}{b.num} </span>
          {inlineFragments(b.text, c)}
        </text>
      );
    case "blockquote":
      return (
        <text>
          <span style={{ fg: c("toolName") }}>│ </span>
          <span style={{ fg: c("fgDim") }}>{inlineFragmentsString(b.text)}</span>
        </text>
      );
    case "hr":
      return <text fg={c("fgDim")}>{"─".repeat(60)}</text>;
    case "empty":
      return <text> </text>;
    case "code":
      return (
        <box flexDirection="column" paddingLeft={2}>
          <text fg={c("fgDim")}>{"```"}{b.lang}</text>
          <For each={b.lines}>
            {(line) => <text fg={c("toolName")}>{line}</text>}
          </For>
          <text fg={c("fgDim")}>{"```"}</text>
        </box>
      );
    case "p":
      return <text>{inlineFragments(b.text, c)}</text>;
  }
}

// --- inline formatting: **bold**, *italic*, `code`, [link](url) ---

type InlineFragment =
  | { kind: "text"; text: string }
  | { kind: "bold"; text: string }
  | { kind: "italic"; text: string }
  | { kind: "code"; text: string }
  | { kind: "link"; text: string; href: string };

function parseInline(input: string): readonly InlineFragment[] {
  const out: InlineFragment[] = [];
  let i = 0;
  let buf = "";
  const flushBuf = (): void => {
    if (buf !== "") {
      out.push({ kind: "text", text: buf });
      buf = "";
    }
  };
  while (i < input.length) {
    const ch = input[i] ?? "";
    // **bold** (two stars, must close)
    if (ch === "*" && input[i + 1] === "*") {
      const end = input.indexOf("**", i + 2);
      if (end !== -1) {
        flushBuf();
        out.push({ kind: "bold", text: input.slice(i + 2, end) });
        i = end + 2;
        continue;
      }
    }
    // *italic* (single star, must close, not part of **)
    if (
      ch === "*" &&
      input[i + 1] !== "*" &&
      (i === 0 || input[i - 1] !== "*")
    ) {
      const end = input.indexOf("*", i + 1);
      if (end !== -1 && input[end + 1] !== "*") {
        flushBuf();
        out.push({ kind: "italic", text: input.slice(i + 1, end) });
        i = end + 1;
        continue;
      }
    }
    // `code`
    if (ch === "`") {
      const end = input.indexOf("`", i + 1);
      if (end !== -1) {
        flushBuf();
        out.push({ kind: "code", text: input.slice(i + 1, end) });
        i = end + 1;
        continue;
      }
    }
    // [label](url)
    if (ch === "[") {
      const close = input.indexOf("]", i + 1);
      if (close !== -1 && input[close + 1] === "(") {
        const urlEnd = input.indexOf(")", close + 2);
        if (urlEnd !== -1) {
          flushBuf();
          out.push({
            kind: "link",
            text: input.slice(i + 1, close),
            href: input.slice(close + 2, urlEnd),
          });
          i = urlEnd + 1;
          continue;
        }
      }
    }
    buf += ch;
    i += 1;
  }
  flushBuf();
  return out;
}

function inlineFragments(text: string, c: (t: string) => string): JSX.Element[] {
  const frags = parseInline(text);
  return frags.map((f) => {
    switch (f.kind) {
      case "bold":
        return <b>{f.text}</b>;
      case "italic":
        return <i>{f.text}</i>;
      case "code":
        return (
          <span style={{ fg: c("toolName") }}>{"`"}{f.text}{"`"}</span>
        );
      case "link":
        return (
          <span style={{ fg: c("toolName") }}>{f.text}</span>
        );
      case "text":
      default:
        return <span>{f.text}</span>;
    }
  });
}

// inlineFragmentsString is a no-format variant used when we already
// know the surrounding context dims the whole line (blockquotes).
function inlineFragmentsString(text: string): string {
  // Strip markdown markers from the text so the dim blockquote
  // doesn't render with stray asterisks.
  return text
    .replace(/\*\*(.+?)\*\*/g, "$1")
    .replace(/\*(.+?)\*/g, "$1")
    .replace(/`(.+?)`/g, "$1");
}
