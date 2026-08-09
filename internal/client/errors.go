package client

// Error code constants matching the server's wire-format error codes.
const (
	// ErrStaleSeq indicates that the command's seq is behind the server's; the
	// client should resync from the snapshot that immediately follows the error.
	ErrStaleSeq = "stale_seq"
	// ErrOutOfTurn indicates that a well-formed command was sent when it was not
	// the sender's turn.
	ErrOutOfTurn = "out_of_turn"
	// ErrIllegalMove indicates that the attempted move violates the game rules;
	// the message field carries the engine's explanation of the violation.
	ErrIllegalMove = "illegal_move"
	// ErrWrongPhase indicates that a well-formed command was sent during a game
	// phase that does not allow it.
	ErrWrongPhase = "wrong_phase"
	// ErrGameOver indicates that the session is in the finished state.
	ErrGameOver = "game_over"
	// ErrMalformedMessage indicates that the message was malformed: bad JSON,
	// missing required fields, an unknown type, or an empty or over-length
	// action_id.
	ErrMalformedMessage = "malformed_message"
	// ErrInternal indicates that an unexpected internal server error occurred.
	ErrInternal = "internal_error"
	// ErrPauseNotAllowed indicates that a pause/resume operation is not allowed
	// in the current game state.
	ErrPauseNotAllowed = "pause_not_allowed"
)

// Recovery action constants returned by ClassifyError.
const (
	// RecoveryResync indicates that the client should resync from the snapshot
	// that the server sends immediately after the error.
	RecoveryResync = "resync"
	// RecoveryWait indicates that the client should wait for the next snapshot
	// instead of retrying; the command was well-formed but is not currently
	// allowed.
	RecoveryWait = "wait"
	// RecoveryRetryDifferent indicates that the client should retry with a
	// different action; the attempted move violated the game rules.
	RecoveryRetryDifferent = "retry_different"
	// RecoveryTerminal indicates that the client should not retry; the session
	// is finished or an internal or unknown error occurred.
	RecoveryTerminal = "terminal"
	// RecoveryFixAndRetry indicates that the client should fix the malformed
	// message, for example its JSON, required fields, type, or action_id, and
	// retry it.
	RecoveryFixAndRetry = "fix_and_retry"
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
