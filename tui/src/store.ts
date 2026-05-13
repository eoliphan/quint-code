import { createStore } from "solid-js/store";
import { type AgentEvent } from "./sdk/agent/types.js";
import { type TurnViewProps, type TurnPart } from "./routes/agent/session/turn.js";
import { type PermissionPromptProps } from "./routes/agent/session/permission-prompt.js";
import { type SubAgentRowProps } from "./routes/agent/session/subagent.js";

export type SessionStore = {
  title: string;
  modelLabel: string;
  turns: TurnViewProps[];
  subAgents: SubAgentRowProps[];
  permission?: PermissionPromptProps;
};

const initial: SessionStore = {
  title: "",
  modelLabel: "",
  turns: [],
  subAgents: [],
};

export const [sessionView, setSessionView] = createStore<SessionStore>(initial);

/** apply mutates the store by absorbing one decoded AgentEvent. The
 *  return value indicates whether the event materially changed visible
 *  state — callers can use it to debounce re-renders if needed. The
 *  store itself is reactive; SolidJS handles the rendering. */
export function apply(ev: AgentEvent): boolean {
  switch (ev.kind) {
    case "session.created":
      setSessionView({
        title: ev.title,
        modelLabel: `${ev.model.provider}/${ev.model.model}`,
        turns: [],
        subAgents: [],
        permission: undefined,
      });
      return true;
    case "session.updated":
      if (ev.title !== undefined) setSessionView("title", ev.title);
      return true;
    case "model.switched":
      setSessionView("modelLabel", `${ev.model.provider}/${ev.model.model}`);
      return true;
    case "turn.started":
      setSessionView("turns", (prev) => [
        ...prev,
        {
          turnID: ev.turn_id,
          role: ev.role,
          state: "running",
          parts: [],
        },
      ]);
      return true;
    case "turn.completed":
      setSessionView("turns", (t) => t.turnID === ev.turn_id, {
        state: "completed",
        verdict: ev.verdict,
      });
      return true;
    case "turn.failed":
      setSessionView("turns", (t) => t.turnID === ev.turn_id, {
        state: "failed",
        verdict: ev.verdict,
      });
      return true;
    case "part.text.completed":
      appendPart(ev.turn_id, {
        kind: "text",
        partID: ev.part_id,
        text: ev.text,
      });
      return true;
    case "part.reasoning.completed":
      appendPart(ev.turn_id, {
        kind: "reasoning",
        partID: ev.part_id,
        text: ev.text,
        expanded: false,
      });
      return true;
    case "part.tool_use.started":
      appendPart(ev.turn_id, {
        kind: "tool_use",
        partID: ev.part_id,
        toolName: ev.tool_name,
        toolCallID: ev.tool_call_id,
        args: ev.args,
      });
      return true;
    case "part.tool_use.completed":
      // Match on tool_call_id rather than part_id — the started event
      // carried a different part_id from the completed one.
      setSessionView("turns", (t) => t.turnID === ev.turn_id, "parts", (p) => {
        return p.kind === "tool_use" && p.toolCallID === ev.tool_call_id;
      }, {
        result: { content: ev.content, isError: ev.is_error },
      });
      return true;
    case "subagent.spawned":
      setSessionView("subAgents", (prev) => [
        ...prev,
        {
          subagentID: ev.subagent_id,
          childSession: ev.child_session,
          prompt: ev.prompt,
        },
      ]);
      return true;
    case "subagent.completed":
      setSessionView("subAgents", (sa) => sa.subagentID === ev.subagent_id, {
        verdict: ev.verdict,
      });
      return true;
    case "permission.requested":
      setSessionView("permission", {
        permissionID: ev.permission_id,
        toolName: ev.tool_name,
        toolCallID: ev.tool_call_id,
        args: ev.args,
        onApprove: () => {},
        onDeny: () => {},
      });
      return true;
    case "permission.resolved":
      setSessionView("permission", undefined);
      return true;
    default:
      return false;
  }
}

function appendPart(turnID: string, part: TurnPart): void {
  setSessionView("turns", (t) => t.turnID === turnID, "parts", (prev) => [...prev, part]);
}
