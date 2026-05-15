// L4 tests — reducer replays a representative event stream.

import { describe, expect, test } from "bun:test";
import { Result } from "../algebra/index.js";
import { reduce, replay } from "../reducer/reduce.js";
import { type AgentEventWire } from "../wire/events.js";
import { hasLiveTurn, isIdle } from "../domain/session.js";

const T0 = new Date(2026, 4, 14, 0, 0, 0);
const T1 = new Date(2026, 4, 14, 0, 0, 1);
const T2 = new Date(2026, 4, 14, 0, 0, 2);
const T3 = new Date(2026, 4, 14, 0, 0, 3);
const T4 = new Date(2026, 4, 14, 0, 0, 4);

function sessionCreated(): AgentEventWire {
  return {
    kind: "session.created",
    session_id: "s1",
    at: T0.toISOString(),
    project_id: "p1",
    title: "test",
    model: { provider: "anthropic", model: "claude-opus-4-7" },
  };
}

function turnStarted(turnId: string, text: string, at: Date): AgentEventWire {
  return {
    kind: "turn.started",
    session_id: "s1",
    turn_id: turnId,
    at: at.toISOString(),
    role: "user",
    first_part: text,
  };
}

function textCompleted(turnId: string, partId: string, text: string, at: Date): AgentEventWire {
  return {
    kind: "part.text.completed",
    session_id: "s1",
    turn_id: turnId,
    at: at.toISOString(),
    part_id: partId,
    text,
  };
}

function turnCompleted(turnId: string, at: Date): AgentEventWire {
  return {
    kind: "turn.completed",
    session_id: "s1",
    turn_id: turnId,
    at: at.toISOString(),
    verdict: "pass",
  };
}

