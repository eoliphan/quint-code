// L6: ToolDot — color-coded status dot for tool calls.
//
//   running → blinking yellow ●
//   ok      → green ●
//   error   → red ●
//
// Ported from tui-react/src/components/ToolCallView.tsx ToolDot.
// minWidth=2 keeps the column-alignment with adjacent text labels.

import { type JSX, createSignal, onCleanup, onMount } from "solid-js";
import { useTheme } from "../theme-context.js";

const DOT = "●";

export interface ToolDotProps {
  readonly state: "running" | "ok" | "error";
}

export function ToolDot(props: ToolDotProps): JSX.Element {
  const theme = useTheme();
  const [blink, setBlink] = createSignal(true);

  onMount(() => {
    if (props.state !== "running") return;
    const id = setInterval(() => setBlink((b) => !b), 500);
    onCleanup(() => clearInterval(id));
  });

  const color = (): string => {
    const t = theme();
    if (props.state === "error") return typeof t["danger"] === "string" ? (t["danger"] as string) : t.fg;
    if (props.state === "running") return typeof t["warning"] === "string" ? (t["warning"] as string) : t.fg;
    return typeof t["success"] === "string" ? (t["success"] as string) : t.fg;
  };

  const glyph = (): string => {
    if (props.state === "running") return blink() ? DOT : " ";
    return DOT;
  };

  return (
    <box minWidth={2}>
      <text fg={color()}>{glyph()}</text>
    </box>
  );
}
