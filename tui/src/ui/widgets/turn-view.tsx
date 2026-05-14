// L7: TurnView — chat feed item. Splits the Turn into a left-border
// User block + a stacked Assistant block, matching the visual shape
// established by mature LLM TUIs.
//
// Convention from the driver: turn.parts[0] is the user's input text;
// everything after is assistant output (text deltas / reasoning /
// tool calls / FPF artifacts). Visually we surface that split: the
// user line is its own bordered card, the assistant body is a stack
// of part renderers indented under it.

import { type JSX, For, Show, createMemo } from "solid-js";
import type { AnyTurn } from "../../core/domain/turn.js";
import type { Part } from "../../core/domain/part.js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { Badge } from "../primitives/badge.js";
import { SpinnerView } from "../primitives/spinner-view.js";
import { PartView } from "./part-view.js";
import { useTheme } from "../theme-context.js";

export interface TurnViewProps {
  readonly turn: AnyTurn;
  readonly onInspectArtifact?: (artifactId: string) => void;
}

export function TurnView(props: TurnViewProps): JSX.Element {
  const t = props.turn;

  // The user's prompt is the FIRST text part attached at StartTurn time.
  // Subsequent text parts belong to the assistant; reasoning/tool/FPF
  // parts also belong to the assistant. The split is structural, not
  // role-encoded, so we walk the parts list once and bucket.
  const split = createMemo<{ userText: string; assistantParts: Part[] }>(() => {
    let userText = "";
    const assistantParts: Part[] = [];
    let foundUser = false;
    for (const p of t.parts) {
      if (!foundUser && p.kind === "text") {
        userText = p.text;
        foundUser = true;
        continue;
      }
      assistantParts.push(p);
    }
    return { userText, assistantParts };
  });

  const theme = useTheme();
  const userBorder = (): string => {
    const c = theme()[t.role === "sub_agent" ? "subAgent" : "user"];
    return typeof c === "string" ? c : theme().fg;
  };

  return (
    <BoxView marginTop={1}>
      <Show when={split().userText}>
        <box
          border={["left"]}
          borderColor={userBorder()}
          paddingTop={1}
          paddingBottom={1}
          paddingLeft={2}
          flexShrink={0}
        >
          <TextView>{split().userText}</TextView>
        </box>
      </Show>
      <BoxView paddingLeft={2} paddingTop={1}>
        <For each={split().assistantParts}>
          {(part) => (
            <PartView part={part} onInspectArtifact={props.onInspectArtifact} />
          )}
        </For>
        <Show when={t.state === "running"}>
          <BoxView flexDirection="row" paddingTop={1}>
            <SpinnerView fg="caret" />
            <TextView fg="fgDim"> thinking…</TextView>
          </BoxView>
        </Show>
        <Show when={t.state === "failed"}>
          <BoxView paddingTop={1}>
            <TextView fg="danger">
              ✗ {(t as { errorMessage?: string }).errorMessage ?? "turn failed"}
            </TextView>
          </BoxView>
        </Show>
        <Show when={t.state === "completed"}>
          <BoxView paddingTop={1}>
            <Badge label="done" fg="success" />
          </BoxView>
        </Show>
      </BoxView>
    </BoxView>
  );
}
