package codebase

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// tsAliasPattern is one `compilerOptions.paths` entry: a prefix/suffix around an
// optional `*` wildcard, with the baseUrl-relative replacement templates.
type tsAliasPattern struct {
	prefix      string
	suffix      string
	wildcard    bool
	replacement string // first target; `*` is filled with the captured portion
}

// tsProjectResolution is the project-level rewrite surface for non-relative module
// specifiers: tsconfig path aliases (`@/x` → `src/x`) and monorepo workspace
// packages (`@scope/ui/w` → `packages/ui/w`). Both turn a specifier that LOOKS
// external into a project-relative base the symbol resolver can match.
type tsProjectResolution struct {
	baseURL    string // project-relative dir the path aliases are rooted at
	aliases    []tsAliasPattern
	workspaces map[string]string // package name -> project-relative dir
}

// tsResolutionCache memoizes the project resolution per project root. Loading
// parses tsconfig + walks workspace dirs, so re-running it for every file in a
// scan would be wasteful; the cache is session-scoped (a rebuild is a fresh
// process, so a mid-session config edit needs a restart to take effect).
var tsResolutionCache sync.Map // projectRoot -> tsProjectResolution

func loadTSProjectResolution(projectRoot string) tsProjectResolution {
	if v, ok := tsResolutionCache.Load(projectRoot); ok {
		return v.(tsProjectResolution)
	}
	res := tsProjectResolution{baseURL: ".", workspaces: map[string]string{}}
	res.baseURL, res.aliases = loadTSAliases(projectRoot)
	res.workspaces = loadTSWorkspaces(projectRoot)
	tsResolutionCache.Store(projectRoot, res)
	return res
}

