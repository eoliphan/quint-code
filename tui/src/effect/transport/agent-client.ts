// L3: Agent surface SDK — typed methods over Transport.
//
// The capability boundary lives HERE: `permissionRespond` requires a
// UserDecision in its TypeScript signature. No TUI code path can issue
// a resolution decision without first constructing a UserDecision via
// L1.5's fromKeystroke — which requires a real KeyboardEvent witness.
// Auto-approval is a compile error.

import { Context, Effect, Either, Layer, Schema as S } from "effect";
import { TransportService } from "./service.js";
import {
  decodeSessionCreateResponse,
  decodeTurnSubmitResponse,
  decodeHealthzResponse,
  decodeSessionListResponse,
  type SessionCreateRequest,
} from "../../core/wire/commands.js";
import { type AuthStatus, decodeAuthStatus } from "../../core/wire/auth-status.js";
import { type RPCError } from "./errors.js";
import { type UserDecision } from "../../core/user-action/user-decision.js";
import {
  type SessionID,
  type TurnID,
  type PermissionID,
} from "../../core/domain/ids.js";
import { type ModelChoice } from "../../core/domain/model-choice.js";
import { type Schema } from "effect";

export interface AgentClient {
  readonly healthz: () => Effect.Effect<{ ok: boolean; subscribers: number }, RPCError>;

  readonly authStatus: () => Effect.Effect<AuthStatus, RPCError>;

  readonly sessionList: () => Effect.Effect<
    { sessions: ReadonlyArray<{ id: string; title: string }> },
    RPCError
  >;

  readonly sessionCreate: (
    body: Schema.Schema.Type<typeof SessionCreateRequest>,
  ) => Effect.Effect<{ id: SessionID }, RPCError>;

  readonly turnSubmit: (id: SessionID, text: string) => Effect.Effect<{ turnId: TurnID }, RPCError>;

  readonly turnCancel: (id: SessionID, turnId: TurnID) => Effect.Effect<void, RPCError>;

  readonly sessionRename: (id: SessionID, title: string) => Effect.Effect<void, RPCError>;

  readonly modelSet: (id: SessionID, model: ModelChoice) => Effect.Effect<void, RPCError>;

  // permissionRespond REQUIRES a UserDecision capability token. The
  // token's existence proves a real keyboard event captured the
  // operator's choice — no code path can fabricate one.
  readonly permissionRespond: (
    id: PermissionID,
    user: UserDecision,
  ) => Effect.Effect<void, RPCError>;

  // listCommands returns the slash-command entries the Go side
  // reads from ~/.claude/commands/. Empty list when the directory
  // is missing; never errors on the happy path.
  readonly listCommands: () => Effect.Effect<
    { commands: ReadonlyArray<{ name: string; description?: string }> },
    RPCError
  >;

  // getCommand returns the markdown body of one command.
  readonly getCommand: (name: string) => Effect.Effect<{ name: string; body: string }, RPCError>;

  // listSkills mirrors listCommands for ~/.claude/skills/.
  readonly listSkills: () => Effect.Effect<
    { skills: ReadonlyArray<{ name: string; description?: string }> },
    RPCError
  >;

  // getSkill returns the markdown body of one skill.
  readonly getSkill: (name: string) => Effect.Effect<{ name: string; body: string }, RPCError>;

  // readFile returns the contents of a project-rooted file via the
  // /file endpoint. Used by the @-mention expander in the
  // dispatcher to inline file contents into the operator's prompt.
  readonly readFile: (
    path: string,
  ) => Effect.Effect<{ path: string; body: string; truncated: boolean; size: number }, RPCError>;
}

export class AgentClientService extends Context.Tag("haft/AgentClient")<
  AgentClientService,
  AgentClient
>() {}

const make: Effect.Effect<AgentClient, never, TransportService> = Effect.gen(function* () {
  const transport = yield* TransportService;

  const healthz: AgentClient["healthz"] = () => transport.getJson("/healthz", decodeHealthzResponse);

  const authStatus: AgentClient["authStatus"] = () =>
    transport.getJson("/auth/status", decodeAuthStatus);

  const sessionList: AgentClient["sessionList"] = () =>
    transport.getJson("/session", decodeSessionListResponse);

  const sessionCreate: AgentClient["sessionCreate"] = (body) =>
    transport.postJson("/session", body, decodeSessionCreateResponse).pipe(
      Effect.map((r) => ({ id: r.session_id as unknown as SessionID })),
    );

  const turnSubmit: AgentClient["turnSubmit"] = (id, text) =>
    transport.postJson(`/session/${id}/turn`, { text }, decodeTurnSubmitResponse).pipe(
      Effect.map((r) => ({ turnId: r.turn_id as unknown as TurnID })),
    );

  const turnCancel: AgentClient["turnCancel"] = (id, turnId) =>
    transport.postVoid(`/session/${id}/cancel`, { turn_id: turnId });

  const sessionRename: AgentClient["sessionRename"] = (id, title) =>
    transport.postVoid(`/session/${id}/rename`, { title });

  const modelSet: AgentClient["modelSet"] = (id, model) =>
    transport.postVoid(`/session/${id}/model`, {
      model: {
        provider: model.provider,
        model: model.model,
        credential_key: model.credentialKey,
      },
    });

  const permissionRespond: AgentClient["permissionRespond"] = (id, user) =>
    transport.postVoid(`/permission/${id}`, {
      decision: user.decision,
      reason: user.reason,
    });

  const CommandsListSchema = S.Struct({
    commands: S.Array(
      S.Struct({ name: S.String, description: S.optional(S.String) }),
    ),
  });
  const SkillsListSchema = S.Struct({
    skills: S.Array(
      S.Struct({ name: S.String, description: S.optional(S.String) }),
    ),
  });
  const ItemBodySchema = S.Struct({ name: S.String, body: S.String });

  const decodeCommandsList = S.decodeUnknownEither(CommandsListSchema);
  const decodeSkillsList = S.decodeUnknownEither(SkillsListSchema);
  const decodeItemBody = S.decodeUnknownEither(ItemBodySchema);

  const listCommands: AgentClient["listCommands"] = () =>
    transport.getJson("/commands", (raw) => decodeCommandsList(raw) as Either.Either<{ commands: ReadonlyArray<{ name: string; description?: string }> }, unknown>);

  const getCommand: AgentClient["getCommand"] = (name) =>
    transport.getJson(`/commands/${encodeURIComponent(name)}`, decodeItemBody);

  const listSkills: AgentClient["listSkills"] = () =>
    transport.getJson("/skills", (raw) => decodeSkillsList(raw) as Either.Either<{ skills: ReadonlyArray<{ name: string; description?: string }> }, unknown>);

  const getSkill: AgentClient["getSkill"] = (name) =>
    transport.getJson(`/skills/${encodeURIComponent(name)}`, decodeItemBody);

  const FileBodySchema = S.Struct({
    path: S.String,
    body: S.String,
    truncated: S.Boolean,
    size: S.Number,
  });
  const decodeFileBody = S.decodeUnknownEither(FileBodySchema);

  const readFile: AgentClient["readFile"] = (path) =>
    transport.getJson(`/file?path=${encodeURIComponent(path)}`, decodeFileBody);

  return {
    healthz,
    authStatus,
    sessionList,
    sessionCreate,
    turnSubmit,
    turnCancel,
    sessionRename,
    modelSet,
    permissionRespond,
    listCommands,
    getCommand,
    listSkills,
    getSkill,
    readFile,
  } satisfies AgentClient;
});

export const AgentClientLive: Layer.Layer<AgentClientService, never, TransportService> =
  Layer.effect(AgentClientService, make);
