package view

// View is the interface a game-specific view package must provide to generate
// wire-format snapshots. Game adapters call their view package's concrete
// functions directly (e.g., heartsview.PlayerView) when broadcasting state;
// conformance to this interface is enforced structurally via compile-time
// checks, not through interface-typed calls.
type View interface {
	// PlayerSnapshot builds a seat-filtered snapshot for the player at seat
	// with the given sequence number.
	PlayerSnapshot(seat, seq int) any

	// ObserverSnapshot builds a full-information snapshot for an observer
	// with the given sequence number.
	ObserverSnapshot(seq int) any
}
