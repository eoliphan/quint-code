// L7: ThinkingIndicator — the bouncing-ball + scanning-verb pulse
// shown while a turn is running. Three modes of dot animation:
// sweep (ball bounces back and forth across 5 positions), blink
// (single center dot pulses), burst (faster sweep, no every-other
// frame slowdown). Mode transitions are randomised so the
// indicator doesn't lock into a hypnotic loop.
//
// Visual:
//   ●∙∙∙∙  Forging…
//   ∙●∙∙∙  forGing…   ← yellow highlight scans the verb
//   ∙∙●∙∙  forGing…
//   ...
//
// Ported from the legacy Ink-based TUI; original at
// tui-react/src/components/ThinkingIndicator.tsx. Timer cadence
// preserved verbatim (sweep ~180-260ms, blink ~250-400ms, burst
// ~50-80ms) so the visual rhythm matches what operators are used to.

import { type JSX, createSignal, createMemo, onCleanup, onMount } from "solid-js";
import { useTheme } from "../theme-context.js";

const VERBS = [
  "Forging",
  "Weaving",
  "Casting",
  "Shaping",
  "Tracing",
  "Probing",
  "Binding",
  "Honing",
  "Carving",
  "Fusing",
] as const;

const DOT = "●";
const SMALL = "∙";
const WIDTH = 5;

type Mode = "sweep" | "blink" | "burst";

interface AnimState {
  mode: Mode;
  pos: number;
  dir: 1 | -1;
  counter: number;
}

export function ThinkingIndicator(): JSX.Element {
  const theme = useTheme();
  const [dotFrame, setDotFrame] = createSignal(SMALL.repeat(WIDTH));
  const [verbTick, setVerbTick] = createSignal(0);

  const state: AnimState = {
    mode: "sweep",
    pos: 0,
    dir: 1,
    counter: 0,
  };

  function renderDots(): void {
    const dots = new Array<string>(WIDTH).fill(SMALL);

    if (state.mode === "blink") {
      const mid = Math.floor(WIDTH / 2);
      if (state.counter % 2 === 0) dots[mid] = DOT;
      state.counter += 1;
      if (state.counter > 4 + Math.floor(Math.random() * 4)) {
        state.mode = "sweep";
        state.counter = 0;
        state.pos = mid;
      }
    } else if (state.mode === "burst") {
      dots[state.pos] = DOT;
      state.pos += state.dir;
      if (state.pos >= WIDTH - 1) {
        state.pos = WIDTH - 1;
        state.dir = -1;
      }
      if (state.pos <= 0) {
        state.pos = 0;
        state.dir = 1;
      }
      state.counter += 1;
      if (state.counter > 8 + Math.floor(Math.random() * 8)) {
        state.mode = "sweep";
        state.counter = 0;
      }
    } else {
      // sweep — every other tick advances the ball so the motion
      // reads at a comfortable pace
      dots[state.pos] = DOT;
      state.counter += 1;
      if (state.counter % 2 === 0) {
        state.pos += state.dir;
        if (state.pos >= WIDTH - 1) {
          state.pos = WIDTH - 1;
          state.dir = -1;
        }
        if (state.pos <= 0) {
          state.pos = 0;
          state.dir = 1;
        }
      }
      // Random transition into a different mode so the animation
      // doesn't lock into a single rhythm
      if (state.counter > 8 && Math.random() < 0.08) {
        state.mode = "blink";
        state.counter = 0;
        state.pos = Math.floor(WIDTH / 2);
      } else if (state.counter > 12 && Math.random() < 0.05) {
        state.mode = "burst";
        state.counter = 0;
      }
    }

    setDotFrame(dots.join(""));
  }

  onMount(() => {
    renderDots();
    let timer: ReturnType<typeof setTimeout> | null = null;
    const tick = (): void => {
      renderDots();
      const ms =
        state.mode === "burst"
          ? 50 + Math.random() * 30
          : state.mode === "blink"
            ? 250 + Math.random() * 150
            : 180 + Math.random() * 80;
      timer = setTimeout(tick, ms);
    };
    timer = setTimeout(tick, 200);
    onCleanup(() => {
      if (timer !== null) clearTimeout(timer);
    });
  });

  onMount(() => {
    const id = setInterval(() => setVerbTick((t) => t + 1), 100);
    onCleanup(() => clearInterval(id));
  });

  // Verb selection rotates every 10 seconds so a long-running turn
  // surfaces the breadth of personality verbs without churning.
  const verb = createMemo(() => {
    const idx = Math.floor(Date.now() / 10000) % VERBS.length;
    return VERBS[idx] ?? "Thinking";
  });

  // The highlight character cycles left → right → left across the
  // verb. cycleLen = 2 * wordLen; positions 0..wordLen-1 are
  // forward, the rest mirror back.
  const highlightPos = createMemo(() => {
    const w = verb().length;
    const cycle = w * 2;
    const pos = verbTick() % cycle;
    return pos < w ? pos : cycle - pos - 1;
  });

  const dotColor = (): string => {
    const v = theme()["warning"];
    return typeof v === "string" ? v : theme().fg;
  };
  const dimColor = (): string => {
    const v = theme()["fgDim"];
    return typeof v === "string" ? v : theme().fg;
  };
  const highlightColor = (): string => {
    const v = theme()["caret"];
    return typeof v === "string" ? v : theme().fg;
  };

  return (
    <text>
      <span style={{ fg: dotColor() }}>{dotFrame()} </span>
      {verb()
        .split("")
        .map((ch, i) =>
          i === highlightPos() ? (
            <b style={{ fg: highlightColor() }}>{ch}</b>
          ) : (
            <span style={{ fg: dimColor() }}>{ch}</span>
          ),
        )}
      <span style={{ fg: dimColor() }}>{"…"}</span>
    </text>
  );
}
