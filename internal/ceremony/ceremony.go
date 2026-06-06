// Package ceremony is the pure core of the ceremony auto-scaler
// (dec-20260604-ceremony-auto-scaler-floor-plus-ask-when-blind): it recommends a
// ceremony mode (tactical/standard/deep) proportioned to a change's RISK, with a
// deterministic safety floor.
//
// Hard invariants (from the decision), enforced here by construction:
//   - A detected HIGH-risk change (irreversible / security / privacy / public-API
//     / data) is NEVER recommended tactical — High maps to deep, full stop.
//   - Risk is detected from path/content patterns + recorded governance facts —
//     NEVER from call-graph fan-out, which is a weak, sometimes-inverted proxy
//     ("leaf" must never read as "safe": a one-line auth change has tiny fan-out
//     yet maximal risk).
//   - When the floor cannot classify a touched file (unrecognized kind, config,
//     non-Go) and governance is silent, it ESCALATES (ask) — it never silently
//     defaults to tactical.
//
// This file is the functional core: pure, deterministic, no IO. The shell
// (recommend.go) gathers file paths, content flags, and governance facts and
// calls Classify; the MCP/skill layer surfaces the recommendation as advice the
// operator overrides in one step (Transformer Mandate — never auto-binds).
package ceremony

import (
	"path/filepath"
	"strings"
)

// RiskBand is the detected risk level of a change, ordered low→high with a
// distinct Unknown for "the floor is blind here" (which escalates, not relaxes).
type RiskBand int

const (
	RiskLow      RiskBand = iota // recognized, no high-risk signal — tactical is safe
	RiskElevated                 // some risk (e.g. governed, medium reversibility) — standard
	RiskUnknown                  // floor cannot classify — ESCALATE (ask), never relax to tactical
	RiskHigh                     // irreversible / security / privacy / public-API / data — never tactical
)

// Mode is the recommended ceremony level (matches the kernel's mode field).
type Mode string

const (
	ModeTactical Mode = "tactical"
	ModeStandard Mode = "standard"
	ModeDeep     Mode = "deep"
)

// Signal is one reason the classifier fired — surfaced to the operator for
// transparency (watched, not optimized, per the decision).
type Signal struct {
	Source string // "path" | "content" | "governance"
	Detail string // the concrete match, e.g. "*.sql (data/migration)"
	Band   RiskBand
}

// GovFacts are the recorded governance facts for the touched files: the
// strongest (lowest) reversibility and any blast-radius text of a governing
// decision. Empty when no governing decision touches the files.
type GovFacts struct {
	Reversibility string // "low" | "medium" | "high" | ""
	BlastRadius   string // free-text blast_radius of a governing decision
}

// Input is everything the floor classifies from. Deliberately NO call-graph
// signal. ContentHighRisk lists files whose CONTENT matched a high-risk token
// (computed by the shell, e.g. a destructive SQL statement) — so a change is
// flaggable by content even when its path looks innocuous.
type Input struct {
	Files           []string
	ContentHighRisk map[string]string // path -> the high-risk token matched in its content
	Gov             GovFacts
}

// Recommendation is the floor's output: the band, the recommended mode, whether
// the caller must ASK (floor was blind), the signals behind it, and the single
// question to ask when blind.
type Recommendation struct {
	Band     RiskBand
	Mode     Mode
	Ask      bool
	Signals  []Signal
	Question string
}

// HighRiskContentTokens are the destructive/data-risk substrings the SHELL greps
// file content for; a match makes the file high-risk regardless of its path.
// Exported so the shell and tests share one list (no drift).
var HighRiskContentTokens = []string{
	// destructive SQL (matched case-insensitively — content is upper-cased)
	"DROP TABLE", "DROP COLUMN", "DROP DATABASE", "DROP SCHEMA", "DROP INDEX",
	"DELETE FROM", "TRUNCATE", "ALTER TABLE",
	// irreversible filesystem destruction (generic Go; ungoverned innocuous-path
	// deletions were a confirmed under-flag in the recall audit)
	"OS.REMOVEALL",
}

// Classify is the deterministic floor. Pure: same Input → same Recommendation.
func Classify(in Input) Recommendation {
	signals := make([]Signal, 0, len(in.Files)+2)

	for _, f := range in.Files {
		if tok, ok := in.ContentHighRisk[f]; ok {
			signals = append(signals, Signal{Source: "content", Detail: f + " contains " + tok, Band: RiskHigh})
			continue
		}
		signals = append(signals, pathSignal(f))
	}
	if g := governanceSignal(in.Gov); g != nil {
		signals = append(signals, *g)
	}

	band := aggregate(signals)
	return Recommendation{
		Band:     band,
		Mode:     modeFor(band),
		Ask:      band == RiskUnknown,
		Signals:  signals,
		Question: questionFor(band, signals),
	}
}

