// L7: PermissionPrompt — captures operator decision into a
// UserDecision capability token and dispatches RespondPermission.
//
// This widget is the SOLE bridge from keyboard input to a
// UserDecision. It uses L1.5's fromKeystroke under the hood. Any
// other widget that wants to resolve a permission must go through
// an Action and the dispatcher — no shortcuts.
//
// Visual design (Claude-Code-flavored modal panel):
//   ┌─────────────────────────────────────────────────────┐
//   │ ⚠ permission required                               │
//   │                                                     │
//   │ tool: bash                                          │
//   │ args:                                               │
//   │   { "command": "rm -rf /tmp/foo" }                 │
//   │                                                     │
//   │ [enter] approve   [esc] deny   (mouse also works)   │
//   └─────────────────────────────────────────────────────┘

import { type JSX, onCleanup, onMount } from "solid-js";
import { useRenderer } from "@opentui/solid";
import type { PermissionPending } from "../../core/domain/permission.js";
import { fromKeystroke } from "../../core/user-action/user-decision.js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { useTheme } from "../theme-context.js";
import type { Action } from "../../effect/state/actions.js";

export interface PermissionPromptProps {
  readonly permission: PermissionPending;
  readonly onResolve: (action: Action) => void;
}

export function PermissionPrompt(props: PermissionPromptProps): JSX.Element {
  const theme = useTheme();
  const p = props.permission;

  const respond = (decision: "approved" | "denied", reason: string): void => {
    const user = fromKeystroke(
      { type: "keydown", key: decision === "approved" ? "Enter" : "Escape" },
      decision,
      reason,
      new Date(),
    );
    props.onResolve({
      tag: "RespondPermission",
      id: p.id,
      user,
    });
  };

  const args = (): string => {
    try {
      return JSON.stringify(p.args, null, 2);
    } catch {
      return String(p.args);
    }
  };

  // Keyboard wiring — Enter approves, Escape denies. The dialog
  // pre-empts the input area's keystream while it is mounted; the
  // input box still receives chars but the operator's expectation
  // is "approve / deny first, then continue typing".
  onMount(() => {
    const renderer = useRenderer();
    const onKey = (ev: { name: string; sequence: string }): void => {
      if (ev.name === "return" || ev.name === "enter") {
        respond("approved", "operator approved via keyboard");
        return;
      }
      if (ev.name === "escape") {
        respond("denied", "operator denied via keyboard");
      }
    };
    renderer.keyInput.on("keypress", onKey);
    onCleanup(() => renderer.keyInput.off("keypress", onKey));
  });

  const accent = (): string => {
    const v = theme()["warning"];
    return typeof v === "string" ? v : theme().fg;
  };

  return (
    <box
      border={["top", "bottom", "left", "right"]}
      borderColor={accent()}
      paddingLeft={2}
      paddingRight={2}
      paddingTop={1}
      paddingBottom={1}
      marginTop={1}
      marginBottom={1}
    >
      <text>
        <b style={{ fg: accent() }}>⚠ permission required</b>
      </text>
      <BoxView paddingTop={1} flexDirection="row">
        <TextView fg="fgDim">tool: </TextView>
        <TextView fg="toolName">
          <b>{p.toolName}</b>
        </TextView>
      </BoxView>
      <BoxView paddingTop={1}>
        <TextView fg="fgDim">args:</TextView>
        <BoxView paddingLeft={2}>
          <TextView fg="toolArgs">{args()}</TextView>
        </BoxView>
      </BoxView>
      <BoxView paddingTop={1} flexDirection="row">
        <TextView
          fg="success"
          onMouseUp={() => respond("approved", "operator approved via mouse")}
        >
          <b>[enter]</b> approve
        </TextView>
        <TextView fg="fgDim">   </TextView>
        <TextView
          fg="danger"
          onMouseUp={() => respond("denied", "operator denied via mouse")}
        >
          <b>[esc]</b> deny
        </TextView>
        <TextView fg="fgDim">   (mouse works too)</TextView>
      </BoxView>
    </box>
  );
}
