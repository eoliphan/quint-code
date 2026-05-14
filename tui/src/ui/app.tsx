// L9: AppShell — composes Effect Layers + Solid context providers and
// mounts the active route.
//
// The shell expects to be invoked from L10 (main.tsx) after the
// runtime has been constructed and the SSE pump has been started.
// AppShell is pure render: it reads accessors from the stores already
// provided via Solid context, calls `dispatch` for any action.
//
// Global keystroke handling: when the active route is "home" the
// OpenTUI <input> element is not mounted, so Enter cannot reach a
// focused widget. AppShell subscribes to the CliRenderer's keyInput
// emitter and routes Enter through the Home action factory. On the
// agent.session route the focused <input> consumes Enter directly via
// onSubmit; the global handler short-circuits to avoid double-firing.

import {
  type JSX,
  Switch,
  Match,
  createSignal,
  onCleanup,
  onMount,
} from "solid-js";
import { useRenderer } from "@opentui/solid";
import { ThemeProvider } from "./theme-context.js";
import { AgentSessionRoute } from "./routes/agent-session.js";
import { HomeRoute } from "./routes/home.js";
import { FPFInspectorRoute } from "./routes/fpf-inspector.js";
import type { Session } from "../core/domain/session.js";
import type { Action, RouteName } from "../effect/state/actions.js";
import type { ToastEntry } from "../effect/state/toast-store.js";
import type { AuthStatus } from "../core/wire/auth-status.js";
import type { ThemeName } from "../effect/state/theme-store.js";
import type { ModelChoice } from "../core/domain/model-choice.js";

export interface AppShellProps {
  readonly activeRoute: () => RouteName;
  readonly themeName: () => ThemeName;
  readonly session: () => Session | undefined;
  readonly toasts: () => ReadonlyArray<ToastEntry>;
  readonly hints: () => ReadonlyArray<{ readonly key: string; readonly action: string }>;
  readonly inspectedArtifactId: () => string | undefined;
  readonly dispatch: (action: Action) => void;
  readonly fetchAuth: () => Promise<AuthStatus>;
}

function modelFromAuth(a: AuthStatus): ModelChoice | undefined {
  if (!a.has_credentials || a.model === "") return undefined;
  return { provider: a.provider, model: a.model };
}

export function AppShell(props: AppShellProps): JSX.Element {
  const [auth, setAuth] = createSignal<AuthStatus | undefined>(undefined);

  onMount(() => {
    props
      .fetchAuth()
      .then((a) => setAuth(() => a))
      .catch(() => {
        // Auth fetch failures surface via toast; the home screen
        // renders the "checking auth…" placeholder until either
        // success or a manual refresh.
      });
  });

  onMount(() => {
    const renderer = useRenderer();
    const onKey = (ev: { name: string; ctrl: boolean }): void => {
      // Only the home route needs a global Enter — agent.session has a
      // focused <input> that owns its own keystroke loop.
      if (props.activeRoute() !== "home") return;
      if (ev.name === "return" || ev.name === "enter") {
        const a = auth();
        if (a === undefined) {
          props.dispatch({ tag: "ShowToast", message: "auth not loaded yet", level: "warn" });
          return;
        }
        const model = modelFromAuth(a);
        if (model === undefined) {
          props.dispatch({
            tag: "ShowToast",
            message: "no credentials — run `haft login`",
            level: "warn",
          });
          return;
        }
        props.dispatch({ tag: "CreateSession", title: "new session", model });
        return;
      }
      if (ev.ctrl && ev.name === "c") {
        // Ctrl+C on home cleanly exits the process — without an
        // explicit handler the focused <input> would swallow it on
        // other routes; on home there is no input, so we wire the
        // exit directly.
        process.exit(0);
      }
    };
    renderer.keyInput.on("keypress", onKey);
    onCleanup(() => {
      renderer.keyInput.off("keypress", onKey);
    });
  });

  return (
    <ThemeProvider active={props.themeName}>
      <Switch>
        <Match when={props.activeRoute() === "home"}>
          <HomeRoute
            auth={auth}
            toasts={props.toasts}
            hints={props.hints}
            dispatch={props.dispatch}
          />
        </Match>
        <Match when={props.activeRoute() === "agent.session"}>
          <AgentSessionRoute
            session={props.session}
            toasts={props.toasts}
            hints={props.hints}
            dispatch={props.dispatch}
          />
        </Match>
        <Match when={props.activeRoute() === "fpf.inspector"}>
          <FPFInspectorRoute
            session={props.session}
            artifactId={props.inspectedArtifactId}
            toasts={props.toasts}
            hints={props.hints}
            dispatch={props.dispatch}
          />
        </Match>
      </Switch>
    </ThemeProvider>
  );
}
