package session

import "github.com/jrgoldfinemiddleton/cardcore-server/internal/api"

// SubmitResult is the synchronous response from a session goroutine to
// Manager.SubmitAction. Snapshot carries the latest game state, and Err
// carries a wire-format error when the command is rejected. Both may be
// non-nil when the command is rejected due to a stale seq, allowing the
// client to resync from the snapshot.
type SubmitResult struct {
	// Snapshot is a marshaled JSON snapshot. Non-nil on success,
	// on stale_seq (the latest snapshot is returned), and on
	// duplicate action_id (the cached snapshot is returned).
	Snapshot []byte
	// Err is a marshaled error message. Non-nil when the command is
	// rejected for any reason (e.g., stale seq, invalid turn, illegal
	// action, wrong phase).
	Err *api.ErrorMessage
}

// command is the sealed interface for all messages sent to a session
// goroutine via its command channel. Only the unexported types in this
// package implement it.
type command interface {
	isCommand()
}

// playCmd represents a human player's action.
type playCmd struct {
	// seat is the player seat index.
	seat int
	// msg is the inbound wire message.
	msg *api.InboundMessage
	// resp receives the synchronous SubmitResult.
	resp chan<- SubmitResult
}

// subscribePlayerCmd registers a player subscriber.
type subscribePlayerCmd struct {
	// seat is the player seat index.
	seat int
	// ch receives marshaled snapshot broadcasts.
	ch chan []byte
}

// subscribeObserverCmd registers an observer subscriber.
type subscribeObserverCmd struct {
	// ch receives marshaled observer snapshot broadcasts.
	ch chan []byte
}

// unsubscribeCmd removes a subscriber.
type unsubscribeCmd struct {
	// seat is the player seat index, or -1 for observer.
	seat int
	// ch is the subscriber channel to remove. Required for observer
	// unsubscription; for players, the seat alone is sufficient.
	ch chan []byte
}

// isCommand marks playCmd as a command.
func (playCmd) isCommand() {}

// isCommand marks subscribePlayerCmd as a command.
func (subscribePlayerCmd) isCommand() {}

// isCommand marks subscribeObserverCmd as a command.
func (subscribeObserverCmd) isCommand() {}

// isCommand marks unsubscribeCmd as a command.
func (unsubscribeCmd) isCommand() {}
