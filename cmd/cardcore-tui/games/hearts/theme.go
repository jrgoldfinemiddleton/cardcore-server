package heartstui

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-tui/theme"
)

// Theme holds the Hearts color palette. It embeds the shell [theme.Theme]
// for all shared colors and adds WinnerBg for the winning card in a trick.
// Every render function accepts a Theme so colors are runtime-constructed
// rather than hardcoded in package-level variables.
type Theme struct {
	// Theme is the embedded shell color palette; its fields are promoted,
	// so render functions access them directly (e.g., theme.Background).
	theme.Theme
	// WinnerBg is the background color for the winning card in a trick.
	WinnerBg color.Color
}

// NewDarkTheme returns the dark Hearts palette: the shell dark theme plus
// the dark WinnerBg color.
func NewDarkTheme() Theme {
	return Theme{
		Theme:    theme.NewDarkTheme(),
		WinnerBg: lipgloss.Color("#533483"),
	}
}

// NewLightTheme returns the light Hearts palette: the shell light theme
// plus the light WinnerBg color.
func NewLightTheme() Theme {
	return Theme{
		Theme:    theme.NewLightTheme(),
		WinnerBg: lipgloss.Color("#E1BEE7"),
	}
}
