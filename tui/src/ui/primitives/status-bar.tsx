// L6: StatusBar — bottom-row context line.
//
// Single-line, dim text with ∙ separators. Surfaces (in order):
//   <project shortened path> ∙ <git branch> ∙ <provider/model>
//   ∙ <streaming indicator> ∙ <toast count>
//
// Layout mirrors the legacy Ink-based StatusBar (see
// tui-react/src/components/StatusBar.tsx) — operators have the
// same visual reference points across both TUIs.

import { type JSX, For, Show } from "solid-js";
import { useTheme } from "../theme-context.js";

export interface StatusBarItem {
  readonly label: string;
  readonly tone?: "muted" | "accent" | "warning" | "danger" | "success";
  readonly bold?: boolean;
}

export interface StatusBarProps {
  readonly items: () => ReadonlyArray<StatusBarItem>;
}

export function StatusBar(props: StatusBarProps): JSX.Element {
  const theme = useTheme();
  const fgFor = (tone: StatusBarItem["tone"]): string => {
    const t = theme();
    switch (tone) {
      case "accent":
        return typeof t["toolName"] === "string" ? (t["toolName"] as string) : t.fg;
      case "warning":
        return typeof t["warning"] === "string" ? (t["warning"] as string) : t.fg;
      case "danger":
        return typeof t["danger"] === "string" ? (t["danger"] as string) : t.fg;
      case "success":
        return typeof t["success"] === "string" ? (t["success"] as string) : t.fg;
      case "muted":
      default:
        return typeof t["fgDim"] === "string" ? (t["fgDim"] as string) : t.fg;
    }
  };
  const sep = (): string => {
    const t = theme();
    return typeof t["fgDim"] === "string" ? (t["fgDim"] as string) : t.fg;
  };

  return (
    <text>
      <For each={props.items()}>
        {(item, idx) => (
          <>
            <Show when={idx() > 0}>
              <span style={{ fg: sep() }}> ∙ </span>
            </Show>
            {item.bold ? (
              <b style={{ fg: fgFor(item.tone) }}>{item.label}</b>
            ) : (
              <span style={{ fg: fgFor(item.tone) }}>{item.label}</span>
            )}
          </>
        )}
      </For>
    </text>
  );
}

// shortenPath collapses $HOME to "~" and trims long paths from the
// left so the visible tail still identifies the project. Cap is the
// max length AFTER tilde substitution; longer paths are sliced from
// the left with a leading "…/".
export function shortenPath(absPath: string, home: string, cap: number = 40): string {
  if (absPath === "") return "";
  let p = absPath;
  if (home && p.startsWith(home)) {
    p = "~" + p.slice(home.length);
  }
  if (p.length <= cap) return p;
  // Slice from the left so the project leaf stays visible.
  return "…/" + p.slice(p.length - cap + 2);
}
