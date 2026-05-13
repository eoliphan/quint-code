import { type JSX } from "solid-js";

export type PermissionPromptProps = {
  permissionID: string;
  toolName: string;
  toolCallID: string;
  args: unknown;
  onApprove: () => void;
  onDeny: (reason: string) => void;
};

export function PermissionPrompt(props: PermissionPromptProps): JSX.Element {
  const argsString = (): string => {
    try {
      return JSON.stringify(props.args, null, 2);
    } catch {
      return String(props.args);
    }
  };
  return (
    <box flexDirection="column" backgroundColor="#1f2937" paddingLeft={2} paddingRight={2} paddingTop={1} paddingBottom={1}>
      <text fg="#fbbf24">⚠ permission required</text>
      <text fg="#e6e6e6">
        tool <text fg="#7dd3fc">{props.toolName}</text> wants to run
      </text>
      <text fg="#a0a0a0">{argsString()}</text>
      <box flexDirection="row" paddingTop={1}>
        <text fg="#86efac" onMouseUp={() => props.onApprove()}>
          [enter] approve
        </text>
        <text fg="#fca5a5" onMouseUp={() => props.onDeny("operator denied")}>
          {"  [esc] deny"}
        </text>
      </box>
    </box>
  );
}