// pathSignal classifies one file by its path/extension. Unrecognized kinds yield
// RiskUnknown (escalate), NEVER RiskLow — only files the floor confidently knows
// are ordinary source are Low.
func pathSignal(f string) Signal {
	lower := strings.ToLower(f)
	base := strings.ToLower(filepath.Base(f))
	ext := strings.ToLower(filepath.Ext(f))

	switch {
	case ext == ".sql" || strings.Contains(lower, "migration"):
		return Signal{Source: "path", Detail: f + " (data / schema migration)", Band: RiskHigh}
	case segHit(lower, securityKeywords):
		return Signal{Source: "path", Detail: f + " (security / privacy surface)", Band: RiskHigh}
	case segHit(lower, financialAuthzKeywords):
		return Signal{Source: "path", Detail: f + " (irreversible financial / authorization surface)", Band: RiskHigh}
	case ext == ".proto" || strings.Contains(lower, "openapi") || strings.Contains(lower, "swagger"):
		return Signal{Source: "path", Detail: f + " (public API surface)", Band: RiskHigh}
	case ext == ".tf" || ext == ".tfvars" || base == "dockerfile" || strings.Contains(lower, "/k8s/") || strings.Contains(lower, "/helm/"):
		return Signal{Source: "path", Detail: f + " (infrastructure / deploy)", Band: RiskHigh}
	case segHit(lower, elevatedKeywords):
		return Signal{Source: "path", Detail: f + " (external effect — notify / publish / release / quota)", Band: RiskElevated}
	case isRecognizedOrdinary(ext):
		return Signal{Source: "path", Detail: f + " (ordinary source)", Band: RiskLow}
	default:
		// Config (.yaml/.json/.toml), non-Go code, or anything unrecognized:
		// the floor is BLIND — escalate, never assume low.
		return Signal{Source: "path", Detail: f + " (unrecognized — risk undetermined)", Band: RiskUnknown}
	}
}

// isRecognizedOrdinary reports the file kinds the floor can confidently treat as
// low-risk-by-default (still overridable by content/governance). Deliberately
// narrow — Go source + docs/tests — so unrecognized kinds escalate.
func isRecognizedOrdinary(ext string) bool {
	switch ext {
	case ".go", ".md", ".txt":
		return true
	default:
		return false
	}
}

// governanceSignal turns recorded governance facts into a risk signal: a
// governing decision with low reversibility, or a blast-radius mentioning a
// high-risk class, makes the change High; medium reversibility makes it Elevated.
func governanceSignal(g GovFacts) *Signal {
	rev := strings.ToLower(strings.TrimSpace(g.Reversibility))
	blast := strings.ToLower(g.BlastRadius)
	if rev == "low" || containsAny(blast, "irreversible", "security", "privacy", "data loss", "public api", "one-way") {
		return &Signal{Source: "governance", Detail: "governing decision: low reversibility / high-risk blast radius", Band: RiskHigh}
	}
	if rev == "medium" {
		return &Signal{Source: "governance", Detail: "governing decision: medium reversibility", Band: RiskElevated}
	}
	return nil
}

// aggregate picks the binding band with the safe priority High > Unknown >
// Elevated > Low: High always wins; an Unknown (blind) file escalates above
// Elevated/Low so it can never be masked by a lower recognized signal.
func aggregate(signals []Signal) RiskBand {
	band := RiskLow
	for _, s := range signals {
		if priority(s.Band) > priority(band) {
			band = s.Band
		}
	}
	return band
}

func priority(b RiskBand) int {
	switch b {
	case RiskHigh:
		return 3
	case RiskUnknown:
		return 2
	case RiskElevated:
		return 1
	default:
		return 0
	}
}

// modeFor maps a band to a recommended mode. The safety floor lives here: High
// is deep (NEVER tactical); Unknown defaults UPWARD to standard while the caller
// asks (never tactical); Elevated is standard; only Low is tactical.
func modeFor(b RiskBand) Mode {
	switch b {
	case RiskHigh:
		return ModeDeep
	case RiskUnknown, RiskElevated:
		return ModeStandard
	default:
		return ModeTactical
	}
}

func questionFor(band RiskBand, signals []Signal) string {
	if band != RiskUnknown {
		return ""
	}
	var unknown []string
	for _, s := range signals {
		if s.Band == RiskUnknown {
			unknown = append(unknown, s.Detail)
		}
	}
	return "The risk of these touched files can't be determined from path/content/governance (" +
		strings.Join(unknown, "; ") +
		"). Is any of them irreversible, security/privacy-sensitive, public-API, or data-affecting? " +
		"If yes → standard/deep; if no → tactical. (Recommending standard until you say.)"
}

// securityKeywords are matched against whole path SEGMENTS (a dir named "auth",
// a file "token.go") — exact-segment match, so "author.go" does NOT match "auth"
// (avoiding a false high-risk flag) while "internal/auth/session.go" does (the
// auth/ dir) and "token.go" does (segment "token").
var securityKeywords = map[string]bool{
	"auth": true, "authn": true, "authz": true, "secret": true, "secrets": true,
	"credential": true, "credentials": true, "token": true, "tokens": true,
	"password": true, "passwords": true, "crypto": true, "permission": true,
	"permissions": true, "rbac": true, "acl": true, "session": true, "sessions": true,
}

// financialAuthzKeywords mark irreversible financial / external-money effects
// and authorization-enforcement surfaces — High. (From the recall audit: these
// live in ordinary-path .go that the SQL/security detectors miss.)
var financialAuthzKeywords = map[string]bool{
	"billing": true, "payment": true, "payments": true, "charge": true, "charges": true,
	"invoice": true, "invoices": true, "invoicing": true, "payout": true, "payouts": true,
	"refund": true, "refunds": true, "subscription": true, "subscriptions": true,
	"scopeauth": true, "authorization": true, "authorize": true,
}

// elevatedKeywords mark softer external/irreversible effects — at least standard
// (never tactical), but not deep: notifications, publishes/releases, quotas.
var elevatedKeywords = map[string]bool{
	"notify": true, "notification": true, "notifications": true,
	"publish": true, "release": true, "releases": true, "quota": true, "quotas": true,
}

// segHit reports whether any whole path SEGMENT (or its extension-stripped form)
// is in set — exact-segment match, so "author.go" never matches "auth" while a
// dir named "billing" or a file "charge.go" does.
func segHit(lowerPath string, set map[string]bool) bool {
	for _, seg := range strings.Split(lowerPath, "/") {
		if set[seg] || set[strings.TrimSuffix(seg, filepath.Ext(seg))] {
			return true
		}
	}
	return false
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
