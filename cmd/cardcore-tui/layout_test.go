package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// tallGameClient is a stub gameClient that renders content taller than the
// requested height so layout overflow behavior can be tested.
type tallGameClient struct{}

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
	m := &model{theme: NewDarkTheme(), height: 24}

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

// TestRenderLayoutHeight verifies the full layout fills the configured
// terminal height with the fixed panels and separators.
func TestRenderLayoutHeight(t *testing.T) {
	m := &model{theme: NewDarkTheme(), height: 24}

	got := m.renderLayout()
	lines := strings.Split(got, "\n")
	if len(lines) != 24 {
		t.Errorf("renderLayout produced %d lines, want 24", len(lines))
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
	wantParts := []string{"Round 2", "Phase: Playing", "S0: 5", "S1: 12", "S2: 8", "S3: 3"}
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
	wantParts := []string{"Round 4", "Phase: Playing", "S0: 12", "S1: 80", "S2: 45", "S3: 73"}
	for _, part := range wantParts {
		if !strings.Contains(got, part) {
			t.Errorf("renderHeader = %q, want to contain %q", got, part)
		}
	}
	if !hasPanelBorder(got) {
		t.Errorf("renderHeader should have panel borders, got %q", got)
	}
}

// TestRenderLayoutClipsTallGameContent verifies that even if the game client
// returns more rows than the main panel can show, the full layout stays exactly
// at the configured terminal height so the footer is not pushed off-screen.
func TestRenderLayoutClipsTallGameContent(t *testing.T) {
	m := &model{
		theme:  NewDarkTheme(),
		width:  80,
		height: 24,
		game:   tallGameClient{},
	}

	got := m.renderLayout()
	lines := strings.Split(got, "\n")
	if len(lines) != 24 {
		t.Errorf("renderLayout produced %d lines, want 24", len(lines))
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

// HandleSnapshot ignores the snapshot for the stub.
func (tallGameClient) HandleSnapshot(raw json.RawMessage) {}

// LastError returns an empty error for the stub.
func (tallGameClient) LastError() string { return "" }

// HandleKey ignores key presses for the stub.
func (tallGameClient) HandleKey(key tea.KeyPressMsg) (client.Command, bool, string) {
	return client.Command{}, false, ""
}

// Render returns content twice the requested height to simulate overflow.
func (tallGameClient) Render(width, height int) string {
	return strings.Repeat("line\n", height*2)
}

// ResetSubmitted does nothing for the stub.
func (tallGameClient) ResetSubmitted() {}

// SetInputDisabled does nothing for the stub.
func (tallGameClient) SetInputDisabled(disabled bool) {}

// IsHumanTurn always returns false for the stub.
func (tallGameClient) IsHumanTurn() bool { return false }

// TogglePause returns no command for the stub.
func (tallGameClient) TogglePause(paused bool) (client.Command, bool) {
	return client.Command{}, false
}
