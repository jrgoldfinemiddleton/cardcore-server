package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// roundTripperFunc adapts a function to http.RoundTripper.
type roundTripperFunc func(*http.Request) (*http.Response, error)

// TestCreateSession verifies that CreateSession sends a POST /sessions
// with the correct request body and parses the 201 response.
func TestCreateSession(t *testing.T) {
	delay, timeout := 100, 5000
	wantCfg := Config{
		Game:            "hearts",
		Seats:           []SeatConfig{{Type: "human"}, {Type: "ai", AIType: "random"}},
		AIActionDelayMS: &delay,
		TurnTimeoutMS:   &timeout,
	}
	wantSessionID := "test-session-123"
	wantSeats := []SeatInfo{
		{Index: 0, Type: "human", Token: "token-0"},
		{Index: 1, Type: "ai"},
	}

	var gotMethod, gotPath string
	var gotBody Config
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(createResponse{
			SessionID: wantSessionID,
			Seats:     wantSeats,
		})
	}))
	defer server.Close()

	client := SessionClient{BaseURL: server.URL}
	sessionID, seats, err := client.CreateSession(t.Context(), wantCfg)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("got method %s, want %s", gotMethod, http.MethodPost)
	}
	if gotPath != "/sessions" {
		t.Errorf("got path %s, want /sessions", gotPath)
	}
	if gotBody.Game != wantCfg.Game {
		t.Errorf("got body.Game %s, want %s", gotBody.Game, wantCfg.Game)
	}
	if len(gotBody.Seats) != len(wantCfg.Seats) {
		t.Errorf("got body.Seats length %d, want %d", len(gotBody.Seats), len(wantCfg.Seats))
	}
	for i := range gotBody.Seats {
		if gotBody.Seats[i] != wantCfg.Seats[i] {
			t.Errorf("got body.Seats[%d] %+v, want %+v", i, gotBody.Seats[i], wantCfg.Seats[i])
		}
	}
	if !intPtrEqual(gotBody.AIActionDelayMS, wantCfg.AIActionDelayMS) {
		t.Errorf("got body.AIActionDelayMS %v, want %v",
			gotBody.AIActionDelayMS, wantCfg.AIActionDelayMS)
	}
	if !intPtrEqual(gotBody.TurnTimeoutMS, wantCfg.TurnTimeoutMS) {
		t.Errorf("got body.TurnTimeoutMS %v, want %v", gotBody.TurnTimeoutMS, wantCfg.TurnTimeoutMS)
	}
	if sessionID != wantSessionID {
		t.Errorf("got sessionID %s, want %s", sessionID, wantSessionID)
	}
	if len(seats) != len(wantSeats) {
		t.Fatalf("got %d seats, want %d", len(seats), len(wantSeats))
	}
	for i := range seats {
		if seats[i] != wantSeats[i] {
			t.Errorf("got seat[%d] %+v, want %+v", i, seats[i], wantSeats[i])
		}
	}
}

// TestStartSession verifies that StartSession sends a POST to the
// correct path and accepts a 200 response.
func TestStartSession(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(startResponse{SessionID: "abc", State: "active"})
	}))
	defer server.Close()

	client := SessionClient{BaseURL: server.URL}
	if err := client.StartSession(t.Context(), "abc"); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("got method %s, want %s", gotMethod, http.MethodPost)
	}
	wantPath := "/sessions/abc/start"
	if gotPath != wantPath {
		t.Errorf("got path %s, want %s", gotPath, wantPath)
	}
}

// TestDeleteSession verifies that DeleteSession sends a DELETE to the
// correct path and accepts a 204 response.
func TestDeleteSession(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := SessionClient{BaseURL: server.URL}
	if err := client.DeleteSession(t.Context(), "xyz"); err != nil {
		t.Fatalf("DeleteSession error: %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("got method %s, want %s", gotMethod, http.MethodDelete)
	}
	if gotPath != "/sessions/xyz" {
		t.Errorf("got path %s, want /sessions/xyz", gotPath)
	}
}

// TestDeleteSessionNotFound verifies that a 404 response is returned as
// an HTTPError.
func TestDeleteSessionNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(struct {
			Error string `json:"error"`
		}{"not found"})
	}))
	defer server.Close()

	client := SessionClient{BaseURL: server.URL}
	err := client.DeleteSession(t.Context(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if he.StatusCode != http.StatusNotFound {
		t.Errorf("got status code %d, want %d", he.StatusCode, http.StatusNotFound)
	}
}

// TestGetSessionSuccess verifies that GetSession sends a GET to the
// correct path and decodes the 200 response into SessionInfo.
func TestGetSessionSuccess(t *testing.T) {
	wantSessionID := "test-session-123"
	wantGame := "hearts"
	wantState := "active"
	wantTurnTimeoutMS := 30000

	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(SessionInfo{
			SessionID:     wantSessionID,
			Game:          wantGame,
			State:         wantState,
			TurnTimeoutMS: wantTurnTimeoutMS,
		})
	}))
	defer server.Close()

	client := SessionClient{BaseURL: server.URL}
	info, err := client.GetSession(t.Context(), wantSessionID)
	if err != nil {
		t.Fatalf("GetSession error: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("got method %s, want %s", gotMethod, http.MethodGet)
	}
	wantPath := "/sessions/" + wantSessionID
	if gotPath != wantPath {
		t.Errorf("got path %s, want %s", gotPath, wantPath)
	}
	if info.SessionID != wantSessionID {
		t.Errorf("got SessionID %s, want %s", info.SessionID, wantSessionID)
	}
	if info.Game != wantGame {
		t.Errorf("got Game %s, want %s", info.Game, wantGame)
	}
	if info.State != wantState {
		t.Errorf("got State %s, want %s", info.State, wantState)
	}
	if info.TurnTimeoutMS != wantTurnTimeoutMS {
		t.Errorf("got TurnTimeoutMS %d, want %d", info.TurnTimeoutMS, wantTurnTimeoutMS)
	}
}

// TestGetSessionNotFound verifies that a 404 response is returned as
// an HTTPError.
func TestGetSessionNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(struct {
			Error string `json:"error"`
		}{"not found"})
	}))
	defer server.Close()

	client := SessionClient{BaseURL: server.URL}
	_, err := client.GetSession(t.Context(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if he.StatusCode != http.StatusNotFound {
		t.Errorf("got status code %d, want %d", he.StatusCode, http.StatusNotFound)
	}
}

// TestHTTPClientOverride verifies that a custom HTTPClient is used when
// set on the SessionClient.
func TestHTTPClientOverride(t *testing.T) {
	var used bool
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		used = true
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       http.NoBody,
			Request:    req,
		}, nil
	})

	client := SessionClient{
		BaseURL:    "http://example.com",
		HTTPClient: &http.Client{Transport: rt},
	}
	_, _, _ = client.CreateSession(t.Context(), Config{Game: "hearts"})

	if !used {
		t.Error("custom HTTPClient was not used")
	}
}

// TestReadErrorMalformedBody verifies that readError handles a non-JSON
// response body gracefully, returning an HTTPError with the correct
// status code and an empty message.
func TestReadErrorMalformedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := SessionClient{BaseURL: server.URL}
	err := client.DeleteSession(t.Context(), "any")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if he.StatusCode != http.StatusInternalServerError {
		t.Errorf("got status code %d, want %d", he.StatusCode, http.StatusInternalServerError)
	}
	if he.Message != "" {
		t.Errorf("got message %q, want empty", he.Message)
	}
}

// RoundTrip implements http.RoundTripper.
func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// intPtrEqual reports whether two *int values are equal (both nil or
// both pointing to the same value).
func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
