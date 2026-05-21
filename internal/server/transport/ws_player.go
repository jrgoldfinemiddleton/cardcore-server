package transport

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// playerConn represents a single player's WebSocket connection.
type playerConn struct {
	ws        *websocket.Conn
	mgr       *session.Manager
	sessionID string
	seat      int
	subCh     chan []byte
	logger    *slog.Logger
}

// run is the goroutine that manages the player's WebSocket connection.
// It defers cleanup (unsubscribe + close), writes snapshots to the
// WebSocket, and drains the channel until the session goroutine closes
// it. TODO: add reader goroutine for inbound messages and writer
// goroutine for snapshot delivery.
func (pc *playerConn) run() {
	defer func() {
		if err := pc.mgr.UnsubscribePlayer(pc.sessionID, pc.seat); err != nil {
			pc.logger.Error("unsubscribe player", "error", err)
		}
		if err := pc.ws.Close(websocket.StatusNormalClosure, ""); err != nil {
			pc.logger.Error("ws close", "error", err)
		}
	}()
	for snap := range pc.subCh {
		if len(snap) == 0 {
			pc.logger.Error("dropped empty snapshot", "session_id", pc.sessionID, "seat", pc.seat)
			continue
		}
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := pc.ws.Write(ctx, websocket.MessageText, snap); err != nil {
				pc.logger.Error("ws write", "error", err)
				return
			}
		}()
	}
}

// handlePlayerWS handles GET /sessions/{id}/ws — the player WebSocket
// upgrade endpoint. It authenticates the client via a bearer token,
// subscribes the player to snapshot updates, upgrades the connection,
// and launches a goroutine to manage the connection.
func (s *Server) handlePlayerWS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	token, err := parseBearerToken(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing or invalid authorization header")
		return
	}

	sessionID, seat, err := s.mgr.LookupToken(token)
	if err != nil {
		s.logger.Info("lookup token failed", "error", err)
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if sessionID != id {
		s.logger.Info("token session mismatch", "want_session", id, "got_session", sessionID)
		writeError(w, http.StatusUnauthorized, "token does not match session")
		return
	}

	subCh, err := s.mgr.SubscribePlayer(id, seat)
	if err != nil {
		s.logger.Info("subscribe player failed", "session_id", id, "seat", seat, "error", err)
		writeError(w, httpStatus(err), err.Error())
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.logger.Error("websocket accept failed", "error", err)
		if unsubErr := s.mgr.UnsubscribePlayer(id, seat); unsubErr != nil {
			s.logger.Error("cleanup unsubscribe failed", "error", unsubErr)
		}
		return
	}
	conn.SetReadLimit(s.wsReadLimit)

	pc := &playerConn{
		ws:        conn,
		mgr:       s.mgr,
		sessionID: id,
		seat:      seat,
		subCh:     subCh,
		logger:    s.logger,
	}
	go pc.run()
}
