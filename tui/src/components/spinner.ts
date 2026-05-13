export const DOTS_FRAMES = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"] as const;
export const LINE_FRAMES = ["|", "/", "-", "\\"] as const;
export const ARROWS_FRAMES = ["←", "↖", "↑", "↗", "→", "↘", "↓", "↙"] as const;

export type SpinnerFrames = readonly string[];

/** advance returns the next frame index, wrapping at the array length.
 *  Pure — the caller owns the index counter. */
export function advance(currentIndex: number, frames: SpinnerFrames): number {
  return (currentIndex + 1) % frames.length;
}

/** frameAt returns the frame at the given index. Safe over array
 *  boundaries (modulo wrap) — never returns undefined for non-empty
 *  frames. */
export function frameAt(index: number, frames: SpinnerFrames): string {
  if (frames.length === 0) return "";
  const wrapped = ((index % frames.length) + frames.length) % frames.length;
  return frames[wrapped] ?? "";
}
