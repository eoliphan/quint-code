// L7: InputArea — operator types here; Enter dispatches SubmitTurn.
//
// Backed by OpenTUI's <input> renderable inside a bordered box. The
// border colour tracks the caret theme token so an active prompt is
// visually distinct from the rest of the chat feed. When `disabled`
// is true (a turn is in flight) the input is replaced by a muted
// "input disabled" line so keystrokes don't go to a stale buffer.

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
    >
      <Show
        when={!props.disabled}
        fallback={
          <BoxView flexDirection="row">
            <TextView fg="warning">⏳ turn in flight — input disabled</TextView>
          </BoxView>
        }
      >
        <BoxView flexDirection="row">
          <TextView fg="caret">› </TextView>
          <input
            focused
            placeholder={props.placeholder ?? "type a message and press enter"}
            onInput={((v: string) => setValue(v)) as never}
            onSubmit={(handleSubmit) as never}
          />
        </BoxView>
      </Show>
    </box>
  );
}
