import { describe, expect, test } from "bun:test";
import { agentErrorMapper } from "./errors.js";

describe("agentErrorMapper", () => {
  test("turn already running matches Go-side ErrTurnAlreadyRunning prefix", () => {
    const err = agentErrorMapper(409, "turn already running on session");
    expect(err.kind).toBe("conflict");
    expect(err.message.startsWith("turn_already_running:")).toBe(true);
  });

  test("turn mismatch matches Go-side ErrTurnMismatch prefix", () => {
    const err = agentErrorMapper(409, "turn id does not match running turn");
    expect(err.kind).toBe("conflict");
    expect(err.message.startsWith("turn_mismatch:")).toBe(true);
  });

  test("404 with no marker passes body verbatim", () => {
    const err = agentErrorMapper(404, "session not found: s1");
    expect(err.kind).toBe("not_found");
    expect(err.message.startsWith("session_not_found:")).toBe(true);
  });

  test("400 without marker tagged validation", () => {
    const err = agentErrorMapper(400, "bad json");
    expect(err.kind).toBe("bad_request");
    expect(err.message.startsWith("validation:")).toBe(true);
  });

  test("5xx tagged server_error", () => {
    const err = agentErrorMapper(503, "db unavailable");
    expect(err.kind).toBe("server_error");
  });
});
