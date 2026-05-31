package transport

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// errorResponse is the JSON body for HTTP error responses.
type errorResponse struct {
	Error string `json:"error"`
}

// httpStatus maps a session-layer error to an HTTP status code.
// Returns 400 for validation errors, 404 for not-found, 409 for
// state conflicts, and 500 for anything unexpected.
func httpStatus(err error) int {
	if errors.Is(err, session.ErrInvalidConfig) {
		return http.StatusBadRequest
	}
	if errors.Is(err, session.ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, session.ErrNotDraft) || errors.Is(err, session.ErrNotActive) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// writeError writes a JSON error response with the given status and message.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

// writeJSON writes a JSON response with the given status code.
// Sets Content-Type before WriteHeader so the header is included
// in the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.With("component", "transport").Error("json encode response", "error", err)
	}
}
