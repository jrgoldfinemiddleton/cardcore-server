package heartsclient

import (
	"testing"
)

// TestNewPauseMessage verifies that NewPauseMessage builds a pause command.
func TestNewPauseMessage(t *testing.T) {
	cmd, err := NewPauseMessage("pause-1", 5)
	if err != nil {
		t.Fatalf("NewPauseMessage: %v", err)
	}
	if cmd.Type != "pause" {
		t.Errorf("Type = %q, want pause", cmd.Type)
	}
	if cmd.ActionID != "pause-1" {
		t.Errorf("ActionID = %q, want pause-1", cmd.ActionID)
	}
	if cmd.Seq != 5 {
		t.Errorf("Seq = %d, want 5", cmd.Seq)
	}
	if string(cmd.Payload) != "{}" {
		t.Errorf("Payload = %q, want {}", string(cmd.Payload))
	}
}

// TestNewResumeMessage verifies that NewResumeMessage builds a resume command.
func TestNewResumeMessage(t *testing.T) {
	cmd, err := NewResumeMessage("resume-1", 6)
	if err != nil {
		t.Fatalf("NewResumeMessage: %v", err)
	}
	if cmd.Type != "resume" {
		t.Errorf("Type = %q, want resume", cmd.Type)
	}
	if cmd.ActionID != "resume-1" {
		t.Errorf("ActionID = %q, want resume-1", cmd.ActionID)
	}
	if cmd.Seq != 6 {
		t.Errorf("Seq = %d, want 6", cmd.Seq)
	}
	if string(cmd.Payload) != "{}" {
		t.Errorf("Payload = %q, want {}", string(cmd.Payload))
	}
}
