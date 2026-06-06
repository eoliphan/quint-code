package main

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestResolveSpecCommit(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "FPF-Spec.md")

	tests := []struct {
		name           string
		explicitCommit string
		want           string
	}{
		{
			name:           "empty",
			explicitCommit: "",
			want:           "",
		},
		{
			name:           "trimmed",
			explicitCommit: "  abc123  ",
			want:           "abc123",
		},
	}

	for _, tt := range tests {
		got := resolveSpecCommit(tt.explicitCommit, specPath)
		if got != tt.want {
			t.Fatalf("%s: resolveSpecCommit(%q) = %q, want %q", tt.name, tt.explicitCommit, got, tt.want)
		}
	}
}

func TestResolveSpecCommit_DetectsGitCommitFromSpecPath(t *testing.T) {
	repoDir := t.TempDir()
	specDir := filepath.Join(repoDir, "data", "FPF")
	specPath := filepath.Join(specDir, "FPF-Spec.md")

	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir spec dir: %v", err)
	}
	if err := os.WriteFile(specPath, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "init")

	want := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))
	got := resolveSpecCommit("", specPath)

	if got != want {
		t.Fatalf("resolveSpecCommit() = %q, want %q", got, want)
	}
}

func TestBuildSpecIndexMetadata_LeavesCommitEmptyOutsideGit(t *testing.T) {
	buildTime := time.Date(2026, time.March, 26, 12, 34, 56, 0, time.UTC)
	specPath := filepath.Join(t.TempDir(), "FPF-Spec.md")
	metadata := buildSpecIndexMetadata(specPath, 42, "", buildTime)

	if metadata["fpf_commit"] != "" {
		t.Fatalf("expected empty fpf_commit outside git, got %q", metadata["fpf_commit"])
	}
	if metadata["indexed_sections"] != "42" {
		t.Fatalf("unexpected indexed_sections %q", metadata["indexed_sections"])
	}
}

func TestVerifyIndexRejectsSchemaVersionMismatch(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "fpf.db")
	expectedCommit := "expected-sha"

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	stmts := []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE fpf_embeddings (provider TEXT NOT NULL, model TEXT NOT NULL, dim INTEGER NOT NULL)`,
		`INSERT INTO meta (key, value) VALUES ('fpf_commit', 'expected-sha')`,
		`INSERT INTO meta (key, value) VALUES ('schema_version', '3')`,
		`INSERT INTO fpf_embeddings (provider, model, dim) VALUES ('local', 'test', 256)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	err = verifyIndex([]string{dbPath, expectedCommit})
	if err == nil {
		t.Fatal("expected schema mismatch error")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("expected schema_version error, got %v", err)
	}
}

func TestBuildIndexRejectsVectorlessBake(t *testing.T) {
	tempDir := t.TempDir()
	specPath := filepath.Join(tempDir, "FPF-Spec.md")
	dbPath := filepath.Join(tempDir, "fpf.db")
	routePath := filepath.Join(tempDir, "routes.json")

	writeIndexerFixture(t, specPath, routePath)
	stubBakeSpecEmbeddings(t, 0, nil)

	err := buildIndex(specPath, dbPath, "", routePath)
	if err == nil {
		t.Fatal("expected vectorless bake to fail")
	}
	if !strings.Contains(err.Error(), "no section vectors baked") {
		t.Fatalf("expected no-vectors error, got %v", err)
	}
}

func TestCleanSpecCommitRef(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "empty defaults to HEAD", in: "", want: "HEAD", ok: true},
		{name: "lowercase sha", in: "0123456789abcdef0123456789abcdef01234567", want: "0123456789abcdef0123456789abcdef01234567", ok: true},
		{name: "uppercase sha normalizes", in: "ABCDEF0123456789ABCDEF0123456789ABCDEF01", want: "abcdef0123456789abcdef0123456789abcdef01", ok: true},
		{name: "option injection rejected", in: "--format=%H", ok: false},
		{name: "short ref rejected", in: "abc123", ok: false},
		{name: "pathspec rejected", in: "HEAD:cmd/indexer/main.go", ok: false},
	}

	for _, tt := range tests {
		got, ok := cleanSpecCommitRef(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("%s: cleanSpecCommitRef(%q) = %q, %v; want %q, %v", tt.name, tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}

	return string(output)
}

func TestBuildIndex_PreservesHeadingOnlyRootPatternShells(t *testing.T) {
	tempDir := t.TempDir()
	specPath := filepath.Join(tempDir, "FPF-Spec.md")
	dbPath := filepath.Join(tempDir, "fpf.db")
	routePath := filepath.Join(tempDir, "routes.json")

	writeIndexerFixture(t, specPath, routePath)
	stubBakeSpecEmbeddings(t, 1, nil)

	if err := buildIndex(specPath, dbPath, "", routePath); err != nil {
		t.Fatalf("buildIndex() error: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow(`SELECT count(*) FROM sections WHERE pattern_id = ?`, "A.17").Scan(&count)
	if err != nil {
		t.Fatalf("count A.17: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected A.17 root shell in built index, got count %d", count)
	}

	var aliasesJSON string
	err = db.QueryRow(`SELECT aliases_json FROM sections WHERE pattern_id = ?`, "A.17").Scan(&aliasesJSON)
	if err != nil {
		t.Fatalf("read aliases_json: %v", err)
	}
	if !strings.Contains(aliasesJSON, "A.CHR-NORM") {
		t.Fatalf("expected technical alias in aliases_json, got %q", aliasesJSON)
	}
}

func writeIndexerFixture(t *testing.T, specPath, routePath string) {
	t.Helper()

	spec := `## A.17 - Canonical “Characteristic” (A.CHR-NORM)

### A.17:1 - Context

To have reproducibility and explainability there is a need to measure various aspects of systems or knowledge artifacts.
`
	routes := `{"routes":[]}`

	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if err := os.WriteFile(routePath, []byte(routes), 0o644); err != nil {
		t.Fatalf("write routes: %v", err)
	}
}

func stubBakeSpecEmbeddings(t *testing.T, baked int, err error) {
	t.Helper()

	original := bakeSpecEmbeddingsFunc
	bakeSpecEmbeddingsFunc = func(string) (int, error) {
		return baked, err
	}
	t.Cleanup(func() {
		bakeSpecEmbeddingsFunc = original
	})
}
