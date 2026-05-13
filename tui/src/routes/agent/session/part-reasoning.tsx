import { Show, type JSX } from "solid-js";

export type PartReasoningProps = {
  text: string;
  expanded?: boolean;
  onToggle?: () => void;
};

export function PartReasoning(props: PartReasoningProps): JSX.Element {
  return (
    <box flexDirection="column" paddingLeft={2}>
      <text fg="#888888" onMouseUp={() => props.onToggle?.()}>
        {props.expanded ? "▼ reasoning" : "▶ reasoning (hidden)"}
      </text>
      <Show when={props.expanded}>
        <text fg="#777777">{props.text}</text>
      </Show>
    </box>
  );
}
