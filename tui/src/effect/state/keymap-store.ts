// L5: KeymapStore — active keymap layer + binding lookup.
//
// Keymaps are stacked: a dialog opening pushes its own keymap on top
// of the route's keymap. Resolution walks top-down and returns the
// first match. Handlers produce Actions which the dispatcher then
// routes through the SDK.

import { Context, Effect, Layer } from "effect";
import { createSignal, type Accessor } from "solid-js";
import type { Action } from "./actions.js";

export interface KeyBinding {
  readonly key: string; // e.g. "Enter", "Escape", "Ctrl+C", "?"
  readonly action: string; // human-readable label for the hint bar
  readonly handler: () => Action | undefined;
}

export interface KeymapLayer {
  readonly id: string;
  readonly bindings: ReadonlyArray<KeyBinding>;
}

export interface KeymapStore {
  readonly stack: Accessor<ReadonlyArray<KeymapLayer>>;
  /** Visible hints for the footer: union of bindings across the stack,
   *  with shadowed keys hidden. */
  readonly visibleHints: Accessor<ReadonlyArray<{ readonly key: string; readonly action: string }>>;
  readonly pushLayer: (layer: KeymapLayer) => void;
  readonly popLayer: (id: string) => void;
  readonly resolve: (key: string) => Action | undefined;
}

export class KeymapStoreService extends Context.Tag("haft/KeymapStore")<
  KeymapStoreService,
  KeymapStore
>() {}

const make = Effect.sync<KeymapStore>(() => {
  const [stack, setStack] = createSignal<ReadonlyArray<KeymapLayer>>([]);

  const visibleHints = (): ReadonlyArray<{ key: string; action: string }> => {
    const s = stack();
    const seen = new Set<string>();
    const hints: Array<{ key: string; action: string }> = [];
    // Walk top-down: a binding from a higher layer shadows a lower
    // one. visibleHints surfaces the effective binding only.
    for (let i = s.length - 1; i >= 0; i--) {
      const layer = s[i];
      if (layer === undefined) continue;
      for (const b of layer.bindings) {
        if (seen.has(b.key)) continue;
        seen.add(b.key);
        hints.push({ key: b.key, action: b.action });
      }
    }
    return hints;
  };

  const pushLayer = (layer: KeymapLayer): void => { setStack((prev) => [...prev, layer]); };

  const popLayer = (id: string): void => { setStack((prev) => prev.filter((l) => l.id !== id)); };

  const resolve = (key: string): Action | undefined => {
    const s = stack();
    for (let i = s.length - 1; i >= 0; i--) {
      const layer = s[i];
      if (layer === undefined) continue;
      for (const b of layer.bindings) {
        if (b.key === key) return b.handler();
      }
    }
    return undefined;
  };

  return { stack, visibleHints, pushLayer, popLayer, resolve };
});

export const KeymapStoreLive: Layer.Layer<KeymapStoreService> = Layer.effect(
  KeymapStoreService,
  make,
);
