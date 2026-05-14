// L7: DecisionRecordView — first-class FPF artifact renderer.
//
// Compact inline render when a decision_record Part appears in a turn.
// Shows the four FPF E.9 components (Problem Frame / Decision /
// Rationale / Consequences) condensed; full detail surfaces in the
// FPF Inspector route on operator click.

import { type JSX, For, Show } from "solid-js";
import type { PartDecisionRecord } from "../../../core/domain/part.js";
import { TextView } from "../../primitives/text-view.js";
import { BoxView } from "../../primitives/box-view.js";
import { Divider } from "../../primitives/divider.js";

export interface DecisionRecordViewProps {
  readonly part: PartDecisionRecord;
  readonly onInspect?: () => void;
}

export function DecisionRecordView(props: DecisionRecordViewProps): JSX.Element {
  const p = props.part;
  return (
    <BoxView paddingLeft={2} paddingTop={1} paddingBottom={1}>
      <TextView fg="warning">◆ Decision</TextView>
      <TextView fg="fg">{p.title}</TextView>
      <Divider />
      <TextView fg="fgMuted">selected: <TextView fg="success">{p.selected}</TextView></TextView>
      <TextView fg="fgMuted">policy: {p.selectionPolicy}</TextView>
      <Show when={p.weakestLink}>
        <TextView fg="warning">WLNK: {p.weakestLink}</TextView>
      </Show>
      <Show when={p.counterargument}>
        <TextView fg="fgMuted">counter: {truncate(p.counterargument, 120)}</TextView>
      </Show>
      <Show when={p.predictions.length > 0}>
        <TextView fg="fgDim">predictions ({p.predictions.length}):</TextView>
        <For each={p.predictions.slice(0, 3)}>
          {(pred) => (
            <TextView fg="fgDim">  · {pred.claim} → {pred.threshold}</TextView>
          )}
        </For>
      </Show>
      <Show when={props.onInspect}>
        <TextView fg="toolName" onMouseUp={() => props.onInspect?.()}>
          [enter] inspect full record
        </TextView>
      </Show>
    </BoxView>
  );
}

function truncate(s: string, n: number): string {
  return s.length > n ? `${s.slice(0, n - 1)}…` : s;
}
