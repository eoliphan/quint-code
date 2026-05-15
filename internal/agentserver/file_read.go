package agentserver

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// handleFileRead returns the contents of a file the operator
// referenced via @-mention in the input area. The path is resolved
// relative to the agent's cwd (the project root); .. traversal is
// rejected so a malicious or buggy TUI can't read /etc/passwd
// through the agent.
//
// Caps the response at 256 KB to keep prompt sizes sensible — long
// files surface as a notice instead of a wall of text.
func (s *Server) handleFileRead(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if rel == "" {
		http.Error(w, "missing ?path", http.StatusBadRequest)
		return
	}
	if strings.Contains(rel, "..") {
		http.Error(w, "path traversal rejected", http.StatusBadRequest)
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		http.Error(w, "cwd: "+err.Error(), http.StatusInternalServerError)
		return
	}
	abs := filepath.Join(cwd, rel)
	// Final safety: ensure resolved path is still inside cwd.
	if !strings.HasPrefix(abs+string(filepath.Separator), cwd+string(filepath.Separator)) && abs != cwd {
		http.Error(w, "path escapes project root", http.StatusBadRequest)
		return
	}
	stat, err := os.Stat(abs)
	if err != nil {
		http.Error(w, "file not found: "+rel, http.StatusNotFound)
		return
	}
	if stat.IsDir() {
		http.Error(w, "is a directory: "+rel, http.StatusBadRequest)
		return
	}
	const maxBytes = 256 * 1024
	data, err := os.ReadFile(abs)
	if err != nil {
		http.Error(w, "read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	truncated := false
	if len(data) > maxBytes {
		data = data[:maxBytes]
		truncated = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":      rel,
		"body":      string(data),
		"truncated": truncated,
		"size":      stat.Size(),
	})
}