describe("reduce — session lifecycle", () => {
  test("session.created bootstraps a fresh SessionIdle", () => {
    const r = reduce(undefined, sessionCreated(), T0);
    expect(Result.isOk(r)).toBe(true);
    if (Result.isOk(r)) {
      expect(r.value.state).toBe("idle");
      expect(r.value.title).toBe("test");
      expect(r.value.model.provider).toBe("anthropic");
    }
  });

  test("turn.started transitions idle → hasLiveTurn", () => {
    const s1 = reduce(undefined, sessionCreated(), T0);
    if (!s1.ok) throw new Error("bootstrap failed");
    const s2 = reduce(s1.value, turnStarted("t1", "hello", T1), T1);
    expect(Result.isOk(s2)).toBe(true);
    if (Result.isOk(s2)) {
      expect(hasLiveTurn(s2.value)).toBe(true);
    }
  });

  test("turn.completed transitions hasLiveTurn → idle, turn in history", () => {
    const events: ReadonlyArray<AgentEventWire> = [
      sessionCreated(),
      turnStarted("t1", "hello", T1),
      textCompleted("t1", "p1", "world", T2),
      turnCompleted("t1", T3),
    ];
    const r = replay(events, (() => {
      const stamps = [T0, T1, T2, T3];
      let i = 0;
      return () => stamps[i++]!;
    })());
    expect(Result.isOk(r)).toBe(true);
    if (Result.isOk(r) && r.value !== undefined) {
      expect(isIdle(r.value)).toBe(true);
      expect(r.value.history.length).toBe(1);
      expect(r.value.history[0]!.state).toBe("completed");
      expect(r.value.history[0]!.parts.length).toBe(2); // firstPart + appended text
    }
  });

  test("permission.requested + permission.resolved round-trip", () => {
    const events: ReadonlyArray<AgentEventWire> = [
      sessionCreated(),
      turnStarted("t1", "do thing", T1),
      {
        kind: "permission.requested",
        session_id: "s1",
        turn_id: "t1",
        at: T2.toISOString(),
        permission_id: "pm1",
        tool_call_id: "tc1",
        tool_name: "bash",
        args: { cmd: "rm -rf /" },
      },
      {
        kind: "permission.resolved",
        session_id: "s1",
        turn_id: "t1",
        at: T3.toISOString(),
        permission_id: "pm1",
        decision: "denied",
        reason: "operator rejected destructive cmd",
      },
      turnCompleted("t1", T4),
    ];
    const r = replay(events, (() => {
      const stamps = [T0, T1, T2, T3, T4];
      let i = 0;
      return () => stamps[i++]!;
    })());
    expect(Result.isOk(r)).toBe(true);
    if (Result.isOk(r) && r.value !== undefined) {
      const perm = r.value.permissions.get("pm1");
      expect(perm?.state).toBe("resolved");
      if (perm?.state === "resolved") {
        expect(perm.decision).toBe("denied");
        expect(perm.reason).toBe("operator rejected destructive cmd");
      }
    }
  });

  test("subagent.spawned + subagent.completed round-trip", () => {
    const events: ReadonlyArray<AgentEventWire> = [
      sessionCreated(),
      turnStarted("t1", "spawn explorer", T1),
      {
        kind: "subagent.spawned",
        session_id: "s1",
        turn_id: "t1",
        at: T2.toISOString(),
        subagent_id: "sa1",
        child_session: "s1-child",
        prompt: "map the codebase",
      },
      {
        kind: "subagent.completed",
        session_id: "s1",
        turn_id: "t1",
        at: T3.toISOString(),
        subagent_id: "sa1",
        verdict: "pass",
      },
    ];
    const r = replay(events, (() => {
      const stamps = [T0, T1, T2, T3];
      let i = 0;
      return () => stamps[i++]!;
    })());
    expect(Result.isOk(r)).toBe(true);
    if (Result.isOk(r) && r.value !== undefined) {
      const link = r.value.subAgents.get("sa1");
      expect(link?.state).toBe("resolved");
      if (link?.state === "resolved") {
        expect(link.verdict).toBe("pass");
      }
    }
  });

  test("event for non-initialized session returns session_not_initialized", () => {
    const r = reduce(undefined, turnStarted("t1", "x", T1), T1);
    expect(Result.isErr(r)).toBe(true);
    if (Result.isErr(r)) {
      expect(r.error.kind).toBe("session_not_initialized");
    }
  });

  test("turn.completed without live turn returns wrong_session_state", () => {
    const s1 = reduce(undefined, sessionCreated(), T0);
    if (!s1.ok) throw new Error("bootstrap");
    const r = reduce(s1.value, turnCompleted("t-nonexistent", T1), T1);
    expect(Result.isErr(r)).toBe(true);
    if (Result.isErr(r)) {
      expect(r.error.kind).toBe("wrong_session_state");
    }
  });

  test("text deltas upsert a streaming text part", () => {
    const s1 = reduce(undefined, sessionCreated(), T0);
    if (!s1.ok) throw new Error("bootstrap");
    const s2 = reduce(s1.value, turnStarted("t1", "hi", T1), T1);
    if (!s2.ok) throw new Error("turn started");
    const delta1: AgentEventWire = {
      kind: "part.text.delta",
      session_id: "s1",
      turn_id: "t1",
      at: T2.toISOString(),
      part_id: "p1",
      delta: "hello",
    };
    const s3 = reduce(s2.value, delta1, T2);
    expect(Result.isOk(s3)).toBe(true);
    if (!Result.isOk(s3) || !hasLiveTurn(s3.value)) throw new Error("s3 invariant");
    // First delta creates a brand-new streaming text part next to the
    // user's input part.
    expect(s3.value.liveTurn.parts.length).toBe(2);
    expect(s3.value.liveTurn.parts[1]?.kind).toBe("text");
    expect((s3.value.liveTurn.parts[1] as { text: string }).text).toBe("hello");

    // Second delta with the same part_id appends to the same part —
    // length stays at 2.
    const delta2: AgentEventWire = { ...delta1, delta: " world" };
    const s4 = reduce(s3.value, delta2, T2);
    expect(Result.isOk(s4)).toBe(true);
    if (!Result.isOk(s4) || !hasLiveTurn(s4.value)) throw new Error("s4 invariant");
    expect(s4.value.liveTurn.parts.length).toBe(2);
    expect((s4.value.liveTurn.parts[1] as { text: string }).text).toBe("hello world");
  });
});
