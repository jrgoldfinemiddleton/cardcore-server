package main

import (
	"strings"
	"testing"
)

// TestRenderFooterErrorPriority verifies the error flash takes priority over
// status messages and connection state.
func TestRenderFooterErrorPriority(t *testing.T) {
	m := &model{
		errMsg:       "validation error",
		statusMsg:    "Game ended",
		disconnected: true,
		theme:        NewDarkTheme(),
	}

	got := m.renderFooter()
	if !strings.Contains(got, "validation error") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "validation error")
	}
}

// TestRenderFooterStatusPriority verifies the mapped WebSocket close reason
// takes priority over the generic "Disconnected" label.
func TestRenderFooterStatusPriority(t *testing.T) {
	m := &model{
		statusMsg:    "Game ended",
		disconnected: true,
		theme:        NewDarkTheme(),
	}

	got := m.renderFooter()
	if !strings.Contains(got, "Game ended") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "Game ended")
	}
	if strings.Contains(got, "Disconnected") {
		t.Errorf("renderFooter = %q, should not contain 'Disconnected' when statusMsg is set", got)
	}
}

// TestRenderFooterDisconnected verifies a disconnected model with no status
// message shows the generic "Disconnected" label.
func TestRenderFooterDisconnected(t *testing.T) {
	m := &model{disconnected: true, theme: NewDarkTheme()}

	got := m.renderFooter()
	if !strings.Contains(got, "Disconnected") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "Disconnected")
	}
}

// TestRenderFooterPaused verifies the paused footer shows the paused label.
func TestRenderFooterPaused(t *testing.T) {
	m := &model{paused: true, theme: NewDarkTheme()}

	got := m.renderFooter()
	if !strings.Contains(got, "Paused") {
		t.Errorf("renderFooter = %q, want to contain 'Paused'", got)
	}
}

// TestRenderFooterConnected verifies the default footer shows "Connected".
func TestRenderFooterConnected(t *testing.T) {
	m := &model{theme: NewDarkTheme()}

	got := m.renderFooter()
	if !strings.Contains(got, "Connected") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "Connected")
	}
}

// TestRenderLayoutBlankLines verifies that the layout includes blank lines
// between the header, main area, and footer.
func TestRenderLayoutBlankLines(t *testing.T) {
	m := &model{theme: NewDarkTheme()}

	got := m.renderLayout()
	lines := strings.Split(got, "\n")

	// Expect header, blank, main, blank, footer.
	if len(lines) < 5 {
		t.Fatalf("renderLayout has %d lines, want at least 5", len(lines))
	}
	stripped1 := stripANSILayout(lines[1])
	if stripped1 != "" && stripped1 != strings.Repeat(" ", 80) {
		t.Errorf("renderLayout blank line after header not blank: %q", lines[1])
	}
	stripped3 := stripANSILayout(lines[3])
	if stripped3 != "" && stripped3 != strings.Repeat(" ", 80) {
		t.Errorf("renderLayout blank line before footer not blank: %q", lines[3])
	}
}

// stripANSILayout removes ANSI escape sequences from s.
func stripANSILayout(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\u001b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
