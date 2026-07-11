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
	if cfg.aiType != "random" {
		t.Errorf("aiType got %q, want %q", cfg.aiType, "random")
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
	t.Setenv("CARDCORE_TUI_DEBUG", "true")
	t.Setenv("CARDCORE_TUI_AI_TYPE", "pimc")

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
	if cfg.observer != false {
		t.Errorf("observer got %v, want false", cfg.observer)
	}
	if cfg.debug != true {
		t.Errorf("debug got %v, want true", cfg.debug)
	}
	if cfg.aiType != "pimc" {
		t.Errorf("aiType got %q, want %q", cfg.aiType, "pimc")
	}
}

// TestParseFlagsEnvFallbackObserver verifies that observer mode can be set via
// environment variables.
func TestParseFlagsEnvFallbackObserver(t *testing.T) {
	t.Setenv("CARDCORE_TUI_OBSERVE", "true")
	t.Setenv("CARDCORE_TUI_AI_TYPE", "heuristic")

	cfg, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.observer != true {
		t.Errorf("observer got %v, want true", cfg.observer)
	}
	if cfg.aiType != "heuristic" {
		t.Errorf("aiType got %q, want %q", cfg.aiType, "heuristic")
	}
}

// TestParseFlagsFlagOverride verifies that explicit flags take precedence over
// environment variables.
func TestParseFlagsFlagOverride(t *testing.T) {
	t.Setenv("CARDCORE_TUI_SERVER", "http://localhost:9090")
	t.Setenv("CARDCORE_TUI_SEAT", "2")
	t.Setenv("CARDCORE_TUI_SESSION", "session-123")
	t.Setenv("CARDCORE_TUI_TOKEN", "token-123")
	t.Setenv("CARDCORE_TUI_AI_TYPE", "pimc")

	cfg, err := parseFlags([]string{
		"-server", "http://localhost:1111",
		"-seat", "1",
		"-ai-type", "random",
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
	if cfg.token != "token-123" {
		t.Errorf("token got %q, want %q", cfg.token, "token-123")
	}
	if cfg.aiType != "random" {
		t.Errorf("aiType got %q, want %q", cfg.aiType, "random")
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
	if _, err := parseFlags([]string{}); err != nil {
		t.Errorf("parseFlags got error %v, want nil for auto-create player mode", err)
	}
	if _, err := parseFlags([]string{"-observe"}); err != nil {
		t.Errorf("parseFlags got error %v, want nil for auto-create observer mode", err)
	}
	if _, err := parseFlags([]string{"-observe", "-session", "x"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for observe with session")
	}
	if _, err := parseFlags([]string{"-token", "x"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for token without session")
	}
	if _, err := parseFlags([]string{"-session", "x"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for session without token")
	}
	if _, err := parseFlags([]string{"-seat", "-1"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for negative seat")
	}
}

// TestParseFlagsAITypeNoSession verifies that -ai-type is accepted without
// -session (auto-create path).
func TestParseFlagsAITypeNoSession(t *testing.T) {
	cfg, err := parseFlags([]string{"-ai-type", "pimc"})
	if err != nil {
		t.Fatalf("parseFlags got error %v, want nil for -ai-type pimc without session", err)
	}
	if cfg.aiType != "pimc" {
		t.Errorf("aiType got %q, want %q", cfg.aiType, "pimc")
	}

	cfg, err = parseFlags([]string{"-ai-type", "heuristic", "-observe"})
	if err != nil {
		t.Fatalf("parseFlags got error %v, want nil for -ai-type heuristic with observe", err)
	}
	if cfg.aiType != "heuristic" {
		t.Errorf("aiType got %q, want %q", cfg.aiType, "heuristic")
	}
}
