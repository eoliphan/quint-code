// L2 tests — Schema round-trips against representative fixtures.
//
// Fixtures mirror the JSON the Go agentserver emits over /event/global.
// Schema decode must accept valid payloads, reject malformed ones with
// a typed ParseError, and reject unknown kinds.

import { describe, expect, test } from "bun:test";
import { Either } from "effect";
import { decodeAgentEvent } from "../wire/events.js";
import { decodeAuthStatus } from "../wire/auth-status.js";
import {
  decodeTurnSubmitResponse,
  decodeHealthzResponse,
} from "../wire/commands.js";

describe("decodeAgentEvent", () => {
  test("decodes session.created", () => {
    const raw = {
      kind: "session.created",
      session_id: "s1",
      at: "2026-05-14T00:00:00Z",
      project_id: "p1",
      title: "test",
      model: { provider: "anthropic", model: "claude-opus-4-7" },
    };
    const r = decodeAgentEvent(raw);
    expect(Either.isRight(r)).toBe(true);
    if (Either.isRight(r) && r.right.kind === "session.created") {
      expect(r.right.title).toBe("test");
    }
  });

  test("decodes part.text.completed", () => {
    const raw = {
      kind: "part.text.completed",
      session_id: "s1",
      turn_id: "t1",
      at: "2026-05-14T00:00:01Z",
      part_id: "p1",
      text: "hello world",
    };
    const r = decodeAgentEvent(raw);
    expect(Either.isRight(r)).toBe(true);
  });

  test("decodes permission.requested with unknown args payload", () => {
    const raw = {
      kind: "permission.requested",
      session_id: "s1",
      turn_id: "t1",
      at: "2026-05-14T00:00:02Z",
      permission_id: "pm1",
      tool_call_id: "tc1",
      tool_name: "bash",
      args: { cmd: "ls" },
    };
    const r = decodeAgentEvent(raw);
    expect(Either.isRight(r)).toBe(true);
  });

  test("rejects unknown kind", () => {
    const raw = { kind: "future.kind.v9", session_id: "s1", at: "2026-05-14T00:00:00Z" };
    const r = decodeAgentEvent(raw);
    expect(Either.isLeft(r)).toBe(true);
  });

  test("rejects missing required field", () => {
    const raw = {
      kind: "session.created",
      session_id: "s1",
      at: "2026-05-14T00:00:00Z",
      // project_id missing
      title: "x",
      model: { provider: "none", model: "" },
    };
    const r = decodeAgentEvent(raw);
    expect(Either.isLeft(r)).toBe(true);
  });

  test("rejects wrong type for verdict literal", () => {
    const raw = {
      kind: "turn.completed",
      session_id: "s1",
      turn_id: "t1",
      at: "2026-05-14T00:00:00Z",
      verdict: "maybe",
    };
    const r = decodeAgentEvent(raw);
    expect(Either.isLeft(r)).toBe(true);
  });
});

describe("decodeAuthStatus", () => {
  test("decodes happy payload", () => {
    const r = decodeAuthStatus({
      provider: "anthropic",
      model: "claude-opus-4-7",
      has_credentials: true,
      expires_at: "2026-08-13T00:00:00Z",
    });
    expect(Either.isRight(r)).toBe(true);
  });

  test("expires_at is optional", () => {
    const r = decodeAuthStatus({ provider: "none", model: "", has_credentials: false });
    expect(Either.isRight(r)).toBe(true);
  });

  test("rejects non-boolean has_credentials", () => {
    const r = decodeAuthStatus({ provider: "x", model: "", has_credentials: "yes" });
    expect(Either.isLeft(r)).toBe(true);
  });
});

describe("RPC response decoders", () => {
  test("turn.submit response", () => {
    const r = decodeTurnSubmitResponse({ turn_id: "t1" });
    expect(Either.isRight(r)).toBe(true);
  });

  test("healthz response", () => {
    const r = decodeHealthzResponse({ ok: true, subscribers: 1 });
    expect(Either.isRight(r)).toBe(true);
  });
});
