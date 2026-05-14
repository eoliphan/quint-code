// L8: SessionRoute — agent chat view composed from L7 widgets.

import { type JSX, For, Show } from "solid-js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { KeyHintBar } from "../primitives/key-hint-bar.js";
import { Divider } from "../primitives/divider.js";
import { SpinnerView } from "../primitives/spinner-view.js";
import { TurnView } from "../widgets/turn-view.js";
import { PermissionPrompt } from "../widgets/permission-prompt.js";
import { InputArea } from "../widgets/input-area.js";
import { ToastStack } from "../widgets/toast-stack.js";
import type { Session } from "../../core/domain/session.js";
import { hasLiveTurn, turns } from "../../core/domain/session.js";
import { modelLabel } from "../../core/domain/model-choice.js";
import type { Action } from "../../effect/state/actions.js";
import type { ToastEntry } from "../../effect/state/toast-store.js";
import type { PermissionPending } from "../../core/domain/permission.js";
import { isPending } from "../../core/domain/permission.js";

export interface AgentSessionRouteProps {
  readonly session: () => Session | undefined;
  readonly toasts: () => ReadonlyArray<ToastEntry>;
  readonly hints: () => ReadonlyArray<{ readonly key: string; readonly action: string }>;
  readonly dispatch: (action: Action) => void;
}

export function AgentSessionRoute(props: AgentSessionRouteProps): JSX.Element {
  const pending = (): PermissionPending | undefined => {
    const s = props.session();
    if (s === undefined) return undefined;
    for (const p of s.permissions.values()) {
      if (isPending(p)) return p;
    }
    return undefined;
  };

  return (
    <BoxView paddingLeft={1} paddingRight={1}>
      <Show
        when={props.session()}
        fallback={
          <BoxView flexDirection="row">
            <SpinnerView />
            <TextView fg="fgMuted"> connecting…</TextView>
          </BoxView>
        }
      >
        {(session) => {
          const s = session();
          return (
            <BoxView>
              <BoxView flexDirection="row">
                <TextView>{s.title}</TextView>
                <TextView fg="fgDim"> · </TextView>
                <TextView fg="toolName">{modelLabel(s.model)}</TextView>
                <Show when={hasLiveTurn(s)}>
                  <TextView fg="fgDim"> · </TextView>
                  <SpinnerView fg="caret" />
                  <TextView fg="fgDim"> turn in flight</TextView>
                </Show>
              </BoxView>
              <Divider />
              <For each={[...turns(s)]}>
                {(t) => (
                  <TurnView
                    turn={t}
                    onInspectArtifact={(id) =>
                      props.dispatch({ tag: "InspectArtifact", artifactId: id })
                    }
                  />
                )}
              </For>
              <Show when={pending()}>
                {(p) => (
                  <PermissionPrompt permission={p()} onResolve={props.dispatch} />
                )}
              </Show>
            </BoxView>
          );
        }}
      </Show>
      <Divider />
      <InputArea
        disabled={(() => {
          const s = props.session();
          return s !== undefined && hasLiveTurn(s);
        })()}
        onSubmit={props.dispatch}
      />
      <ToastStack entries={props.toasts} />
      <KeyHintBar hints={props.hints} />
    </BoxView>
  );
}
