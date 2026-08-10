// Package theme defines the color palette for the game-agnostic TUI shell.
//
// The Theme struct holds the colors shared by the shell layout, the pre-game
// menu, and card games in general: backgrounds, text, accents, suit colors,
// and semantic colors (dimmed, error, footer, panel borders). Game packages
// embed Theme to add game-specific fields (Hearts adds WinnerBg for the
// winning card in a trick).
package theme
