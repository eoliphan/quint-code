package fpf

import (
	"database/sql"
	"strings"
)

const SpecRetrievalModeSemantic = "semantic"

// SpecRetrievalModeFTS forces the deterministic FTS+tier path, bypassing the
// hybrid — a reproducible escape hatch.
const SpecRetrievalModeFTS = "fts"

// SpecRetrievalRequest captures deterministic spec retrieval controls for
// higher-level agent, CLI, and MCP surfaces.
type SpecRetrievalRequest struct {
	Query string
	Limit int
	Tier  string
	Full  bool
	Mode  string
	// HybridSearch, when set by the composition root, makes spec search hybrid
	// (FTS+graph fused with baked section vectors) BY DEFAULT. fpf stays
	// provider-free: the implementation lives in the shell (internal/cli) and is
	// injected as this func. nil => the deterministic SearchSpecWithOptions path.
	HybridSearch func(db *sql.DB, query string, limit int) ([]SpecSearchResult, error)
}

// SpecRetrievalResult is the structured retrieval response returned to shell
// layers before any surface-specific formatting is applied.
type SpecRetrievalResult struct {
	Query   string
	Results []SpecRetrievedSection
}

// SpecRetrievedSection is a presentation-ready FPF section hit with either
// snippet-sized content or the full section body.
type SpecRetrievedSection struct {
	PatternID string
	Heading   string
	Tier      string
	Reason    string
	Summary   string
	Content   string
}

// RetrieveSpec resolves deterministic FPF search hits and hydrates content for
// downstream CLI, MCP, and agent surfaces.
func RetrieveSpec(db *sql.DB, request SpecRetrievalRequest) (SpecRetrievalResult, error) {
	query := strings.TrimSpace(request.Query)
	searchResults, err := retrieveSpecSearchResults(db, query, request)
	if err != nil {
		return SpecRetrievalResult{}, err
	}

	results := make([]SpecRetrievedSection, 0, len(searchResults))
	for _, searchResult := range searchResults {
		results = append(results, hydrateRetrievedSection(db, searchResult, request.Full))
	}

	return SpecRetrievalResult{
		Query:   query,
		Results: results,
	}, nil
}

func retrieveSpecSearchResults(db *sql.DB, query string, request SpecRetrievalRequest) ([]SpecSearchResult, error) {
	deterministic := func() ([]SpecSearchResult, error) {
		// Only the tree sub-mode is a SearchSpecWithOptions mode; retrieval-layer
		// modes (fts, the retired semantic) map to the deterministic default.
		specMode := ""
		if strings.EqualFold(strings.TrimSpace(request.Mode), SpecSearchModeTree) {
			specMode = SpecSearchModeTree
		}
		return SearchSpecWithOptions(db, query, SpecSearchOptions{
			Limit: request.Limit,
			Tier:  request.Tier,
			Mode:  specMode,
		})
	}
	// Explicit fts mode forces the reproducible deterministic path.
	if normalizeSpecRetrievalMode(request.Mode) == SpecRetrievalModeFTS {
		return deterministic()
	}
	// Hybrid by default when the composition root wired it (degrade-safe — the
	// hybrid itself falls back to the deterministic path when the sidecar or
	// baked vectors are unavailable).
	if request.HybridSearch != nil {
		return request.HybridSearch(db, query, request.Limit)
	}
	return deterministic()
}

func normalizeSpecRetrievalMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case SpecRetrievalModeFTS:
		return SpecRetrievalModeFTS
	}
	return ""
}

func hydrateRetrievedSection(db *sql.DB, searchResult SpecSearchResult, full bool) SpecRetrievedSection {
	content := searchResult.Snippet
	if full {
		body, err := GetSpecSection(db, firstNonEmpty(searchResult.PatternID, searchResult.Heading))
		if err == nil {
			content = body
		}
	}

	return SpecRetrievedSection{
		PatternID: searchResult.PatternID,
		Heading:   searchResult.Heading,
		Tier:      searchResult.Tier,
		Reason:    searchResult.Reason,
		Summary:   searchResult.Summary,
		Content:   content,
	}
}
