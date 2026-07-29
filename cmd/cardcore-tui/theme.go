package main

import heartstui "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-tui/games/hearts"

// Theme is an alias for [heartstui.Theme] that lets the game-agnostic TUI
// shell (main, model, layout, status) use the same Theme type without
// importing the game-specific [heartstui] package.
type Theme = heartstui.Theme

// NewDarkTheme returns a dark-themed color palette for the TUI.
func NewDarkTheme() Theme {
	return heartstui.NewDarkTheme()
}

// NewLightTheme returns a light-themed color palette for the TUI.
func NewLightTheme() Theme {
	return heartstui.NewLightTheme()
}
