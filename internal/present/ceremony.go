package present

import (
	"fmt"
	"strings"

	"github.com/m0n0x41d/haft/internal/ceremony"
)

// CeremonyResponse renders the ceremony floor's recommendation: the suggested
// mode, the risk band, the signals behind it (transparency), and — when the
// floor was blind — the single escalation question. It is ADVICE: the operator
// binds the mode in one step (Transformer Mandate — never auto-applied).
func CeremonyResponse(rec ceremony.Recommendation, files []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Recommended ceremony: %s  _(risk: %s)_\n\n", rec.Mode, bandWord(rec.Band))

	if len(files) > 0 {
		fmt.Fprintf(&b, "For: %s\n\n", strings.Join(files, ", "))
	}

	if len(rec.Signals) > 0 {
		b.WriteString("Why:\n")
		for _, s := range rec.Signals {
			fmt.Fprintf(&b, "- %s _(%s, %s)_\n", s.Detail, s.Source, bandWord(s.Band))
		}
		b.WriteString("\n")
	}

	if rec.Ask {
		b.WriteString("### ⚠ Floor is blind here — one question before you proceed\n")
		b.WriteString(rec.Question + "\n\n")
	}

	switch rec.Band {
	case ceremony.RiskHigh:
		b.WriteString("This is a HIGH-risk change (irreversible / security / privacy / public-API / data) — it is never routed tactical. ")
	case ceremony.RiskUnknown:
		b.WriteString("Defaulting upward to standard until you answer — never silently tactical when the risk is undetermined. ")
	}
	b.WriteString("This is a recommendation. You choose the mode and bind it in one step (Transformer Mandate — haft never auto-selects).\n")
	return b.String()
}

func bandWord(b ceremony.RiskBand) string {
	switch b {
	case ceremony.RiskHigh:
		return "high"
	case ceremony.RiskUnknown:
		return "undetermined"
	case ceremony.RiskElevated:
		return "elevated"
	default:
		return "low"
	}
}
