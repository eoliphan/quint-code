// L7: InputArea — operator types here; Enter dispatches SubmitTurn.
//
// Uses OpenTUI's <input> renderable. The input element flex-grows
// inside the bordered row so long Cyrillic / multi-language text
// expands to the full bordered width rather than colliding with the
// caret column. The OpenTUI input is single-line and scrolls
// horizontally; the visible cursor follows the typing position so
// the operator always sees what they're adding.

import { type JSX, createSignal, Show } from "solid-js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { useTheme } from "../theme-context.js";
import type { Action } from "../../effect/state/actions.js";

export interface InputAreaProps {
  readonly placeholder?: string;
  readonly disabled?: boolean;
  readonly onSubmit: (action: Action) => void;
}

export function InputArea(props: InputAreaProps): JSX.Element {
  const [_value, setValue] = createSignal("");
  const theme = useTheme();

  const handleSubmit = (text: string): void => {
    const trimmed = text.trim();
    if (trimmed.length === 0) return;
    props.onSubmit({ tag: "SubmitTurn", text: trimmed });
    setValue("");
  };

  const borderColor = (): string => {
    const t = theme();
    return props.disabled
      ? typeof t.fgDim === "string"
        ? t.fgDim
        : t.fg
      : typeof t.caret === "string"
        ? t.caret
        : t.fg;
  };

  return (
    <box
      border={["top", "bottom", "left", "right"]}
      borderColor={borderColor()}
      paddingLeft={1}
      paddingRight={1}
      marginTop={1}
      flexShrink={0}
    >
      <Show
        when={!props.disabled}
        fallback={
          <BoxView flexDirection="row">
            <TextView fg="warning">⏳ turn in flight — input disabled</TextView>
          </BoxView>
        }
      >
        <box flexDirection="row" flexGrow={1}>
          <text fg={borderColor()}>› </text>
          <input
            focused
            flexGrow={1}
            placeholder={props.placeholder ?? "type a message and press enter"}
            onInput={((v: string) => setValue(v)) as never}
            onSubmit={(handleSubmit) as never}
          />
        </box>
      </Show>
    </box>
  );
}
