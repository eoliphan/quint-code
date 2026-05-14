// L6: Animated spinner — frames cycled by an interval signal.

import { type JSX, createSignal, onCleanup, onMount } from "solid-js";
import { advance, DOTS_FRAMES, frameAt, type SpinnerFrames } from "../../components/spinner.js";
import { TextView } from "./text-view.js";

export interface SpinnerViewProps {
  readonly frames?: SpinnerFrames;
  readonly intervalMs?: number;
  readonly fg?: "fg" | "fgMuted" | "fgDim" | "caret" | "warning";
}

export function SpinnerView(props: SpinnerViewProps): JSX.Element {
  const frames = props.frames ?? DOTS_FRAMES;
  const interval = props.intervalMs ?? 80;
  const [idx, setIdx] = createSignal(0);
  let handle: ReturnType<typeof setInterval> | undefined;

  onMount(() => {
    handle = setInterval(() => {
      setIdx((cur) => advance(cur, frames));
    }, interval);
  });
  onCleanup(() => {
    if (handle !== undefined) clearInterval(handle);
  });

  return <TextView fg={props.fg ?? "caret"}>{frameAt(idx(), frames)}</TextView>;
}
