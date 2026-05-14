// L6: Compact label, used for status pills and counts.

import { type JSX } from "solid-js";
import { TextView } from "./text-view.js";
import type { Theme } from "../../themes/index.js";

type TokenName = keyof Theme;

export interface BadgeProps {
  readonly label: string;
  readonly fg?: TokenName;
}

export function Badge(props: BadgeProps): JSX.Element {
  return (
    <TextView fg={props.fg ?? "fgMuted"}>
      [{props.label}]
    </TextView>
  );
}
