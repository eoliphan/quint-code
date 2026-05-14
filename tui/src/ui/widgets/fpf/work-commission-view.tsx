// L7: WorkCommissionView — FPF WorkCommission renderer.

import { type JSX, Show, For } from "solid-js";
import type { PartWorkCommission } from "../../../core/domain/part.js";
import { TextView } from "../../primitives/text-view.js";
import { BoxView } from "../../primitives/box-view.js";
import { Badge } from "../../primitives/badge.js";
import type { Theme } from "../../../themes/index.js";

type ColorToken = keyof Theme;

export interface WorkCommissionViewProps {
  readonly part: PartWorkCommission;
  readonly onInspect?: () => void;
}

export function WorkCommissionView(props: WorkCommissionViewProps): JSX.Element {
  const p = props.part;
  const statusFg = (): ColorToken => {
    if (p.status === "completed") return "success";
    if (p.status === "failed" || p.status === "blocked" || p.status === "expired") return "danger";
    if (p.status === "cancelled") return "warning";
    return "toolName";
  };
  return (
    <BoxView paddingLeft={2} paddingTop={1}>
      <TextView fg="subAgent">
        ⚙ WorkCommission <Badge label={p.status} fg={statusFg()} />
      </TextView>
      <TextView fg="fgMuted">decision: {p.decisionRef}</TextView>
      <Show when={p.allowedPaths.length > 0}>
        <TextView fg="fgDim">scope ({p.allowedPaths.length} paths):</TextView>
        <For each={p.allowedPaths.slice(0, 3)}>
          {(path) => <TextView fg="fgDim">  · {path}</TextView>}
        </For>
      </Show>
      <Show when={props.onInspect}>
        <TextView fg="toolName" onMouseUp={() => props.onInspect?.()}>
          [enter] inspect commission
        </TextView>
      </Show>
    </BoxView>
  );
}
