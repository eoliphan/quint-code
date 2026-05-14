// L7: TurnView — composes part renderers under a turn header.

import { type JSX, For } from "solid-js";
import type { AnyTurn } from "../../core/domain/turn.js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { Badge } from "../primitives/badge.js";
import { PartView } from "./part-view.js";
import type { Theme } from "../../themes/index.js";

type Token = keyof Theme;

export interface TurnViewProps {
  readonly turn: AnyTurn;
  readonly onInspectArtifact?: (artifactId: string) => void;
}

export function TurnView(props: TurnViewProps): JSX.Element {
  const t = props.turn;
  return (
    <BoxView paddingTop={1}>
      <TextView fg={roleColor(t.role)}>
        {marker(t.role)} {t.state}
        {t.state !== "running" ? <Badge label={t.verdict} /> : null}
      </TextView>
      <For each={[...t.parts]}>
        {(part) => <PartView part={part} onInspectArtifact={props.onInspectArtifact} />}
      </For>
    </BoxView>
  );
}

function marker(role: AnyTurn["role"]): string {
  if (role === "user") return "›";
  if (role === "sub_agent") return "⤷";
  return "·";
}

function roleColor(role: AnyTurn["role"]): Token {
  if (role === "user") return "user";
  if (role === "sub_agent") return "subAgent";
  return "assistant";
}
