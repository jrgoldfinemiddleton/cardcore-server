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
	m := &model{disconnected: true}

	got := m.renderFooter()
	if !strings.Contains(got, "Disconnected") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "Disconnected")
	}
}

// TestRenderFooterConnected verifies the default footer shows "Connected".
func TestRenderFooterConnected(t *testing.T) {
	m := &model{}

	got := m.renderFooter()
	if !strings.Contains(got, "Connected") {
		t.Errorf("renderFooter = %q, want to contain %q", got, "Connected")
	}
}
