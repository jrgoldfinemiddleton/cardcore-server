package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// parseBearerToken extracts the bearer token from the Authorization
// header of r. The header must be in the form "Bearer <token>".
// Returns an error if the header is missing, empty, or malformed.
func parseBearerToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	const prefix = "Bearer "
	if len(auth) < len(prefix) || auth[:len(prefix)] != prefix {
		return "", fmt.Errorf("invalid authorization header format")
	}

	return auth[len(prefix):], nil
}

// readWSJSON reads a text message from ws and unmarshals it into v.
func readWSJSON(ctx context.Context, ws *websocket.Conn, v any) error {
	typ, r, err := ws.Reader(ctx)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if typ != websocket.MessageText {
		return fmt.Errorf("unexpected message type %d", typ)
	}
	if err := json.NewDecoder(r).Decode(v); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// writeWSBytes writes pre-marshaled bytes as a text message on ws.
// It uses a 30-second timeout context.
func writeWSBytes(ctx context.Context, ws *websocket.Conn, b []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return ws.Write(ctx, websocket.MessageText, b)
}

// writeErrorToOutCh marshals an ErrorMessage and sends it to outCh,
// respecting context cancellation to avoid blocking on a dead channel.
func writeErrorToOutCh(ctx context.Context, outCh chan []byte, code, message, actionID string) {
	em := api.ErrorMessage{
		Type:      "error",
		ErrorCode: code,
		Message:   message,
		ActionID:  actionID,
	}
	b, err := json.Marshal(em)
	if err != nil {
		return
	}
	select {
	case outCh <- b:
	case <-ctx.Done():
	}
}
