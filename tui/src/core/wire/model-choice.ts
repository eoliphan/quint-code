// L2: ModelChoice wire schema.

import { Schema } from "effect";

export const ModelChoiceSchema = Schema.Struct({
  provider: Schema.String,
  model: Schema.String,
  credential_key: Schema.optional(Schema.String),
});

export type ModelChoiceWire = Schema.Schema.Type<typeof ModelChoiceSchema>;
