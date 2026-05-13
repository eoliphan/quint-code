import { describe, expect, test } from "bun:test";
import { renderKeyHint, renderHintRow } from "./key-hint.js";

describe("key-hint", () => {
  test("renderKeyHint formats single chip", () => {
    expect(renderKeyHint({ key: "esc", action: "close" })).toBe("esc close");
  });

  test("renderHintRow joins with dot separator", () => {
    const row = renderHintRow([
      { key: "esc", action: "close" },
      { key: "enter", action: "submit" },
    ]);
    expect(row).toBe("esc close · enter submit");
  });

  test("renderHintRow on empty input returns empty", () => {
    expect(renderHintRow([])).toBe("");
  });
});
