// L6: Horizontal divider — single line in fgDim.

import { type JSX } from "solid-js";
import { TextView } from "./text-view.js";

export interface DividerProps {
  readonly width?: number;
}

export function Divider(props: DividerProps): JSX.Element {
  const w = props.width ?? 60;
  const line = (): string => "─".repeat(w);
  return <TextView fg="fgDim">{line()}</TextView>;
}
