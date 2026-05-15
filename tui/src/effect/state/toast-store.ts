// L5: ToastStore — transient notifications.

import { Context, Effect, Layer } from "effect";
import { createSignal, type Accessor } from "solid-js";

export type ToastLevel = "info" | "warn" | "error";

export interface ToastEntry {
  readonly id: string;
  readonly message: string;
  readonly level: ToastLevel;
  readonly createdAt: Date;
}

export interface ToastStore {
  readonly entries: Accessor<ReadonlyArray<ToastEntry>>;
  readonly push: (message: string, level: ToastLevel) => void;
  readonly dismiss: (id: string) => void;
  readonly clear: () => void;
}

export class ToastStoreService extends Context.Tag("haft/ToastStore")<
  ToastStoreService,
  ToastStore
>() {}

const make = Effect.sync<ToastStore>(() => {
  const [entries, setEntries] = createSignal<ReadonlyArray<ToastEntry>>([]);
  let seq = 0;

  const push = (message: string, level: ToastLevel): void => {
    seq += 1;
    const id = `toast-${seq}`;
    const entry: ToastEntry = {
      id,
      message,
      level,
      createdAt: new Date(),
    };
    setEntries((prev) => [...prev, entry]);
    // Auto-dismiss after a level-dependent timeout so error toasts
    // don't accumulate forever. Info: 4s, warn: 6s, error: 10s.
    const ttl = level === "error" ? 10_000 : level === "warn" ? 6_000 : 4_000;
    setTimeout(() => {
      setEntries((prev) => prev.filter((e) => e.id !== id));
    }, ttl);
  };

  const dismiss = (id: string): void => { setEntries((prev) => prev.filter((e) => e.id !== id)); };

  const clear = (): void => { setEntries(() => []); };

  return { entries, push, dismiss, clear };
});

export const ToastStoreLive: Layer.Layer<ToastStoreService> = Layer.effect(
  ToastStoreService,
  make,
);
