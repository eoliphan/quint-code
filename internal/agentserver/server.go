package agentserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/m0n0x41d/haft/internal/agentcore"
	"github.com/m0n0x41d/haft/internal/agentproto"
	"github.com/m0n0x41d/haft/internal/agentstore"
)

// Server is the HTTP+SSE front door for the agent stack. It owns no
// business logic — it parses requests, hands typed commands to the
// Dispatcher, broadcasts the resulting events, and writes JSON responses.
//
// Lifecycle: NewServer constructs, Start binds and begins serving on a
// background goroutine, Shutdown drains in-flight requests and closes
// listeners. Start picks a free port if Addr is "127.0.0.1:0".
type Server struct {
	Addr       string
	Store      *agentstore.Store
	Dispatcher Dispatcher
	Hub        *Hub
	// AuthStatus exposes the current auth state on GET /auth/status. When
	// nil the endpoint returns a default anonymous payload so the TUI
	// can still render an honest "no credentials configured" header in
	// dev / test deployments without breaking the wire contract.
	AuthStatus AuthStatusProvider

	listener net.Listener
	httpSrv  *http.Server
}

// NewServer builds an unstarted Server. Dispatcher and Store must be
// non-nil. If hub is nil a fresh Hub is created; pass an explicit Hub when
// the Dispatcher publishes events through one (otherwise the dispatcher
// broadcasts to a hub no SSE subscriber is listening on).
func NewServer(addr string, store *agentstore.Store, dispatcher Dispatcher, hub *Hub) *Server {
	if hub == nil {
		hub = NewHub()
	}
	return &Server{
		Addr:       addr,
		Store:      store,
		Dispatcher: dispatcher,
		Hub:        hub,
	}
}

// Start binds and begins serving. Returns the actual listen address (with
// the chosen port resolved if Addr ended in ":0"). Errors propagate
// listener-bind failures; HTTP errors after Start are surfaced via the
// returned errChan.
func (s *Server) Start() (string, <-chan error, error) {
	if s.Dispatcher == nil {
		return "", nil, errors.New("agentserver: Dispatcher required")
	}
	if s.Store == nil {
		return "", nil, errors.New("agentserver: Store required")
	}
	if s.Hub == nil {
		s.Hub = NewHub()
	}
	addr := s.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	s.listener = l

	mux := http.NewServeMux()
	s.routes(mux)

	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		err := s.httpSrv.Serve(l)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	return l.Addr().String(), errCh, nil
}

// Shutdown gracefully stops the HTTP server and closes the listener.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /event/global", s.handleEventStream)
	mux.HandleFunc("GET /session", s.handleSessionList)
	mux.HandleFunc("GET /session/{id}", s.handleSessionGet)
	mux.HandleFunc("POST /session", s.handleSessionCreate)
	mux.HandleFunc("POST /session/{id}/turn", s.handleTurnSubmit)
	mux.HandleFunc("POST /session/{id}/cancel", s.handleTurnCancel)
	mux.HandleFunc("POST /session/{id}/rename", s.handleSessionRename)
	mux.HandleFunc("POST /session/{id}/model", s.handleModelSet)
	mux.HandleFunc("POST /permission/{id}", s.handlePermissionRespond)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /auth/status", s.handleAuthStatus)
	mux.HandleFunc("GET /commands", s.handleCommandsList)
	mux.HandleFunc("GET /commands/{name}", s.handleCommandGet)
	mux.HandleFunc("GET /skills", s.handleSkillsList)
	mux.HandleFunc("GET /skills/{name}", s.handleSkillGet)
	mux.HandleFunc("GET /file", s.handleFileRead)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "subscribers": s.Hub.Count()})
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ch, cancel := s.Hub.Subscribe()
	defer cancel()

	// Initial comment frame so the client knows the stream is open before
	// the first real event arrives.
	_, _ = fmt.Fprintf(w, ":connected\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, open := <-ch:
			if !open {
				return
			}
			payload, err := agentproto.EncodeEvent(ev)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\n", ev.Kind())
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	res, err := s.dispatch(r.Context(), agentproto.SessionListCmd{ProjectID: projectID})
	s.writeResult(w, res, err)
}

func (s *Server) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	id := agentcore.SessionID(r.PathValue("id"))
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}
	res, err := s.dispatch(r.Context(), agentproto.SessionResumeCmd{SessionID: id})
	s.writeResult(w, res, err)
}

