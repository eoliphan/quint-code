// L5: Action dispatcher — the only legitimate write surface.
//
// Every route / widget that wants to mutate state constructs an Action
// and calls dispatch(action). The handler maps each tag to a side
// effect (RPC via Agent SDK) + a follow-up store update where needed.
// The SSE event stream feeding SessionStore is the authoritative
// state-mutation path; Actions usually issue RPCs whose effects come
// back as events.

import { Context, Effect, Layer } from "effect";
import { AgentClientService, type AgentClient } from "../transport/agent-client.js";
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
        // Expand @file mentions before sending. Each @path is
        // replaced by a fenced code block containing the file's
        // contents (truncated server-side at 256KB). Failures fall
        // through as a notice so the operator sees why the
        // expansion didn't happen.
        return Effect.gen(function* () {
          const expanded = yield* expandMentions(action.text, client);
          yield* client.turnSubmit(sid, expanded).pipe(Effect.asVoid);
        }).pipe(Effect.catchAll(reportRpc("submit turn")));
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
        // Navigate immediately so the agent.session route's focused
        // <input> mounts before the operator finishes typing the next
        // character. Awaiting the RPC before route.navigate leaves a
        // ~50ms window where the home route is still active but
        // there's nothing left to focus on the home side either — the
        // first post-Enter keystroke gets dropped on the floor.
        // The brief "connecting…" fallback the agent.session route
        // renders while session() is still undefined is the correct
        // visual signal for the in-flight RPC.
        return Effect.sync(() => route.navigate("agent.session")).pipe(
          Effect.flatMap(() =>
            client.sessionCreate({
              project_id: "haft",
              title: action.title,
              model: {
                provider: action.model.provider,
                model: action.model.model,
                credential_key: action.model.credentialKey,
              },
            }),
          ),
          Effect.asVoid,
          Effect.catchAll((err) =>
            Effect.gen(function* () {
              // RPC failed after we navigated forward — surface the
              // error AND bounce back to home so the operator can
              // retry. Without the route reset they're stranded on
              // an agent.session that will never get a session.
              yield* reportRpc("create session")(err);
              yield* Effect.sync(() => route.navigate("home"));
            }),
          ),
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

// expandMentions scans the operator's prompt for @path tokens
// (whitespace-delimited, must start with a letter, dot, or slash).
// Each is fetched via /file and inlined as a fenced code block.
// Failures degrade gracefully — the original @token stays in the
// prompt so the model can see what was attempted.
function expandMentions(
  text: string,
  client: AgentClient,
): Effect.Effect<string, never, never> {
  return Effect.gen(function* () {
    // Capture @relative/path/with/slashes_and-dots.ext tokens.
    // Reject pure email-looking "@name" without "/" or "." so a
    // chat about "@user" doesn't trigger file reads.
    const re = /@([A-Za-z0-9_./-]+)/g;
    const matches: { full: string; path: string }[] = [];
    for (let m = re.exec(text); m !== null; m = re.exec(text)) {
      const p = m[1] ?? "";
      if (p === "" || (!p.includes("/") && !p.includes("."))) continue;
      matches.push({ full: m[0], path: p });
    }
    if (matches.length === 0) return text;
    let out = text;
    for (const { full, path } of matches) {
      const body = yield* (client.readFile(path) as Effect.Effect<{ path: string; body: string; truncated: boolean }, unknown, never>).pipe(
        Effect.either,
      );
      if (body._tag === "Right") {
        const note = body.right.truncated ? " (truncated)" : "";
        const block = `\n\n=== ${path}${note} ===\n\`\`\`\n${body.right.body}\n\`\`\`\n`;
        out = out.replace(full, block);
      }
      // Failure: leave the @token in place; the LLM sees it as a
      // literal hint that the operator wanted to reference that
      // file. A toast surfaces the issue separately at the call
      // site.
    }
    return out;
  });
}
