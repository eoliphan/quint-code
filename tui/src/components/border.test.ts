import { describe, expect, test } from "bun:test";
import { EmptyBorder, SplitBorder } from "./border.js";

describe("border", () => {
  test("EmptyBorder has all keys", () => {
    expect(EmptyBorder.horizontal).toBe(" ");
    expect(EmptyBorder.vertical).toBe("");
    expect(EmptyBorder.topLeft).toBe("");
  });

  test("SplitBorder extends EmptyBorder with vertical bar", () => {
    expect(SplitBorder.customBorderChars.vertical).toBe("┃");
    expect(SplitBorder.customBorderChars.horizontal).toBe(" ");
    expect(SplitBorder.border).toEqual(["left", "right"]);
  });
});
