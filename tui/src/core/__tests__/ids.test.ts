// L1 tests — branded ID smart constructors.

import { describe, expect, test } from "bun:test";
import { Result } from "../algebra/index.js";
import { sessionID, turnID, partID, permissionID, subAgentID } from "../domain/ids.js";

describe("ID smart constructors", () => {
  test("accept non-empty strings", () => {
    expect(Result.isOk(sessionID("s1"))).toBe(true);
    expect(Result.isOk(turnID("t1"))).toBe(true);
    expect(Result.isOk(partID("p1"))).toBe(true);
    expect(Result.isOk(permissionID("pm1"))).toBe(true);
    expect(Result.isOk(subAgentID("sa1"))).toBe(true);
  });

  test("reject empty strings", () => {
    const r = sessionID("");
    expect(Result.isErr(r)).toBe(true);
    if (Result.isErr(r)) {
      expect(r.error.kind).toBe("id_empty");
      expect(r.error.tag).toBe("SessionID");
    }
  });

  test("reject pathologically long strings", () => {
    const longStr = "x".repeat(257);
    const r = partID(longStr);
    expect(Result.isErr(r)).toBe(true);
    if (Result.isErr(r)) expect(r.error.kind).toBe("id_too_long");
  });
});
