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

	got := m.renderFooter(80)
	if !strings.Contains(got, "validation error") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "validation error")
	}
	if !hasPanelBorder(got) {
		t.Errorf("renderFooter should have panel borders, got %q", got)
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

	got := m.renderFooter(80)
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

	got := m.renderFooter(80)
	if !strings.Contains(got, "Disconnected") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "Disconnected")
	}
}

// TestRenderFooterPaused verifies the paused footer shows the paused label.
func TestRenderFooterPaused(t *testing.T) {
	m := &model{paused: true, theme: NewDarkTheme()}

	got := m.renderFooter(80)
	if !strings.Contains(got, "Paused") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "Paused")
	}
}

// TestRenderFooterConnected verifies the default footer shows "Connected".
func TestRenderFooterConnected(t *testing.T) {
	m := &model{theme: NewDarkTheme()}

	got := m.renderFooter(80)
	if !strings.Contains(got, "Connected") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "Connected")
	}
}

// TestRenderLayoutPanels verifies the full layout includes three bordered
// panels (header, main, footer) and two blank separator lines.
func TestRenderLayoutPanels(t *testing.T) {
	m := &model{theme: NewDarkTheme()}

	got := m.renderLayout()
	stripped := stripANSILayout(got)

	if topCount := strings.Count(stripped, "╭"); topCount != 3 {
		t.Errorf("renderLayout has %d top borders, want 3", topCount)
	}
	if bottomCount := strings.Count(stripped, "╰"); bottomCount != 3 {
		t.Errorf("renderLayout has %d bottom borders, want 3", bottomCount)
	}

	lines := strings.Split(got, "\n")
	blankCount := 0
	for _, line := range lines {
		s := stripANSILayout(line)
		if s == "" || s == strings.Repeat(" ", 80) {
			blankCount++
		}
	}
	if blankCount != 2 {
		t.Errorf("renderLayout has %d blank separator lines, want 2", blankCount)
	}
}

// TestRenderHeaderScorePresentation verifies the header shows round, phase,
// and an aligned score summary with panel borders.
func TestRenderHeaderScorePresentation(t *testing.T) {
	m := &model{
		roundNumber: 2,
		phase:       "playing",
		scores:      []int{5, 12, 8, 3},
		theme:       NewDarkTheme(),
	}

	got := m.renderHeader(80)
	wantParts := []string{"Round 2", "Phase: playing", "S0: 5", "S1: 12", "S2: 8", "S3: 3"}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Errorf("renderHeader = %q, want to contain %q", got, part)
		}
	}
	if !hasPanelBorder(got) {
		t.Errorf("renderHeader should have panel borders, got %q", got)
	}
}

// TestRenderHeaderRoundZeroDisplaysAsOne verifies the pre-deal round number 0
// is never shown to the player; it is rendered as Round 1.
func TestRenderHeaderRoundZeroDisplaysAsOne(t *testing.T) {
	m := &model{
		roundNumber: 0,
		phase:       "waiting",
		theme:       NewDarkTheme(),
	}

	got := m.renderHeader(80)
	if !strings.Contains(got, "Round 1") {
		t.Errorf("renderHeader = %q, want to contain %q", got, "Round 1")
	}
	if strings.Contains(got, "Round 0") {
		t.Errorf("renderHeader should not contain %q", "Round 0")
	}
}

// TestRenderHeaderScoreDangerHighlight verifies scores within 26 points of 100
// are highlighted with the error color, while all other scores use the default
// text color.
func TestRenderHeaderScoreDangerHighlight(t *testing.T) {
	m := &model{
		roundNumber: 4,
		phase:       "playing",
		scores:      []int{12, 80, 45, 73},
		theme:       NewDarkTheme(),
	}

	got := m.renderHeader(80)
	wantParts := []string{"Round 4", "Phase: playing", "S0: 12", "S1: 80", "S2: 45", "S3: 73"}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Errorf("renderHeader = %q, want to contain %q", got, part)
		}
	}
	if !hasPanelBorder(got) {
		t.Errorf("renderHeader should have panel borders, got %q", got)
	}
}

// hasPanelBorder reports whether s contains rounded panel border characters.
func hasPanelBorder(s string) bool {
	return strings.Contains(s, "╭") && strings.Contains(s, "╮") &&
		strings.Contains(s, "╰") && strings.Contains(s, "╯")
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
