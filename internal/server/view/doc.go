// Package view defines the interface between the session layer and game-specific
// snapshot generators. Each game implements a concrete view that produces
// player-filtered and observer snapshots from the current game state.
//
// See ADR-007 (doc/decisions/007-state-sync-model.md).
package view
