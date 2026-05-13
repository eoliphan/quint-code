import { Show, type JSX } from "solid-js";

export type PartToolUseProps = {
  toolName: string;
  toolCallID: string;
  args: unknown;
  /** Set when the tool has completed; undefined while in-flight. */
  result?: {
    content: string;
    isError: boolean;
  };
};

export function PartToolUse(props: PartToolUseProps): JSX.Element {
  const argsString = (): string => {
    try {
      return JSON.stringify(props.args, null, 2);
    } catch {
      return String(props.args);
    }
  };
  return (
    <box flexDirection="column" paddingLeft={2} paddingTop={1}>
      <text fg="#7dd3fc">
        ▸ {props.toolName} <text fg="#666666">[{props.toolCallID}]</text>
      </text>
      <text fg="#a0a0a0">{argsString()}</text>
      <Show when={props.result}>
        {(result) => (
          <text fg={result().isError ? "#fca5a5" : "#86efac"}>
            {result().isError ? "✗ " : "✓ "}
            {result().content}
          </text>
        )}
      </Show>
    </box>
  );
}
