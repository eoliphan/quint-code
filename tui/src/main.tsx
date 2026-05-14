// L10: Process entry — THE ONLY side-effect layer.
//
// Reads HAFT_AGENT_PORT, constructs the Effect runtime with the full
// Layer composition (TransportConfig → Transport → AgentClient + all
// stores → Dispatcher), starts the SSE pump feeding SessionStore,
// queries auth status, registers signal handlers, mounts the AppShell
// under OpenTUI's renderer.

import { render } from "@opentui/solid";
import { Effect, Layer, Stream, Runtime, Exit } from "effect";
import * as Config from "./effect/transport/config.js";
import {
  TransportLive,
  TransportService,
  transportLayerWithFetch,
} from "./effect/transport/service.js";
import {
  AgentClientLive,
  AgentClientService,
} from "./effect/transport/agent-client.js";
import {
  SessionStoreLive,
  SessionStoreService,
  pumpEventsIntoStore,
} from "./effect/state/session-store.js";
import { ThemeStoreLive, ThemeStoreService } from "./effect/state/theme-store.js";
import { RouteStoreLive, RouteStoreService } from "./effect/state/route-store.js";
import { DialogStoreLive, DialogStoreService } from "./effect/state/dialog-store.js";
import { ToastStoreLive, ToastStoreService } from "./effect/state/toast-store.js";
import { KeymapStoreLive, KeymapStoreService } from "./effect/state/keymap-store.js";
import { DispatcherLive, DispatcherService } from "./effect/state/dispatcher.js";
import { AppShell } from "./ui/app.js";
import type { Action } from "./effect/state/actions.js";
import type { AuthStatus } from "./core/wire/auth-status.js";

void transportLayerWithFetch;

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

// Layer composition (bottom-up):
//   ConfigLive          -> TransportConfigService
//   TransportLive       <- TransportConfigService, provides TransportService
//   AgentClientLive     <- TransportService,       provides AgentClientService
//   StoresLive          -> Session/Theme/Route/Dialog/Toast/Keymap
//   DispatcherLive      <- AgentClientService + StoresLive
//
// AppLive merges everything so the program can `yield*` any Service.
const configLive = Config.layer(Config.makeConfig(baseUrl));
const transportLive = Layer.provide(TransportLive, configLive);
const clientLive = Layer.provide(AgentClientLive, transportLive);
const storesLive = Layer.mergeAll(
  SessionStoreLive,
  ThemeStoreLive,
  RouteStoreLive,
  DialogStoreLive,
  ToastStoreLive,
  KeymapStoreLive,
);
const dispatcherDeps = Layer.merge(clientLive, storesLive);
const dispatcherLive = Layer.provide(DispatcherLive, dispatcherDeps);
const AppLive = Layer.mergeAll(transportLive, clientLive, storesLive, dispatcherLive);

const program = Effect.gen(function* () {
  const transport = yield* TransportService;
  const client = yield* AgentClientService;
  const session = yield* SessionStoreService;
  const theme = yield* ThemeStoreService;
  const route = yield* RouteStoreService;
  const toast = yield* ToastStoreService;
  const keymap = yield* KeymapStoreService;
  const dispatcher = yield* DispatcherService;
  void DialogStoreService;

  // Bootstrap keymap with the route's default bindings.
  keymap.pushLayer({
    id: "agent.session",
    bindings: [
      { key: "Tab", action: "switch", handler: () => undefined },
      { key: "Enter", action: "submit", handler: () => undefined },
      { key: "Ctrl+C", action: "cancel", handler: () => undefined },
      { key: "?", action: "help", handler: () => undefined },
      { key: "t", action: "theme", handler: (): Action => ({ tag: "CycleTheme" }) },
    ],
  });

  // Start the SSE pump in a forked fiber so it runs concurrently with
  // the UI. The pump pushes every wire event through the reducer into
  // the SessionStore.
  yield* Effect.forkDaemon(
    pumpEventsIntoStore(session, transport.subscribeEvents() as Stream.Stream<import("./core/wire/events.js").AgentEventWire, unknown>),
  );

  // Captured AgentClient + Dispatcher are already provided in the
  // outer runtime — fetchAuth and dispatch just call them directly.
  const fetchAuth = (): Promise<AuthStatus> =>
    Effect.runPromise(client.authStatus() as Effect.Effect<AuthStatus, unknown, never>);

  const dispatch = (action: Action): void => {
    void Effect.runPromise(dispatcher.dispatch(action));
  };

  // Mount the AppShell.
  render(() => (
    <AppShell
      activeRoute={route.active}
      themeName={theme.active}
      session={session.current}
      toasts={toast.entries}
      hints={keymap.visibleHints}
      inspectedArtifactId={route.inspectedArtifactId}
      dispatch={dispatch}
      fetchAuth={fetchAuth}
    />
  ));
});

// Run the boot program with the full layer composition; runPromise
// finalises the runtime fibers on completion.
Effect.runPromise(program.pipe(Effect.provide(AppLive))).catch((e: unknown) => {
  process.stderr.write(`haft-tui: boot failure: ${String(e)}\n`);
  process.exit(1);
});

void Runtime;

process.on("SIGINT", () => {
  process.exit(0);
});
process.on("SIGTERM", () => {
  process.exit(0);
});

void Exit;
