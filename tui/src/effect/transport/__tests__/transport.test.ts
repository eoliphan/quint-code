// L3 tests — transport service against a scripted fetch.

import { describe, expect, test } from "bun:test";
import { Effect, Either, Layer } from "effect";
import * as Config from "../config.js";
import { TransportService, transportLayerWithFetch } from "../service.js";
import { decodeHealthzResponse } from "../../../core/wire/commands.js";

function fakeFetch(responses: ReadonlyArray<Response | Error>): typeof fetch {
  let i = 0;
  const fn = async (_url: RequestInfo | URL, _init?: RequestInit): Promise<Response> => {
    if (i >= responses.length) throw new Error("fakeFetch: exhausted");
    const r = responses[i++];
    if (!r) throw new Error("fakeFetch: undefined response");
    if (r instanceof Error) throw r;
    return r;
  };
  (fn as unknown as { preconnect: () => void }).preconnect = (): void => {};
  return fn as unknown as typeof fetch;
}

function makeLayer(fetchImpl: typeof fetch): Layer.Layer<TransportService> {
  return Layer.provide(
    transportLayerWithFetch(fetchImpl),
    Config.layer(Config.makeConfig("http://localhost:9999")),
  );
}

describe("TransportService", () => {
  test("getJson decodes a 200 response", async () => {
    const fetchImpl = fakeFetch([
      new Response(JSON.stringify({ ok: true, subscribers: 0 }), { status: 200 }),
    ]);
    const program = Effect.gen(function* () {
      const t = yield* TransportService;
      return yield* t.getJson("/healthz", decodeHealthzResponse);
    });
    const result = await Effect.runPromise(Effect.provide(program, makeLayer(fetchImpl)));
    expect(result).toEqual({ ok: true, subscribers: 0 });
  });

  test("postJson 4xx bubbles immediately as HttpStatusError with marker", async () => {
    const fetchImpl = fakeFetch([
      new Response("turn already running on session s1", { status: 409 }),
    ]);
    const program = Effect.gen(function* () {
      const t = yield* TransportService;
      return yield* t.postJson("/session/s1/turn", { text: "hi" }, () => Either.right({}));
    });
    const exit = await Effect.runPromiseExit(Effect.provide(program, makeLayer(fetchImpl)));
    expect(exit._tag).toBe("Failure");
    if (exit._tag === "Failure") {
      // Surface the typed error via Cause.
      const failure = exit.cause;
      const causeStr = JSON.stringify(failure);
      expect(causeStr).toContain("HttpStatusError");
      expect(causeStr).toContain("turn_already_running");
    }
  });

  test("getJson decode failure surfaces DecodeError", async () => {
    const fetchImpl = fakeFetch([new Response("not json", { status: 200 })]);
    const program = Effect.gen(function* () {
      const t = yield* TransportService;
      return yield* t.getJson("/healthz", decodeHealthzResponse);
    });
    const exit = await Effect.runPromiseExit(Effect.provide(program, makeLayer(fetchImpl)));
    expect(exit._tag).toBe("Failure");
    if (exit._tag === "Failure") {
      expect(JSON.stringify(exit.cause)).toContain("DecodeError");
    }
  });

  test("5xx retries up to maxAttempts then fails", async () => {
    const fetchImpl = fakeFetch([
      new Response("transient", { status: 503 }),
      new Response("transient", { status: 503 }),
      new Response("transient", { status: 503 }),
    ]);
    const program = Effect.gen(function* () {
      const t = yield* TransportService;
      return yield* t.getJson("/healthz", decodeHealthzResponse);
    });
    const exit = await Effect.runPromiseExit(Effect.provide(program, makeLayer(fetchImpl)));
    expect(exit._tag).toBe("Failure");
  });

  test("5xx then 2xx succeeds via retry", async () => {
    const fetchImpl = fakeFetch([
      new Response("transient", { status: 503 }),
      new Response(JSON.stringify({ ok: true, subscribers: 0 }), { status: 200 }),
    ]);
    const program = Effect.gen(function* () {
      const t = yield* TransportService;
      return yield* t.getJson("/healthz", decodeHealthzResponse);
    });
    const result = await Effect.runPromise(Effect.provide(program, makeLayer(fetchImpl)));
    expect(result.ok).toBe(true);
  });
});
