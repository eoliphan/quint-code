import { describe, expect, test } from "bun:test";
import { getJSON, postJSON, subscribeSSE } from "./transport.js";
import { RPCError } from "./types.js";

// fakeFetch builds a deterministic fetch that returns scripted Response
// objects in sequence. Each call consumes the next entry; out-of-bounds
// calls throw.
function fakeFetch(responses: Array<Response | Error>): typeof fetch {
  let i = 0;
  const fn = async (_url: RequestInfo | URL, _init?: RequestInit): Promise<Response> => {
    if (i >= responses.length) throw new Error("fakeFetch: out of scripted responses");
    const r = responses[i++];
    if (!r) throw new Error("fakeFetch: undefined response slot");
    if (r instanceof Error) throw r;
    return r;
  };
  // Bun's typeof fetch requires a preconnect property; stub it for type compat.
  (fn as unknown as { preconnect: () => void }).preconnect = (): void => {};
  return fn as unknown as typeof fetch;
}

describe("postJSON", () => {
  test("ok response decodes JSON body", async () => {
    const fetch = fakeFetch([new Response(JSON.stringify({ ok: true }), { status: 200 })]);
    const result = await postJSON({ baseUrl: "http://x", fetch }, "/p", {});
    expect(result).toEqual({ ok: true });
  });

  test("4xx bubbles immediately with no retry", async () => {
    const fetch = fakeFetch([new Response("validation failed", { status: 400 })]);
    try {
      await postJSON({ baseUrl: "http://x", fetch, maxAttempts: 3 }, "/p", {});
      throw new Error("expected throw");
    } catch (e) {
      expect(e).toBeInstanceOf(RPCError);
      expect((e as RPCError).kind).toBe("bad_request");
    }
  });

  test("5xx retries up to maxAttempts then throws", async () => {
    const responses = [
      new Response("temporary", { status: 503 }),
      new Response("temporary", { status: 503 }),
      new Response("temporary", { status: 503 }),
    ];
    const fetch = fakeFetch(responses);
    try {
      await postJSON({ baseUrl: "http://x", fetch, maxAttempts: 3 }, "/p", {});
      throw new Error("expected throw");
    } catch (e) {
      expect(e).toBeInstanceOf(RPCError);
      expect((e as RPCError).kind).toBe("server_error");
    }
  });

  test("5xx then 2xx succeeds via retry", async () => {
    const fetch = fakeFetch([
      new Response("temporary", { status: 503 }),
      new Response(JSON.stringify({ ok: true }), { status: 200 }),
    ]);
    const result = await postJSON({ baseUrl: "http://x", fetch, maxAttempts: 3 }, "/p", {});
    expect(result).toEqual({ ok: true });
  });
});

describe("getJSON", () => {
  test("ok response decodes JSON body", async () => {
    const fetch = fakeFetch([new Response(JSON.stringify({ provider: "none" }), { status: 200 })]);
    const result = await getJSON({ baseUrl: "http://x", fetch }, "/auth/status");
    expect(result).toEqual({ provider: "none" });
  });
});

describe("subscribeSSE", () => {
  test("decodes one event per frame", async () => {
    const sseBody = `:connected\n\ndata: {"kind":"session.created","session_id":"s1","at":"2026-05-13T00:00:00Z","project_id":"p","title":"x","model":{"provider":"none","model":""}}\n\n`;
    const fetch = fakeFetch([new Response(sseBody, { status: 200 })]);
    const events: unknown[] = [];
    const controller = new AbortController();
    let closed = false;
    const subscribePromise = subscribeSSE({
      url: "http://x/event/global",
      decode: (raw) => ({ ok: true, event: raw }),
      onEvent: (r) => {
        if (r.ok) events.push(r.event);
        if (events.length === 1) {
          controller.abort();
          closed = true;
        }
      },
      signal: controller.signal,
      fetch,
    });
    await subscribePromise;
    expect(events.length).toBe(1);
    expect(closed).toBe(true);
  });

  test("malformed JSON in data line surfaces decode-failure", async () => {
    const sseBody = `data: not json\n\n`;
    const fetch = fakeFetch([new Response(sseBody, { status: 200 })]);
    const results: Array<{ ok: boolean; reason?: string }> = [];
    const controller = new AbortController();
    const subscribePromise = subscribeSSE({
      url: "http://x/event/global",
      decode: (raw) => ({ ok: true, event: raw }),
      onEvent: (r) => {
        results.push(r);
        if (results.length === 1) controller.abort();
      },
      signal: controller.signal,
      fetch,
    });
    await subscribePromise;
    expect(results.length).toBe(1);
    const first = results[0];
    expect(first).toBeDefined();
    expect(first?.ok).toBe(false);
    expect(first?.reason).toBe("malformed");
  });
});
