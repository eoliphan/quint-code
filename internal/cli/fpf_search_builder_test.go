package cli

import (
	"github.com/m0n0x41d/haft/internal/fpf"
	"github.com/m0n0x41d/haft/internal/tools"
)

// buildFPFSearchFunc returns the FPFSearchFunc the test suite uses to
// exercise FPF spec lookups end-to-end. The function wraps embedded FPF
// retrieval via `internal/fpf`. In production the MCP server inlines the
// equivalent logic (see internal/cli/serve.go); this helper exists for
// tests that need a callable handle.
func buildFPFSearchFunc() tools.FPFSearchFunc {
	return func(request tools.FPFSearchRequest) (string, error) {
		retrieval, err := retrieveEmbeddedFPF(fpf.SpecRetrievalRequest{
			Query: request.Query,
			Limit: request.Limit,
			Full:  request.Full,
			Mode:  request.Mode,
		})
		if err != nil {
			return "", err
		}

		return formatAgentFPFSearchWithExplain(retrieval.Query, presentFPFRetrieval(retrieval.Results), request.Explain), nil
	}
}
