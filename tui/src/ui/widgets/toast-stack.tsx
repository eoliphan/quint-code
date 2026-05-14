// L7: ToastStack — overlay rendering ToastStore entries.

import { type JSX, For } from "solid-js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import type { ToastEntry } from "../../effect/state/toast-store.js";
import type { Theme } from "../../themes/index.js";

type Token = keyof Theme;

export interface ToastStackProps {
  readonly entries: () => ReadonlyArray<ToastEntry>;
}

export function ToastStack(props: ToastStackProps): JSX.Element {
  return (
    <BoxView paddingTop={1}>
      <For each={[...props.entries()]}>
        {(entry) => (
          <BoxView paddingLeft={2}>
            <TextView fg={levelFg(entry.level)}>
              [{entry.level}] {entry.message}
            </TextView>
          </BoxView>
        )}
      </For>
    </BoxView>
  );
}

function levelFg(level: ToastEntry["level"]): Token {
  if (level === "error") return "danger";
  if (level === "warn") return "warning";
  return "fgMuted";
}
