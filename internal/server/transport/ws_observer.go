package transport

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// observerConn represents a single observer's WebSocket connection.
// Observers receive broadcast snapshots but do not send commands.
type observerConn struct {
	ws        *websocket.Conn
	mgr       *session.Manager
	sessionID string
	subCh     chan []byte
	logger    *slog.Logger
}

// run is the goroutine that manages the observer's WebSocket
// connection. It defers cleanup (unsubscribe + close), writes snapshots
// to the WebSocket, and drains the channel until the session goroutine
// closes it. TODO: extract writer goroutine for snapshot delivery.
func (oc *observerConn) run() {
	defer func() {
		if err := oc.mgr.UnsubscribeObserver(oc.sessionID, oc.subCh); err != nil {
			oc.logger.Error("unsubscribe observer", "error", err)
		}
		if err := oc.ws.Close(websocket.StatusNormalClosure, ""); err != nil {
			oc.logger.Error("ws close", "error", err)
		}
	}()
	for snap := range oc.subCh {
		if len(snap) == 0 {
			oc.logger.Error("dropped empty snapshot", "session_id", oc.sessionID)
			continue
		}
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := oc.ws.Write(ctx, websocket.MessageText, snap); err != nil {
				oc.logger.Error("ws write", "error", err)
				return
			}
		}()
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
	go oc.run()
}
