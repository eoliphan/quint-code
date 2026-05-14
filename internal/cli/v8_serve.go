package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentserver"
	"github.com/m0n0x41d/haft/internal/agentstore"
)

// v8Cmd is the umbrella for v8-stack commands. Bare `haft v8` prints
// help; subcommands live underneath. Today only `serve` exists.
// Production `haft agent` still routes to the legacy TUI; the v8 default
// flip lands in t15 cobra cut-over.
var v8Cmd = &cobra.Command{
	Use:   "v8",
	Short: "v8 agent stack commands (smoke / dogfood)",
	Long: `v8 hosts the new agent stack — algebraic types (agentcore), wire
protocol (agentproto), append-only journal (agentstore), HTTP+SSE server
(agentserver), turn driver (agentdriver).

v8.0 ships the TUI process spawned via haft agent (cobra cut-over in t15).
This subcommand exposes the backend stack standalone so operators and TUI
dev work can curl /event/global, POST sessions, and exercise the wire
protocol without the legacy default-route flip.`,
}

var v8ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the v8 agentserver standalone (no TUI, no LLM by default)",
	Long: `Boot the v8 agent stack as a standalone HTTP+SSE server bound to
127.0.0.1:0 (or --addr). Prints a one-line JSON startup record to stdout
once the listener is ready — capture .port to point curl / TUI dev at it.

By default uses StoreDispatcher (session lifecycle only, no turns drive).
Pass --with-driver to wire in a real provider — placeholder flag for the
later slice that adapts internal/provider to the v8 Driver shape.

Honors SIGINT / SIGTERM with graceful shutdown.`,
	RunE: runV8Serve,
}

var (
	v8ServeAddr      string
	v8ServeStoreRoot string
	v8ServeWithDrv   bool
)

func init() {
	v8ServeCmd.Flags().StringVar(&v8ServeAddr, "addr", "127.0.0.1:0", "TCP bind address (127.0.0.1:0 picks a free port)")
	v8ServeCmd.Flags().StringVar(&v8ServeStoreRoot, "store-root", ".haft/agent-store", "Per-session journal root directory (created if absent)")
	v8ServeCmd.Flags().BoolVar(&v8ServeWithDrv, "with-driver", false, "Wire DriverDispatcher (real provider stack) instead of StoreDispatcher (smoke only). NOT IMPLEMENTED yet; flag reserved.")
	v8Cmd.AddCommand(v8ServeCmd)
	rootCmd.AddCommand(v8Cmd)
}

func runV8Serve(_ *cobra.Command, _ []string) error {
	if v8ServeWithDrv {
		return errors.New("--with-driver: production wiring not implemented yet; ship via the cobra cut-over (t15+)")
	}
	if err := os.MkdirAll(v8ServeStoreRoot, 0o755); err != nil {
		return fmt.Errorf("create store root: %w", err)
	}
	storeRoot, err := filepath.Abs(v8ServeStoreRoot)
	if err != nil {
		return fmt.Errorf("resolve store root: %w", err)
	}

	store, err := agentstore.NewStore(storeRoot)
	if err != nil {
		return fmt.Errorf("open agentstore: %w", err)
	}
	defer func() { _ = store.Close() }()

	var idCounter atomic.Int64
	dispatcher := &agentserver.StoreDispatcher{
		Store: store,
		IDGen: func() agentcore.SessionID {
			// Production uses uuid.NewString() — keep deterministic
			// fallback if the random reader is unavailable on this host.
			id := uuid.NewString()
			if id == "" {
				id = fmt.Sprintf("sess-%d", idCounter.Add(1))
			}
			return agentcore.SessionID(id)
		},
		Now: time.Now,
	}

	srv := agentserver.NewServer(v8ServeAddr, store, dispatcher, nil)
	srv.AuthStatus = agentserver.AuthStatusFunc(realAuthStatus)

	boundAddr, errCh, err := srv.Start()
	if err != nil {
		return fmt.Errorf("start agentserver: %w", err)
	}
	startup := map[string]any{
		"port":       portFromAddr(boundAddr),
		"addr":       boundAddr,
		"store_root": storeRoot,
		"driver":     "store",
	}
	if data, err := json.Marshal(startup); err == nil {
		fmt.Println(string(data))
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		fmt.Fprintf(os.Stderr, "v8 serve: received %s, shutting down\n", sig)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

// portFromAddr extracts the TCP port from a "host:port" listen address.
// Returns 0 on parse failure rather than erroring; the startup JSON is
// best-effort metadata, not a transport contract.
func portFromAddr(addr string) int {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			var port int
			if _, err := fmt.Sscanf(addr[i+1:], "%d", &port); err == nil {
				return port
			}
			break
		}
	}
	return 0
}
