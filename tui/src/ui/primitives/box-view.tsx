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
  readonly marginTop?: number;
  readonly marginBottom?: number;
  readonly marginLeft?: number;
  readonly marginRight?: number;
  readonly flexDirection?: "row" | "column";
  readonly flexGrow?: number;
  readonly flexShrink?: number;
  readonly alignItems?: "flex-start" | "flex-end" | "center" | "stretch";
  readonly justifyContent?: "flex-start" | "flex-end" | "center" | "space-between" | "space-around" | "space-evenly";
  readonly width?: number;
  readonly minHeight?: number;
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
      flexGrow={props.flexGrow}
      flexShrink={props.flexShrink}
      paddingLeft={props.paddingLeft}
      paddingRight={props.paddingRight}
      paddingTop={props.paddingTop}
      paddingBottom={props.paddingBottom}
      marginTop={props.marginTop}
      marginBottom={props.marginBottom}
      marginLeft={props.marginLeft}
      marginRight={props.marginRight}
      alignItems={props.alignItems}
      justifyContent={props.justifyContent}
      width={props.width}
      minHeight={props.minHeight}
    >
      {props.children}
    </box>
  );
}
