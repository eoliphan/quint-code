// L5: Action sum — closed set of UI-originated state mutations.
//
// Routes and widgets dispatch Actions; the action-handler layer
// (handlers.ts) maps each tag to side effects (typically an RPC via
// the Agent SDK) and store updates. No store mutation outside this
// channel — silent updates from random components are unrepresentable
// because every dispatch site has to construct a value of this union.

import type { SessionID, TurnID, PermissionID } from "../../core/domain/ids.js";
import type { ModelChoice } from "../../core/domain/model-choice.js";
import type { UserDecision } from "../../core/user-action/user-decision.js";
import type { ThemeName } from "./theme-store.js";

export type Action =
  | { readonly tag: "SubmitTurn"; readonly text: string }
  | { readonly tag: "CancelTurn"; readonly turnId: TurnID }
  | { readonly tag: "RespondPermission"; readonly id: PermissionID; readonly user: UserDecision }
  | { readonly tag: "SwitchModel"; readonly model: ModelChoice }
  | { readonly tag: "RenameSession"; readonly title: string }
  | { readonly tag: "CreateSession"; readonly title: string; readonly model: ModelChoice }
  | { readonly tag: "ResumeSession"; readonly id: SessionID }
  | { readonly tag: "OpenDialog"; readonly spec: DialogSpec }
  | { readonly tag: "CloseDialog" }
  | { readonly tag: "ShowToast"; readonly message: string; readonly level: "info" | "warn" | "error" }
  | { readonly tag: "NavigateRoute"; readonly to: RouteName }
  | { readonly tag: "SetTheme"; readonly name: ThemeName }
  | { readonly tag: "CycleTheme" }
  | { readonly tag: "InspectArtifact"; readonly artifactId: string };

export type RouteName = "agent.session" | "home" | "fpf.inspector";

// DialogSpec is a closed sum over the dialogs the app can open. Each
// variant carries the props the dialog renderer needs.
export type DialogSpec =
  | { readonly kind: "confirm"; readonly title: string; readonly message: string; readonly onConfirm: Action }
  | { readonly kind: "select"; readonly title: string; readonly options: ReadonlyArray<{ readonly label: string; readonly action: Action }> }
  | { readonly kind: "prompt"; readonly title: string; readonly initial: string; readonly onSubmit: (text: string) => Action }
  | { readonly kind: "themePicker" }
  | { readonly kind: "modelPicker" }
  | { readonly kind: "help" }
  | { readonly kind: "sessionList" };

export function assertNeverAction(a: never): never {
  throw new Error(`unhandled action: ${JSON.stringify(a)}`);
}
