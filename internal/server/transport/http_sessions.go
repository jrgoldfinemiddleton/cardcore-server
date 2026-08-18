package transport

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// createResponse is the JSON body for POST /sessions responses.
type createResponse struct {
	// SessionID is the newly created session's identifier.
	SessionID string `json:"session_id"`
	// Seats holds the created seats, including the freshly minted bearer
	// token for each human seat (empty for AI seats). This is the only
	// response that surfaces tokens.
	Seats []session.Seat `json:"seats"`
}

// listResponse is the JSON body for GET /sessions responses.
type listResponse struct {
	// Sessions holds a summary of every session that is not expired.
	Sessions []session.Summary `json:"sessions"`
}

// startResponse is the JSON body for POST /sessions/{id}/start responses.
type startResponse struct {
	// SessionID is the started session's identifier.
	SessionID string `json:"session_id"`
	// State is the session's lifecycle state after the start, always
	// "active".
	State string `json:"state"`
}

// patchResponse is the JSON body for PATCH /sessions/{id} responses.
type patchResponse struct {
	// Info is the full session detail after the patch; it never contains
	// seat tokens.
	session.Info
	// SeatTokens holds freshly minted bearer tokens for every human seat
	// when the patch replaced the seat configuration, invalidating all
	// former tokens. It is omitted when the patch did not change seats.
	SeatTokens []session.Seat `json:"seat_tokens,omitempty"`
}

// registerRoutes adds all HTTP and WebSocket routes to s.mux.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST /sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("PATCH /sessions/{id}", s.handlePatchSession)
	s.mux.HandleFunc("POST /sessions/{id}/start", s.handleStartSession)
	s.mux.HandleFunc("DELETE /sessions/{id}", s.handleDeleteSession)
	s.mux.HandleFunc("GET /sessions/{id}/ws", s.handlePlayerWS)
	s.mux.HandleFunc("GET /sessions/{id}/ws/observe", s.handleObserverWS)
}

// handleCreateSession handles POST /sessions.
func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var cfg session.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	info, seats, err := s.mgr.Create(cfg)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}

	slog.With("component", "transport").Info("session created via http",
		"session_id", info.SessionID,
		"game", cfg.Game,
		"seats", len(cfg.Seats),
	)

	writeJSON(w, http.StatusCreated, createResponse{
		SessionID: info.SessionID,
		Seats:     seats,
	})
}

// handleListSessions handles GET /sessions.
func (s *Server) handleListSessions(w http.ResponseWriter, _ *http.Request) {
	summaries := s.mgr.List()
	writeJSON(w, http.StatusOK, listResponse{Sessions: summaries})
}

// handleGetSession handles GET /sessions/{id}.
func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, err := s.mgr.Get(id)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handlePatchSession handles PATCH /sessions/{id}.
func (s *Server) handlePatchSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var patch session.PatchConfig
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	info, seats, err := s.mgr.Update(id, patch)
	if err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}
	resp := patchResponse{Info: *info}
	if seats != nil {
		resp.SeatTokens = seats
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleStartSession handles POST /sessions/{id}/start.
func (s *Server) handleStartSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_ = r.Body.Close()

	if err := s.mgr.Start(id); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}

	slog.With("component", "transport").Info("session started via http",
		"session_id", id,
	)

	writeJSON(w, http.StatusOK, startResponse{
		SessionID: id,
		State:     string(session.Active),
	})
}

// handleDeleteSession handles DELETE /sessions/{id}.
func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.mgr.Delete(id); err != nil {
		writeError(w, httpStatus(err), err.Error())
		return
	}

	slog.With("component", "transport").Info("session deleted via http",
		"session_id", id,
	)

	w.WriteHeader(http.StatusNoContent)
}
