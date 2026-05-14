// L5: RouteStore — active route + navigate.

import { Context, Effect, Layer } from "effect";
import { createSignal, type Accessor } from "solid-js";
import type { RouteName } from "./actions.js";

export interface RouteStore {
  readonly active: Accessor<RouteName>;
  readonly navigate: (to: RouteName) => void;
  readonly inspectArtifact: (id: string) => void;
  readonly inspectedArtifactId: Accessor<string | undefined>;
}

export class RouteStoreService extends Context.Tag("haft/RouteStore")<
  RouteStoreService,
  RouteStore
>() {}

const make = Effect.sync<RouteStore>(() => {
  const [active, setActive] = createSignal<RouteName>("home");
  const [inspected, setInspected] = createSignal<string | undefined>(undefined);

  const navigate = (to: RouteName): void => { setActive(() => to); };

  const inspectArtifact = (id: string): void => {
    setInspected(() => id);
    setActive(() => "fpf.inspector");
  };

  return { active, navigate, inspectArtifact, inspectedArtifactId: inspected };
});

export const RouteStoreLive: Layer.Layer<RouteStoreService> = Layer.effect(
  RouteStoreService,
  make,
);
