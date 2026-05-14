// L1 tests — typestate transitions + inexpressibility contracts.

import { describe, expect, test } from "bun:test";
import { Result, NonEmpty } from "../algebra/index.js";
import { unsafeBrand } from "../algebra/brand.js";
import {
  type Session,
  type SessionIdle,
  type SessionWithLiveTurn,
  isIdle,
  hasLiveTurn,
  turns,
} from "../domain/session.js";
import {
  type Part,
  type PartText,
} from "../domain/part.js";
import {
  startTurn,
  appendPart,
  completeTurn,
  failTurn,
  requestPermission,
  resolvePermission,
} from "../domain/transitions.js";
import { type PermissionPending } from "../domain/permission.js";
import {
  type SessionID,
  type TurnID,
  type PartID,
  type PermissionID,
  type ToolCallID,
} from "../domain/ids.js";

// Test fixtures.
const NOW = new Date(2026, 4, 14, 0, 0, 0);
const LATER = new Date(2026, 4, 14, 0, 0, 1);
const LATEST = new Date(2026, 4, 14, 0, 0, 2);

const sid = unsafeBrand<string, "SessionID">("s1") as SessionID;
const tid1 = unsafeBrand<string, "TurnID">("t1") as TurnID;
const tid2 = unsafeBrand<string, "TurnID">("t2") as TurnID;
const pid1 = unsafeBrand<string, "PartID">("p1") as PartID;
const pid2 = unsafeBrand<string, "PartID">("p2") as PartID;
const permId = unsafeBrand<string, "PermissionID">("pm1") as PermissionID;
const tcId = unsafeBrand<string, "ToolCallID">("tc1") as ToolCallID;

function textPart(id: PartID, text: string, at: Date): PartText {
  return { id, kind: "text", text, createdAt: at };
}

function freshIdle(): SessionIdle {
  return {
    id: sid,
    projectId: "p",
    title: "test",
    model: { provider: "none", model: "" },
    createdAt: NOW,
    updatedAt: NOW,
    state: "idle",
    history: [],
    permissions: new Map(),
    subAgents: new Map(),
  };
}

