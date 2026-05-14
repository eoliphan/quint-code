// L2: Wire-level error envelopes.
//
// The Go-side agentserver emits typed errors via the body text + HTTP
// status. The wire layer's job is to detect them at the boundary and
// produce a closed sum the transport (L3) can pattern-match on.

import { Schema } from "effect";

// Markers the Go side emits in error bodies; matched in L3 by regex
// after decoding the response.
export const WIRE_ERROR_MARKERS = [
  "turn_already_running",
  "turn_mismatch",
  "turn_not_running",
  "permission_not_found",
  "unsupported_command",
  "session_not_found",
] as const;

export type WireErrorMarker = (typeof WIRE_ERROR_MARKERS)[number];

export const WireErrorBodySchema = Schema.Struct({
  error: Schema.String,
});

export const decodeWireErrorBody = Schema.decodeUnknownEither(WireErrorBodySchema);
