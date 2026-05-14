// L3: TransportService — typed RPC + SSE over Effect.
//
// Methods:
//   getJson<TRes>(path, decoder)  -> Effect<TRes, RPCError>
//   postJson<TReq, TRes>(path, body, decoder) -> Effect<TRes, RPCError>
//   postVoid<TReq>(path, body)    -> Effect<void, RPCError>
//   subscribeEvents()             -> Stream<AgentEventWire, RPCError>
//
// Retry on 5xx (up to maxAttempts), no retry on 4xx (caller fault).
// AbortSignal-based teardown via the Effect runtime's scope. SSE reader
// reconnects with exponential backoff on non-abort disconnects.

import { Context, Effect, Either, Layer, Schedule, Stream, type StreamEmit } from "effect";
import { TransportConfigService } from "./config.js";
import {
  AbortedError,
  DecodeError,
  HttpStatusError,
  NetworkError,
  type RPCError,
  detectMarker,
} from "./errors.js";
import { type AgentEventWire, decodeAgentEvent } from "../../core/wire/events.js";

export interface Transport {
  readonly getJson: <TRes>(
    path: string,
    decode: (raw: unknown) => Either.Either<TRes, unknown>,
  ) => Effect.Effect<TRes, RPCError>;

  readonly postJson: <TReq, TRes>(
    path: string,
    body: TReq,
    decode: (raw: unknown) => Either.Either<TRes, unknown>,
  ) => Effect.Effect<TRes, RPCError>;

  readonly postVoid: <TReq>(path: string, body: TReq) => Effect.Effect<void, RPCError>;

  readonly subscribeEvents: () => Stream.Stream<AgentEventWire, RPCError>;
}

export class TransportService extends Context.Tag("haft/Transport")<TransportService, Transport>() {}

const fiveXX = (status: number): boolean => status >= 500 && status < 600;

function classifyHttp(url: string, status: number, body: string): HttpStatusError {
  return new HttpStatusError({
    url,
    status,
    body,
    marker: detectMarker(body),
  });
}

function abortHandler(url: string, signal: AbortSignal): Effect.Effect<void, AbortedError> {
  return Effect.async<void, AbortedError>((resume) => {
    if (signal.aborted) {
      resume(Effect.fail(new AbortedError({ url })));
      return;
    }
    const onAbort = (): void => resume(Effect.fail(new AbortedError({ url })));
    signal.addEventListener("abort", onAbort, { once: true });
    return Effect.sync(() => signal.removeEventListener("abort", onAbort));
  });
}
void abortHandler; // reserved for future cancellation surface

// ---- live implementation ----

const makeLive = (fetchImpl: typeof fetch): Effect.Effect<Transport, never, TransportConfigService> =>
  Effect.gen(function* () {
    const config = yield* TransportConfigService;

    const callOnce = (
      method: "GET" | "POST",
      path: string,
      body: unknown,
    ): Effect.Effect<{ status: number; text: string }, RPCError> =>
      Effect.tryPromise({
        try: async () => {
          const resp = await fetchImpl(config.baseUrl + path, {
            method,
            headers: method === "POST" ? { "Content-Type": "application/json" } : undefined,
            body: method === "POST" ? JSON.stringify(body) : undefined,
          });
          const text = await resp.text();
          return { status: resp.status, text };
        },
        catch: (cause) => new NetworkError({ url: config.baseUrl + path, cause }),
      });

    const callWithRetry = (
      method: "GET" | "POST",
      path: string,
      body: unknown,
    ): Effect.Effect<{ status: number; text: string }, RPCError> => {
      const url = config.baseUrl + path;
      return callOnce(method, path, body).pipe(
        Effect.flatMap((res) => {
          if (res.status >= 200 && res.status < 300) return Effect.succeed(res);
          if (fiveXX(res.status)) {
            return Effect.fail(classifyHttp(url, res.status, res.text));
          }
          // 4xx — caller fault, no retry. Bubble immediately as a
          // typed HttpStatusError with marker so callers can branch.
          return Effect.fail(classifyHttp(url, res.status, res.text));
        }),
        Effect.retry({
          times: config.maxAttempts - 1,
          while: (e: RPCError) => e._tag === "HttpStatusError" && fiveXX(e.status),
          schedule: Schedule.exponential("250 millis").pipe(Schedule.compose(Schedule.elapsed)),
        }),
      );
    };

    const getJson: Transport["getJson"] = (path, decode) =>
      callWithRetry("GET", path, undefined).pipe(
        Effect.flatMap((res) => {
          let raw: unknown;
          try {
            raw = JSON.parse(res.text);
          } catch (cause) {
            return Effect.fail(new DecodeError({ url: config.baseUrl + path, body: res.text, cause }));
          }
          const decoded = decode(raw);
          if (Either.isLeft(decoded)) {
            return Effect.fail(new DecodeError({ url: config.baseUrl + path, body: res.text, cause: decoded.left }));
          }
          return Effect.succeed(decoded.right);
        }),
      );

    const postJson: Transport["postJson"] = (path, body, decode) =>
      callWithRetry("POST", path, body).pipe(
        Effect.flatMap((res) => {
          if (res.text === "") {
            return Effect.fail(
              new DecodeError({ url: config.baseUrl + path, body: "", cause: "empty body" }),
            );
          }
          let raw: unknown;
          try {
            raw = JSON.parse(res.text);
          } catch (cause) {
            return Effect.fail(new DecodeError({ url: config.baseUrl + path, body: res.text, cause }));
          }
          const decoded = decode(raw);
          if (Either.isLeft(decoded)) {
            return Effect.fail(new DecodeError({ url: config.baseUrl + path, body: res.text, cause: decoded.left }));
          }
          return Effect.succeed(decoded.right);
        }),
      );

    const postVoid: Transport["postVoid"] = (path, body) =>
      callWithRetry("POST", path, body).pipe(Effect.asVoid);

    const subscribeEvents: Transport["subscribeEvents"] = () => sseStream(fetchImpl, config.baseUrl);

    return {
      getJson,
      postJson,
      postVoid,
      subscribeEvents,
    } satisfies Transport;
  });

