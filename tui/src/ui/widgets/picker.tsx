// L7: Picker — generic filter+select overlay.
//
// Reused by slash commands, skills, model selection, session list,
// and file selection. Keyboard:
//   typing       — narrows the filter (matches label / id / desc)
//   ↑ / ↓        — moves the cursor
//   Enter        — selects current item
//   Esc          — cancels
//
// Mounted as a modal box at the top of the route; consumes its own
// keyboard via the renderer's keyInput emitter so the host route's
// input is left untouched. Use the `active` prop to mount/unmount
// the overlay reactively.

import { type JSX, For, Show, createMemo, createSignal, onCleanup, onMount } from "solid-js";
import { useRenderer } from "@opentui/solid";
import { useTheme } from "../theme-context.js";

export interface PickerItem {
  readonly id: string;
  readonly label: string;
  readonly desc?: string;
}

export interface PickerProps {
  readonly active: () => boolean;
  readonly title: string;
  readonly items: () => readonly PickerItem[];
  readonly onSelect: (item: PickerItem) => void;
  readonly onCancel: () => void;
  /** Maximum number of items visible at once. Defaults to 12. */
  readonly maxVisible?: number;
}

const DEFAULT_MAX = 12;

export function Picker(props: PickerProps): JSX.Element {
  const theme = useTheme();
  const [filter, setFilter] = createSignal("");
  const [cursor, setCursor] = createSignal(0);

  const filtered = createMemo((): readonly PickerItem[] => {
    const f = filter().toLowerCase();
    const all = props.items();
    if (f === "") return all;
    return all.filter(
      (it) =>
        it.label.toLowerCase().includes(f) ||
        it.id.toLowerCase().includes(f) ||
        (it.desc?.toLowerCase().includes(f) ?? false),
    );
  });

  const visible = createMemo((): readonly PickerItem[] => {
    const max = props.maxVisible ?? DEFAULT_MAX;
    const list = filtered();
    if (list.length <= max) return list;
    // Window the visible slice around the cursor so up/down past the
    // edge of the visible window scrolls through the rest of the list.
    const c = cursor();
    const start = Math.max(0, Math.min(c - Math.floor(max / 2), list.length - max));
    return list.slice(start, start + max);
  });

  onMount(() => {
    const renderer = useRenderer();
    const onKey = (ev: { name: string; ctrl: boolean; shift: boolean; sequence: string }): void => {
      if (!props.active()) return;
      if (ev.name === "escape") {
        props.onCancel();
        return;
      }
      if (ev.name === "return" || ev.name === "enter") {
        const list = filtered();
        const item = list[cursor()];
        if (item !== undefined) props.onSelect(item);
        return;
      }
      if (ev.name === "up") {
        setCursor((c) => Math.max(0, c - 1));
        return;
      }
      if (ev.name === "down") {
        setCursor((c) => Math.min(filtered().length - 1, c + 1));
        return;
      }
      if (ev.name === "backspace") {
        setFilter((f) => f.slice(0, -1));
        setCursor(0);
        return;
      }
      // Printable character — append to filter.
      if (ev.sequence && ev.sequence.length === 1 && ev.sequence >= " ") {
        setFilter((f) => f + ev.sequence);
        setCursor(0);
      }
    };
    renderer.keyInput.on("keypress", onKey);
    onCleanup(() => renderer.keyInput.off("keypress", onKey));
  });

  // Clamp cursor when the filtered list shrinks below the current
  // index. Keep cursor at 0 when the filter empties the list to
  // avoid out-of-range reads on Enter.
  const clampedCursor = createMemo(() => {
    const list = filtered();
    if (list.length === 0) return 0;
    if (cursor() >= list.length) return list.length - 1;
    return cursor();
  });

  return (
    <Show when={props.active()}>
      <box
        border={["top", "bottom", "left", "right"]}
        borderColor={(theme()["toolName"] as string) ?? theme().fg}
        paddingLeft={1}
        paddingRight={1}
        paddingTop={1}
        paddingBottom={1}
        marginTop={1}
        marginBottom={1}
      >
        <text>
          <b style={{ fg: (theme()["toolName"] as string) ?? theme().fg }}>{props.title}</b>
        </text>
        <text>
          <span style={{ fg: (theme()["fgDim"] as string) ?? theme().fg }}>
            {"› "}
          </span>
          <span>{filter() === "" ? "(type to filter)" : filter()}</span>
        </text>
        <text fg={(theme()["fgDim"] as string) ?? theme().fg}>{"─".repeat(40)}</text>
        <Show
          when={visible().length > 0}
          fallback={
            <text fg={(theme()["fgMuted"] as string) ?? theme().fg}>(no matches)</text>
          }
        >
          <For each={visible()}>
            {(item) => {
              const isActive = (): boolean => {
                const list = filtered();
                const ai = list[clampedCursor()];
                return ai !== undefined && ai.id === item.id;
              };
              return (
                <text>
                  <Show when={isActive()} fallback={<span>  </span>}>
                    <b style={{ fg: (theme()["caret"] as string) ?? theme().fg }}>
                      {"▸ "}
                    </b>
                  </Show>
                  <Show
                    when={isActive()}
                    fallback={<span>{item.label}</span>}
                  >
                    <b style={{ fg: (theme()["toolName"] as string) ?? theme().fg }}>
                      {item.label}
                    </b>
                  </Show>
                  <Show when={item.desc}>
                    <span style={{ fg: (theme()["fgDim"] as string) ?? theme().fg }}>
                      {"  " + (item.desc ?? "")}
                    </span>
                  </Show>
                </text>
              );
            }}
          </For>
        </Show>
        <text fg={(theme()["fgDim"] as string) ?? theme().fg}>
          {`${filtered().length} item${filtered().length === 1 ? "" : "s"} · ↑↓ navigate · Enter select · Esc cancel`}
        </text>
      </box>
    </Show>
  );
}
