// L6: Footer key-hint bar — reads visible hints from KeymapStore.

import { type JSX, For } from "solid-js";
import { TextView } from "./text-view.js";

export interface KeyHintBarProps {
  readonly hints: () => ReadonlyArray<{ readonly key: string; readonly action: string }>;
}

export function KeyHintBar(props: KeyHintBarProps): JSX.Element {
  return (
    <text fg="#666666">
      <For each={props.hints()}>
        {(hint, idx) => (
          <span>
            {idx() > 0 ? " · " : ""}
            <TextView fg="toolName">{hint.key}</TextView>
            {" "}
            {hint.action}
          </span>
        )}
      </For>
    </text>
  );
}
