package main

import (
	"testing"
)

// TestParseFlagsDefaults verifies default flag values when no arguments or
// environment variables are provided.
func TestParseFlagsDefaults(t *testing.T) {
	cfg, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.server != "http://localhost:8080" {
		t.Errorf("server got %q, want %q", cfg.server, "http://localhost:8080")
	}
	if cfg.game != "hearts" {
		t.Errorf("game got %q, want %q", cfg.game, "hearts")
	}
	if cfg.session != "" {
		t.Errorf("session got %q, want empty", cfg.session)
	}
	if cfg.token != "" {
		t.Errorf("token got %q, want empty", cfg.token)
	}
	if cfg.seat != 0 {
		t.Errorf("seat got %d, want 0", cfg.seat)
	}
	if cfg.observer != false {
		t.Errorf("observer got %v, want false", cfg.observer)
	}
	if cfg.debug != false {
		t.Errorf("debug got %v, want false", cfg.debug)
	}
}

// TestParseFlagsEnvFallback verifies that environment variables are used as
// defaults when no flags are provided.
func TestParseFlagsEnvFallback(t *testing.T) {
	t.Setenv("CARDCORE_TUI_SERVER", "http://localhost:9090")
	t.Setenv("CARDCORE_TUI_GAME", "hearts")
	t.Setenv("CARDCORE_TUI_SESSION", "session-123")
	t.Setenv("CARDCORE_TUI_TOKEN", "token-123")
	t.Setenv("CARDCORE_TUI_SEAT", "2")
	t.Setenv("CARDCORE_TUI_OBSERVE", "true")
	t.Setenv("CARDCORE_TUI_DEBUG", "true")

	cfg, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.server != "http://localhost:9090" {
		t.Errorf("server got %q, want %q", cfg.server, "http://localhost:9090")
	}
	if cfg.game != "hearts" {
		t.Errorf("game got %q, want %q", cfg.game, "hearts")
	}
	if cfg.session != "session-123" {
		t.Errorf("session got %q, want %q", cfg.session, "session-123")
	}
	if cfg.token != "token-123" {
		t.Errorf("token got %q, want %q", cfg.token, "token-123")
	}
	if cfg.seat != 2 {
		t.Errorf("seat got %d, want 2", cfg.seat)
	}
	if cfg.observer != true {
		t.Errorf("observer got %v, want true", cfg.observer)
	}
	if cfg.debug != true {
		t.Errorf("debug got %v, want true", cfg.debug)
	}
}

// TestParseFlagsFlagOverride verifies that explicit flags take precedence over
// environment variables.
func TestParseFlagsFlagOverride(t *testing.T) {
	t.Setenv("CARDCORE_TUI_SERVER", "http://localhost:9090")
	t.Setenv("CARDCORE_TUI_SEAT", "2")
	t.Setenv("CARDCORE_TUI_SESSION", "session-123")

	cfg, err := parseFlags([]string{
		"-server", "http://localhost:1111",
		"-seat", "1",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.server != "http://localhost:1111" {
		t.Errorf("server got %q, want %q", cfg.server, "http://localhost:1111")
	}
	if cfg.seat != 1 {
		t.Errorf("seat got %d, want 1", cfg.seat)
	}
	if cfg.session != "session-123" {
		t.Errorf("session got %q, want %q", cfg.session, "session-123")
	}
}

// TestParseFlagsInvalidEnv verifies that invalid environment variable values
// fall back to hardcoded defaults.
func TestParseFlagsInvalidEnv(t *testing.T) {
	t.Setenv("CARDCORE_TUI_SEAT", "not-an-int")
	t.Setenv("CARDCORE_TUI_OBSERVE", "not-a-bool")

	cfg, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.seat != 0 {
		t.Errorf("seat got %d, want 0", cfg.seat)
	}
	if cfg.observer != false {
		t.Errorf("observer got %v, want false", cfg.observer)
	}
}

// TestParseFlagsValidation verifies flag validation rules.
func TestParseFlagsValidation(t *testing.T) {
	if _, err := parseFlags([]string{"-observe"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for observer without session")
	}
	if _, err := parseFlags([]string{"-token", "x"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for token without session")
	}
	if _, err := parseFlags([]string{"-seat", "-1"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for negative seat")
	}
}
