package ceremony

import (
	"os"
	"path/filepath"
	"strings"
)

// GovLookup returns the recorded governance facts for one touched file. Injected
// by the shell (serve.go) so this layer stays decoupled from the artifact store
// — and nil-safe: with no lookup the floor runs on path/content alone (it still
// honors every safety invariant; governance is an enhancement, not a crutch).
type GovLookup func(file string) GovFacts

// maxContentScanBytes caps the per-file content read so a huge generated file
// never stalls the at-query-time classification (local-first, no daemon).
const maxContentScanBytes = 2_000_000

// Recommend is the shell: it gathers high-risk CONTENT flags from the working
// tree and GOVERNANCE facts via the injected lookup, then runs the pure
// Classify. Reads happen at query time from the working tree + graph — no
// background process.
func Recommend(projectRoot string, files []string, gov GovLookup) Recommendation {
	in := Input{Files: files, ContentHighRisk: map[string]string{}}
	for _, f := range files {
		if tok := scanContent(projectRoot, f); tok != "" {
			in.ContentHighRisk[f] = tok
		}
	}
	if gov != nil {
		in.Gov = mergeGov(files, gov)
	}
	return Classify(in)
}

// scanContent reads a file and returns the first high-risk token it contains
// (e.g. a destructive SQL statement), or "" — so a change is flaggable by what
// it DOES even when its path looks innocuous. Best-effort: unreadable / oversize
// files contribute no content signal (the path floor still applies).
func scanContent(projectRoot, f string) string {
	b, err := os.ReadFile(filepath.Join(projectRoot, f))
	if err != nil || len(b) > maxContentScanBytes {
		return ""
	}
	up := strings.ToUpper(string(b))
	for _, tok := range HighRiskContentTokens {
		if strings.Contains(up, tok) {
			return tok
		}
	}
	return ""
}

// mergeGov collapses per-file governance facts into one: the STRONGEST (lowest)
// recorded reversibility wins, and the first non-empty blast-radius text is
// kept — so a single governed-as-risky file pulls the whole change up.
func mergeGov(files []string, gov GovLookup) GovFacts {
	out := GovFacts{}
	for _, f := range files {
		g := gov(f)
		if revRank(g.Reversibility) > revRank(out.Reversibility) {
			out.Reversibility = g.Reversibility
		}
		if out.BlastRadius == "" {
			out.BlastRadius = strings.TrimSpace(g.BlastRadius)
		}
	}
	return out
}

// revRank orders reversibility by RISK: low (hardest to undo) is strongest.
func revRank(r string) int {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "low":
		return 3
	case "medium":
		return 2
	case "high":
		return 1
	default:
		return 0
	}
}
