package heartstui

import (
	"testing"
)

// TestNewDarkThemeWinnerBg verifies the dark Hearts palette embeds a
// populated shell palette and adds a non-nil WinnerBg color.
func TestNewDarkThemeWinnerBg(t *testing.T) {
	got := NewDarkTheme()
	if got.Background == nil {
		t.Errorf("NewDarkTheme().Background = nil, want non-nil (promoted from the shell theme)")
	}
	if got.WinnerBg == nil {
		t.Errorf("NewDarkTheme().WinnerBg = nil, want non-nil")
	}
}

// TestNewLightThemeWinnerBg verifies the light Hearts palette embeds a
// populated shell palette and adds a non-nil WinnerBg color.
func TestNewLightThemeWinnerBg(t *testing.T) {
	got := NewLightTheme()
	if got.Background == nil {
		t.Errorf("NewLightTheme().Background = nil, want non-nil (promoted from the shell theme)")
	}
	if got.WinnerBg == nil {
		t.Errorf("NewLightTheme().WinnerBg = nil, want non-nil")
	}
}

// TestThemesWinnerBgDistinct verifies dark and light themes use different
// WinnerBg colors.
func TestThemesWinnerBgDistinct(t *testing.T) {
	dark := NewDarkTheme()
	light := NewLightTheme()

	dr, dg, db, da := dark.WinnerBg.RGBA()
	lr, lg, lb, la := light.WinnerBg.RGBA()
	if dr == lr && dg == lg && db == lb && da == la {
		t.Errorf("dark and light WinnerBg colors are equal, want distinct")
	}
}
