import { Show, type JSX } from "solid-js";
import { activeRoute, registerRoute } from "./router.js";
import { SessionView } from "./routes/agent/session/index.js";
import { sessionView } from "./store.js";

registerRoute({
  name: "agent",
  render: () => (
    <SessionView
      title={sessionView.title}
      modelLabel={sessionView.modelLabel}
      turns={sessionView.turns}
      subAgents={sessionView.subAgents}
      permission={sessionView.permission}
    />
  ),
});

export function App(): JSX.Element {
  return (
    <Show when={activeRoute()} fallback={<text fg="#666666">loading…</text>}>
      {(route) => route().render()}
    </Show>
  );
}
