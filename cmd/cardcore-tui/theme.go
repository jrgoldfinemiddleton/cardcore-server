package main

import heartstui "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-tui/hearts"

// Theme is the color palette for the TUI. It is an alias for
// heartstui.Theme so both the game-agnostic shell and the Hearts render
// functions share a single type without circular imports.
type Theme = heartstui.Theme

// NewDarkTheme returns a dark-themed color palette for the TUI.
func NewDarkTheme() Theme {
	return heartstui.NewDarkTheme()
}

// NewLightTheme returns a light-themed color palette for the TUI.
func NewLightTheme() Theme {
	return heartstui.NewLightTheme()
}
