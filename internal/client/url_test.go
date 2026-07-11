package client

import "testing"

// TestWebSocketURL verifies that WebSocketURL converts HTTP base URLs to
// WebSocket URLs and handles trailing slashes and observer paths.
func TestWebSocketURL(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		sessionID string
		path      string
		want      string
	}{
		{
			name:      "http to ws",
			baseURL:   "http://localhost:8080",
			sessionID: "abc123",
			path:      "/ws",
			want:      "ws://localhost:8080/sessions/abc123/ws",
		},
		{
			name:      "https to wss",
			baseURL:   "https://example.com",
			sessionID: "xyz789",
			path:      "/ws",
			want:      "wss://example.com/sessions/xyz789/ws",
		},
		{
			name:      "trailing slash",
			baseURL:   "http://localhost:8080/",
			sessionID: "abc123",
			path:      "/ws",
			want:      "ws://localhost:8080/sessions/abc123/ws",
		},
		{
			name:      "observe path",
			baseURL:   "http://localhost:8080",
			sessionID: "abc123",
			path:      "/ws/observe",
			want:      "ws://localhost:8080/sessions/abc123/ws/observe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WebSocketURL(tt.baseURL, tt.sessionID, tt.path)
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
