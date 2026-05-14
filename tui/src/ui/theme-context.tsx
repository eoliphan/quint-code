// L6: ThemeContext — Solid context that surfaces the current theme
// tokens to UI primitives without prop drilling.
//
// The L5 ThemeStore is wrapped by a context provider; primitives
// (Text, Box, KeyHint, etc.) call `useTheme()` to read tokens. The
// runtime construction in L9 mounts ThemeProvider around the route
// tree with the active Theme accessor wired in.

import { createContext, useContext, type JSX } from "solid-js";
import { darkTheme, lightTheme, highContrastTheme, type Theme } from "../themes/index.js";
import type { ThemeName } from "../effect/state/theme-store.js";

const ThemeContext = createContext<() => Theme>(() => darkTheme);

export interface ThemeProviderProps {
  readonly active: () => ThemeName;
  readonly children: JSX.Element;
}

export function ThemeProvider(props: ThemeProviderProps): JSX.Element {
  const resolve = (): Theme => {
    const name = props.active();
    if (name === "light") return lightTheme;
    if (name === "high-contrast") return highContrastTheme;
    return darkTheme;
  };
  return (
    <ThemeContext.Provider value={resolve}>
      {props.children}
    </ThemeContext.Provider>
  );
}

export function useTheme(): () => Theme {
  return useContext(ThemeContext);
}
