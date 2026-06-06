package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/m0n0x41d/haft/internal/embedding"
	"github.com/m0n0x41d/haft/internal/fpf"
	_ "modernc.org/sqlite"
)

// verifyIndex is the CI guard: the FPF index is baked locally (heavy CPU
// inference unfit for runners) and committed, so CI must check the committed
// fpf.db is fresh (matches the submodule SHA) and carries baked vectors — never
// re-bake. Fails loudly so a maintainer who forgot `task fpf-refresh` cannot
// ship a stale or vectorless index.
func verifyIndex(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: indexer -verify <fpf.db> <expected-fpf-commit-sha>")
	}
	dbPath, expectedSHA := args[0], strings.TrimSpace(args[1])

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()

	commit, err := fpf.GetSpecMeta(db, "fpf_commit")
	if err != nil {
		return fmt.Errorf("read fpf_commit meta: %w", err)
	}
	if strings.TrimSpace(commit) != expectedSHA {
		return fmt.Errorf("fpf.db is STALE: meta fpf_commit=%q but submodule HEAD=%q — run `task fpf-refresh` and commit the result", commit, expectedSHA)
	}

	schemaVersion, err := fpf.GetSpecMeta(db, "schema_version")
	if err != nil {
		return fmt.Errorf("read schema_version meta: %w", err)
	}
	if strings.TrimSpace(schemaVersion) != fpf.SpecIndexSchemaVersion {
		return fmt.Errorf("fpf.db schema_version=%q but code expects %q — run `task fpf-refresh` and commit the result", schemaVersion, fpf.SpecIndexSchemaVersion)
	}

	_, _, _, count, err := fpf.SpecEmbeddingContract(db)
	if err != nil {
		return fmt.Errorf("read embedding contract: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("fpf.db has NO baked vectors — run `task fpf-refresh` with the embedding sidecar installed, then commit")
	}

	fmt.Printf("fpf.db OK: commit %s, %d baked section vectors\n", commit[:min(8, len(commit))], count)
	return nil
}

const routeArtifactPath = "internal/fpf/fpf-routes.json"
const patternsDir = "internal/fpf/patterns"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: indexer <FPF-Spec.md> [output.db] [fpf-commit-sha]  |  indexer -verify <fpf.db> <expected-sha>")
	}
	if os.Args[1] == "-verify" {
		return verifyIndex(os.Args[2:])
	}

	specPath := os.Args[1]
	dbPath := filepath.Join("internal", "cli", "fpf.db")
	if len(os.Args) >= 3 {
		dbPath = os.Args[2]
	}
	commitSHA := ""
	if len(os.Args) >= 4 {
		commitSHA = os.Args[3]
	}

	return buildIndex(specPath, dbPath, commitSHA, routeArtifactPath)
}

func buildIndex(specPath, dbPath, commitSHA, routePath string) error {
	corpus, err := fpf.LoadSpecIndexCorpus(specPath)
	if err != nil {
		return fmt.Errorf("load production spec corpus: %w", err)
	}

	routes, err := fpf.LoadRoutes(routePath)
	if err != nil {
		return fmt.Errorf("loading routes: %w", err)
	}

	// Load compiled pattern files and merge into the corpus
	patternChunks, err := fpf.LoadPatternChunks(patternsDir)
	if err != nil {
		return fmt.Errorf("loading patterns: %w", err)
	}

	allChunks := make([]fpf.SpecChunk, 0, len(corpus.Indexed)+len(patternChunks))
	allChunks = append(allChunks, corpus.Indexed...)
	allChunks = append(allChunks, patternChunks...)

	if err := fpf.BuildSpecIndex(dbPath, allChunks, routes); err != nil {
		return fmt.Errorf("building index: %w", err)
	}

	// build_time is the FPF spec COMMIT's date, not wall-clock time, so the
	// index is byte-reproducible: a given submodule SHA always yields the same
	// fpf.db (committed == rebuild; every release-matrix platform ships identical
	// bytes). Wall-clock time.Now() would drift every build.
	metadata := buildSpecIndexMetadata(specPath, len(allChunks), commitSHA, resolveSpecBuildTime(commitSHA, specPath))
	if err := fpf.SetSpecMetaEntries(dbPath, metadata); err != nil {
		return fmt.Errorf("setting meta: %w", err)
	}

	baked, err := bakeSpecEmbeddings(dbPath)
	if err != nil {
		return fmt.Errorf("bake embeddings: %w", err)
	}

	fmt.Printf("Indexed %d chunks (%d spec + %d patterns) into %s; baked %d section vectors\n",
		len(allChunks), len(corpus.Indexed), len(patternChunks), dbPath, baked)
	return nil
}

