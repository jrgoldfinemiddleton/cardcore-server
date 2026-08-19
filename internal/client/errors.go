package client

// Error code constants matching the server's wire-format error codes.
const (
	// ErrStaleSeq indicates that the command's seq is behind the server's; the
	// server sends a fresh snapshot immediately before the error, and the
	// client should resync from it.
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
	// ErrMalformedMessage indicates that the message was malformed: a missing
	// required field, a negative seq, an empty or over-length action_id, an
	// unknown message type, or an unparsable or otherwise invalid payload.
	// Envelope JSON that does not parse at all produces no error message;
	// the server closes the connection instead.
	ErrMalformedMessage = "malformed_message"
	// ErrInternal indicates that an unexpected internal server error occurred.
	ErrInternal = "internal_error"
	// ErrPauseNotAllowed indicates that a pause/resume operation is not allowed
	// in the current game state.
	ErrPauseNotAllowed = "pause_not_allowed"
	// ErrGamePaused indicates that a gameplay command was sent while the game
	// is paused. The command is not applied; the client should wait for the
	// resume snapshot instead of retrying.
	ErrGamePaused = "game_paused"
)

// Recovery action constants returned by ClassifyError.
const (
	// RecoveryResync indicates that the client should resync from the fresh
	// snapshot the server sends immediately before the error.
	RecoveryResync = "resync"
	// RecoveryWait indicates that the client should wait for the next snapshot
	// instead of retrying; the command was well-formed but is not currently
	// allowed.
	RecoveryWait = "wait"
	// RecoveryRetryDifferent is returned for illegal_move: the attempted move
	// violated the game rules. ADR-013 classifies illegal_move as fatal — a
	// structured client prevents it via client-side validation rather than
	// retrying with a different action. The reason for classifying illegal_move
	// as fatal for structured clients is that the client's structure clearly
	// failed to prevent the illegal move, and retrying with a different action
	// is not guaranteed to succeed. However, unstructured clients may be able to
	// recover by retrying with a different action.
	RecoveryRetryDifferent = "retry_different"
	// RecoveryTerminal indicates that the client should not retry; the session
	// is finished, a pause/resume operation was rejected, or an internal or
	// unknown error occurred.
	RecoveryTerminal = "terminal"
	// RecoveryFixAndRetry is returned for malformed_message: the envelope or
	// payload was invalid. ADR-013 classifies malformed_message as fatal — a
	// structured client validates before sending rather than fixing and
	// retrying a rejected message. The assumption is that a structured client
	// must be buggy or broken if it has sent a malformed message, and cannot be
	// trusted further.
	RecoveryFixAndRetry = "fix_and_retry"
)

// ClassifyError maps a server error code to the client's recovery action.
// Unknown codes return RecoveryTerminal to prevent infinite retry loops.
func ClassifyError(code string) string {
	switch code {
	case ErrStaleSeq:
		return RecoveryResync
	case ErrOutOfTurn, ErrWrongPhase, ErrGamePaused:
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
