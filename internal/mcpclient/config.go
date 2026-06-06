// Package mcpclient — minimal MCP (Model Context Protocol) client.
//
// Speaks JSON-RPC 2.0 over stdio against externally-configured MCP
// server processes (the ones in ~/.claude.json mcpServers and the
// project-local .mcp.json). Surfaces the servers' tools through a
// tools.ToolExecutor adapter so they appear in the v8 driver's
// registry alongside the haft built-in + kernel tools.
//
// This is a minimal client — only the tools surface of MCP is
// implemented. resources / prompts / sampling are out of scope; the
// haft agent uses tools exclusively.
package mcpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ServerConfig is one entry in the mcpServers map of ~/.claude.json
// or .mcp.json. Mirrors the Claude Code / OpenCode / Codex shape.
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"` // "stdio" (default) or "sse" (not supported)
	// CWD is the working directory the server process inherits.
	// Empty falls through to the haft agent's cwd.
	CWD string `json:"cwd,omitempty"`
}

// LoadConfig reads ~/.claude.json + the project-local .mcp.json (if
// present in projectRoot) and merges them into a single name → server
// map. Project-local entries win on collision.
func LoadConfig(projectRoot string) (map[string]ServerConfig, error) {
	merged := map[string]ServerConfig{}

	// Global ~/.claude.json
	home, err := os.UserHomeDir()
	if err == nil {
		if servers, lerr := readClaudeConfig(filepath.Join(home, ".claude.json")); lerr == nil {
			for name, cfg := range servers {
				merged[name] = cfg
			}
		}
	}

	// Project-local .mcp.json
	if projectRoot != "" {
		local := filepath.Join(projectRoot, ".mcp.json")
		if servers, lerr := readMcpJSON(local); lerr == nil {
			for name, cfg := range servers {
				merged[name] = cfg
			}
		}
	}

	// Drop stdio-only constraint: SSE servers can't be spawned by
	// the v8 client yet. Warn-style: log to stderr but keep the
	// process running on the rest of the configured servers.
	for name, cfg := range merged {
		if cfg.Type != "" && cfg.Type != "stdio" {
			fmt.Fprintf(os.Stderr, "mcpclient: skipping %q — transport %q not supported (only stdio for v8.0)\n", name, cfg.Type)
			delete(merged, name)
		}
		if cfg.Command == "" {
			fmt.Fprintf(os.Stderr, "mcpclient: skipping %q — empty command\n", name)
			delete(merged, name)
		}
	}
	return merged, nil
}

// readClaudeConfig extracts the mcpServers map from a ~/.claude.json
// shaped file. The rest of the file is opaque to us.
func readClaudeConfig(path string) (map[string]ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		MCPServers map[string]ServerConfig `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	return envelope.MCPServers, nil
}

// readMcpJSON reads a standalone .mcp.json. The shape can be either
// the same envelope as .claude.json OR a bare map; accept both.
func readMcpJSON(path string) (map[string]ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Try envelope shape first.
	var envelope struct {
		MCPServers map[string]ServerConfig `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.MCPServers != nil {
		return envelope.MCPServers, nil
	}
	// Bare map.
	var bare map[string]ServerConfig
	if err := json.Unmarshal(data, &bare); err == nil {
		return bare, nil
	}
	return nil, fmt.Errorf("mcpclient: %s: not a recognized mcpServers shape", path)
}

// SortedNames returns the configured server names in a stable order
// so log lines + tool listings stay deterministic.
func SortedNames(cfgs map[string]ServerConfig) []string {
	out := make([]string, 0, len(cfgs))
	for k := range cfgs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
