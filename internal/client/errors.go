package client

// Error code constants matching the server's wire-format error codes.
const (
	ErrStaleSeq         = "stale_seq"
	ErrOutOfTurn        = "out_of_turn"
	ErrIllegalMove      = "illegal_move"
	ErrWrongPhase       = "wrong_phase"
	ErrGameOver         = "game_over"
	ErrMalformedMessage = "malformed_message"
	ErrInternal         = "internal_error"
	// ErrPauseNotAllowed indicates that a pause/resume operation is not allowed
	// in the current game state.
	ErrPauseNotAllowed = "pause_not_allowed"
)

// Recovery action constants returned by ClassifyError.
const (
	RecoveryResync         = "resync"
	RecoveryWait           = "wait"
	RecoveryRetryDifferent = "retry_different"
	RecoveryTerminal       = "terminal"
	RecoveryFixAndRetry    = "fix_and_retry"
)

// ClassifyError maps a server error code to the client's recovery action.
// Unknown codes return RecoveryTerminal to prevent infinite retry loops.
func ClassifyError(code string) string {
	switch code {
	case ErrStaleSeq:
		return RecoveryResync
	case ErrOutOfTurn, ErrWrongPhase:
		return RecoveryWait
	case ErrIllegalMove:
		return RecoveryRetryDifferent
	case ErrGameOver:
		return RecoveryTerminal
	case ErrMalformedMessage:
		return RecoveryFixAndRetry
	case ErrInternal:
		return RecoveryTerminal
	default:
		return RecoveryTerminal
	}
}
