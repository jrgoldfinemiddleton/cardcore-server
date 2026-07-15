package heartstui

import (
	"testing"
)

// TestNewDarkThemeNonNil verifies the dark theme returns a non-nil palette.
func TestNewDarkThemeNonNil(t *testing.T) {
	got := NewDarkTheme()
	if got.Background == nil {
		t.Errorf("NewDarkTheme().Background = nil, want non-nil")
	}
	if got.Text == nil {
		t.Errorf("NewDarkTheme().Text = nil, want non-nil")
	}
	if got.Accent == nil {
		t.Errorf("NewDarkTheme().Accent = nil, want non-nil")
	}
}

// TestNewLightThemeNonNil verifies the light theme returns a non-nil palette.
func TestNewLightThemeNonNil(t *testing.T) {
	got := NewLightTheme()
	if got.Background == nil {
		t.Errorf("NewLightTheme().Background = nil, want non-nil")
	}
	if got.Text == nil {
		t.Errorf("NewLightTheme().Text = nil, want non-nil")
	}
	if got.Accent == nil {
		t.Errorf("NewLightTheme().Accent = nil, want non-nil")
	}
}

// TestThemesDistinct verifies dark and light themes use different colors.
func TestThemesDistinct(t *testing.T) {
	dark := NewDarkTheme()
	light := NewLightTheme()

	dr, dg, db, da := dark.Background.RGBA()
	lr, lg, lb, la := light.Background.RGBA()
	if dr == lr && dg == lg && db == lb && da == la {
		t.Errorf("dark and light Background colors are equal, want distinct")
	}
}
