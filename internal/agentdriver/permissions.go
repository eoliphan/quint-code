package agentdriver

import (
	"context"
	"fmt"
	"sync"

	"github.com/m0n0x41d/haft/internal/agentcore"
)

// PermissionGate is the synchronization primitive that lets the driver
// (a goroutine running the turn loop) wait for the operator's decision
// (delivered by an HTTP handler in another goroutine).
//
// Driver flow:
//  1. driver registers a permission via Open(id, ...) — receives a chan
//  2. driver emits PermissionRequestedEvent on the sink (over SSE)
//  3. driver blocks on the chan
//  4. operator POSTs /permission/:id with their decision
//  5. HTTP handler calls Resolve(id, decision)
//  6. chan receives, driver continues
//
// Open and Resolve must be safe for concurrent use; the driver runs in
// one goroutine, HTTP handlers run in many.
type PermissionGate struct {
	mu      sync.Mutex
	pending map[agentcore.PermissionID]chan permissionResolution
}

type permissionResolution struct {
	decision agentcore.PermissionDecision
	reason   string
}

// NewPermissionGate constructs an empty gate.
func NewPermissionGate() *PermissionGate {
	return &PermissionGate{pending: map[agentcore.PermissionID]chan permissionResolution{}}
}

// ErrUnknownPermission is returned by Resolve when the gate has no record
// of the given permission ID. Indicates a programmer error or a stale
// request from the operator. Wraps agentcore.ErrPermissionNotFound so the
// HTTP layer can map both to 404 without importing this package.
var ErrUnknownPermission = fmt.Errorf("agentdriver: unknown permission id: %w", agentcore.ErrPermissionNotFound)

// ErrAlreadyResolved is returned by Resolve when the permission has
// already been decided. Idempotent rejection — the original decision is
// not overwritten.
var ErrAlreadyResolved = fmt.Errorf("agentdriver: permission already resolved: %w", agentcore.ErrPermissionResolved)

// ErrInvalidDecision is returned by Resolve when the decision is not one
// of the known values (approved/denied). Guards the driver against
// client typos or malicious payloads — defense in depth on top of the
// driver's own approved-only check. Wraps agentcore.ErrPermissionDecision
// so HTTP transport can surface it as 400.
var ErrInvalidDecision = fmt.Errorf("agentdriver: invalid permission decision: %w", agentcore.ErrPermissionDecision)

// Open registers a permission request and returns a channel the driver
// blocks on until Resolve is called. Buffered with size 1 so Resolve never
// blocks even if the driver hasn't started reading yet.
func (g *PermissionGate) Open(id agentcore.PermissionID) <-chan permissionResolution {
	ch := make(chan permissionResolution, 1)
	g.mu.Lock()
	g.pending[id] = ch
	g.mu.Unlock()
	return ch
}

// Resolve delivers the operator's decision to the waiting driver goroutine.
// Idempotent failure: a second Resolve for the same ID returns
// ErrAlreadyResolved without disturbing the first decision.
func (g *PermissionGate) Resolve(id agentcore.PermissionID, decision agentcore.PermissionDecision, reason string) error {
	switch decision {
	case agentcore.PermissionApproved, agentcore.PermissionDenied:
	default:
		return fmt.Errorf("%w: %q", ErrInvalidDecision, string(decision))
	}
	g.mu.Lock()
	ch, ok := g.pending[id]
	if !ok {
		g.mu.Unlock()
		return ErrUnknownPermission
	}
	delete(g.pending, id)
	g.mu.Unlock()
	select {
	case ch <- permissionResolution{decision: decision, reason: reason}:
		return nil
	default:
		// Buffered chan must have been resolved already; impossible if we
		// hold the lock above, but guard against future race-condition
		// regressions.
		return ErrAlreadyResolved
	}
}

// Discard removes a pending permission without resolving it. Used when
// the driver failed to publish the corresponding permission.requested
// event and must roll back the gate registration.
func (g *PermissionGate) Discard(id agentcore.PermissionID) {
	g.mu.Lock()
	delete(g.pending, id)
	g.mu.Unlock()
}

// Wait blocks until the gate resolves the given permission or the context
// is canceled. Cancellation cleans up the pending entry to avoid leaking
// channels.
func (g *PermissionGate) Wait(ctx context.Context, id agentcore.PermissionID, ch <-chan permissionResolution) (agentcore.PermissionDecision, string, error) {
	select {
	case res := <-ch:
		return res.decision, res.reason, nil
	case <-ctx.Done():
		g.mu.Lock()
		delete(g.pending, id)
		g.mu.Unlock()
		return "", "", ctx.Err()
	}
}
