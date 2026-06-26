package transport

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// playerConn represents a single player's WebSocket connection.
type playerConn struct {
	// ws is the underlying WebSocket connection.
	ws *websocket.Conn
	// mgr is the session manager used for subscriptions and
	// action submission.
	mgr *session.Manager
	// sessionID identifies the game session this player belongs to.
	sessionID string
	// seat is the player seat index.
	seat int
	// subCh receives game snapshots from the session goroutine.
	subCh chan session.SubscriberMessage
	// outCh is an internal queue the reader uses to send error
	// responses to the writer goroutine.
	outCh chan []byte
	// logger is the structured logger.
	logger *slog.Logger
	// closing references the server's shutdown flag. When true, run()
	// skips the final NormalClosure frame and the writer skips cancelling
	// the read context, leaving Shutdown's conn.Close(GoingAway) as the
	// sole teardown path. Cancelling the read context during shutdown
	// would race that close frame and drop it.
	closing *atomic.Bool
}

// run is the goroutine that manages the player's WebSocket connection.
// It spawns a reader and a writer goroutine, waits for both to exit,
// then unsubscribes and closes the WebSocket cleanly.
func (pc *playerConn) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	pc.logger.Debug("player connection started",
		"session_id", pc.sessionID,
		"seat", pc.seat,
	)

	var wg sync.WaitGroup

	wg.Go(func() {
		pc.reader(ctx, cancel)
	})

	wg.Go(func() {
		pc.writer(ctx, cancel)
	})

	wg.Wait()

	pc.logger.Debug("player connection ended",
		"session_id", pc.sessionID,
		"seat", pc.seat,
	)

	// Both goroutines exited. Unsubscribe and close WS.
	if err := pc.mgr.UnsubscribePlayer(pc.sessionID, pc.seat); err != nil {
		pc.logger.Error("unsubscribe player", "error", err)
	}
	if pc.closing != nil && pc.closing.Load() {
		return
	}
	if err := pc.ws.Close(websocket.StatusNormalClosure, ""); err != nil {
		pc.logger.Error("ws close", "error", err)
	}
}

// reader reads JSON messages from the WebSocket, validates them, and
// submits actions to the session manager. All responses are sent to the
// writer goroutine via outCh.
func (pc *playerConn) reader(ctx context.Context, cancel context.CancelFunc) {
	defer cancel() // signal writer to exit on any return path

	for {
		var msg api.InboundMessage
		if err := readWSJSON(ctx, pc.ws, &msg); err != nil {
			if ctx.Err() != nil {
				// Context cancelled (writer exited or run() closing).
				return
			}
			pc.logger.Error("ws read", "error", err)
			return
		}

		if err := api.ValidateInboundMessage(&msg); err != nil {
			writeErrorToOutCh(ctx, pc.outCh, api.ErrMalformedMessage, err.Error(), msg.ActionID)
			continue
		}

		pc.logger.Debug("player message received",
			"session_id", pc.sessionID,
			"seat", pc.seat,
			"type", msg.Type,
			"action_id", msg.ActionID,
		)

		result, err := pc.mgr.SubmitAction(pc.sessionID, pc.seat, &msg)
		if err != nil {
			// Session management error (not found, not active, queue full).
			writeErrorToOutCh(ctx, pc.outCh, api.ErrInternal, err.Error(), msg.ActionID)
			continue
		}

		if result.Snapshot != nil {
			// stale_seq or duplicate action_id — snapshot first.
			select {
			case pc.outCh <- result.Snapshot:
			case <-ctx.Done():
				return
			}
		}

		if result.Err != nil {
			// Game-level error (wrong turn, illegal move, wrong phase).
			errBytes, err := json.Marshal(result.Err)
			if err != nil {
				pc.logger.Error("marshal error response", "error", err)
				continue
			}
			select {
			case pc.outCh <- errBytes:
			case <-ctx.Done():
				return
			}
		}
	}
}

// writer is the exclusive owner of all outbound WebSocket traffic.
// It multiplexes snapshots from the session goroutine (subCh) and
// error responses from the reader (outCh).
func (pc *playerConn) writer(ctx context.Context, cancel context.CancelFunc) {
	skipCancel := false
	defer func() {
		if !skipCancel {
			cancel() // signal reader to exit on any return path
		}
	}()

	for {
		select {
		case msg, ok := <-pc.subCh:
			if !ok {
				// Session goroutine closed subCh (game over or kicked).
				// During shutdown, leave the reader parked so Shutdown's
				// conn.Close(GoingAway) drives teardown instead of an
				// abrupt read-context cancel that drops the close frame.
				if pc.closing != nil && pc.closing.Load() {
					skipCancel = true
				}
				return
			}
			if msg.CloseCode != 0 {
				code := websocket.StatusCode(msg.CloseCode)
				if err := pc.ws.Close(code, "snapshot marshal failure"); err != nil {
					pc.logger.Error("ws close", "error", err)
				}
				return
			}
			if len(msg.Data) == 0 {
				pc.logger.Error("dropped empty snapshot",
					"session_id", pc.sessionID, "seat", pc.seat)
				continue
			}
			pc.logger.Debug("player writing snapshot",
				"session_id", pc.sessionID,
				"seat", pc.seat,
				"len", len(msg.Data),
			)
			if err := writeWSBytes(ctx, pc.ws, msg.Data); err != nil {
				pc.logger.Error("ws write snapshot", "error", err)
				return
			}

		case msg, ok := <-pc.outCh:
			if !ok {
				return
			}
			if len(msg) == 0 {
				pc.logger.Error("dropped empty message",
					"session_id", pc.sessionID, "seat", pc.seat)
				continue
			}
			pc.logger.Debug("player writing message",
				"session_id", pc.sessionID,
				"seat", pc.seat,
				"len", len(msg),
			)
			if err := writeWSBytes(ctx, pc.ws, msg); err != nil {
				pc.logger.Error("ws write message", "error", err)
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// handlePlayerWS handles GET /sessions/{id}/ws — the player WebSocket
// upgrade endpoint. It authenticates the client via a bearer token,
// subscribes the player to snapshot updates, upgrades the connection,
// and launches a goroutine to manage the connection.
func (s *Server) handlePlayerWS(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.With("component", "transport")
	id := r.PathValue("id")

	token, err := parseBearerToken(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing or invalid authorization header")
		return
	}

	sessionID, seat, err := s.mgr.LookupToken(token)
	if err != nil {
		logger.Warn("lookup token failed", "error", err)
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if sessionID != id {
		logger.Warn("token session mismatch", "want_session", id, "got_session", sessionID)
		writeError(w, http.StatusUnauthorized, "token does not match session")
		return
	}

	subCh, err := s.mgr.SubscribePlayer(id, seat)
	if err != nil {
		logger.Warn("subscribe player failed", "session_id", id, "seat", seat, "error", err)
		writeError(w, httpStatus(err), err.Error())
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		logger.Error("websocket accept failed", "error", err)
		if unsubErr := s.mgr.UnsubscribePlayer(id, seat); unsubErr != nil {
			logger.Error("cleanup unsubscribe failed", "error", unsubErr)
		}
		return
	}
	conn.SetReadLimit(s.wsReadLimit)

	logger.Info("player connected",
		"session_id", id,
		"seat", seat,
	)

	pc := &playerConn{
		ws:        conn,
		mgr:       s.mgr,
		sessionID: id,
		seat:      seat,
		subCh:     subCh,
		outCh:     make(chan []byte, 16),
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
		pc.run(context.Background())
		s.UnregisterWSConn(conn)
	}()
}
