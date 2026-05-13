import { createSignal, type Accessor } from "solid-js";
import { type Theme } from "./types.js";
import { darkTheme } from "./dark.js";
import { lightTheme } from "./light.js";
import { highContrastTheme } from "./high-contrast.js";

export { darkTheme, lightTheme, highContrastTheme };
export type { Theme };
export { THEME_TOKEN_KEYS } from "./types.js";

export const THEMES = {
  dark: darkTheme,
  light: lightTheme,
  "high-contrast": highContrastTheme,
} as const satisfies Record<Theme["name"], Theme>;

export type ThemeName = keyof typeof THEMES;

const [active, setActive] = createSignal<ThemeName>("dark");

/** currentTheme returns the active theme. Reactive — components can
 *  consume it directly inside Solid JSX expressions. */
export const currentTheme: Accessor<Theme> = () => THEMES[active()];

/** activeName returns just the name (for status footers / settings). */
export const activeName: Accessor<ThemeName> = active;

/** setTheme switches the active theme by name. Throws on unknown
 *  names — caller is responsible for using THEME_NAMES as the
 *  authoritative source of selectable values. */
export function setTheme(name: ThemeName): void {
  if (!(name in THEMES)) throw new Error(`theme: unknown name ${name}`);
  setActive(name);
}

/** cycleTheme advances to the next theme in declaration order (dark
 *  → light → high-contrast → dark). Useful for a single keystroke
 *  cycle binding in t12+ route footers. */
export function cycleTheme(): ThemeName {
  const names = Object.keys(THEMES) as ThemeName[];
  const cur = active();
  const idx = names.indexOf(cur);
  const next = names[(idx + 1) % names.length] ?? "dark";
  setActive(next);
  return next;
}

export const THEME_NAMES: readonly ThemeName[] = Object.keys(THEMES) as ThemeName[];
