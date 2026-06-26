// Package heartstui provides pure rendering and command-building functions for
// the Hearts card game terminal UI. Every function takes data and UI state as
// parameters and returns strings or command structs — no global state, no
// goroutines, no I/O.
//
// The package is organized into five files:
//
//   - card.go: card symbol mapping and styled card/hand rendering
//   - views.go: player-facing phase views (passing, playing)
//   - observer.go: observer view showing all hands and scores
//   - commands.go: command builders that produce client.Command values
//   - client.go: stateful Client adapter that ties the pure functions to the TUI
package heartstui
