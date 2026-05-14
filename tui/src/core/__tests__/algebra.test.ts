// L0 tests — basic algebra primitives.

import { describe, expect, test } from "bun:test";
import { Result, Option, NonEmpty } from "../algebra/index.js";

describe("Result", () => {
  test("ok / err / map / flatMap", () => {
    const r1 = Result.ok(1);
    const r2 = Result.map(r1, (n) => n + 1);
    expect(r2).toEqual({ ok: true, value: 2 });

    const r3 = Result.flatMap(r2, (n) => (n > 1 ? Result.ok(n * 10) : Result.err("low")));
    expect(r3).toEqual({ ok: true, value: 20 });

    const r4 = Result.flatMap(Result.ok(0), (n) => (n > 1 ? Result.ok(n) : Result.err("low" as const)));
    expect(r4).toEqual({ ok: false, error: "low" });
  });

  test("isOk / isErr narrow", () => {
    const r: Result.Result<number, string> = Result.ok(7);
    if (Result.isOk(r)) {
      // Inside this branch, TS narrows to { ok: true; value: number }.
      expect(r.value).toBe(7);
    } else {
      throw new Error("unreachable");
    }
  });
});

describe("Option", () => {
  test("some / none / map / flatMap", () => {
    const o = Option.some(5);
    expect(Option.isSome(o)).toBe(true);
    const o2 = Option.map(o, (n) => n * 2);
    if (Option.isSome(o2)) expect(o2.value).toBe(10);
  });

  test("fromNullable", () => {
    expect(Option.isNone(Option.fromNullable<number>(null))).toBe(true);
    expect(Option.isNone(Option.fromNullable<number>(undefined))).toBe(true);
    const o = Option.fromNullable<number>(0);
    expect(Option.isSome(o)).toBe(true);
    if (Option.isSome(o)) expect(o.value).toBe(0);
  });
});

describe("NonEmpty", () => {
  test("head / tail / append", () => {
    const ne = NonEmpty.nonEmpty(1, 2, 3);
    expect(NonEmpty.head(ne)).toBe(1);
    expect(NonEmpty.tail(ne)).toEqual([2, 3]);
    expect(NonEmpty.append(ne, 4)).toEqual([1, 2, 3, 4]);
  });
});
