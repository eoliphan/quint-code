import { describe, expect, test } from "bun:test";
import { decodeAgentEvent } from "./events.js";

describe("decodeAgentEvent", () => {
  test("decodes every known kind", () => {
    const known: import("./types.js").EventKind[] = [
      "session.created",
      "session.updated",
      "session.archived",
      "turn.started",
      "turn.completed",
      "turn.failed",
      "part.text.delta",
      "part.text.completed",
      "part.reasoning.delta",
      "part.reasoning.completed",
      "part.tool_use.started",
      "part.tool_use.completed",
      "part.file_ref",
      "part.step.boundary",
      "subagent.spawned",
      "subagent.completed",
      "permission.requested",
      "permission.resolved",
      "model.switched",
      "auth.expired",
    ];
    for (const kind of known) {
      const result = decodeAgentEvent({ kind, session_id: "s", at: "2026-05-13T00:00:00Z" });
      if (!result.ok) {
        throw new Error(`expected ok for ${kind}, got ${JSON.stringify(result)}`);
      }
      expect(result.event.kind).toBe(kind);
    }
  });

  test("returns unknown_kind for unrecognised kinds", () => {
    const result = decodeAgentEvent({ kind: "future.kind.v9", session_id: "s", at: "2026-05-13T00:00:00Z" });
    expect(result.ok).toBe(false);
    if (!result.ok && result.reason === "unknown_kind") {
      expect(result.kind).toBe("future.kind.v9");
    } else {
      throw new Error(`expected unknown_kind, got ${JSON.stringify(result)}`);
    }
  });

  test("returns malformed for non-object input", () => {
    const cases: unknown[] = [null, "string", 42, [], undefined];
    for (const raw of cases) {
      const result = decodeAgentEvent(raw);
      expect(result.ok).toBe(false);
      if (!result.ok) {
        expect(result.reason).toBe("malformed");
      }
    }
  });

  test("returns malformed for missing kind", () => {
    const result = decodeAgentEvent({ session_id: "s", at: "2026-05-13T00:00:00Z" });
    expect(result.ok).toBe(false);
    if (!result.ok && result.reason === "malformed") {
      expect(result.error).toContain("missing or non-string kind");
    } else {
      throw new Error(`expected malformed, got ${JSON.stringify(result)}`);
    }
  });
});
