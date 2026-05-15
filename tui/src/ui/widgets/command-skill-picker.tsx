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

type PickerMode = null | "commands" | "skills";

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
        // The command body is the literal markdown the operator
        // would paste into chat. Submit it as the next turn.
        props.dispatch({ tag: "SubmitTurn", text: body.body });
      } else if (m === "skills") {
        const body = (await run(client.getSkill(item.id) as never)) as { name: string; body: string };
        // Skills are inlined as "Apply skill <name>:" prefixes so
        // the agent uses the skill's instructions rather than
        // echoing the whole markdown back.
        props.dispatch({
          tag: "SubmitTurn",
          text: `Use the "${item.id}" skill for this conversation. Skill definition:\n\n${body.body}`,
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
      if (ev.ctrl && (ev.name === "k" || ev.sequence === "")) {
        void openSkills();
      }
    };
    renderer.keyInput.on("keypress", onKey);
    onCleanup(() => renderer.keyInput.off("keypress", onKey));
  });

  return (
    <Show when={mode() !== null}>
      <Picker
        active={() => mode() !== null}
        title={mode() === "commands" ? "Slash commands" : "Skills"}
        items={items}
        onSelect={(item) => void onSelect(item)}
        onCancel={onCancel}
      />
    </Show>
  );
}
