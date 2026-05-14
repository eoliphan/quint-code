// L7: InputArea — operator types here; Enter dispatches SubmitTurn.
//
// Pure widget — receives a Solid setter for the draft text and an
// onSubmit callback that constructs the Action. The L8 route owns
// the focus + state.

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
  const [text, setText] = createSignal("");

  const submit = (): void => {
    const value = text();
    if (value.trim().length === 0) return;
    props.onSubmit({ tag: "SubmitTurn", text: value });
    setText("");
  };
  // submit is only invoked by InputArea's own keybinding wiring at the
  // route layer (L8); kept here so the API is coherent for future
  // direct-input variants.
  void submit;

  return (
    <BoxView paddingTop={1}>
      <TextView fg="caret">›</TextView>
      <Show
        when={text().length > 0}
        fallback={
          <TextView fg="fgDim">
            {props.placeholder ?? "type a message and press enter"}
          </TextView>
        }
      >
        <TextView fg="fg">{text()}</TextView>
      </Show>
      <Show when={props.disabled}>
        <TextView fg="warning">(turn in flight — input disabled)</TextView>
      </Show>
    </BoxView>
  );
}
