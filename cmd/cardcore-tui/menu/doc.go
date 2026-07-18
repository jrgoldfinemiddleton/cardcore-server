// Package menu implements the initial menu wizard for the cardcore TUI.
//
// It presents a vertical list of configuration options — game, server, AI
// difficulty, observer mode, and theme — plus a Start Game action. The user
// navigates with Up/Down arrows, cycles enum values (AI difficulty, observer,
// theme) with Enter, and either starts the game or cancels with Esc.
//
// The package is game-agnostic: it only produces a Config that the caller
// translates into the authoritative tuiConfig. No I/O, network calls, or
// WebSocket logic lives here.
package menu
