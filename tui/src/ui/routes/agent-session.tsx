// L8: SessionRoute — agent chat view composed from L7 widgets.
//
// Layout: reverse-chat. Messages anchor to the bottom of the chat
// area; older turns push UP out of view as new ones land at the
// bottom. The status bar at the very bottom surfaces project path,
// git branch, model, and live-turn indicator — same shape Claude
// Code uses, ported from the legacy haft StatusBar.

import { type JSX, For, Show, onCleanup, onMount } from "solid-js";
import { useRenderer } from "@opentui/solid";
import type { TurnID } from "../../core/domain/ids.js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { KeyHintBar } from "../primitives/key-hint-bar.js";
import { Divider } from "../primitives/divider.js";
import { SpinnerView } from "../primitives/spinner-view.js";
import { StatusBar, shortenPath, type StatusBarItem } from "../primitives/status-bar.js";
import { formatTokens } from "../../effect/state/token-store.js";
import { TurnView } from "../widgets/turn-view.js";
import { ThinkingIndicator } from "../widgets/thinking-indicator.js";
import { PermissionPrompt } from "../widgets/permission-prompt.js";
import { InputArea } from "../widgets/input-area.js";
import { ToastStack } from "../widgets/toast-stack.js";
import { CommandSkillPicker } from "../widgets/command-skill-picker.js";
import type { Session } from "../../core/domain/session.js";
import { hasLiveTurn, turns } from "../../core/domain/session.js";
import { modelLabel } from "../../core/domain/model-choice.js";
import type { Action } from "../../effect/state/actions.js";
import type { ToastEntry } from "../../effect/state/toast-store.js";
import type { AuthStatus } from "../../core/wire/auth-status.js";
import type { PermissionPending } from "../../core/domain/permission.js";
import { isPending } from "../../core/domain/permission.js";

export interface AgentSessionRouteProps {
  readonly auth: () => AuthStatus | undefined;
  readonly session: () => Session | undefined;
  readonly toasts: () => ReadonlyArray<ToastEntry>;
  readonly hints: () => ReadonlyArray<{ readonly key: string; readonly action: string }>;
  readonly tokenTotal: () => number;
  readonly dispatch: (action: Action) => void;
}

const HOME = process.env["HOME"] ?? "";

export function AgentSessionRoute(props: AgentSessionRouteProps): JSX.Element {
  const pending = (): PermissionPending | undefined => {
    const s = props.session();
    if (s === undefined) return undefined;
    for (const p of s.permissions.values()) {
      if (isPending(p)) return p;
    }
    return undefined;
  };

  const turnList = (): ReadonlyArray<ReturnType<typeof turns>[number]> => {
    const s = props.session();
    if (s === undefined) return [];
    return turns(s);
  };

  const inFlight = (): boolean => {
    const s = props.session();
    return s !== undefined && hasLiveTurn(s);
  };

  const liveTurnId = (): TurnID | undefined => {
    const s = props.session();
    if (s === undefined || !hasLiveTurn(s)) return undefined;
    return s.liveTurn.id;
  };

  // Global keystroke handler scoped to the agent.session route.
  // Esc + Ctrl+C while a turn is running both dispatch CancelTurn so
  // the operator can interrupt a long generation. Ctrl+C without
  // an active turn falls through to the AppShell's home-route
  // handler which exits the process; we don't exit here to avoid
  // losing the operator's session by accident.
  onMount(() => {
    const renderer = useRenderer();
    const onKey = (ev: { name: string; ctrl: boolean }): void => {
      if (!inFlight()) return;
      const tid = liveTurnId();
      if (tid === undefined) return;
      if (ev.name === "escape" || (ev.ctrl && ev.name === "c")) {
        props.dispatch({ tag: "CancelTurn", turnId: tid });
        props.dispatch({ tag: "ShowToast", message: "cancel requested", level: "info" });
      }
    };
    renderer.keyInput.on("keypress", onKey);
    onCleanup(() => renderer.keyInput.off("keypress", onKey));
  });

  // Status-bar items. Mirrors the legacy Ink StatusBar shape: project
  // path · branch · provider/model · streaming. Items are added
  // conditionally so a missing branch (non-git project) doesn't
  // surface as an empty pill.
  const statusItems = (): readonly StatusBarItem[] => {
    const items: StatusBarItem[] = [];
    const a = props.auth();
    if (a?.project_root) {
      items.push({ label: shortenPath(a.project_root, HOME, 36), tone: "muted" });
    }
    if (a?.git_branch) {
      items.push({ label: a.git_branch, tone: "accent" });
    }
    const s = props.session();
    if (s) {
      items.push({ label: modelLabel(s.model), tone: "muted" });
    } else if (a?.model) {
      items.push({ label: `${a.provider}/${a.model}`, tone: "muted" });
    }
    if (inFlight()) {
      items.push({ label: "stream", tone: "success", bold: true });
    }
    const t = props.tokenTotal();
    if (t > 0) {
      items.push({ label: `${formatTokens(t)} tokens`, tone: "muted" });
    }
    return items;
  };

  return (
    <BoxView flexGrow={1} paddingLeft={1} paddingRight={1}>
      {/* Header — fixed at top */}
      <Show
        when={props.session()}
        fallback={
          <BoxView flexDirection="row" paddingTop={1}>
            <SpinnerView />
            <TextView fg="fgMuted"> connecting…</TextView>
          </BoxView>
        }
      >
        <BoxView flexDirection="row" paddingTop={1} paddingBottom={1}>
          <TextView fg="toolName">▣ </TextView>
          <TextView>{props.session()?.title ?? ""}</TextView>
          <TextView fg="fgDim"> · </TextView>
          <TextView fg="toolName">
            {(() => {
              const s = props.session();
              return s === undefined ? "" : modelLabel(s.model);
            })()}
          </TextView>
          <Show when={inFlight()}>
            <TextView fg="fgDim"> · </TextView>
            <ThinkingIndicator />
          </Show>
        </BoxView>
      </Show>
      <Divider />

      <BoxView flexGrow={1} justifyContent="flex-end">
        <scrollbox flexGrow={1} stickyScroll stickyStart="bottom">
          <BoxView justifyContent="flex-end" flexGrow={1}>
            <For each={turnList()}>
              {(t) => (
                <TurnView
                  turn={t}
                  onInspectArtifact={(id) =>
                    props.dispatch({ tag: "InspectArtifact", artifactId: id })
                  }
                />
              )}
            </For>
          </BoxView>
        </scrollbox>
      </BoxView>

      <Show when={pending()}>
        {(p) => <PermissionPrompt permission={p()} onResolve={props.dispatch} />}
      </Show>

      <CommandSkillPicker enabled={() => !inFlight()} dispatch={props.dispatch} />

      <Divider />
      <InputArea disabled={inFlight()} onSubmit={props.dispatch} />
      <ToastStack entries={props.toasts} />
      <KeyHintBar hints={props.hints} />
      <BoxView paddingLeft={1} paddingRight={1}>
        <StatusBar items={statusItems} />
      </BoxView>
    </BoxView>
  );
}
