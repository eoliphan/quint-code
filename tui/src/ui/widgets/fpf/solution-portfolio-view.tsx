// L7: SolutionPortfolioView — FPF Solution Portfolio renderer.

import { type JSX, For, Show } from "solid-js";
import type { PartSolutionPortfolio } from "../../../core/domain/part.js";
import { TextView } from "../../primitives/text-view.js";
import { BoxView } from "../../primitives/box-view.js";

export interface SolutionPortfolioViewProps {
  readonly part: PartSolutionPortfolio;
  readonly onInspect?: () => void;
}

export function SolutionPortfolioView(props: SolutionPortfolioViewProps): JSX.Element {
  const p = props.part;
  return (
    <BoxView paddingLeft={2} paddingTop={1}>
      <TextView fg="subAgent">◐ Solution Portfolio ({p.variants.length} variants)</TextView>
      <For each={p.variants}>
        {(v) => {
          const onPareto = (): boolean => p.paretoFront.includes(v.id);
          return (
            <BoxView paddingLeft={2}>
              <TextView fg={onPareto() ? "success" : "fgMuted"}>
                {onPareto() ? "★ " : "○ "}
                {v.id} — {v.title}
                {v.steppingStone ? " [stepping-stone]" : ""}
              </TextView>
              <TextView fg="fgDim">  WLNK: {v.weakestLink}</TextView>
              <TextView fg="fgDim">  novelty: {v.noveltyMarker}</TextView>
            </BoxView>
          );
        }}
      </For>
      <Show when={p.paretoFront.length > 0}>
        <TextView fg="fgMuted">Pareto front: {p.paretoFront.join(", ")}</TextView>
      </Show>
      <Show when={props.onInspect}>
        <TextView fg="toolName" onMouseUp={() => props.onInspect?.()}>
          [enter] inspect portfolio
        </TextView>
      </Show>
    </BoxView>
  );
}
