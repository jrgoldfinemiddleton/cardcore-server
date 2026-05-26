package transport

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

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
}

// run is the goroutine that manages the observer's WebSocket connection.
// It spawns a writer goroutine for snapshot delivery and a CloseRead
// goroutine for ping/pong/close frame handling. When either exits, it
// cancels the context, both goroutines return, then it unsubscribes
// and closes the WebSocket cleanly.
func (oc *observerConn) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup

	// CloseRead handles ping/pong and close frames automatically.
	wg.Go(func() {
		oc.ws.CloseRead(ctx)
	})

	wg.Go(func() {
		oc.writer(ctx, cancel)
	})

	wg.Wait()

	// Both goroutines exited. Unsubscribe and close WS.
	if err := oc.mgr.UnsubscribeObserver(oc.sessionID, oc.subCh); err != nil {
		oc.logger.Error("unsubscribe observer", "error", err)
	}
	if err := oc.ws.Close(websocket.StatusNormalClosure, ""); err != nil {
		oc.logger.Error("ws close", "error", err)
	}
}

// writer is the exclusive owner of all outbound WebSocket traffic for
// the observer. It reads snapshots from the session goroutine (subCh)
// and writes them to the WebSocket.
func (oc *observerConn) writer(ctx context.Context, cancel context.CancelFunc) {
	defer cancel() // signal CloseRead goroutine to exit on any return path

	for {
		select {
		case msg, ok := <-oc.subCh:
			if !ok {
				// Session goroutine closed subCh (game over or kicked).
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
			if err := writeWSBytes(ctx, oc.ws, msg.Data); err != nil {
				oc.logger.Error("ws write snapshot", "error", err)
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
	id := r.PathValue("id")

	subCh, err := s.mgr.SubscribeObserver(id)
	if err != nil {
		s.logger.Info("subscribe observer failed", "session_id", id, "error", err)
		writeError(w, httpStatus(err), err.Error())
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.logger.Error("websocket accept failed", "error", err)
		if unsubErr := s.mgr.UnsubscribeObserver(id, subCh); unsubErr != nil {
			s.logger.Error("cleanup unsubscribe failed", "error", unsubErr)
		}
		return
	}
	conn.SetReadLimit(s.wsReadLimit)

	oc := &observerConn{
		ws:        conn,
		mgr:       s.mgr,
		sessionID: id,
		subCh:     subCh,
		logger:    s.logger,
	}
	s.RegisterWSConn(conn)
	go func() {
		oc.run(context.Background())
		s.UnregisterWSConn(conn)
	}()
}
