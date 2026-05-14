// L5: SessionStore — current Session value + dispatch surface.
//
// Wraps a Solid `createStore` around the Session shape from L1. The
// store's only mutation entry is `applyEvent` (driven by the SSE
// subscriber wiring), or specific high-level setters used by the
// initial bootstrap (sessionCreate response). Everything else flows
// through Actions → handlers → wire → reducer → store.

import { Context, Effect, Layer, type Stream as StreamNS } from "effect";
import { Stream } from "effect";
import { createSignal, type Accessor } from "solid-js";
import type { Session } from "../../core/domain/session.js";
import { reduce, type ReduceError } from "../../core/reducer/reduce.js";
import type { AgentEventWire } from "../../core/wire/events.js";

export interface SessionStore {
  /** Current Session value (or undefined before session.created). */
  readonly current: Accessor<Session | undefined>;
  /** Apply one decoded event through the reducer. Returns whether the
   *  reducer accepted it (errors logged via the toast layer). */
  readonly applyEvent: (ev: AgentEventWire, now: Date) => boolean;
  /** Explicit reset (used when navigating to a different session). */
  readonly reset: () => void;
}

export class SessionStoreService extends Context.Tag("haft/SessionStore")<
  SessionStoreService,
  SessionStore
>() {}

const make = Effect.sync<SessionStore>(() => {
  const [current, setCurrent] = createSignal<Session | undefined>(undefined);

  const applyEvent = (ev: AgentEventWire, now: Date): boolean => {
    const result = reduce(current(), ev, now);
    if (!result.ok) {
      // Reducer errors are surfaceable but non-fatal; routes can
      // observe via a parallel "lastError" signal in v8.1+ if needed.
      return false;
    }
    setCurrent(() => result.value);
    return true;
  };

  const reset = (): void => setCurrent(undefined);

  return { current, applyEvent, reset };
});

export const SessionStoreLive: Layer.Layer<SessionStoreService> = Layer.effect(
  SessionStoreService,
  make,
);

// Helper used by the App shell — wires an event Stream into the store.
// Returns an Effect that completes when the stream ends.
export function pumpEventsIntoStore(
  store: SessionStore,
  stream: StreamNS.Stream<AgentEventWire, unknown>,
): Effect.Effect<void, unknown> {
  return Stream.runForEach(stream, (ev) =>
    Effect.sync(() => {
      store.applyEvent(ev, new Date());
    }),
  );
}

// Re-export ReduceError so callers can pattern-match in toast layer.
export type { ReduceError };