func (s *Server) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	var cmd agentproto.SessionCreateCmd
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, "decode session.create: "+err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.dispatch(r.Context(), cmd)
	s.writeResult(w, res, err)
}

func (s *Server) handleSessionRename(w http.ResponseWriter, r *http.Request) {
	id := agentcore.SessionID(r.PathValue("id"))
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "decode rename: "+err.Error(), http.StatusBadRequest)
		return
	}
	cmd := agentproto.SessionRenameCmd{SessionID: id, Title: body.Title}
	res, err := s.dispatch(r.Context(), cmd)
	s.writeResult(w, res, err)
}

func (s *Server) handleTurnSubmit(w http.ResponseWriter, r *http.Request) {
	id := agentcore.SessionID(r.PathValue("id"))
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "decode turn.submit: "+err.Error(), http.StatusBadRequest)
		return
	}
	cmd := agentproto.TurnSubmitCmd{SessionID: id, Text: body.Text}
	res, err := s.dispatch(r.Context(), cmd)
	s.writeResult(w, res, err)
}

func (s *Server) handleTurnCancel(w http.ResponseWriter, r *http.Request) {
	id := agentcore.SessionID(r.PathValue("id"))
	var body struct {
		TurnID agentcore.TurnID `json:"turn_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "decode turn.cancel: "+err.Error(), http.StatusBadRequest)
		return
	}
	cmd := agentproto.TurnCancelCmd{SessionID: id, TurnID: body.TurnID}
	res, err := s.dispatch(r.Context(), cmd)
	s.writeResult(w, res, err)
}

func (s *Server) handlePermissionRespond(w http.ResponseWriter, r *http.Request) {
	pid := agentcore.PermissionID(r.PathValue("id"))
	var body struct {
		SessionID agentcore.SessionID          `json:"session_id"`
		Decision  agentcore.PermissionDecision `json:"decision"`
		Reason    string                       `json:"reason,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "decode permission.respond: "+err.Error(), http.StatusBadRequest)
		return
	}
	cmd := agentproto.PermissionRespondCmd{
		SessionID:    body.SessionID,
		PermissionID: pid,
		Decision:     body.Decision,
		Reason:       body.Reason,
	}
	res, err := s.dispatch(r.Context(), cmd)
	s.writeResult(w, res, err)
}

func (s *Server) handleModelSet(w http.ResponseWriter, r *http.Request) {
	id := agentcore.SessionID(r.PathValue("id"))
	var body struct {
		Model agentcore.ModelChoice `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "decode model.set: "+err.Error(), http.StatusBadRequest)
		return
	}
	cmd := agentproto.ModelSetCmd{SessionID: id, Model: body.Model}
	res, err := s.dispatch(r.Context(), cmd)
	s.writeResult(w, res, err)
}

// dispatch routes a typed command through the injected Dispatcher and
// publishes any returned events to the Hub. Errors include sentinel values
// so writeResult can map them to HTTP status codes.
func (s *Server) dispatch(ctx context.Context, cmd agentproto.RPCCommand) (DispatchResult, error) {
	res, err := s.Dispatcher.Dispatch(ctx, cmd)
	if err != nil {
		return res, err
	}
	for _, ev := range res.Events {
		s.Hub.Publish(ev)
	}
	return res, nil
}

func (s *Server) writeResult(w http.ResponseWriter, res DispatchResult, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrUnsupportedCommand):
			status = http.StatusNotImplemented
		case errors.Is(err, agentstore.ErrSessionNotFound):
			status = http.StatusNotFound
		case errors.Is(err, agentcore.ErrTurnAlreadyRunning):
			status = http.StatusConflict
		case errors.Is(err, ErrTurnNotRunning):
			status = http.StatusNotFound
		case errors.Is(err, ErrTurnMismatch):
			status = http.StatusConflict
		case errors.Is(err, agentcore.ErrPermissionNotFound):
			status = http.StatusNotFound
		case errors.Is(err, agentcore.ErrPermissionDecision):
			status = http.StatusBadRequest
		case strings.Contains(err.Error(), "decode") || strings.Contains(err.Error(), "missing"):
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	if res.Response == nil {
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
		return
	}
	writeJSON(w, http.StatusOK, res.Response)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}
