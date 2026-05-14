// L7: Dialog — generic modal wrapper that closes via Action dispatch.

import { type JSX } from "solid-js";
import { TextView } from "../primitives/text-view.js";
import { BoxView } from "../primitives/box-view.js";
import { Divider } from "../primitives/divider.js";

export interface DialogProps {
  readonly title: string;
  readonly children: JSX.Element;
}

export function Dialog(props: DialogProps): JSX.Element {
  return (
    <BoxView bg="bgOverlay" paddingLeft={2} paddingRight={2} paddingTop={1} paddingBottom={1}>
      <TextView fg="toolName">▣ {props.title}</TextView>
      <Divider />
      {props.children}
    </BoxView>
  );
}
