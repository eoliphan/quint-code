export type Theme = {
  /** Theme identifier — must be unique across the registered set. */
  name: "dark" | "light" | "high-contrast";

  /** Foreground colors. */
  fg: string;
  fgMuted: string;
  fgDim: string;

  /** Backgrounds. */
  bg: string;
  bgPanel: string;
  bgOverlay: string;

  /** Semantic colors used by routes/agent/session. */
  user: string;
  subAgent: string;
  assistant: string;

  /** Tool-use rendering. */
  toolName: string;
  toolArgs: string;
  toolResult: string;
  toolError: string;

  /** Permission prompt + alerts. */
  warning: string;
  success: string;
  danger: string;

  /** Streaming caret. */
  caret: string;
};

/** Token names — the closed key set. Listing them allows the test
 *  layer to assert every theme implements every key. */
export const THEME_TOKEN_KEYS = [
  "fg",
  "fgMuted",
  "fgDim",
  "bg",
  "bgPanel",
  "bgOverlay",
  "user",
  "subAgent",
  "assistant",
  "toolName",
  "toolArgs",
  "toolResult",
  "toolError",
  "warning",
  "success",
  "danger",
  "caret",
] as const satisfies readonly (keyof Theme)[];
