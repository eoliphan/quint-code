// L6: Themed box wrapper.

import { type JSX } from "solid-js";
import { useTheme } from "../theme-context.js";
import type { Theme } from "../../themes/index.js";

type TokenName = keyof Theme;

export interface BoxViewProps {
  readonly bg?: TokenName;
  readonly paddingLeft?: number;
  readonly paddingRight?: number;
  readonly paddingTop?: number;
  readonly paddingBottom?: number;
  readonly flexDirection?: "row" | "column";
  readonly alignItems?: "flex-start" | "flex-end" | "center" | "stretch";
  readonly width?: number;
  readonly children: JSX.Element;
}

export function BoxView(props: BoxViewProps): JSX.Element {
  const theme = useTheme();
  const bg = (): string | undefined => {
    if (props.bg === undefined) return undefined;
    const v = theme()[props.bg];
    return typeof v === "string" ? v : undefined;
  };
  return (
    <box
      backgroundColor={bg()}
      flexDirection={props.flexDirection ?? "column"}
      paddingLeft={props.paddingLeft}
      paddingRight={props.paddingRight}
      paddingTop={props.paddingTop}
      paddingBottom={props.paddingBottom}
      alignItems={props.alignItems}
      width={props.width}
    >
      {props.children}
    </box>
  );
}
