// L5: ThemeStore — active theme + cycle/set.

import { Context, Effect, Layer } from "effect";
import { createSignal, type Accessor } from "solid-js";

export type ThemeName = "dark" | "light" | "high-contrast";

export interface ThemeStore {
  readonly active: Accessor<ThemeName>;
  readonly set: (name: ThemeName) => void;
  readonly cycle: () => ThemeName;
  readonly names: ReadonlyArray<ThemeName>;
}

export class ThemeStoreService extends Context.Tag("haft/ThemeStore")<
  ThemeStoreService,
  ThemeStore
>() {}

const NAMES: ReadonlyArray<ThemeName> = ["dark", "light", "high-contrast"];

const make = Effect.sync<ThemeStore>(() => {
  const [active, setActive] = createSignal<ThemeName>("dark");

  const set = (name: ThemeName): void => { setActive(() => name); };

  const cycle = (): ThemeName => {
    const cur = active();
    const idx = NAMES.indexOf(cur);
    const next = NAMES[(idx + 1) % NAMES.length] ?? "dark";
    setActive(() => next);
    return next;
  };

  return { active, set, cycle, names: NAMES };
});

export const ThemeStoreLive: Layer.Layer<ThemeStoreService> = Layer.effect(
  ThemeStoreService,
  make,
);
