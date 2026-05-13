import { describe, expect, test } from "bun:test";
import { advance, frameAt, DOTS_FRAMES } from "./spinner.js";

describe("spinner", () => {
  test("advance wraps at frame length", () => {
    expect(advance(0, DOTS_FRAMES)).toBe(1);
    expect(advance(DOTS_FRAMES.length - 1, DOTS_FRAMES)).toBe(0);
  });

  test("frameAt wraps positive and negative indices", () => {
    expect(frameAt(0, DOTS_FRAMES)).toBe(DOTS_FRAMES[0]);
    expect(frameAt(DOTS_FRAMES.length, DOTS_FRAMES)).toBe(DOTS_FRAMES[0]);
    expect(frameAt(-1, DOTS_FRAMES)).toBe(DOTS_FRAMES[DOTS_FRAMES.length - 1]);
  });

  test("frameAt over empty frames returns empty string", () => {
    expect(frameAt(0, [])).toBe("");
  });
});
