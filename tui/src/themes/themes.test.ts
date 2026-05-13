import { describe, expect, test } from "bun:test";
import { THEMES, THEME_NAMES, cycleTheme, currentTheme, setTheme } from "./index.js";
import { THEME_TOKEN_KEYS, type Theme } from "./types.js";

describe("themes", () => {
  test("ships exactly three themes per v8.0 plan", () => {
    expect(THEME_NAMES.length).toBe(3);
    expect(THEME_NAMES).toContain("dark");
    expect(THEME_NAMES).toContain("light");
    expect(THEME_NAMES).toContain("high-contrast");
  });

  test("every theme implements every token key", () => {
    for (const name of THEME_NAMES) {
      const theme = THEMES[name] as Theme;
      for (const key of THEME_TOKEN_KEYS) {
        const value = theme[key];
        expect(typeof value).toBe("string");
        expect(value.startsWith("#")).toBe(true);
        expect(value.length).toBeGreaterThanOrEqual(4);
      }
    }
  });

  test("setTheme + currentTheme round-trip", () => {
    setTheme("light");
    expect(currentTheme().name).toBe("light");
    setTheme("dark");
    expect(currentTheme().name).toBe("dark");
  });

  test("cycleTheme advances dark → light → high-contrast → dark", () => {
    setTheme("dark");
    expect(cycleTheme()).toBe("light");
    expect(cycleTheme()).toBe("high-contrast");
    expect(cycleTheme()).toBe("dark");
  });

  test("setTheme rejects unknown name", () => {
    expect(() => setTheme("synthwave" as never)).toThrow();
  });
});
