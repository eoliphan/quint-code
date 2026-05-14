// L6: Themed text. Color comes from theme token, NOT a raw hex.

import { type JSX } from "solid-js";
import { useTheme } from "../theme-context.js";
import type { Theme } from "../../themes/index.js";

type TokenName = keyof Theme;

export interface TextViewProps {
  readonly fg?: TokenName;
  readonly onMouseUp?: () => void;
  readonly children: JSX.Element | string | number;
}

export function TextView(props: TextViewProps): JSX.Element {
  const theme = useTheme();
  const color = (): string => {
    const t = theme();
    const k = props.fg ?? "fg";
    const v = t[k];
    return typeof v === "string" ? v : t.fg;
  };
  return (
    <text fg={color()} onMouseUp={props.onMouseUp}>
      {props.children}
    </text>
  );
}
