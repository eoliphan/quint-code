// L7: NoteView — FPF Note renderer.

import { type JSX, Show } from "solid-js";
import type { PartNote } from "../../../core/domain/part.js";
import { TextView } from "../../primitives/text-view.js";
import { BoxView } from "../../primitives/box-view.js";

export interface NoteViewProps {
  readonly part: PartNote;
  readonly onInspect?: () => void;
}

export function NoteView(props: NoteViewProps): JSX.Element {
  const p = props.part;
  return (
    <BoxView paddingLeft={2} paddingTop={1}>
      <TextView fg="fgMuted">▷ Note</TextView>
      <TextView fg="fg">{p.title}</TextView>
      <TextView fg="fgMuted">rationale: {p.rationale}</TextView>
      <Show when={props.onInspect}>
        <TextView fg="toolName" onMouseUp={() => props.onInspect?.()}>
          [enter] inspect note
        </TextView>
      </Show>
    </BoxView>
  );
}
