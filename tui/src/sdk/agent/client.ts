import { type DecodeResult } from "../core/types.js";
import { getJSON, postJSON, subscribeSSE, type RPCConfig } from "../core/transport.js";
import { decodeAgentEvent } from "./events.js";
import { agentErrorMapper } from "./errors.js";
import {
  type AgentEvent,
  type AuthStatus,
  type ModelChoice,
  type SessionID,
  type TurnID,
  type PermissionID,
} from "./types.js";

/** Construction options. Tests inject a custom fetch and override the
 *  AbortSignal so the SSE reader can be torn down deterministically. */
export interface AgentClientOptions {
  /** Base URL of the agentserver, e.g. "http://127.0.0.1:54321".
   *  No trailing slash. */
  baseUrl: string;
  /** AbortSignal that cancels the SSE subscription on tear-down. */
  signal: AbortSignal;
  /** Custom fetch — injected for tests. Defaults to global fetch. */
  fetch?: typeof fetch;
  /** Max RPC attempts. Default 3. */
  maxAttempts?: number;
}

/** AgentClient is the load-bearing TS-side API for the v8 TUI route
 *  layer. State is held by the caller (Solid stores in t13); the
 *  client itself is stateless past the abort signal. */
export class AgentClient {
  private readonly rpc: RPCConfig;
  private readonly signal: AbortSignal;
  private readonly fetchFn: typeof fetch;

  constructor(opts: AgentClientOptions) {
    this.signal = opts.signal;
    this.fetchFn = opts.fetch ?? fetch;
    this.rpc = {
      baseUrl: opts.baseUrl,
      errorMapper: agentErrorMapper,
      fetch: this.fetchFn,
      maxAttempts: opts.maxAttempts ?? 3,
    };
  }

  /** Subscribe to /event/global. The callback fires per decoded or
   *  unknown event in arrival order. Reconnect + backoff is handled
   *  internally; abort the signal to tear down. */
  subscribe(onEvent: (result: DecodeResult<AgentEvent>) => void): Promise<void> {
    return subscribeSSE<AgentEvent>({
      url: this.rpc.baseUrl + "/event/global",
      decode: decodeAgentEvent,
      onEvent,
      signal: this.signal,
      fetch: this.fetchFn,
    });
  }

  /** GET /healthz — ping. Returns the {ok, subscribers} payload. */
  healthz(): Promise<{ ok: boolean; subscribers: number }> {
    return getJSON(this.rpc, "/healthz");
  }

  /** GET /auth/status — auth surface for the session header. */
  authStatus(): Promise<AuthStatus> {
    return getJSON(this.rpc, "/auth/status");
  }

  /** GET /session — list sessions. */
  sessionList(): Promise<{ sessions: Array<{ id: SessionID; title: string }> }> {
    return getJSON(this.rpc, "/session");
  }

  /** GET /session/:id — fetch a session payload. */
  sessionGet(id: SessionID): Promise<unknown> {
    return getJSON(this.rpc, `/session/${id}`);
  }

  /** POST /session — create a session. */
  sessionCreate(body: { project_id: string; title: string; model: ModelChoice }): Promise<{ id: SessionID }> {
    return postJSON(this.rpc, "/session", body);
  }

  /** POST /session/:id/turn — submit a user turn. */
  turnSubmit(id: SessionID, body: { text: string }): Promise<{ turn_id: TurnID }> {
    return postJSON(this.rpc, `/session/${id}/turn`, body);
  }

  /** POST /session/:id/cancel — cancel the current turn. */
  turnCancel(id: SessionID, body: { turn_id: TurnID }): Promise<void> {
    return postJSON(this.rpc, `/session/${id}/cancel`, body);
  }

  /** POST /session/:id/rename — change session title. */
  sessionRename(id: SessionID, body: { title: string }): Promise<void> {
    return postJSON(this.rpc, `/session/${id}/rename`, body);
  }

  /** POST /session/:id/model — switch the model for the next turn. */
  modelSet(id: SessionID, body: { model: ModelChoice }): Promise<void> {
    return postJSON(this.rpc, `/session/${id}/model`, body);
  }

  /** POST /permission/:id — resolve a permission request. */
  permissionRespond(
    id: PermissionID,
    body: { decision: "approved" | "denied"; reason: string },
  ): Promise<void> {
    return postJSON(this.rpc, `/permission/${id}`, body);
  }
}
