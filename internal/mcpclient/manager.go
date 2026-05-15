package mcpclient

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/m0n0x41d/haft/internal/agent"
)

// Manager owns the lifetime of every configured MCP server process.
// One Manager per agent run. The Register hook exposes each server's
// tools to a tools.Registry via the ToolExecutor interface.
type Manager struct {
	clients []*Client
}

// Start spawns every server listed in the config map. Failures on
// individual servers are logged but don't abort the whole batch —
// the agent runs with whichever servers succeeded.
func Start(ctx context.Context, cfgs map[string]ServerConfig) *Manager {
	m := &Manager{}
	for _, name := range SortedNames(cfgs) {
		cfg := cfgs[name]
		client := New(name, cfg)
		startCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := client.Start(startCtx, 10*time.Second)
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "mcpclient: %s start failed: %v\n", name, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "mcpclient: %s started with %d tools\n", name, len(client.Tools()))
		m.clients = append(m.clients, client)
	}
	return m
}

// Close shuts every server down. Idempotent. Returns the first error.
func (m *Manager) Close() error {
	var firstErr error
	for _, c := range m.clients {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return nil
}

// ToolExecutors returns one ToolExecutor per advertised tool across
// every running server. The tool name is prefixed with the server
// name + "__" so two servers with the same tool can't collide. The
// returned slice can be passed to (tools.Registry).Register
// directly.
type ToolExecutor struct {
	server     *Client
	prefixed   string
	underlying string
	schema     agent.ToolSchema
}

func (m *Manager) ToolExecutors() []*ToolExecutor {
	out := []*ToolExecutor{}
	for _, c := range m.clients {
		for _, t := range c.Tools() {
			prefixed := mangleName(c.Name(), t.Name)
			out = append(out, &ToolExecutor{
				server:     c,
				prefixed:   prefixed,
				underlying: t.Name,
				schema: agent.ToolSchema{
					Name:        prefixed,
					Description: t.Description,
					Parameters:  t.InputSchema,
				},
			})
		}
	}
	return out
}

// Name implements tools.ToolExecutor.
func (te *ToolExecutor) Name() string { return te.prefixed }

// Schema implements tools.ToolExecutor.
func (te *ToolExecutor) Schema() agent.ToolSchema { return te.schema }

// Execute implements tools.ToolExecutor. Dispatches to the right
// MCP server's tools/call and unwraps the response into the
// agent.ToolResult shape used by tools.Registry.
func (te *ToolExecutor) Execute(ctx context.Context, argsJSON string) (agent.ToolResult, error) {
	content, isErr, err := te.server.CallTool(ctx, te.underlying, argsJSON)
	if err != nil {
		return agent.ToolResult{}, err
	}
	display := content
	if isErr {
		display = "tool error: " + content
	}
	return agent.ToolResult{DisplayText: display}, nil
}

// mangleName joins the server prefix and the tool name. OpenAI's
// Responses API restricts tool names to /[a-zA-Z0-9_-]{1,64}/, so
// we replace non-conforming characters with "_" and truncate.
func mangleName(server, tool string) string {
	raw := server + "__" + tool
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		ok := (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '_' || ch == '-'
		if ok {
			out = append(out, ch)
		} else {
			out = append(out, '_')
		}
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return strings.TrimRight(string(out), "_")
}
