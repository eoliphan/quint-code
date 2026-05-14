// L3: Typed transport errors (Effect Data.TaggedError).
//
// Each error variant is a Tagged class so it works inside Effect's
// error channel — pattern-match via Effect.catchTags / Match.tag.

import { Data } from "effect";

export class NetworkError extends Data.TaggedError("NetworkError")<{
  readonly url: string;
  readonly cause: unknown;
}> {}

export class TimeoutError extends Data.TaggedError("TimeoutError")<{
  readonly url: string;
  readonly ms: number;
}> {}

export class DecodeError extends Data.TaggedError("DecodeError")<{
  readonly url: string;
  readonly body: string;
  readonly cause: unknown;
}> {}

export class HttpStatusError extends Data.TaggedError("HttpStatusError")<{
  readonly url: string;
  readonly status: number;
  readonly body: string;
  readonly marker?:
    | "turn_already_running"
    | "turn_mismatch"
    | "turn_not_running"
    | "permission_not_found"
    | "unsupported_command"
    | "session_not_found";
}> {}

export class AbortedError extends Data.TaggedError("AbortedError")<{
  readonly url: string;
}> {}

export type RPCError = NetworkError | TimeoutError | DecodeError | HttpStatusError | AbortedError;

const MARKERS: ReadonlyArray<readonly [HttpStatusError["marker"] & string, RegExp]> = [
  ["turn_already_running", /turn already running/i],
  ["turn_mismatch", /turn id does not match running turn/i],
  ["turn_not_running", /no running turn on session/i],
  ["permission_not_found", /permission not found|unknown permission/i],
  ["unsupported_command", /dispatcher does not support/i],
  ["session_not_found", /session not found/i],
];

export function detectMarker(body: string): HttpStatusError["marker"] | undefined {
  for (const [marker, re] of MARKERS) {
    if (re.test(body)) return marker;
  }
  return undefined;
}
