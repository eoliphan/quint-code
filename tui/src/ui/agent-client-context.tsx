// L9: AgentClientContext — surfaces AgentClient + Effect Runtime to
// any descendant route/widget without prop-drilling.
//
// main.tsx wraps AppShell in <AgentClientProvider client={client}
// run={runPromise}> after constructing the Effect runtime. Routes
// that need to call RPCs (slash/skill pickers, model picker,
// session list) read the client via useAgentClient() and an Effect
// runner via useRunEffect().

import { type JSX, createContext, useContext } from "solid-js";
import { Effect } from "effect";
import type { AgentClient } from "../effect/transport/agent-client.js";

export interface AgentRuntimeContextValue {
  readonly client: AgentClient;
  // run is the Effect runner with the AppLive layer baked in. The
  // caller hands an Effect<A, E, never> (after Effect.provide) and
  // gets a Promise back. Routes use this to call AgentClient
  // methods without re-wiring their own runtime.
  readonly run: <A, E>(eff: Effect.Effect<A, E, never>) => Promise<A>;
}

const Ctx = createContext<AgentRuntimeContextValue | undefined>(undefined);

export interface AgentClientProviderProps {
  readonly value: AgentRuntimeContextValue;
  readonly children: JSX.Element;
}

export function AgentClientProvider(props: AgentClientProviderProps): JSX.Element {
  return <Ctx.Provider value={props.value}>{props.children}</Ctx.Provider>;
}

export function useAgentClient(): AgentClient {
  const c = useContext(Ctx);
  if (c === undefined) {
    throw new Error("useAgentClient called outside AgentClientProvider");
  }
  return c.client;
}

export function useRunEffect(): AgentRuntimeContextValue["run"] {
  const c = useContext(Ctx);
  if (c === undefined) {
    throw new Error("useRunEffect called outside AgentClientProvider");
  }
  return c.run;
}
