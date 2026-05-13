// haft: original
// Bun build script: bundle tui/src/main.ts into tui/dist/haft-tui.js as
// a single-file ES module. Production embed pipeline (t17) reads the
// bundle from this path via Go embed.FS and extracts on first run.
//
// Keep the build entry single-file deliberately — the v8 TUI process is
// spawned via Bun + HAFT_AGENT_PORT env from G6 (t16), not loaded as a
// library. Subentry bundling is unnecessary complexity at v8.0 scope.

import { build } from "bun";
import { createSolidTransformPlugin } from "@opentui/solid/bun-plugin";

const result = await build({
  entrypoints: ["./src/main.tsx"],
  outdir: "./dist",
  target: "bun",
  format: "esm",
  naming: "haft-tui.js",
  minify: false,
  plugins: [createSolidTransformPlugin()],
});

if (!result.success) {
  for (const log of result.logs) {
    console.error(log);
  }
  process.exit(1);
}

console.log(`built ${result.outputs.length} artifact(s) → tui/dist/haft-tui.js`);
