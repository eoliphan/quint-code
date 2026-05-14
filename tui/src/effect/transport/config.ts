// L3: TransportConfig — read once at boot, immutable thereafter.

import { Context, Layer, Effect } from "effect";

export interface TransportConfig {
  readonly baseUrl: string;
  readonly maxAttempts: number;
  readonly timeoutMs: number;
}

export class TransportConfigService extends Context.Tag("haft/TransportConfig")<
  TransportConfigService,
  TransportConfig
>() {}

export const makeConfig = (baseUrl: string): TransportConfig => ({
  baseUrl,
  maxAttempts: 3,
  timeoutMs: 30_000,
});

export const layer = (config: TransportConfig): Layer.Layer<TransportConfigService> =>
  Layer.effect(TransportConfigService, Effect.succeed(config));
