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
  // project_root + git_branch surface the StatusBar context. Both
  // optional — the Go side leaves them empty when running outside
  // a haft project or a git repo.
  project_root: Schema.optional(Schema.String),
  git_branch: Schema.optional(Schema.String),
});

export type AuthStatus = Schema.Schema.Type<typeof AuthStatusSchema>;

export const decodeAuthStatus = Schema.decodeUnknownEither(AuthStatusSchema);
