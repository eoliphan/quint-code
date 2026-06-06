package mcpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// Client wraps one MCP server child process speaking JSON-RPC 2.0
// over stdio. Newline-delimited JSON (one message per line) is the
// transport — Content-Length framing isn't required by the spec for
// stdio and the major reference clients (Claude Code, Codex) use
// NDJSON.
//
// Lifecycle: New -> Start (spawns process + does the initialize +
// tools/list handshake) -> CallTool repeatedly -> Close. Concurrent
// CallTool invocations are safe; each gets its own request id and
// blocks on a per-request channel.
type Client struct {
	name string
	cfg  ServerConfig

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  io.ReadCloser
	closing atomic.Bool

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan jsonrpcResp

	tools []ToolMeta
}

// ToolMeta is the publicly-readable shape of a tool advertised by
// the MCP server. inputSchema is the raw JSON schema the LLM sees.
type ToolMeta struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type jsonrpcReq struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// New constructs a client with no live process. Call Start to spawn.
func New(name string, cfg ServerConfig) *Client {
	return &Client{
		name:    name,
		cfg:     cfg,
		pending: map[int64]chan jsonrpcResp{},
	}
}

// Name returns the configured server name. Used as a prefix for the
// advertised tool names so two servers can't collide on a tool name.
func (c *Client) Name() string { return c.name }

// Tools returns the cached tool metadata produced by the initialize
// + tools/list handshake. Safe to call after Start returns.
func (c *Client) Tools() []ToolMeta { return c.tools }

// Start spawns the server process, performs the initialize
// handshake, and caches the tool list. Times out after `timeout` if
// the server doesn't respond.
func (c *Client) Start(ctx context.Context, timeout time.Duration) error {
	cmd := exec.CommandContext(ctx, c.cfg.Command, c.cfg.Args...)
	cmd.Env = mergeEnv(c.cfg.Env)
	if c.cfg.CWD != "" {
		cmd.Dir = c.cfg.CWD
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcpclient %s: stdin: %w", c.name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcpclient %s: stdout: %w", c.name, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("mcpclient %s: stderr: %w", c.name, err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcpclient %s: spawn: %w", c.name, err)
	}
	c.cmd = cmd
	c.stdin = stdin
	c.stdout = bufio.NewReader(stdout)
	c.stderr = stderr

	// Drain stderr in the background so the server doesn't block on
	// a full pipe buffer. Lines go to our own stderr prefixed with
	// the server name for easy diagnosis.
	go c.drainStderr()

	// Reader loop: parse NDJSON, dispatch responses to waiting
	// CallTool goroutines via the per-request channel.
	go c.readLoop()

	// Initialize handshake.
	initParams := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "haft", "version": "v8.0"},
	}
	if _, err := c.call(ctx, timeout, "initialize", initParams); err != nil {
		_ = c.Close()
		return fmt.Errorf("mcpclient %s: initialize: %w", c.name, err)
	}
	// Initialized notification (no id, no response expected).
	if err := c.notify("notifications/initialized", nil); err != nil {
		// Best effort.
		fmt.Fprintf(os.Stderr, "mcpclient %s: initialized notify: %v\n", c.name, err)
	}

	// tools/list.
	raw, err := c.call(ctx, timeout, "tools/list", map[string]any{})
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("mcpclient %s: tools/list: %w", c.name, err)
	}
	var listResp struct {
		Tools []ToolMeta `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listResp); err != nil {
		_ = c.Close()
		return fmt.Errorf("mcpclient %s: tools/list decode: %w", c.name, err)
	}
	c.tools = listResp.Tools
	return nil
}

// CallTool invokes one of the server's tools by name with the JSON
// argument blob. The returned string is the concatenated text of all
// `content` items the server returned; isError tracks the server's
// `isError` flag. Errors at the transport level surface as a
// non-nil err.
func (c *Client) CallTool(ctx context.Context, name string, argsJSON string) (string, bool, error) {
	if c.closing.Load() {
		return "", true, fmt.Errorf("mcpclient %s: closed", c.name)
	}
	var args any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", true, fmt.Errorf("mcpclient %s: args decode: %w", c.name, err)
		}
	} else {
		args = map[string]any{}
	}
	raw, err := c.call(ctx, 30*time.Second, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", true, err
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
		IsError bool `json:"isError,omitempty"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", true, fmt.Errorf("mcpclient %s: response decode: %w", c.name, err)
	}
	out := ""
	for _, c := range resp.Content {
		if c.Type == "text" && c.Text != "" {
			if out != "" {
				out += "\n"
			}
			out += c.Text
		}
	}
	return out, resp.IsError, nil
}

// Close stops the server process. Idempotent.
func (c *Client) Close() error {
	if !c.closing.CompareAndSwap(false, true) {
		return nil
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	return nil
}

// --- internal RPC machinery ---

func (c *Client) call(ctx context.Context, timeout time.Duration, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan jsonrpcResp, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := jsonrpcReq{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(append(body, '\n')); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout after %s on %s", timeout, method)
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc %s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *Client) notify(method string, params any) error {
	req := jsonrpcReq{JSONRPC: "2.0", Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(body, '\n'))
	return err
}

func (c *Client) readLoop() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			if !c.closing.Load() {
				fmt.Fprintf(os.Stderr, "mcpclient %s: read: %v\n", c.name, err)
			}
			return
		}
		if len(line) == 0 {
			continue
		}
		var resp jsonrpcResp
		if err := json.Unmarshal(line, &resp); err != nil {
			// Could be a server-side notification (no id, no result).
			// Drop silently — we don't subscribe to any notifications.
			continue
		}
		if resp.ID == 0 {
			// Server-side notification.
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		c.mu.Unlock()
		if !ok {
			continue
		}
		select {
		case ch <- resp:
		default:
		}
	}
}

func (c *Client) drainStderr() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		fmt.Fprintf(os.Stderr, "[mcp:%s] %s\n", c.name, scanner.Text())
	}
}

func mergeEnv(extra map[string]string) []string {
	base := os.Environ()
	for k, v := range extra {
		base = append(base, k+"="+v)
	}
	return base
}
