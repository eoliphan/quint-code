import { type DecodeResult, type ErrorMapper, RPCError, defaultErrorMapper } from "./types.js";

/** Event-decoder contract injected by each surface. Receives the raw
 *  parsed JSON line; returns DecodeResult. Implementations switch on
 *  the discriminator key the surface chose (agentproto uses "kind"). */
export type EventDecoder<TEvent> = (raw: unknown) => DecodeResult<TEvent>;

/** SSE consumer config. */
export interface SSEConfig<TEvent> {
  /** Full URL including any query params; transport does not augment. */
  url: string;
  /** Decoder for the wire envelope. */
  decode: EventDecoder<TEvent>;
  /** Called once per decoded or unknown event. The handler is sync; if
   *  it needs to do async work it should not block the next event —
   *  push to a queue and process out-of-band. */
  onEvent: (result: DecodeResult<TEvent>) => void;
  /** Called when the connection closes (graceful or error). The reader
   *  invokes the reconnect policy on its own — onClose is a hook for
   *  surface bookkeeping (UI banner, telemetry). */
  onClose?: (error: Error | undefined) => void;
  /** AbortSignal used to cancel the SSE read. Set on the SDK client at
   *  construction time so a single cancel tears down all transport.  */
  signal: AbortSignal;
  /** Custom fetch — injected for tests. Defaults to global fetch. */
  fetch?: typeof fetch;
  /** Reconnect backoff in ms. Default exponential 250 → 8000. */
  backoffMs?: (attempt: number) => number;
}

const defaultBackoff = (attempt: number): number => {
  const ms = 250 * Math.pow(2, attempt);
  return ms > 8000 ? 8000 : ms;
};

/** subscribeSSE reads a server-sent-events stream until the signal
 *  aborts or the server closes. On non-abort close the reader
 *  reconnects with exponential backoff. Events arrive at onEvent in
 *  arrival order. */
export async function subscribeSSE<TEvent>(cfg: SSEConfig<TEvent>): Promise<void> {
  const doFetch = cfg.fetch ?? fetch;
  const backoff = cfg.backoffMs ?? defaultBackoff;
  let attempt = 0;

  while (!cfg.signal.aborted) {
    try {
      const resp = await doFetch(cfg.url, {
        method: "GET",
        headers: { Accept: "text/event-stream" },
        signal: cfg.signal,
      });
      if (!resp.ok || !resp.body) {
        throw new Error(`SSE open: status ${resp.status}`);
      }
      attempt = 0; // reset backoff after a successful open
      await readSSEStream(resp.body, cfg);
      cfg.onClose?.(undefined);
    } catch (err) {
      if (cfg.signal.aborted) {
        cfg.onClose?.(undefined);
        return;
      }
      cfg.onClose?.(err as Error);
      const delay = backoff(attempt++);
      await sleep(delay, cfg.signal);
    }
  }
}

async function readSSEStream<TEvent>(body: ReadableStream<Uint8Array>, cfg: SSEConfig<TEvent>): Promise<void> {
  const reader = body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buf = "";
  while (!cfg.signal.aborted) {
    const { value, done } = await reader.read();
    if (done) return;
    buf += decoder.decode(value, { stream: true });
    let nlIdx = buf.indexOf("\n\n");
    while (nlIdx !== -1) {
      const frame = buf.slice(0, nlIdx);
      buf = buf.slice(nlIdx + 2);
      dispatchSSEFrame(frame, cfg);
      nlIdx = buf.indexOf("\n\n");
    }
  }
}

function dispatchSSEFrame<TEvent>(frame: string, cfg: SSEConfig<TEvent>): void {
  // Each frame is one or more lines. We only care about `data:` lines
  // — agentproto encodes one JSON event per frame, but the spec allows
  // multi-line data; concatenate per RFC.
  const dataLines: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith(":")) continue; // comment frame (e.g. ":connected")
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).replace(/^ /, ""));
    }
  }
  if (dataLines.length === 0) return;
  const payload = dataLines.join("\n");
  let raw: unknown;
  try {
    raw = JSON.parse(payload);
  } catch (e) {
    cfg.onEvent({ ok: false, reason: "malformed", error: (e as Error).message, raw: payload });
    return;
  }
  cfg.onEvent(cfg.decode(raw));
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const t = setTimeout(() => {
      signal.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    const onAbort = (): void => {
      clearTimeout(t);
      resolve();
    };
    signal.addEventListener("abort", onAbort, { once: true });
  });
}

/** RPC poster config. */
export interface RPCConfig {
  baseUrl: string;
  errorMapper?: ErrorMapper;
  fetch?: typeof fetch;
  /** Retry on 5xx + network errors. Default 3 attempts. */
  maxAttempts?: number;
}

/** postJSON posts a body and decodes the JSON response. Non-2xx maps
 *  through errorMapper into a typed RPCError. Retries 5xx + network
 *  failures; 4xx responses bubble immediately (caller fault). */
export async function postJSON<TBody, TResponse>(
  cfg: RPCConfig,
  path: string,
  body: TBody,
): Promise<TResponse> {
  const doFetch = cfg.fetch ?? fetch;
  const mapErr = cfg.errorMapper ?? defaultErrorMapper;
  const max = cfg.maxAttempts ?? 3;
  let lastErr: Error | undefined;
  for (let attempt = 0; attempt < max; attempt++) {
    try {
      const resp = await doFetch(cfg.baseUrl + path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!resp.ok) {
        const text = await resp.text();
        const err = mapErr(resp.status, text);
        // 4xx: caller fault, no retry.
        if (resp.status >= 400 && resp.status < 500) throw err;
        lastErr = err;
        continue;
      }
      const text = await resp.text();
      if (text === "") return undefined as TResponse;
      try {
        return JSON.parse(text) as TResponse;
      } catch (e) {
        throw new RPCError("decode", resp.status, text, `response decode: ${(e as Error).message}`);
      }
    } catch (e) {
      if (e instanceof RPCError && e.kind !== "server_error") throw e;
      lastErr = e as Error;
    }
  }
  throw lastErr ?? new RPCError("network", 0, "", "exhausted retries with no recorded error");
}

/** getJSON is a thin GET sibling of postJSON with the same retry +
 *  error-mapping semantics. */
export async function getJSON<TResponse>(cfg: RPCConfig, path: string): Promise<TResponse> {
  const doFetch = cfg.fetch ?? fetch;
  const mapErr = cfg.errorMapper ?? defaultErrorMapper;
  const max = cfg.maxAttempts ?? 3;
  let lastErr: Error | undefined;
  for (let attempt = 0; attempt < max; attempt++) {
    try {
      const resp = await doFetch(cfg.baseUrl + path);
      if (!resp.ok) {
        const text = await resp.text();
        const err = mapErr(resp.status, text);
        if (resp.status >= 400 && resp.status < 500) throw err;
        lastErr = err;
        continue;
      }
      const text = await resp.text();
      try {
        return JSON.parse(text) as TResponse;
      } catch (e) {
        throw new RPCError("decode", resp.status, text, `response decode: ${(e as Error).message}`);
      }
    } catch (e) {
      if (e instanceof RPCError && e.kind !== "server_error") throw e;
      lastErr = e as Error;
    }
  }
  throw lastErr ?? new RPCError("network", 0, "", "exhausted retries with no recorded error");
}
