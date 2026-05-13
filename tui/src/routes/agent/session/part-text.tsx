import { type JSX } from "solid-js";

export type PartTextProps = {
  text: string;
  isStreaming?: boolean;
};

export function PartText(props: PartTextProps): JSX.Element {
  return (
    <text fg="#e6e6e6">
      {props.text}
      {props.isStreaming ? "▎" : ""}
    </text>
  );
}