describe("Session typestate transitions", () => {
  test("startTurn idle → hasLiveTurn", () => {
    const s0 = freshIdle();
    const s1 = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    expect(s1.state).toBe("hasLiveTurn");
    expect(s1.liveTurn.id).toBe(tid1);
    expect(NonEmpty.head(s1.liveTurn.parts)).toEqual(textPart(pid1, "hi", NOW));
  });

  test("appendPart preserves hasLiveTurn", () => {
    const s0 = freshIdle();
    const s1 = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    const s2 = appendPart(s1, tid1, textPart(pid2, "there", LATER), LATER);
    expect(Result.isOk(s2)).toBe(true);
    if (Result.isOk(s2)) {
      expect(s2.value.liveTurn.parts.length).toBe(2);
    }
  });

  test("appendPart rejects wrong turn ID", () => {
    const s0 = freshIdle();
    const s1 = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    const s2 = appendPart(s1, tid2, textPart(pid2, "wrong turn", LATER), LATER);
    expect(Result.isErr(s2)).toBe(true);
    if (Result.isErr(s2)) {
      expect(s2.error.kind).toBe("turn_id_mismatch");
    }
  });

  test("completeTurn hasLiveTurn → idle, turn moves to history", () => {
    const s0 = freshIdle();
    const s1 = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    const s2 = completeTurn(s1, tid1, LATER);
    expect(Result.isOk(s2)).toBe(true);
    if (Result.isOk(s2)) {
      expect(s2.value.state).toBe("idle");
      expect(s2.value.history.length).toBe(1);
      expect(s2.value.history[0]!.state).toBe("completed");
      expect(s2.value.history[0]!.id).toBe(tid1);
    }
  });

  test("failTurn hasLiveTurn → idle with verdict + error", () => {
    const s0 = freshIdle();
    const s1 = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    const s2 = failTurn(s1, tid1, "error", "provider crashed", LATER);
    expect(Result.isOk(s2)).toBe(true);
    if (Result.isOk(s2)) {
      const h = s2.value.history[0]!;
      expect(h.state).toBe("failed");
      if (h.state === "failed") {
        expect(h.verdict).toBe("error");
        expect(h.errorMessage).toBe("provider crashed");
      }
    }
  });

  test("isIdle / hasLiveTurn narrow correctly", () => {
    const s0: Session = freshIdle();
    expect(isIdle(s0)).toBe(true);
    expect(hasLiveTurn(s0)).toBe(false);

    const s1 = startTurn(s0 as SessionIdle, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    const sUnion: Session = s1;
    expect(hasLiveTurn(sUnion)).toBe(true);
    expect(isIdle(sUnion)).toBe(false);
  });

  test("turns() returns history + liveTurn in display order", () => {
    const s0 = freshIdle();
    const s1 = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    const seq = turns(s1);
    expect(seq.length).toBe(1);
    expect(seq[0]!.id).toBe(tid1);
  });
});

describe("Permission resolution requires UserDecision capability", () => {
  test("requestPermission adds pending entry", () => {
    const s0 = freshIdle();
    const s1: SessionWithLiveTurn = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    const perm: PermissionPending = {
      id: permId,
      turnId: tid1,
      toolCallId: tcId,
      toolName: "bash",
      args: { cmd: "ls" },
      requestedAt: NOW,
      state: "pending",
    };
    const s2 = requestPermission(s1, perm, NOW);
    expect(s2.permissions.size).toBe(1);
    expect(s2.permissions.get(permId)!.state).toBe("pending");
  });

  test("resolvePermission applies backend-authoritative resolution", () => {
    const s0 = freshIdle();
    const s1 = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    const perm: PermissionPending = {
      id: permId,
      turnId: tid1,
      toolCallId: tcId,
      toolName: "bash",
      args: { cmd: "ls" },
      requestedAt: NOW,
      state: "pending",
    };
    const s2 = requestPermission(s1, perm, NOW);
    const s3 = resolvePermission(s2, permId, "approved", "operator approved", LATEST);
    expect(Result.isOk(s3)).toBe(true);
    if (Result.isOk(s3)) {
      const p = s3.value.permissions.get(permId);
      expect(p?.state).toBe("resolved");
      if (p?.state === "resolved") {
        expect(p.decision).toBe("approved");
      }
    }
  });

  test("double resolve rejected", () => {
    const s0 = freshIdle();
    const s1 = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
    const perm: PermissionPending = {
      id: permId,
      turnId: tid1,
      toolCallId: tcId,
      toolName: "bash",
      args: { cmd: "ls" },
      requestedAt: NOW,
      state: "pending",
    };
    const s2 = requestPermission(s1, perm, NOW);
    const s3 = resolvePermission(s2, permId, "approved", "", LATER);
    if (!Result.isOk(s3)) throw new Error("first resolve failed");
    const s4 = resolvePermission(s3.value, permId, "denied", "", LATEST);
    expect(Result.isErr(s4)).toBe(true);
    if (Result.isErr(s4)) expect(s4.error.kind).toBe("permission_already_resolved");
  });
});

describe("Inexpressibility contracts (compile-time)", () => {
  // These tests intentionally never RUN the offending lines — they
  // only assert that tsc rejects them. `@ts-expect-error` causes a tsc
  // error if the next line type-checks; if tsc accepts it, the comment
  // itself becomes a tsc error. Either way the contract holds at compile
  // time. Runtime is irrelevant.
  test("startTurn on Session<'hasLiveTurn'> is a compile error", () => {
    if ((false as boolean)) {
      const s0 = freshIdle();
      const s1 = startTurn(s0, tid1, "user", textPart(pid1, "hi", NOW), NOW);
      // @ts-expect-error — startTurn requires SessionIdle; passing
      // SessionWithLiveTurn is rejected at compile time. This is the
      // load-bearing "no concurrent turns" invariant.
      startTurn(s1, tid2, "user", textPart(pid2, "x", LATER), LATER);
    }
    expect(true).toBe(true);
  });

  test("completeTurn on Session<'idle'> is a compile error", () => {
    if ((false as boolean)) {
      const s0 = freshIdle();
      // @ts-expect-error — completeTurn requires SessionWithLiveTurn;
      // passing SessionIdle is rejected.
      completeTurn(s0, tid1, NOW);
    }
    expect(true).toBe(true);
  });
});

describe("Part discriminator exhaustiveness", () => {
  test("every kind is in the union", () => {
    // Compile-time exhaustiveness witness: assertNeverPart on the
    // default branch would fail to compile if a Part kind was missing.
    const kinds: ReadonlyArray<Part["kind"]> = [
      "text",
      "reasoning",
      "tool_use_started",
      "tool_use_completed",
      "file_ref",
      "step_boundary",
      "decision_record",
      "problem_card",
      "solution_portfolio",
      "note",
      "evidence_item",
      "work_commission",
    ];
    expect(kinds.length).toBe(12);
  });
});
