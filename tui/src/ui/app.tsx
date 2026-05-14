// L9: AppShell — composes Effect Layers + Solid context providers and
// mounts the active route.
//
// The shell expects to be invoked from L10 (main.tsx) after the
// runtime has been constructed and the SSE pump has been started.
// AppShell is pure render: it reads accessors from the stores already
// provided via Solid context, calls `dispatch` for any action.

import { type JSX, Switch, Match, createSignal, onMount } from "solid-js";
import { ThemeProvider } from "./theme-context.js";
import { AgentSessionRoute } from "./routes/agent-session.js";
import { HomeRoute } from "./routes/home.js";
import { FPFInspectorRoute } from "./routes/fpf-inspector.js";
import type { Session } from "../core/domain/session.js";
import type { Action, RouteName } from "../effect/state/actions.js";
import type { ToastEntry } from "../effect/state/toast-store.js";
import type { AuthStatus } from "../core/wire/auth-status.js";
import type { ThemeName } from "../effect/state/theme-store.js";

export interface AppShellProps {
  // accessors threaded from the Effect runtime + Solid stores in L10
  readonly activeRoute: () => RouteName;
  readonly themeName: () => ThemeName;
  readonly session: () => Session | undefined;
  readonly toasts: () => ReadonlyArray<ToastEntry>;
  readonly hints: () => ReadonlyArray<{ readonly key: string; readonly action: string }>;
  readonly inspectedArtifactId: () => string | undefined;
  readonly dispatch: (action: Action) => void;
  // auth surface — fetched at boot via AgentClient.authStatus
  readonly fetchAuth: () => Promise<AuthStatus>;
}

export function AppShell(props: AppShellProps): JSX.Element {
  const [auth, setAuth] = createSignal<AuthStatus | undefined>(undefined);

  onMount(() => {
    props
      .fetchAuth()
      .then((a) => setAuth(() => a))
      .catch(() => {
        // Auth fetch failures surface via toast (dispatched by handler
        // chain); the home screen renders an "unknown" state.
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
