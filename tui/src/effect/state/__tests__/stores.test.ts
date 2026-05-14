// L5 tests — store layers compose, dispatch routes, theme cycles.

import { describe, expect, test } from "bun:test";
import { Effect, Layer } from "effect";
import { ThemeStoreLive, ThemeStoreService } from "../theme-store.js";
import { RouteStoreLive, RouteStoreService } from "../route-store.js";
import { DialogStoreLive, DialogStoreService } from "../dialog-store.js";
import { ToastStoreLive, ToastStoreService } from "../toast-store.js";
import { KeymapStoreLive, KeymapStoreService } from "../keymap-store.js";
import { SessionStoreLive, SessionStoreService } from "../session-store.js";
import type { AgentEventWire } from "../../../core/wire/events.js";

describe("ThemeStore", () => {
  test("cycle dark → light → high-contrast → dark", async () => {
    const program = Effect.gen(function* () {
      const t = yield* ThemeStoreService;
      expect(t.active()).toBe("dark");
      expect(t.cycle()).toBe("light");
      expect(t.cycle()).toBe("high-contrast");
      expect(t.cycle()).toBe("dark");
      t.set("light");
      expect(t.active()).toBe("light");
    });
    await Effect.runPromise(Effect.provide(program, ThemeStoreLive));
  });
});

describe("RouteStore", () => {
  test("navigate + inspectArtifact", async () => {
    const program = Effect.gen(function* () {
      const r = yield* RouteStoreService;
      expect(r.active()).toBe("agent.session");
      r.navigate("home");
      expect(r.active()).toBe("home");
      r.inspectArtifact("dec-123");
      expect(r.active()).toBe("fpf.inspector");
      expect(r.inspectedArtifactId()).toBe("dec-123");
    });
    await Effect.runPromise(Effect.provide(program, RouteStoreLive));
  });
});

describe("DialogStore", () => {
  test("LIFO stack: open / open / close → second remains", async () => {
    const program = Effect.gen(function* () {
      const d = yield* DialogStoreService;
      expect(d.current()).toBeUndefined();
      d.open({ kind: "help" });
      d.open({ kind: "themePicker" });
      expect(d.current()?.kind).toBe("themePicker");
      d.close();
      expect(d.current()?.kind).toBe("help");
      d.closeAll();
      expect(d.current()).toBeUndefined();
    });
    await Effect.runPromise(Effect.provide(program, DialogStoreLive));
  });
});

describe("ToastStore", () => {
  test("push / dismiss / clear", async () => {
    const program = Effect.gen(function* () {
      const t = yield* ToastStoreService;
      t.push("a", "info");
      t.push("b", "warn");
      expect(t.entries().length).toBe(2);
      const first = t.entries()[0]!;
      t.dismiss(first.id);
      expect(t.entries().length).toBe(1);
      t.clear();
      expect(t.entries().length).toBe(0);
    });
    await Effect.runPromise(Effect.provide(program, ToastStoreLive));
  });
});

describe("KeymapStore", () => {
  test("visibleHints surfaces top-layer binding for shadowed keys", async () => {
    const program = Effect.gen(function* () {
      const k = yield* KeymapStoreService;
      k.pushLayer({
        id: "route",
        bindings: [
          { key: "Enter", action: "submit", handler: () => undefined },
          { key: "?", action: "help", handler: () => undefined },
        ],
      });
      k.pushLayer({
        id: "dialog",
        bindings: [
          { key: "Enter", action: "confirm", handler: () => undefined },
          { key: "Escape", action: "cancel", handler: () => undefined },
        ],
      });
      const hints = k.visibleHints();
      // dialog shadows route on Enter, exposes Escape, route '?' shows.
      const enter = hints.find((h) => h.key === "Enter");
      expect(enter?.action).toBe("confirm");
      const help = hints.find((h) => h.key === "?");
      expect(help?.action).toBe("help");
      const esc = hints.find((h) => h.key === "Escape");
      expect(esc?.action).toBe("cancel");

      k.popLayer("dialog");
      const after = k.visibleHints();
      expect(after.find((h) => h.key === "Enter")?.action).toBe("submit");
      expect(after.find((h) => h.key === "Escape")).toBeUndefined();
    });
    await Effect.runPromise(Effect.provide(program, KeymapStoreLive));
  });
});

describe("SessionStore.applyEvent drives reducer", () => {
  test("session.created bootstraps; turn.started transitions state", async () => {
    const program = Effect.gen(function* () {
      const s = yield* SessionStoreService;
      expect(s.current()).toBeUndefined();
      const evCreated: AgentEventWire = {
        kind: "session.created",
        session_id: "s1",
        at: "2026-05-14T00:00:00Z",
        project_id: "p1",
        title: "test",
        model: { provider: "none", model: "" },
      };
      const ok1 = s.applyEvent(evCreated, new Date(2026, 4, 14));
      expect(ok1).toBe(true);
      expect(s.current()?.state).toBe("idle");
      const evStart: AgentEventWire = {
        kind: "turn.started",
        session_id: "s1",
        turn_id: "t1",
        at: "2026-05-14T00:00:01Z",
        role: "user",
        first_part: "hi",
      };
      const ok2 = s.applyEvent(evStart, new Date(2026, 4, 14));
      expect(ok2).toBe(true);
      expect(s.current()?.state).toBe("hasLiveTurn");
    });
    const layer = Layer.merge(SessionStoreLive, ToastStoreLive);
    void layer;
    await Effect.runPromise(Effect.provide(program, SessionStoreLive));
  });
});
