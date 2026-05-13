/** Branded string IDs. Each surface defines its own brands on top. */
export type Brand<T, B extends string> = T & { readonly __brand: B };

/** Result of attempting to decode a wire event. Discriminated union so
 *  unknown-kind events flow through the same channel as decoded ones
 *  rather than throwing, which would crash the SSE reader. */
export type DecodeResult<TEvent> =
  | { ok: true; event: TEvent }
  | { ok: false; reason: "unknown_kind"; kind: string; raw: unknown }
  | { ok: false; reason: "malformed"; error: string; raw: unknown };

/** Typed error categories the RPC poster maps from HTTP responses.
 *  Surfaces inject their own error mappers; this is the closed set
 *  the transport itself recognises. */
export type RPCErrorKind =
  | "bad_request"          // 400
  | "not_found"            // 404
  | "conflict"             // 409
  | "server_error"         // 5xx
  | "network"              // fetch threw
  | "decode"               // response body not JSON
  | "timeout";             // request exceeded budget

export class RPCError extends Error {
  constructor(
    public readonly kind: RPCErrorKind,
    public readonly status: number,
    public readonly body: string,
    message: string,
  ) {
    super(message);
    this.name = "RPCError";
  }
}

/** Mapper called on every non-2xx response. Surfaces translate to
 *  domain-specific typed errors (e.g. ErrTurnAlreadyRunning → 409). */
export type ErrorMapper = (status: number, body: string) => RPCError;

/** Default mapper: maps status codes to the closed RPCErrorKind set
 *  with no surface-specific interpretation. */
export const defaultErrorMapper: ErrorMapper = (status, body) => {
  if (status === 400) return new RPCError("bad_request", status, body, `400 bad_request: ${body}`);
  if (status === 404) return new RPCError("not_found", status, body, `404 not_found: ${body}`);
  if (status === 409) return new RPCError("conflict", status, body, `409 conflict: ${body}`);
  if (status >= 500) return new RPCError("server_error", status, body, `${status} server_error: ${body}`);
  return new RPCError("server_error", status, body, `${status} unexpected: ${body}`);
};
