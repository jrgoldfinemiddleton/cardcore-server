package view

// View is the interface a game-specific view package must provide to generate
// wire-format snapshots. The session layer calls these methods from the game
// adapter when broadcasting state to players and observers.
type View interface {
	// PlayerSnapshot builds a seat-filtered snapshot for the player at seat
	// with the given sequence number.
	PlayerSnapshot(seat, seq int) any

	// ObserverSnapshot builds a full-information snapshot for an observer
	// with the given sequence number.
	ObserverSnapshot(seq int) any
}
