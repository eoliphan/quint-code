// L5: DialogStore — open dialog stack (LIFO).

import { Context, Effect, Layer } from "effect";
import { createSignal, type Accessor } from "solid-js";
import type { DialogSpec } from "./actions.js";

export interface DialogStore {
  readonly current: Accessor<DialogSpec | undefined>;
  readonly stack: Accessor<ReadonlyArray<DialogSpec>>;
  readonly open: (spec: DialogSpec) => void;
  readonly close: () => void;
  readonly closeAll: () => void;
}

export class DialogStoreService extends Context.Tag("haft/DialogStore")<
  DialogStoreService,
  DialogStore
>() {}

const make = Effect.sync<DialogStore>(() => {
  const [stack, setStack] = createSignal<ReadonlyArray<DialogSpec>>([]);

  const current = (): DialogSpec | undefined => {
    const s = stack();
    return s.length === 0 ? undefined : s[s.length - 1];
  };

  const open = (spec: DialogSpec): void => { setStack((prev) => [...prev, spec]); };

  const close = (): void => { setStack((prev) => (prev.length === 0 ? prev : prev.slice(0, -1))); };

  const closeAll = (): void => { setStack(() => []); };

  return { current, stack, open, close, closeAll };
});

export const DialogStoreLive: Layer.Layer<DialogStoreService> = Layer.effect(
  DialogStoreService,
  make,
);
