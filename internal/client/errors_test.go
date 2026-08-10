package client

import (
	"testing"
)

// TestClassifyError verifies that server error codes map to the correct
// client recovery actions.
func TestClassifyError(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{ErrStaleSeq, RecoveryResync},
		{ErrOutOfTurn, RecoveryWait},
		{ErrWrongPhase, RecoveryWait},
		{ErrIllegalMove, RecoveryRetryDifferent},
		{ErrGameOver, RecoveryTerminal},
		{ErrMalformedMessage, RecoveryFixAndRetry},
		{ErrInternal, RecoveryTerminal},
		{ErrPauseNotAllowed, RecoveryTerminal},
		{ErrGamePaused, RecoveryWait},
		{"unknown_code", RecoveryTerminal},
		{"", RecoveryTerminal},
	}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			if got := ClassifyError(tc.code); got != tc.want {
				t.Errorf("ClassifyError(%q): got %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}
