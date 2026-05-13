package cli

import (
	"fmt"
	"os"
	"sync"
)

// legacyTUIBannerOnce gates the deprecation banner so it prints exactly
// once per process lifetime. Repeated `haft agent` invocations within
// the same parent process (e.g. test runners that reuse the binary)
// see the banner on the first call only.
var legacyTUIBannerOnce sync.Once

const legacyTUIBannerText = `[DEPRECATED] You are running the legacy React+Ink haft TUI (tui-react/).
            This surface is scheduled for conditional removal in v8.1 once
            telemetry / release-thread feedback confirms <5% interactive usage.
            The v8 TUI (Bun + OpenTUI + SolidJS) is opt-in today:
                haft agent --v8
            See DR-v8-sunset (dec-20260513-v8-sunset-retroactive-366181da) for
            the full migration policy. Pass --legacy-tui to silence the flip
            once the default routes to v8.
`

// emitLegacyTUIBanner prints the deprecation notice once per process
// lifetime. Writes to stderr so it never mixes with stdout-driven
// JSON / pipe output the agent itself produces.
func emitLegacyTUIBanner() {
	legacyTUIBannerOnce.Do(func() {
		fmt.Fprint(os.Stderr, legacyTUIBannerText)
	})
}
