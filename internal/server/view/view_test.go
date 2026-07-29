package view_test

import (
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/view"
	heartsview "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/view/games/hearts"
)

// Compile-time check that the Hearts view implementation satisfies the
// generic view interface.
var _ view.View = heartsview.ViewState{}

// TestHeartsViewImplementsInterface is a runtime guard for the same check.
func TestHeartsViewImplementsInterface(t *testing.T) {
	t.Helper()
	if _, ok := any(heartsview.ViewState{}).(view.View); !ok {
		t.Fatal("heartsview.ViewState does not implement view.View")
	}
}
