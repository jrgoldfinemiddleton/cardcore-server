// Package heartstui provides rendering and command-building functions for
// the Hearts card game terminal UI. The render functions take data and UI
// state as parameters and return strings — no global state, no goroutines,
// no I/O.
//
// The package is organized into seven files:
//
//   - card.go: card symbol mapping and styled card/hand rendering
//   - views.go: player-facing phase views (passing, playing, round/game over)
//   - observer.go: observer view showing all hands and scores
//   - commands.go: command builders that produce [client.Command] values
//   - client.go: stateful Client adapter that ties the pure functions to the TUI
//   - theme.go: Theme struct with dark and light color palettes
//   - session.go: CreateSession helper that auto-creates a Hearts session
//     over HTTP via the shared client engine
//
// See doc/games/hearts/protocol.md.
package heartstui
