package agentserver

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// commandsDir / skillsDir resolve the conventional locations of
// ~/.claude/commands and ~/.claude/skills. The TUI uses these to
// surface slash-command and skill pickers; the Go side keeps the
// filesystem walk out of the bun process. Both endpoints degrade
// gracefully — a missing directory returns an empty list rather
// than an error so the TUI never blocks on a stat.

func commandsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "commands")
}

func skillsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "skills")
}

// listMarkdownEntries returns a sorted list of (name, description)
// pairs for every .md file in root. description is the first
// non-empty non-frontmatter line of the file, trimmed to 120 chars.
// Subdirectories are walked one level deep and surfaced as their
// own entries (skills are organized as ~/.claude/skills/<slug>/
// directories containing skill.md).
type listing struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func listMarkdownEntries(root string) []listing {
	out := []listing{}
	if root == "" {
		return out
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			// Skill directory — read its skill.md if present.
			candidate := filepath.Join(root, e.Name(), "skill.md")
			if data, rerr := os.ReadFile(candidate); rerr == nil {
				out = append(out, listing{Name: e.Name(), Description: firstDescriptionLine(string(data))})
			}
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		full := filepath.Join(root, e.Name())
		data, rerr := os.ReadFile(full)
		if rerr != nil {
			out = append(out, listing{Name: name})
			continue
		}
		out = append(out, listing{Name: name, Description: firstDescriptionLine(string(data))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// firstDescriptionLine returns the first non-empty non-frontmatter
// non-header line of a markdown body, truncated to 120 chars. The
// first heading line (# Title) is preferred when present because
// haft slash commands lead with the H1 description.
func firstDescriptionLine(body string) string {
	lines := strings.Split(body, "\n")
	inFrontmatter := false
	heading := ""
	for i, ln := range lines {
		if i == 0 && strings.TrimSpace(ln) == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if strings.TrimSpace(ln) == "---" {
				inFrontmatter = false
			}
			continue
		}
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "# ") && heading == "" {
			heading = strings.TrimPrefix(t, "# ")
			continue
		}
		if strings.HasPrefix(t, "#") {
			continue
		}
		// First non-empty, non-heading line — return it (truncated).
		return truncate(t, 120)
	}
	if heading != "" {
		return truncate(heading, 120)
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func readMarkdownByName(root, name string) (string, bool) {
	if root == "" || name == "" {
		return "", false
	}
	// Direct .md
	flat := filepath.Join(root, name+".md")
	if data, err := os.ReadFile(flat); err == nil {
		return string(data), true
	}
	// Skill directory
	skill := filepath.Join(root, name, "skill.md")
	if data, err := os.ReadFile(skill); err == nil {
		return string(data), true
	}
	return "", false
}

// safeName rejects "../"-style traversal in the {name} path param.
func safeName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	for _, r := range name {
		if r == '/' || r == '\\' || r == '.' && false {
			return false
		}
	}
	return !strings.Contains(name, "..")
}

func (s *Server) handleCommandsList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"commands": listMarkdownEntries(commandsDir())})
}

func (s *Server) handleCommandGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !safeName(name) {
		http.Error(w, "invalid command name", http.StatusBadRequest)
		return
	}
	body, ok := readMarkdownByName(commandsDir(), name)
	if !ok {
		http.Error(w, "command not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "body": body})
}

func (s *Server) handleSkillsList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"skills": listMarkdownEntries(skillsDir())})
}

func (s *Server) handleSkillGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !safeName(name) {
		http.Error(w, "invalid skill name", http.StatusBadRequest)
		return
	}
	body, ok := readMarkdownByName(skillsDir(), name)
	if !ok {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "body": body})
}
