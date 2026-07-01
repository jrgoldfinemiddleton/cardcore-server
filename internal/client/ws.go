package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// ConnectionClosedError is returned when the server closes the WebSocket
// connection with a specific close code.
type ConnectionClosedError struct {
	// Code is the WebSocket close code (1000, 1001, 1009, 1011).
	Code int
	// Reason is the close reason string from the server.
	Reason string
}

// Conn manages a WebSocket connection to the cardcore server, handling
// snapshot reads with maxSeenSeq deduplication and command writes.
type Conn struct {
	// ws is the underlying WebSocket connection.
	ws *websocket.Conn
	// mu protects maxSeenSeq.
	mu sync.Mutex
	// maxSeenSeq is the highest snapshot sequence number accepted.
	// Snapshots with seq <= maxSeenSeq are discarded.
	maxSeenSeq int
	// Logger is the structured logger for connection operations.
	// If nil, slog.Default is used.
	Logger *slog.Logger
}

// Error returns a string including the close code and reason.
func (e *ConnectionClosedError) Error() string {
	return fmt.Sprintf("websocket closed: code=%d reason=%q", e.Code, e.Reason)
}

// Connect upgrades to a WebSocket connection at the given URL. If token
// is non-empty, it is sent as a Bearer authorization header.
func (c *Conn) Connect(ctx context.Context, url, token string) error {
	opts := &websocket.DialOptions{}
	if token != "" {
		opts.HTTPHeader = http.Header{
			"Authorization": []string{"Bearer " + token},
		}
	}
	conn, _, err := websocket.Dial(ctx, url, opts)
	if err != nil {
		c.logger().Error("websocket dial", "url", url, "error", err)
		return fmt.Errorf("dial: %w", err)
	}
	c.ws = conn
	c.logger().Info("websocket connected", "url", url)
	return nil
}

// ReadSnapshot blocks until it receives a snapshot with seq greater than
// maxSeenSeq, discarding any stale snapshots (seq <= maxSeenSeq) it
// encounters. It may block indefinitely if no fresh snapshot arrives
// and ctx has no deadline. This is the sole owner of maxSeenSeq
// filtering per ADR-011.
//
// Because this function blocks, callers on a UI thread should run it
// in a dedicated goroutine and forward results asynchronously.
//
// Returns the raw JSON snapshot message or an error. If the WebSocket
// is closed by the server, it returns a ConnectionClosedError with the
// close code and reason.  If the server sends an error message, the second
// return value is an *ErrorMessage with the error details.
func (c *Conn) ReadSnapshot(ctx context.Context) (json.RawMessage, error) {
	for {
		typ, data, err := c.ws.Read(ctx)
		if err != nil {
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				c.logger().Warn("websocket closed",
					"code", int(ce.Code), "reason", ce.Reason)
				return nil, &ConnectionClosedError{
					Code:   int(ce.Code),
					Reason: ce.Reason,
				}
			}
			c.logger().Debug("websocket read error", "error", err)
			return nil, fmt.Errorf("read: %w", err)
		}
		if typ != websocket.MessageText {
			c.logger().Debug("skipping non-text message", "type", typ)
			continue
		}

		var envelope struct {
			Type string `json:"type"`
			Seq  int    `json:"seq"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			c.logger().Error("unmarshal message envelope", "error", err)
			return nil, fmt.Errorf("unmarshal: %w", err)
		}

		if envelope.Type == "error" {
			var em ErrorMessage
			if err := json.Unmarshal(data, &em); err != nil {
				return nil, fmt.Errorf("unmarshal error: %w", err)
			}
			c.mu.Lock()
			if em.CurrentSeq > c.maxSeenSeq {
				c.maxSeenSeq = em.CurrentSeq
			}
			c.mu.Unlock()
			c.logger().Warn("server error",
				"error_code", em.ErrorCode,
				"message", em.Message,
				"current_seq", em.CurrentSeq)
			return nil, &em
		}

		if envelope.Type != "snapshot" {
			c.logger().Debug("skipping non-snapshot message", "type", envelope.Type)
			continue
		}

		c.mu.Lock()
		if envelope.Seq <= c.maxSeenSeq {
			c.mu.Unlock()
			c.logger().Debug("discarding stale snapshot",
				"seq", envelope.Seq,
				"max_seen", c.maxSeenSeq)
			continue
		}
		c.maxSeenSeq = envelope.Seq
		c.mu.Unlock()

		c.logger().Debug("accepted snapshot", "seq", envelope.Seq)
		return json.RawMessage(data), nil
	}
}

// SendCommand marshals the message and writes it as a text frame on the
// WebSocket.
func (c *Conn) SendCommand(ctx context.Context, msg Command) error {
	c.mu.Lock()
	msg.Seq = c.maxSeenSeq
	c.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if err := c.ws.Write(ctx, websocket.MessageText, data); err != nil {
		c.logger().Error("websocket write", "error", err)
		return fmt.Errorf("write: %w", err)
	}
	c.logger().Debug("command sent", "type", msg.Type, "action_id", msg.ActionID, "seq", msg.Seq)
	return nil
}

// MaxSeenSeq returns the current maxSeenSeq value.
func (c *Conn) MaxSeenSeq() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxSeenSeq
}

// Close closes the WebSocket connection with StatusNormalClosure.
func (c *Conn) Close() error {
	if c.ws == nil {
		return nil
	}
	c.logger().Debug("closing websocket")
	return c.ws.Close(websocket.StatusNormalClosure, "")
}

// logger returns the configured logger or slog.Default if nil.
func (c *Conn) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}
