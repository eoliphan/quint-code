// L5: Action dispatcher — the only legitimate write surface.
//
// Every route / widget that wants to mutate state constructs an Action
// and calls dispatch(action). The handler maps each tag to a side
// effect (RPC via Agent SDK) + a follow-up store update where needed.
// The SSE event stream feeding SessionStore is the authoritative
// state-mutation path; Actions usually issue RPCs whose effects come
// back as events.

import { Context, Effect, Layer } from "effect";
import { AgentClientService } from "../transport/agent-client.js";
import type { Action } from "./actions.js";
import { assertNeverAction } from "./actions.js";
import { SessionStoreService } from "./session-store.js";
import { ThemeStoreService } from "./theme-store.js";
import { RouteStoreService } from "./route-store.js";
import { DialogStoreService } from "./dialog-store.js";
import { ToastStoreService } from "./toast-store.js";
import type { SessionID } from "../../core/domain/ids.js";

export interface Dispatcher {
  readonly dispatch: (action: Action) => Effect.Effect<void, never>;
}

export class DispatcherService extends Context.Tag("haft/Dispatcher")<
  DispatcherService,
  Dispatcher
>() {}

type Deps =
  | AgentClientService
  | SessionStoreService
  | ThemeStoreService
  | RouteStoreService
  | DialogStoreService
  | ToastStoreService;

const make: Effect.Effect<Dispatcher, never, Deps> = Effect.gen(function* () {
  const client = yield* AgentClientService;
  const session = yield* SessionStoreService;
  const theme = yield* ThemeStoreService;
  const route = yield* RouteStoreService;
  const dialog = yield* DialogStoreService;
  const toast = yield* ToastStoreService;

  const sessionIdOrToast = (): SessionID | undefined => {
    const s = session.current();
    if (s === undefined) {
      toast.push("no active session", "warn");
      return undefined;
    }
    return s.id;
  };

  const reportRpc = (action: string) =>
    (err: unknown): Effect.Effect<void, never> =>
      Effect.sync(() => {
        const msg = err instanceof Error ? err.message : String(err);
        toast.push(`${action} failed: ${msg}`, "error");
      });

  const dispatch = (action: Action): Effect.Effect<void, never> => {
    switch (action.tag) {
      case "SubmitTurn": {
        const sid = sessionIdOrToast();
        if (sid === undefined) return Effect.void;
        return client.turnSubmit(sid, action.text).pipe(
          Effect.asVoid,
          Effect.catchAll(reportRpc("submit turn")),
        );
      }
      case "CancelTurn": {
        const sid = sessionIdOrToast();
        if (sid === undefined) return Effect.void;
        return client.turnCancel(sid, action.turnId).pipe(
          Effect.catchAll(reportRpc("cancel turn")),
        );
      }
      case "RespondPermission":
        return client.permissionRespond(action.id, action.user).pipe(
          Effect.catchAll(reportRpc("respond permission")),
        );
      case "SwitchModel": {
        const sid = sessionIdOrToast();
        if (sid === undefined) return Effect.void;
        return client.modelSet(sid, action.model).pipe(
          Effect.catchAll(reportRpc("switch model")),
        );
      }
      case "RenameSession": {
        const sid = sessionIdOrToast();
        if (sid === undefined) return Effect.void;
        return client.sessionRename(sid, action.title).pipe(
          Effect.catchAll(reportRpc("rename session")),
        );
      }
      case "CreateSession":
        return client
          .sessionCreate({
            project_id: "haft",
            title: action.title,
            model: {
              provider: action.model.provider,
              model: action.model.model,
              credential_key: action.model.credentialKey,
            },
          })
          .pipe(
            Effect.asVoid,
            Effect.catchAll(reportRpc("create session")),
          );
      case "ResumeSession":
        return Effect.sync(() => {
          // Resume is a navigation + future-fetch; for v8.0 alpha we
          // navigate to the session route and let the store re-bind
          // when the SSE stream replays the session's recorded events.
          route.navigate("agent.session");
          session.reset();
          toast.push(`resuming ${action.id}`, "info");
        });
      case "OpenDialog":
        return Effect.sync(() => dialog.open(action.spec));
      case "CloseDialog":
        return Effect.sync(() => dialog.close());
      case "ShowToast":
        return Effect.sync(() => toast.push(action.message, action.level));
      case "NavigateRoute":
        return Effect.sync(() => route.navigate(action.to));
      case "SetTheme":
        return Effect.sync(() => theme.set(action.name));
      case "CycleTheme":
        return Effect.sync(() => {
          const next = theme.cycle();
          toast.push(`theme: ${next}`, "info");
        });
      case "InspectArtifact":
        return Effect.sync(() => route.inspectArtifact(action.artifactId));
      default:
        return assertNeverAction(action);
    }
  };

  return { dispatch } satisfies Dispatcher;
});

export const DispatcherLive: Layer.Layer<DispatcherService, never, Deps> = Layer.effect(
  DispatcherService,
  make,
);