// bakeSpecEmbeddings embeds every section into fpf_embeddings via the local
// sidecar (MRL-256). When haft-embed is absent the index is still valid (empty
// vectors table) and the runtime degrades to FTS — a missing sidecar must never
// fail a build. Returns the number of vectors baked.
func bakeSpecEmbeddings(dbPath string) (int, error) {
	emb, err := embedding.New(embedding.Config{Provider: embedding.ProviderLocal, Model: os.Getenv("HAFT_EMBED_MODEL"), Dim: specEmbeddingBakeDim()})
	if err != nil {
		if embedding.Degraded(err) {
			fmt.Println("haft-embed absent at index time — fpf.db built WITHOUT vectors (runtime degrades to FTS)")
			return 0, nil
		}
		return 0, fmt.Errorf("start embedder: %w", err)
	}
	defer func() { _ = emb.Close() }()

	ctx := context.Background()
	return fpf.BakeSpecEmbeddings(ctx, dbPath, indexEmbedderAdapter{embedder: emb}, bakeScopeFromEnv())
}

// specEmbeddingBakeDim is the MRL truncation target for the bake. Default 256
// (the shipped contract); HAFT_FPF_BAKE_DIM overrides — 0 means the model's
// native width (use for non-MRL models like bge where truncation hurts).
func specEmbeddingBakeDim() int {
	if v := strings.TrimSpace(os.Getenv("HAFT_FPF_BAKE_DIM")); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			return d
		}
	}
	return 256
}

// bakeScopeFromEnv selects the embedding scope. Default is the full corpus;
// HAFT_FPF_BAKE_SCOPE=patterns restricts the bake to the 66 compiled pattern
// cards (seconds vs ~tens of minutes) — used to measure whether prose sections
// earn their place before committing the scope.
func bakeScopeFromEnv() fpf.SpecEmbeddingScope {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("HAFT_FPF_BAKE_SCOPE")), "patterns") {
		return fpf.ScopePatternCards
	}
	return fpf.ScopeAllSections
}

// indexEmbedderAdapter bridges the local embedding.Embedder to the provider-free
// fpf.SemanticEmbedder port (mirror of internal/embedding's openAIAdapter). It
// embeds sections in the document role.
type indexEmbedderAdapter struct {
	embedder embedding.Embedder
}

func (a indexEmbedderAdapter) Descriptor() fpf.SemanticEmbedderDescriptor {
	d := a.embedder.Descriptor()
	return fpf.SemanticEmbedderDescriptor{Provider: d.Provider, Model: d.Model, Dimensions: d.Dimensions}
}

func (a indexEmbedderAdapter) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	return a.embedder.Embed(ctx, embedding.RoleDocument, texts)
}

func buildSpecIndexMetadata(specPath string, indexedSections int, explicitCommit string, buildTime time.Time) map[string]string {
	return map[string]string{
		"fpf_commit":       resolveSpecCommit(explicitCommit, specPath),
		"indexed_sections": fmt.Sprintf("%d", indexedSections),
		"build_time":       buildTime.UTC().Format(time.RFC3339),
		"spec_path":        filepath.Clean(specPath),
		"schema_version":   fpf.SpecIndexSchemaVersion,
	}
}

func resolveSpecCommit(explicitCommit, specPath string) string {
	commit := strings.TrimSpace(explicitCommit)
	if commit != "" {
		return commit
	}

	return detectSpecCommit(specPath)
}

// resolveSpecBuildTime returns the committer date of the FPF spec commit, so the
// index build is deterministic. Falls back to the Unix epoch when git/the commit
// is unavailable — still deterministic (never wall-clock).
func resolveSpecBuildTime(commitSHA, specPath string) time.Time {
	epoch := time.Unix(0, 0).UTC()
	gitDir, err := specGitLookupDir(specPath)
	if err != nil {
		return epoch
	}
	ref, ok := cleanSpecCommitRef(commitSHA)
	if !ok {
		return epoch
	}
	cmd := exec.Command("git", "show", "-s", "--format=%cI", ref)
	cmd.Dir = gitDir
	output, err := cmd.Output()
	if err != nil {
		return epoch
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(string(output)))
	if err != nil {
		return epoch
	}
	return parsed.UTC()
}

func cleanSpecCommitRef(commitSHA string) (string, bool) {
	ref := strings.TrimSpace(commitSHA)
	if ref == "" {
		return "HEAD", true
	}
	if len(ref) != 40 {
		return "", false
	}
	for _, r := range ref {
		if !isHexCommitRune(r) {
			return "", false
		}
	}
	return strings.ToLower(ref), true
}

func isHexCommitRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

func detectSpecCommit(specPath string) string {
	gitDir, err := specGitLookupDir(specPath)
	if err != nil {
		return ""
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = gitDir

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

func specGitLookupDir(specPath string) (string, error) {
	absPath, err := filepath.Abs(specPath)
	if err != nil {
		return "", err
	}

	return filepath.Dir(absPath), nil
}
