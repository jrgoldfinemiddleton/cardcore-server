package transport

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// observerConn represents a single observer's WebSocket connection.
// Observers receive broadcast snapshots but do not send commands.
type observerConn struct {
	// ws is the underlying WebSocket connection.
	ws *websocket.Conn
	// mgr is the session manager used for subscriptions.
	mgr *session.Manager
	// sessionID identifies the game session being observed.
	sessionID string
	// subCh receives observer snapshots from the session goroutine.
	subCh chan session.SubscriberMessage
	// logger is the structured logger.
	logger *slog.Logger
	// closing references the server's shutdown flag. When true, run()
	// skips the final NormalClosure frame and the writer skips cancelling
	// the read context, leaving Shutdown's conn.Close(GoingAway) as the
	// sole teardown path. Cancelling the read context during shutdown
	// would race that close frame and drop it.
	closing *atomic.Bool
}

// run is the goroutine that manages the observer's WebSocket connection.
// It spawns a writer goroutine for snapshot delivery and a CloseRead
// goroutine for ping/pong/close frame handling. When either exits, it
// cancels the context, both goroutines return, then it unsubscribes
// and closes the WebSocket cleanly.
func (oc *observerConn) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	oc.logger.Debug("observer connection started",
		"session_id", oc.sessionID,
	)

	var wg sync.WaitGroup

	// CloseRead handles ping/pong and close frames automatically. It
	// spawns its own read goroutine and returns a context cancelled when
	// the connection closes; wait on that so run() does not return (and
	// fire defer cancel()) before Shutdown's conn.Close(GoingAway) has
	// torn the connection down during shutdown.
	wg.Go(func() {
		readCtx := oc.ws.CloseRead(ctx)
		<-readCtx.Done()
	})

	wg.Go(func() {
		oc.writer(ctx, cancel)
	})

	wg.Wait()

	oc.logger.Debug("observer connection ended",
		"session_id", oc.sessionID,
	)

	// Both goroutines exited. Unsubscribe and close WS.
	if err := oc.mgr.UnsubscribeObserver(oc.sessionID, oc.subCh); err != nil {
		if errors.Is(err, session.ErrNotActive) {
			oc.logger.Debug("unsubscribe observer skipped", "reason", "session not active")
		} else {
			oc.logger.Error("unsubscribe observer", "error", err)
		}
	}
	if oc.closing != nil && oc.closing.Load() {
		return
	}
	if err := oc.ws.Close(websocket.StatusNormalClosure, ""); err != nil {
		if !errors.Is(err, net.ErrClosed) {
			oc.logger.Error("ws close", "error", err)
		}
	}
}

// writer is the exclusive owner of all outbound WebSocket traffic for
// the observer. It reads snapshots from the session goroutine (subCh)
// and writes them to the WebSocket.
func (oc *observerConn) writer(ctx context.Context, cancel context.CancelFunc) {
	skipCancel := false
	defer func() {
		if !skipCancel {
			cancel() // signal CloseRead goroutine to exit on any return path
		}
	}()

	for {
		select {
		case msg, ok := <-oc.subCh:
			if !ok {
				// Session goroutine closed subCh (game over or kicked).
				// During shutdown, leave the read goroutine parked so
				// Shutdown's conn.Close(GoingAway) drives teardown instead
				// of an abrupt read-context cancel that drops the frame.
				if oc.closing != nil && oc.closing.Load() {
					skipCancel = true
				}
				return
			}
			if msg.CloseCode != 0 {
				code := websocket.StatusCode(msg.CloseCode)
				if err := oc.ws.Close(code, "snapshot marshal failure"); err != nil {
					oc.logger.Error("ws close", "error", err)
				}
				return
			}
			if len(msg.Data) == 0 {
				oc.logger.Error("dropped empty snapshot",
					"session_id", oc.sessionID)
				continue
			}
			oc.logger.Debug("observer writing snapshot",
				"session_id", oc.sessionID, "len", len(msg.Data))
			if err := writeWSBytes(ctx, oc.ws, msg.Data); err != nil {
				if errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
					oc.logger.Warn("ws write snapshot aborted", "reason", err)
				} else {
					oc.logger.Error("ws write snapshot", "error", err)
				}
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// handleObserverWS handles GET /sessions/{id}/ws/observe — the observer
// WebSocket upgrade endpoint. It subscribes the observer to broadcast
// snapshots, upgrades the connection, and launches a goroutine to
// manage the connection. Observers receive snapshots but cannot send
// commands, so no authentication is required.
func (s *Server) handleObserverWS(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.With("component", "transport")
	id := r.PathValue("id")

	subCh, err := s.mgr.SubscribeObserver(id)
	if err != nil {
		logger.Warn("subscribe observer failed", "session_id", id, "error", err)
		writeError(w, httpStatus(err), err.Error())
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		logger.Error("websocket accept failed", "error", err)
		if unsubErr := s.mgr.UnsubscribeObserver(id, subCh); unsubErr != nil {
			logger.Error("cleanup unsubscribe failed", "error", unsubErr)
		}
		return
	}
	conn.SetReadLimit(s.wsReadLimit)

	logger.Info("observer connected",
		"session_id", id,
	)

	oc := &observerConn{
		ws:        conn,
		mgr:       s.mgr,
		sessionID: id,
		subCh:     subCh,
		logger:    logger,
		closing:   &s.closing,
	}
	s.RegisterWSConn(conn)
	go func() {
		// If Shutdown began before this connection registered, its close
		// sweep missed us; send GoingAway here so a connection accepted
		// during shutdown still sees 1001 rather than an abrupt close.
		if s.closing.Load() {
			_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
		}
		oc.run(context.Background())
		s.UnregisterWSConn(conn)
	}()
}
