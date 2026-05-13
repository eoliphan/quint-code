import { Show, type JSX } from "solid-js";
import { type Verdict } from "../../../sdk/agent/types.js";

export type SubAgentRowProps = {
  subagentID: string;
  childSession: string;
  prompt: string;
  /** Undefined while the child is still running. */
  verdict?: Verdict;
};

export function SubAgentRow(props: SubAgentRowProps): JSX.Element {
  return (
    <box flexDirection="row" paddingLeft={4} paddingTop={1}>
      <text fg="#c084fc">⤷ subagent</text>
      <text fg="#a0a0a0">
        {" "}
        {props.prompt}
      </text>
      <Show when={props.verdict}>
        {(verdict) => (
          <text fg={verdictColor(verdict())}>
            {" "}
            [{verdict()}]
          </text>
        )}
      </Show>
    </box>
  );
}

function verdictColor(v: Verdict): string {
  if (v === "pass") return "#86efac";
  if (v === "canceled") return "#fbbf24";
  return "#fca5a5";
}