// loadTSAliases reads tsconfig.json (then jsconfig.json) and returns the
// baseUrl-relative path-alias patterns, most-specific first. `extends` chains and
// non-tsconfig bundler configs are out of scope for v1.
func loadTSAliases(projectRoot string) (string, []tsAliasPattern) {
	var raw []byte
	for _, name := range []string{"tsconfig.json", "jsconfig.json"} {
		if b, err := os.ReadFile(filepath.Join(projectRoot, name)); err == nil {
			raw = b
			break
		}
	}
	if raw == nil {
		return ".", nil
	}
	var cfg struct {
		CompilerOptions struct {
			BaseURL string              `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal([]byte(stripJSONC(string(raw))), &cfg); err != nil {
		return ".", nil
	}
	baseURL := cfg.CompilerOptions.BaseURL
	if baseURL == "" {
		baseURL = "."
	}

	var patterns []tsAliasPattern
	for key, targets := range cfg.CompilerOptions.Paths {
		if len(targets) == 0 {
			continue
		}
		prefix, suffix, wildcard := splitAliasKey(key)
		patterns = append(patterns, tsAliasPattern{
			prefix:      prefix,
			suffix:      suffix,
			wildcard:    wildcard,
			replacement: targets[0],
		})
	}
	// Most specific first: longer prefix wins, then literal (non-wildcard) before
	// wildcard so an exact alias is preferred over a `*` match.
	sort.SliceStable(patterns, func(i, j int) bool {
		if len(patterns[i].prefix) != len(patterns[j].prefix) {
			return len(patterns[i].prefix) > len(patterns[j].prefix)
		}
		return !patterns[i].wildcard && patterns[j].wildcard
	})
	return baseURL, patterns
}

// splitAliasKey breaks a `paths` key around its single `*` wildcard.
func splitAliasKey(key string) (prefix, suffix string, wildcard bool) {
	if i := strings.IndexByte(key, '*'); i >= 0 {
		return key[:i], key[i+1:], true
	}
	return key, "", false
}

// loadTSWorkspaces maps each monorepo member package's declared name to its
// project-relative directory, from `package.json` `workspaces` (array or
// `{packages:[…]}`). One level of trailing `/*` glob is expanded. Returns an
// empty map for single-package repos (the common case pays nothing).
func loadTSWorkspaces(projectRoot string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(filepath.Join(projectRoot, "package.json"))
	if err != nil {
		return out
	}
	var root struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal([]byte(stripJSONC(string(raw))), &root); err != nil || root.Workspaces == nil {
		return out
	}
	globs := parseWorkspaceGlobs(root.Workspaces)
	for _, g := range globs {
		for _, dir := range expandWorkspaceGlob(projectRoot, g) {
			if name := readPackageName(filepath.Join(projectRoot, dir)); name != "" {
				out[name] = filepath.ToSlash(dir)
			}
		}
	}
	return out
}

// parseWorkspaceGlobs accepts both the array form (`["packages/*"]`) and the
// object form (`{"packages": ["packages/*"]}`).
func parseWorkspaceGlobs(raw json.RawMessage) []string {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	var obj struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Packages
	}
	return nil
}

// expandWorkspaceGlob resolves one workspace glob to member directories. A
// trailing `/*` lists immediate subdirectories; any other entry is treated as a
// literal directory. `**` and deeper globs are out of scope for v1.
func expandWorkspaceGlob(projectRoot, glob string) []string {
	glob = strings.TrimSuffix(glob, "/")
	if !strings.HasSuffix(glob, "/*") {
		return []string{glob}
	}
	base := strings.TrimSuffix(glob, "/*")
	entries, err := os.ReadDir(filepath.Join(projectRoot, base))
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(base, e.Name()))
		}
	}
	return dirs
}

func readPackageName(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(stripJSONC(string(raw))), &pkg); err != nil {
		return ""
	}
	return pkg.Name
}

// resolveTSModuleSpecifier turns an import specifier into a project-relative base
// path (extension stripped). Relative specifiers join the importing dir; otherwise
// a tsconfig path alias, then a workspace package, is tried. Returns ok=false for
// a genuinely external dependency (no project node).
func resolveTSModuleSpecifier(raw, fileDir string, res tsProjectResolution) (string, bool) {
	if strings.HasPrefix(raw, ".") {
		return strings.TrimPrefix(filepath.ToSlash(filepath.Join(fileDir, raw)), "/"), true
	}
	if base, ok := resolveAlias(raw, res); ok {
		return base, true
	}
	if base, ok := resolveWorkspace(raw, res.workspaces); ok {
		return base, true
	}
	return "", false
}

func resolveAlias(raw string, res tsProjectResolution) (string, bool) {
	for _, p := range res.aliases {
		captured, ok := matchAlias(raw, p)
		if !ok {
			continue
		}
		repl := strings.ReplaceAll(p.replacement, "*", captured)
		base := filepath.Join(res.baseURL, repl)
		return strings.TrimPrefix(filepath.ToSlash(base), "./"), true
	}
	return "", false
}

// matchAlias reports whether raw matches the pattern and returns the `*` capture.
func matchAlias(raw string, p tsAliasPattern) (string, bool) {
	if !p.wildcard {
		return "", raw == p.prefix
	}
	if !strings.HasPrefix(raw, p.prefix) || !strings.HasSuffix(raw, p.suffix) {
		return "", false
	}
	if len(raw) < len(p.prefix)+len(p.suffix) {
		return "", false
	}
	return raw[len(p.prefix) : len(raw)-len(p.suffix)], true
}

// resolveWorkspace rewrites `@scope/pkg/sub` to `<member dir>/sub` using the
// longest matching package name.
func resolveWorkspace(raw string, workspaces map[string]string) (string, bool) {
	bestName, bestDir := "", ""
	for name, dir := range workspaces {
		if raw != name && !strings.HasPrefix(raw, name+"/") {
			continue
		}
		if len(name) > len(bestName) {
			bestName, bestDir = name, dir
		}
	}
	if bestName == "" {
		return "", false
	}
	sub := strings.TrimPrefix(raw[len(bestName):], "/")
	return strings.TrimPrefix(filepath.ToSlash(filepath.Join(bestDir, sub)), "/"), true
}

// stripJSONC removes `//` and `/* */` comments and trailing commas so a tsconfig
// or package.json carrying the usual editor annotations parses as plain JSON. A
// string-aware single pass — comment markers and commas inside string literals
// are preserved.
func stripJSONC(src string) string {
	out := make([]byte, 0, len(src))
	inString, escaped := false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && (src[i] != '*' || src[i+1] != '/') {
				i++
			}
			i++ // land on '/', loop's i++ steps past it
		case c == '}' || c == ']':
			out = dropTrailingComma(out)
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

// dropTrailingComma trims trailing whitespace and a single trailing comma from the
// emitted buffer — called when a `}`/`]` closer is reached.
func dropTrailingComma(out []byte) []byte {
	j := len(out)
	for j > 0 && (out[j-1] == ' ' || out[j-1] == '\t' || out[j-1] == '\n' || out[j-1] == '\r') {
		j--
	}
	if j > 0 && out[j-1] == ',' {
		return out[:j-1]
	}
	return out
}
