// L2: AuthStatusPayload wire schema.
//
// Mirrors internal/agentproto.AuthStatusPayload on the Go side. Snake_case
// JSON keys per the v7.1.0 hardening pass; Effect Schema validates on
// decode and yields a typed value.

import { Schema } from "effect";

export const AuthStatusSchema = Schema.Struct({
  provider: Schema.String,
  model: Schema.String,
  has_credentials: Schema.Boolean,
  expires_at: Schema.optional(Schema.String),
});

export type AuthStatus = Schema.Schema.Type<typeof AuthStatusSchema>;

export const decodeAuthStatus = Schema.decodeUnknownEither(AuthStatusSchema);
