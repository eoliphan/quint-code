// L7: EvidenceItemView — FPF Evidence renderer.

import { type JSX, Show } from "solid-js";
import type { PartEvidenceItem } from "../../../core/domain/part.js";
import { TextView } from "../../primitives/text-view.js";
import { BoxView } from "../../primitives/box-view.js";

export interface EvidenceItemViewProps {
  readonly part: PartEvidenceItem;
  readonly onInspect?: () => void;
}

export function EvidenceItemView(props: EvidenceItemViewProps): JSX.Element {
  const p = props.part;
  const verdictFg = (): "success" | "warning" | "danger" => {
    if (p.verdict === "supports") return "success";
    if (p.verdict === "weakens") return "warning";
    return "danger";
  };
  return (
    <BoxView paddingLeft={2} paddingTop={1}>
      <TextView fg={verdictFg()}>
        {p.verdict === "supports" ? "✓" : p.verdict === "weakens" ? "?" : "✗"} Evidence [{p.type}] CL{p.congruenceLevel}
      </TextView>
      <TextView fg="fg">{p.content}</TextView>
      <Show when={props.onInspect}>
        <TextView fg="toolName" onMouseUp={() => props.onInspect?.()}>
          [enter] inspect evidence
        </TextView>
      </Show>
    </BoxView>
  );
}
