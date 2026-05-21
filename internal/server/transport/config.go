package transport

import (
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// Config holds the parameters for starting the HTTP/WebSocket server.
type Config struct {
	// Manager is the session manager that handles game lifecycle.
	Manager *session.Manager
	// Addr is the TCP address to listen on. Use ":0" to let the OS
	// assign a free port (useful in tests).
	Addr string
	// ReadTimeout is the maximum duration for reading the entire
	// request, including the body.
	ReadTimeout time.Duration
	// WriteTimeout is the maximum duration before timing out writes of
	// the response.
	WriteTimeout time.Duration
	// MaxHeaderBytes controls the maximum number of bytes the server will
	// read parsing the request header.
	MaxHeaderBytes int
	// WSReadLimit is the maximum size of a single WebSocket message in
	// bytes. Messages exceeding this limit cause the connection to close
	// with code 1009 (Message Too Big). Default: 65536.
	WSReadLimit int64
}