function sseStream(
  fetchImpl: typeof fetch,
  baseUrl: string,
): Stream.Stream<AgentEventWire, RPCError> {
  return Stream.async<AgentEventWire, RPCError>((emit) => {
    const ctrl = new AbortController();
    void (async () => {
      let attempt = 0;
      while (!ctrl.signal.aborted) {
        try {
          const resp = await fetchImpl(baseUrl + "/event/global", {
            method: "GET",
            headers: { Accept: "text/event-stream" },
            signal: ctrl.signal,
          });
          if (!resp.ok || resp.body === null) {
            throw new Error(`SSE open failed: ${resp.status}`);
          }
          attempt = 0;
          await readSSEStream(resp.body, emit, ctrl.signal);
        } catch (e) {
          if (ctrl.signal.aborted) {
            void emit.end();
            return;
          }
          // Backoff before reconnect.
          await sleep(Math.min(8000, 250 * Math.pow(2, attempt++)));
        }
      }
      void emit.end();
    })();
    return Effect.sync(() => ctrl.abort());
  });
}

async function readSSEStream(
  body: ReadableStream<Uint8Array>,
  emit: StreamEmit.Emit<never, RPCError, AgentEventWire, void>,
  signal: AbortSignal,
): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buf = "";
  while (!signal.aborted) {
    const { value, done } = await reader.read();
    if (done) return;
    buf += decoder.decode(value, { stream: true });
    let nlIdx = buf.indexOf("\n\n");
    while (nlIdx !== -1) {
      const frame = buf.slice(0, nlIdx);
      buf = buf.slice(nlIdx + 2);
      dispatchFrame(frame, emit);
      nlIdx = buf.indexOf("\n\n");
    }
  }
}

function dispatchFrame(
  frame: string,
  emit: StreamEmit.Emit<never, RPCError, AgentEventWire, void>,
): void {
  const dataLines: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith(":")) continue;
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).replace(/^ /, ""));
    }
  }
  if (dataLines.length === 0) return;
  const payload = dataLines.join("\n");
  let raw: unknown;
  try {
    raw = JSON.parse(payload);
  } catch {
    // Drop malformed JSON silently — would surface as a parse error
    // if we wanted to fail the stream; for v8.0 alpha we tolerate.
    return;
  }
  const decoded = decodeAgentEvent(raw);
  if (Either.isRight(decoded)) {
    void emit.single(decoded.right);
  }
  // Unknown / invalid events: drop silently. The route layer may want
  // a notification channel for this in v8.1+.
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

// ---- public Layer ----

export const TransportLive: Layer.Layer<TransportService, never, TransportConfigService> =
  Layer.effect(TransportService, makeLive(fetch));

// TransportTest accepts an injected fetch — used in unit tests.
export const transportLayerWithFetch = (
  fetchImpl: typeof fetch,
): Layer.Layer<TransportService, never, TransportConfigService> =>
  Layer.effect(TransportService, makeLive(fetchImpl));
