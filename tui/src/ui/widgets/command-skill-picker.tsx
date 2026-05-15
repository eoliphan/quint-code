// L7: CommandSkillPicker — wraps the generic Picker for slash
// commands and skills.
//
// Activation:
//   "/"      → opens the slash-command picker. Selected command's
//              body is dispatched as the next user prompt.
//   Ctrl+K   → opens the skill picker. Selected skill is dispatched
//              with a "use this skill" framing so the agent picks
//              up the context.
//
// Both pickers fetch their lists lazily from the agent server's
// /commands and /skills endpoints; the response is cached for the
// lifetime of the route mount.

import {
  type JSX,
  Show,
  createSignal,
  onCleanup,
  onMount,
} from "solid-js";
import { useRenderer } from "@opentui/solid";
import { Picker, type PickerItem } from "./picker.js";
import { useAgentClient, useRunEffect } from "../agent-client-context.js";
import type { Action } from "../../effect/state/actions.js";

type PickerMode = null | "commands" | "skills" | "sessions" | "models";

// Static model list. Mirrors the openai/anthropic models haft
// supports; the operator's chosen credential set determines which
// of these actually work.
const MODELS: ReadonlyArray<{ id: string; label: string; desc?: string }> = [
  { id: "gpt-5.4", label: "gpt-5.4", desc: "openai · reasoning · 1M context" },
  { id: "gpt-5.3", label: "gpt-5.3", desc: "openai · reasoning · 400k" },
  { id: "gpt-5.2", label: "gpt-5.2", desc: "openai · reasoning · 256k" },
  { id: "gpt-5-mini", label: "gpt-5-mini", desc: "openai · light · 1M context" },
  { id: "claude-opus-4-7", label: "claude-opus-4-7", desc: "anthropic · top tier" },
  { id: "claude-sonnet-4-6", label: "claude-sonnet-4-6", desc: "anthropic · fast" },
  { id: "claude-haiku-4-5", label: "claude-haiku-4-5", desc: "anthropic · cheapest" },
];

export interface CommandSkillPickerProps {
  /** True while the host route should accept the global / and Ctrl+K
   *  triggers (e.g. only on agent.session, never on home). */
  readonly enabled: () => boolean;
  readonly dispatch: (a: Action) => void;
}

export function CommandSkillPicker(props: CommandSkillPickerProps): JSX.Element {
  const client = useAgentClient();
  const run = useRunEffect();

  const [mode, setMode] = createSignal<PickerMode>(null);
  const [items, setItems] = createSignal<readonly PickerItem[]>([]);

  const openCommands = async (): Promise<void> => {
    try {
      const r = await run(client.listCommands() as never) as { commands: ReadonlyArray<{ name: string; description?: string }> };
      setItems(
        r.commands.map((c) => ({
          id: c.name,
          label: `/${c.name}`,
          desc: c.description,
        })),
      );
      setMode("commands");
    } catch {
      props.dispatch({ tag: "ShowToast", message: "failed to load slash commands", level: "error" });
    }
  };

  const openSessions = async (): Promise<void> => {
    try {
      const r = (await run(client.sessionList() as never)) as {
        sessions: ReadonlyArray<{ id: string; title: string }>;
      };
      setItems(
        r.sessions.map((s) => ({
          id: s.id,
          label: s.title === "" ? "(untitled)" : s.title,
          desc: s.id,
        })),
      );
      setMode("sessions");
    } catch {
      props.dispatch({ tag: "ShowToast", message: "failed to load sessions", level: "error" });
    }
  };

  const openModels = (): void => {
    setItems(MODELS.map((m) => ({ id: m.id, label: m.label, desc: m.desc })));
    setMode("models");
  };

  const openSkills = async (): Promise<void> => {
    try {
      const r = await run(client.listSkills() as never) as { skills: ReadonlyArray<{ name: string; description?: string }> };
      setItems(
        r.skills.map((s) => ({
          id: s.name,
          label: s.name,
          desc: s.description,
        })),
      );
      setMode("skills");
    } catch {
      props.dispatch({ tag: "ShowToast", message: "failed to load skills", level: "error" });
    }
  };

  const onSelect = async (item: PickerItem): Promise<void> => {
    const m = mode();
    setMode(null);
    try {
      if (m === "commands") {
        const body = (await run(client.getCommand(item.id) as never)) as { name: string; body: string };
        props.dispatch({ tag: "SubmitTurn", text: body.body });
      } else if (m === "skills") {
        const body = (await run(client.getSkill(item.id) as never)) as { name: string; body: string };
        props.dispatch({
          tag: "SubmitTurn",
          text: `Use the "${item.id}" skill for this conversation. Skill definition:\n\n${body.body}`,
        });
      } else if (m === "sessions") {
        props.dispatch({ tag: "ResumeSession", id: item.id as never });
      } else if (m === "models") {
        // Heuristic: openai for gpt-*, anthropic for claude-*.
        const provider = item.id.startsWith("gpt-") ? "openai" : "anthropic";
        props.dispatch({
          tag: "SwitchModel",
          model: { provider, model: item.id },
        });
        props.dispatch({
          tag: "ShowToast",
          message: `model: ${item.id}`,
          level: "info",
        });
      }
    } catch {
      props.dispatch({ tag: "ShowToast", message: "failed to load item body", level: "error" });
    }
  };

  const onCancel = (): void => {
    setMode(null);
  };

  // Global keypress listener — `/` opens commands, Ctrl+K opens
  // skills. Only fires while enabled() is true and no picker is
  // already mounted, so the picker's own keystream stays
  // uncontested.
  onMount(() => {
    const renderer = useRenderer();
    const onKey = (ev: { name: string; ctrl: boolean; shift: boolean; sequence: string }): void => {
      if (!props.enabled()) return;
      if (mode() !== null) return;
      if (ev.sequence === "/") {
        void openCommands();
        return;
      }
      if (ev.ctrl && ev.name === "k") {
        void openSkills();
        return;
      }
      if (ev.ctrl && ev.name === "s") {
        void openSessions();
        return;
      }
      if (ev.ctrl && ev.name === "r") {
        openModels();
      }
    };
    renderer.keyInput.on("keypress", onKey);
    onCleanup(() => renderer.keyInput.off("keypress", onKey));
  });

  return (
    <Show when={mode() !== null}>
      <Picker
        active={() => mode() !== null}
        title={pickerTitle(mode())}
        items={items}
        onSelect={(item) => void onSelect(item)}
        onCancel={onCancel}
      />
    </Show>
  );
}

function pickerTitle(m: PickerMode): string {
  switch (m) {
    case "commands":
      return "Slash commands · /";
    case "skills":
      return "Skills · Ctrl+K";
    case "sessions":
      return "Sessions · Ctrl+S";
    case "models":
      return "Models · Ctrl+R";
    case null:
    default:
      return "";
  }
}
