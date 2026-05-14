// L7: ProblemCardView — FPF ProblemCard renderer.

import { type JSX, For, Show } from "solid-js";
import type { PartProblemCard } from "../../../core/domain/part.js";
import { TextView } from "../../primitives/text-view.js";
import { BoxView } from "../../primitives/box-view.js";

export interface ProblemCardViewProps {
  readonly part: PartProblemCard;
  readonly onInspect?: () => void;
}

export function ProblemCardView(props: ProblemCardViewProps): JSX.Element {
  const p = props.part;
  return (
    <BoxView paddingLeft={2} paddingTop={1}>
      <TextView fg="toolName">◇ Problem [{p.problemType}]</TextView>
      <TextView fg="fg">{p.title}</TextView>
      <TextView fg="fgMuted">signal: {p.signal}</TextView>
      <Show when={p.constraints.length > 0}>
        <TextView fg="fgDim">constraints:</TextView>
        <For each={p.constraints.slice(0, 3)}>
          {(c) => <TextView fg="fgDim">  · {c}</TextView>}
        </For>
      </Show>
      <TextView fg="fgMuted">acceptance: {p.acceptance}</TextView>
      <Show when={props.onInspect}>
        <TextView fg="toolName" onMouseUp={() => props.onInspect?.()}>
          [enter] inspect frame
        </TextView>
      </Show>
    </BoxView>
  );
}
