// L8: HomeRoute — startup screen with auth state + new session prompt.
//
// Keystrokes: Enter on this route dispatches CreateSession bound to
// the auth response's provider/model so the resulting session is
// driveable. Routing into agent.session is the dispatcher's job (see
// dispatcher.ts CreateSession case).

import { type JSX, Show } from "solid-js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { KeyHintBar } from "../primitives/key-hint-bar.js";
import { Divider } from "../primitives/divider.js";
import { ToastStack } from "../widgets/toast-stack.js";
import type { Action } from "../../effect/state/actions.js";
import type { ToastEntry } from "../../effect/state/toast-store.js";
import type { AuthStatus } from "../../core/wire/auth-status.js";
import type { ModelChoice } from "../../core/domain/model-choice.js";
import { HAFT_LOGO_LINES } from "../../components/logo.js";

export interface HomeRouteProps {
  readonly auth: () => AuthStatus | undefined;
  readonly toasts: () => ReadonlyArray<ToastEntry>;
  readonly hints: () => ReadonlyArray<{ readonly key: string; readonly action: string }>;
  readonly dispatch: (action: Action) => void;
}

// modelFromAuth: AuthStatus -> ModelChoice. Returns undefined when
// credentials are missing — caller must keep the Enter handler disabled
// in that case (CreateSession to an unauthenticated backend would 500
// instead of producing a useful toast). Only the openai/codex provider
// is driveable by v8.0 alpha (see internal/cli/v8_agent.go
// buildDispatcher) — non-openai responses degrade to a warning toast
// rather than a session creation attempt that the Go side would refuse.
function modelFromAuth(a: AuthStatus): ModelChoice | undefined {
  if (!a.has_credentials || a.model === "") return undefined;
  // AuthStatus.provider is a free-form string from the Go side. v8.0
  // only adapts the openai surface to agentdriver.Provider; other
  // providers are surfaced verbatim so a misconfigured machine still
  // explains itself via the toast.
  return { provider: a.provider, model: a.model };
}

export function HomeRoute(props: HomeRouteProps): JSX.Element {
  const handleEnter = (): void => {
    const a = props.auth();
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
  };

  return (
    <BoxView paddingLeft={2} paddingTop={1}>
      {HAFT_LOGO_LINES.map((line) => (
        <TextView fg="toolName">{line}</TextView>
      ))}
      <Divider />
      <Show
        when={props.auth()}
        fallback={
          <TextView fg="fgMuted">checking auth…</TextView>
        }
      >
        {(auth) => (
          <BoxView>
            <Show
              when={auth().has_credentials}
              fallback={
                <BoxView>
                  <TextView fg="warning">⚠ no credentials configured</TextView>
                  <TextView fg="fgMuted">
                    run `haft login` from CLI to authenticate via ChatGPT-Sub
                    (device flow).
                  </TextView>
                </BoxView>
              }
            >
              <TextView fg="success">
                ✓ signed in: {auth().provider} / {auth().model}
              </TextView>
              <Show when={auth().expires_at !== undefined}>
                <TextView fg="fgDim">expires: {auth().expires_at}</TextView>
              </Show>
            </Show>
          </BoxView>
        )}
      </Show>
      <Divider />
      <TextView fg="fg">press [enter] to start a new session</TextView>
      <TextView fg="toolName" onMouseUp={handleEnter}>
        ➤ new session
      </TextView>
      <ToastStack entries={props.toasts} />
      <KeyHintBar hints={props.hints} />
    </BoxView>
  );
}

// onEnterFor: route-level Enter handler factory. Exported so the
// AppShell's global keystroke handler can route Enter on the home
// route through here without re-declaring the logic.
export function homeEnterHandler(props: HomeRouteProps): () => void {
  return () => {
    const a = props.auth();
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
  };
}
