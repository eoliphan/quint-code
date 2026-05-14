// L6: Footer key-hint bar — reads visible hints from KeymapStore.
//
// Inline text markup only: <text> is the block-level OpenTUI primitive
// and CANNOT be nested inside its inline children (span / b / strong /
// i / em / u / a). Re-using TextView here would mount a nested <text>
// inside <span> and the OpenTUI Solid reconciler enters an infinite
// child-resolution loop. Inline highlights apply `fg` directly to the
// inline element.

import { type JSX, For } from "solid-js";
import { useTheme } from "../theme-context.js";

export interface KeyHintBarProps {
  readonly hints: () => ReadonlyArray<{ readonly key: string; readonly action: string }>;
}

export function KeyHintBar(props: KeyHintBarProps): JSX.Element {
  const theme = useTheme();
  const keyColor = (): string => {
    const t = theme();
    const v = t["toolName"];
    return typeof v === "string" ? v : t.fg;
  };
  const baseColor = (): string => {
    const t = theme();
    const v = t["fgDim"];
    return typeof v === "string" ? v : t.fg;
  };
  return (
    <text fg={baseColor()}>
      <For each={props.hints()}>
        {(hint, idx) => (
          <span>
            {idx() > 0 ? " · " : ""}
            <b style={{ fg: keyColor() }}>{hint.key}</b>
            {" "}
            {hint.action}
          </span>
        )}
      </For>
    </text>
  );
}
