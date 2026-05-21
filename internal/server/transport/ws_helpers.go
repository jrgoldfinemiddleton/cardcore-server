package transport

import (
	"fmt"
	"net/http"
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
