// L3: Agent surface SDK — typed methods over Transport.
//
// The capability boundary lives HERE: `permissionRespond` requires a
// UserDecision in its TypeScript signature. No TUI code path can issue
// a resolution decision without first constructing a UserDecision via
// L1.5's fromKeystroke — which requires a real KeyboardEvent witness.
// Auto-approval is a compile error.

import { Context, Effect, Layer } from "effect";
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
      Effect.map((r) => ({ id: r.id as unknown as SessionID })),
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
  } satisfies AgentClient;
});

export const AgentClientLive: Layer.Layer<AgentClientService, never, TransportService> =
  Layer.effect(AgentClientService, make);
