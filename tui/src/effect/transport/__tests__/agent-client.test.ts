// L3 tests — agent-client surface invariants.
//
// The load-bearing assertion: permissionRespond requires a
// UserDecision. The @ts-expect-error contract proves no code path can
// auto-approve.

import { describe, expect, test } from "bun:test";
import { Effect, Layer } from "effect";
import * as Config from "../config.js";
import { AgentClientLive, AgentClientService } from "../agent-client.js";
import { transportLayerWithFetch } from "../service.js";
import { fromKeystroke } from "../../../core/user-action/user-decision.js";
import { unsafeBrand } from "../../../core/algebra/brand.js";
import type { PermissionID } from "../../../core/domain/ids.js";

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

function makeLayers(fetchImpl: typeof fetch): Layer.Layer<AgentClientService> {
  return Layer.provide(
    AgentClientLive,
    Layer.provide(
      transportLayerWithFetch(fetchImpl),
      Config.layer(Config.makeConfig("http://localhost:9999")),
    ),
  );
}

describe("AgentClient.permissionRespond capability contract", () => {
  test("permissionRespond accepts UserDecision and POSTs decision+reason", async () => {
    const calls: Array<{ url: RequestInfo | URL; init?: RequestInit }> = [];
    const fetchImpl = ((url: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
      calls.push({ url, init });
      return Promise.resolve(new Response("", { status: 200 }));
    }) as unknown as typeof fetch;
    (fetchImpl as unknown as { preconnect: () => void }).preconnect = (): void => {};

    const permId = unsafeBrand<string, "PermissionID">("pm1") as PermissionID;
    const decision = fromKeystroke(
      { type: "keydown", key: "Enter" },
      "approved",
      "operator pressed Enter",
      new Date(),
    );

    const program = Effect.gen(function* () {
      const c = yield* AgentClientService;
      return yield* c.permissionRespond(permId, decision);
    });
    await Effect.runPromise(Effect.provide(program, makeLayers(fetchImpl)));

    expect(calls.length).toBe(1);
    expect(String(calls[0]!.url)).toContain("/permission/pm1");
    const body = JSON.parse(calls[0]!.init!.body as string);
    expect(body.decision).toBe("approved");
    expect(body.reason).toBe("operator pressed Enter");
  });

  test("permissionRespond without UserDecision is a compile error", () => {
    // Compile-time contract — block never executes at runtime.
    if ((false as boolean)) {
      const permId = unsafeBrand<string, "PermissionID">("pm1") as PermissionID;
      const program = Effect.gen(function* () {
        const c = yield* AgentClientService;
        // @ts-expect-error — permissionRespond REQUIRES UserDecision.
        // Passing a plain string is rejected. This is the load-bearing
        // "no auto-approval" capability contract.
        return yield* c.permissionRespond(permId, "approved");
      });
      void program;
    }
    expect(true).toBe(true);
  });
});

describe("AgentClient happy paths", () => {
  test("healthz", async () => {
    const fetchImpl = fakeFetch([
      new Response(JSON.stringify({ ok: true, subscribers: 1 }), { status: 200 }),
    ]);
    const program = Effect.gen(function* () {
      const c = yield* AgentClientService;
      return yield* c.healthz();
    });
    const r = await Effect.runPromise(Effect.provide(program, makeLayers(fetchImpl)));
    expect(r.ok).toBe(true);
  });

  test("turnSubmit returns branded TurnID", async () => {
    const fetchImpl = fakeFetch([
      new Response(JSON.stringify({ turn_id: "t1" }), { status: 200 }),
    ]);
    const sid = unsafeBrand<string, "SessionID">("s1") as import("../../../core/domain/ids.js").SessionID;
    const program = Effect.gen(function* () {
      const c = yield* AgentClientService;
      return yield* c.turnSubmit(sid, "hello");
    });
    const r = await Effect.runPromise(Effect.provide(program, makeLayers(fetchImpl)));
    expect(String(r.turnId)).toBe("t1");
  });
});
