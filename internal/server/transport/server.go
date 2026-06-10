package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// Server is the HTTP and WebSocket game server.
type Server struct {
	// srv is the underlying HTTP server.
	srv *http.Server
	// mgr is the session manager.
	mgr *session.Manager
	// logger is the structured logger.
	logger *slog.Logger
	// mu protects listener.
	mu sync.RWMutex
	// listener is the TCP listener, stored so Addr() can return the
	// actual bound address (needed when Addr is ":0").
	listener net.Listener
	// mux is the HTTP request multiplexer (router) that maps URL patterns
	// to handler functions.
	mux *http.ServeMux
	// wsReadLimit is the maximum size of a single WebSocket message in
	// bytes. Default is 65536.
	wsReadLimit int64
	// wsConns tracks active WebSocket connections for graceful shutdown.
	// Keys are *websocket.Conn; values are struct{}.
	wsConns sync.Map
	// closing is set to true when Shutdown() begins to prevent transport
	// goroutines from sending a redundant NormalClosure close frame after
	// Shutdown() has already sent GoingAway.
	closing atomic.Bool
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// NewServer creates a new server with the given configuration.
func NewServer(cfg Config) *Server {
	logger := slog.Default()

	wsReadLimit := cfg.WSReadLimit
	if wsReadLimit == 0 {
		wsReadLimit = 65536
	}

	s := &Server{
		mgr:         cfg.Manager,
		logger:      logger,
		mux:         http.NewServeMux(),
		wsReadLimit: wsReadLimit,
	}
	s.registerRoutes()

	addr := cfg.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}

	s.srv = &http.Server{
		Addr:           addr,
		Handler:        recoveryMiddleware(requestLogMiddleware(s.mux, s.logger), s.logger),
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		MaxHeaderBytes: cfg.MaxHeaderBytes,
	}

	return s
}

// Start begins listening on the configured address and serving HTTP
// requests. It blocks until the server is shut down.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.logger.Info("server listening",
		"addr", s.Addr(),
	)

	return s.srv.Serve(ln)
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// Shutdown gracefully shuts down the server. It sends a GoingAway close
// frame to every tracked WebSocket connection, deletes all non-expired
// sessions from the [session.Manager], and then shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("server shutdown initiated")

	s.closing.Store(true)

	// Close WebSocket connections before deleting sessions so the
	// GoingAway status reaches clients before playerConn.run() sends
	// NormalClosure on goroutine exit. Each close runs in its own
	// goroutine: coder/websocket's Close writes and flushes the
	// GoingAway frame immediately, then blocks up to 5s in the close
	// handshake waiting for the peer to echo a close frame. A
	// connection's reader goroutine holds the read lock while parked in
	// Read, so a synchronous Close here would serialize behind that 5s
	// handshake per connection. Fanning out lets every GoingAway frame
	// hit the wire at once; the handshake wait then drains off the
	// shutdown path.
	var wg sync.WaitGroup
	s.wsConns.Range(func(key, value any) bool {
		conn := key.(*websocket.Conn)
		wg.Go(func() {
			_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
		})
		return true
	})

	// Wait for the close handshakes to complete, but cap the wait so a
	// non-echoing client cannot stall shutdown for the full 5s per
	// connection. The grace window is long enough for the already-sent
	// GoingAway frames to reach clients before the HTTP server tears
	// down; clients see status 1001 rather than a raw connection error.
	closed := make(chan struct{})
	go func() {
		wg.Wait()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(250 * time.Millisecond):
	}

	for _, summary := range s.mgr.List() {
		if summary.State != session.Expired {
			_ = s.mgr.Delete(summary.SessionID)
		}
	}

	return s.srv.Shutdown(ctx)
}

// Addr returns the actual TCP address the server is listening on.
// Returns an empty string if Start has not been called.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// RegisterWSConn adds conn to the active WebSocket connection registry.
func (s *Server) RegisterWSConn(conn *websocket.Conn) {
	s.wsConns.Store(conn, struct{}{})
}

// UnregisterWSConn removes conn from the active WebSocket connection
// registry.
func (s *Server) UnregisterWSConn(conn *websocket.Conn) {
	s.wsConns.Delete(conn)
}

// Hijack delegates to the underlying ResponseWriter if it implements
// http.Hijacker. This is required for WebSocket upgrades when the
// responseWriter wrapper is active.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying ResponseWriter does not implement Hijacker")
	}
	return hj.Hijack()
}

// WriteHeader captures the status code before delegating.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// recoveryMiddleware recovers from panics in HTTP handlers and logs
// the stack trace. It re-panics http.ErrAbortHandler so that
// net/http can handle it correctly.
func recoveryMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	logger = logger.With("component", "transport")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				logger.Error("handler panic",
					"error", rec,
					"stack", string(debug.Stack()),
					"path", r.URL.Path,
					"method", r.Method,
				)
				http.Error(w, "internal server error",
					http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// requestLogMiddleware logs each HTTP request with method, path,
// status, and duration.
func requestLogMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	logger = logger.With("component", "transport")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(ww, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.statusCode,
			"duration", time.Since(start),
		)
	})
}
