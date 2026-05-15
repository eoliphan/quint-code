// L5: TokenStore — cumulative token counter for the active
// session.
//
// Updated by the SSE pump on every turn.completed event with a
// non-zero `tokens` field. Reset to 0 when the operator creates a
// new session. StatusBar reads `total()` and renders an "Nk tokens"
// chip.

import { Context, Effect, Layer } from "effect";
import { createSignal, type Accessor } from "solid-js";

export interface TokenStore {
  readonly total: Accessor<number>;
  readonly add: (delta: number) => void;
  readonly reset: () => void;
}

export class TokenStoreService extends Context.Tag("haft/TokenStore")<
  TokenStoreService,
  TokenStore
>() {}

const make = Effect.sync<TokenStore>(() => {
  const [total, setTotal] = createSignal(0);
  return {
    total,
    add: (delta: number): void => {
      if (delta <= 0) return;
      setTotal((t) => t + delta);
    },
    reset: (): void => {
      setTotal(0);
    },
  };
});

export const TokenStoreLive: Layer.Layer<TokenStoreService> = Layer.effect(
  TokenStoreService,
  make,
);

// formatTokens renders a number as "1.2k" / "3.5M" / raw when small.
// Used by StatusBar so the chip stays narrow.
export function formatTokens(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return (n / 1000).toFixed(1) + "k";
  return (n / 1_000_000).toFixed(1) + "M";
}
