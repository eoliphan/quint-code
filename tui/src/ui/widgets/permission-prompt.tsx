// L7: PermissionPrompt — captures operator decision into a
// UserDecision capability token and dispatches RespondPermission.
//
// This widget is the SOLE bridge from keyboard input to a
// UserDecision. It uses L1.5's fromKeystroke under the hood. Any
// other widget that wants to resolve a permission must go through an
// Action and the dispatcher — no shortcuts.

import { type JSX } from "solid-js";
import type { PermissionPending } from "../../core/domain/permission.js";
import { fromKeystroke } from "../../core/user-action/user-decision.js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import type { Action } from "../../effect/state/actions.js";

export interface PermissionPromptProps {
  readonly permission: PermissionPending;
  readonly onResolve: (action: Action) => void;
}

export function PermissionPrompt(props: PermissionPromptProps): JSX.Element {
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

  return (
    <BoxView bg="bgPanel" paddingLeft={2} paddingRight={2} paddingTop={1} paddingBottom={1}>
      <TextView fg="warning">⚠ permission required</TextView>
      <TextView>
        tool <TextView fg="toolName">{p.toolName}</TextView> wants to run
      </TextView>
      <TextView fg="toolArgs">{args()}</TextView>
      <BoxView flexDirection="row" paddingTop={1}>
        <TextView
          fg="success"
          onMouseUp={() => respond("approved", "operator approved via mouse")}
        >
          [enter] approve
        </TextView>
        <TextView
          fg="danger"
          onMouseUp={() => respond("denied", "operator denied via mouse")}
        >
          {"  [esc] deny"}
        </TextView>
      </BoxView>
    </BoxView>
  );
}
