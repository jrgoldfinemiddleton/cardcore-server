package heartstui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds the color palette for the TUI. Every render function accepts a
// Theme so colors are runtime-constructed rather than hardcoded in
// package-level variables.
type Theme struct {
	// Background is the base background color for the layout and hand area.
	Background color.Color
	// Text is the default foreground color for body text.
	Text color.Color
	// Accent is the highlight color for headers and borders.
	Accent color.Color
	// RedSuit is the color for hearts and diamonds.
	RedSuit color.Color
	// DarkSuit is the color for clubs and spades.
	DarkSuit color.Color
	// Dimmed is the muted color for disabled or illegal cards.
	Dimmed color.Color
	// WinnerBg is the background color for the winning card in a trick.
	WinnerBg color.Color
	// FooterBg is the background color for the footer status bar.
	FooterBg color.Color
	// PanelBorder is the border color for bordered panels.
	PanelBorder color.Color
	// Error is the foreground color for error flash messages.
	Error color.Color
}

// NewDarkTheme returns a dark-themed color palette with the approved hex
// values for backgrounds, suits, accents, and semantic colors.
func NewDarkTheme() Theme {
	return Theme{
		Background:  lipgloss.Color("#1A1A2E"),
		Text:        lipgloss.Color("#FAFAFA"),
		Accent:      lipgloss.Color("#E94560"),
		RedSuit:     lipgloss.Color("#E94560"),
		DarkSuit:    lipgloss.Color("#FAFAFA"),
		Dimmed:      lipgloss.Color("#555555"),
		WinnerBg:    lipgloss.Color("#533483"),
		FooterBg:    lipgloss.Color("#16213E"),
		PanelBorder: lipgloss.Color("#E94560"),
		Error:       lipgloss.Color("#FF0000"),
	}
}

// NewLightTheme returns a light-themed color palette with the approved hex
// values for backgrounds, suits, accents, and semantic colors.
func NewLightTheme() Theme {
	return Theme{
		Background:  lipgloss.Color("#F5F5F0"),
		Text:        lipgloss.Color("#1A1A2E"),
		Accent:      lipgloss.Color("#C62828"),
		RedSuit:     lipgloss.Color("#C62828"),
		DarkSuit:    lipgloss.Color("#2D2D2D"),
		Dimmed:      lipgloss.Color("#BDBDBD"),
		WinnerBg:    lipgloss.Color("#E1BEE7"),
		FooterBg:    lipgloss.Color("#E0E0E0"),
		PanelBorder: lipgloss.Color("#C62828"),
		Error:       lipgloss.Color("#D32F2F"),
	}
}
