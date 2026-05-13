import { render } from "@opentui/solid";
import { AgentClient, type AgentEvent } from "./sdk/agent/index.js";
import { apply } from "./store.js";
import { App } from "./app.js";

function readPort(): number {
  const raw = process.env["HAFT_AGENT_PORT"];
  if (raw === undefined || raw === "") {
    process.stderr.write("haft-tui: HAFT_AGENT_PORT not set\n");
    process.exit(1);
  }
  const port = Number.parseInt(raw, 10);
  if (Number.isNaN(port) || port <= 0 || port > 65535) {
    process.stderr.write(`haft-tui: invalid HAFT_AGENT_PORT=${raw}\n`);
    process.exit(1);
  }
  return port;
}

const port = readPort();
const baseUrl = `http://127.0.0.1:${port}`;
const teardown = new AbortController();

const client = new AgentClient({
  baseUrl,
  signal: teardown.signal,
});

// Fire-and-forget the SSE subscription. The transport handles
// reconnect-on-disconnect itself; teardown.abort() ends the loop.
client
  .subscribe((result) => {
    if (result.ok) {
      apply(result.event as AgentEvent);
    }
    // unknown_kind / malformed events are dropped silently in v8.0;
    // a UI banner can land in v8.1+ if operator appetite emerges.
  })
  .catch((err: unknown) => {
    process.stderr.write(`haft-tui: SSE subscription error: ${String(err)}\n`);
  });

process.on("SIGINT", () => {
  teardown.abort();
  process.exit(0);
});
process.on("SIGTERM", () => {
  teardown.abort();
  process.exit(0);
});

render(() => <App />);
