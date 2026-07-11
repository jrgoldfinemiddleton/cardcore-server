package client

import (
	"fmt"
	"strings"
)

// WebSocketURL converts an HTTP base URL to a WebSocket URL for a session.
// It replaces http:// with ws:// and https:// with wss://, then appends
// the session path.
func WebSocketURL(baseURL, sessionID, path string) string {
	u := strings.TrimSuffix(baseURL, "/")
	u = strings.Replace(u, "http://", "ws://", 1)
	u = strings.Replace(u, "https://", "wss://", 1)
	return fmt.Sprintf("%s/sessions/%s%s", u, sessionID, path)
}
