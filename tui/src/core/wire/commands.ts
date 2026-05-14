// L2: RPC command schemas (TS → JSON encoders + response decoders).
//
// Each RPC verb has a Request schema and a Response schema. Encoding
// is total (TS shape → unknown JSON), decoding returns Either with a
// typed ParseError on failure.

import { Schema } from "effect";
import { ModelChoiceSchema } from "./model-choice.js";

// ---- session.create ----

export const SessionCreateRequest = Schema.Struct({
  project_id: Schema.String,
  title: Schema.String,
  model: ModelChoiceSchema,
});

export const SessionCreateResponse = Schema.Struct({
  session_id: Schema.String,
});

// ---- turn.submit ----

export const TurnSubmitRequest = Schema.Struct({
  text: Schema.String,
});

export const TurnSubmitResponse = Schema.Struct({
  turn_id: Schema.String,
});

// ---- turn.cancel ----

export const TurnCancelRequest = Schema.Struct({
  turn_id: Schema.String,
});

// ---- session.rename ----

export const SessionRenameRequest = Schema.Struct({
  title: Schema.String,
});

// ---- model.set ----

export const ModelSetRequest = Schema.Struct({
  model: ModelChoiceSchema,
});

// ---- permission.respond ----

export const PermissionRespondRequest = Schema.Struct({
  decision: Schema.Literal("approved", "denied"),
  reason: Schema.String,
});

// ---- healthz ----

export const HealthzResponse = Schema.Struct({
  ok: Schema.Boolean,
  subscribers: Schema.Number,
});

// ---- session list ----

export const SessionListResponse = Schema.Struct({
  sessions: Schema.Array(
    Schema.Struct({
      id: Schema.String,
      title: Schema.String,
    }),
  ),
});

// ---- Decoders ----

export const decodeSessionCreateResponse = Schema.decodeUnknownEither(SessionCreateResponse);
export const decodeTurnSubmitResponse = Schema.decodeUnknownEither(TurnSubmitResponse);
export const decodeHealthzResponse = Schema.decodeUnknownEither(HealthzResponse);
export const decodeSessionListResponse = Schema.decodeUnknownEither(SessionListResponse);
