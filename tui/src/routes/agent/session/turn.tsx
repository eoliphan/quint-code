import { For, type JSX } from "solid-js";
import { type TurnState, type Verdict } from "../../../sdk/agent/types.js";
import { PartText } from "./part-text.js";
import { PartReasoning } from "./part-reasoning.js";
import { PartToolUse } from "./part-tool-use.js";

export type TurnPart =
  | { kind: "text"; partID: string; text: string; isStreaming?: boolean }
  | { kind: "reasoning"; partID: string; text: string; expanded?: boolean }
  | {
      kind: "tool_use";
      partID: string;
      toolName: string;
      toolCallID: string;
      args: unknown;
      result?: { content: string; isError: boolean };
    };

export type TurnViewProps = {
  turnID: string;
  role: "user" | "sub_agent" | "assistant";
  state: TurnState;
  verdict?: Verdict;
  parts: readonly TurnPart[];
};

export function TurnView(props: TurnViewProps): JSX.Element {
  return (
    <box flexDirection="column" paddingTop={1}>
      <text fg={roleColor(props.role)}>
        {roleMarker(props.role)} {props.state}
        {props.verdict ? ` · ${props.verdict}` : ""}
      </text>
      <For each={props.parts}>{(part) => renderPart(part)}</For>
    </box>
  );
}

function renderPart(part: TurnPart): JSX.Element {
  if (part.kind === "text") {
    return <PartText text={part.text} isStreaming={part.isStreaming} />;
  }
  if (part.kind === "reasoning") {
    return <PartReasoning text={part.text} expanded={part.expanded} />;
  }
  return (
    <PartToolUse
      toolName={part.toolName}
      toolCallID={part.toolCallID}
      args={part.args}
      result={part.result}
    />
  );
}

function roleMarker(role: TurnViewProps["role"]): string {
  if (role === "user") return "›";
  if (role === "sub_agent") return "⤷";
  return "·";
}

function roleColor(role: TurnViewProps["role"]): string {
  if (role === "user") return "#86efac";
  if (role === "sub_agent") return "#c084fc";
  return "#a0a0a0";
}
