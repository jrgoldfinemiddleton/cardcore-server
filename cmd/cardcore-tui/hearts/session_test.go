package heartstui

import (
	"context"
	"strings"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// TestCreateSessionInvalidAIType verifies that CreateSession rejects an
// unsupported AI type before issuing any HTTP request.
func TestCreateSessionInvalidAIType(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sc := &client.SessionClient{BaseURL: "http://localhost:0"}
	_, _, _, err := CreateSession(ctx, sc, "bogus", false, nil, nil)
	if err == nil {
		t.Fatal("CreateSession got nil error, want error for invalid aiType")
	}
	if !strings.Contains(err.Error(), "invalid ai_type") {
		t.Errorf("error %q does not contain \"invalid ai_type\"", err.Error())
	}
}
