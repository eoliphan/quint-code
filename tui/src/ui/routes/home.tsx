// L8: HomeRoute — startup screen with session list + new session prompt.

import { type JSX, Show } from "solid-js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { KeyHintBar } from "../primitives/key-hint-bar.js";
import { Divider } from "../primitives/divider.js";
import { ToastStack } from "../widgets/toast-stack.js";
import type { Action } from "../../effect/state/actions.js";
import type { ToastEntry } from "../../effect/state/toast-store.js";
import type { AuthStatus } from "../../core/wire/auth-status.js";
import { HAFT_LOGO_LINES } from "../../components/logo.js";

export interface HomeRouteProps {
  readonly auth: () => AuthStatus | undefined;
  readonly toasts: () => ReadonlyArray<ToastEntry>;
  readonly hints: () => ReadonlyArray<{ readonly key: string; readonly action: string }>;
  readonly dispatch: (action: Action) => void;
}

export function HomeRoute(props: HomeRouteProps): JSX.Element {
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
      <TextView
        fg="toolName"
        onMouseUp={() =>
          props.dispatch({
            tag: "CreateSession",
            title: "new session",
            model: { provider: "anthropic", model: "claude-opus-4-7" },
          })
        }
      >
        ➤ new session
      </TextView>
      <ToastStack entries={props.toasts} />
      <KeyHintBar hints={props.hints} />
    </BoxView>
  );
}
