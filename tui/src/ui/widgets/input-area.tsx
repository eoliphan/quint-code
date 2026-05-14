// L7: InputArea — operator types here; Enter dispatches SubmitTurn.
//
// Backed by OpenTUI's <input> renderable. Enter triggers onSubmit
// which the route maps to a SubmitTurn Action. When `disabled` is
// true (a turn is in flight), focus is dropped so keystrokes are
// ignored — the surface is read-only until the turn ends.

import { type JSX, createSignal, Show } from "solid-js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import type { Action } from "../../effect/state/actions.js";

export interface InputAreaProps {
  readonly placeholder?: string;
  readonly disabled?: boolean;
  readonly onSubmit: (action: Action) => void;
}

export function InputArea(props: InputAreaProps): JSX.Element {
  // The OpenTUI <input> owns its own buffer; we surface a controlled
  // value only when the caller cares (for now nobody outside does).
  const [_value, setValue] = createSignal("");

  const handleSubmit = (text: string): void => {
    const trimmed = text.trim();
    if (trimmed.length === 0) return;
    props.onSubmit({ tag: "SubmitTurn", text: trimmed });
    setValue("");
  };

  return (
    <BoxView paddingTop={1} flexDirection="row">
      <TextView fg="caret">›</TextView>
      <Show
        when={!props.disabled}
        fallback={
          <BoxView paddingLeft={1}>
            <TextView fg="warning">turn in flight — input disabled</TextView>
          </BoxView>
        }
      >
        <input
          focused
          placeholder={props.placeholder ?? "type a message and press enter"}
          onInput={((v: string) => setValue(v)) as never}
          onSubmit={(handleSubmit) as never}
        />
      </Show>
    </BoxView>
  );
}
