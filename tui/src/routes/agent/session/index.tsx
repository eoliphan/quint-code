import { For, Show, type JSX } from "solid-js";
import { renderHintRow } from "../../../components/key-hint.js";
import { type TurnViewProps, TurnView } from "./turn.js";
import {
  PermissionPrompt,
  type PermissionPromptProps,
} from "./permission-prompt.js";
import { SubAgentRow, type SubAgentRowProps } from "./subagent.js";

export type SessionViewProps = {
  title: string;
  modelLabel: string;
  turns: readonly TurnViewProps[];
  subAgents: readonly SubAgentRowProps[];
  /** Set when an unresolved permission is active. */
  permission?: PermissionPromptProps;
};

const FOOTER_HINTS = [
  { key: "tab", action: "switch" },
  { key: "enter", action: "submit" },
  { key: "ctrl+c", action: "cancel" },
  { key: "?", action: "help" },
] as const;

export function SessionView(props: SessionViewProps): JSX.Element {
  return (
    <box flexDirection="column">
      <box flexDirection="row" paddingBottom={1}>
        <text fg="#e6e6e6">{props.title}</text>
        <text fg="#666666"> · </text>
        <text fg="#7dd3fc">{props.modelLabel}</text>
      </box>

      <For each={props.turns}>{(t) => <TurnView {...t} />}</For>
      <For each={props.subAgents}>{(sa) => <SubAgentRow {...sa} />}</For>

      <Show when={props.permission}>
        {(perm) => <PermissionPrompt {...perm()} />}
      </Show>

      <box paddingTop={1}>
        <text fg="#666666">{renderHintRow(FOOTER_HINTS)}</text>
      </box>
    </box>
  );
}
