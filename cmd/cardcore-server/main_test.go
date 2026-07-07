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

	want := &serverConfig{
		addr:                    "127.0.0.1:8080",
		logLevel:                "info",
		shutdownTimeout:         10,
		aiActionDelay:           1000,
		dealDisplayDelay:        1500,
		turnTimeout:             30000,
		heartsTrickDisplayDelay: 3000,
		heartsRoundDisplayDelay: 5000,
	}
	if !serverConfigsEqual(cfg, want) {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

// TestParseFlagsEnvFallback verifies that environment variables are used as
// defaults when no flags are provided.
func TestParseFlagsEnvFallback(t *testing.T) {
	t.Setenv("CARDCORE_SERVER_ADDR", "0.0.0.0:9090")
	t.Setenv("CARDCORE_SERVER_LOG_LEVEL", "debug")
	t.Setenv("CARDCORE_SERVER_SHUTDOWN_TIMEOUT", "30")
	t.Setenv("CARDCORE_SERVER_AI_ACTION_DELAY_MS", "2000")
	t.Setenv("CARDCORE_SERVER_DEAL_DISPLAY_DELAY_MS", "2500")
	t.Setenv("CARDCORE_SERVER_TURN_TIMEOUT_MS", "60000")
	t.Setenv("CARDCORE_SERVER_HEARTS_TRICK_DISPLAY_DELAY_MS", "4000")
	t.Setenv("CARDCORE_SERVER_HEARTS_ROUND_DISPLAY_DELAY_MS", "6000")

	cfg, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	want := &serverConfig{
		addr:                    "0.0.0.0:9090",
		logLevel:                "debug",
		shutdownTimeout:         30,
		aiActionDelay:           2000,
		dealDisplayDelay:        2500,
		turnTimeout:             60000,
		heartsTrickDisplayDelay: 4000,
		heartsRoundDisplayDelay: 6000,
	}
	if !serverConfigsEqual(cfg, want) {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

// TestParseFlagsFlagOverride verifies that explicit flags take precedence over
// environment variables.
func TestParseFlagsFlagOverride(t *testing.T) {
	t.Setenv("CARDCORE_SERVER_ADDR", "0.0.0.0:9090")
	t.Setenv("CARDCORE_SERVER_AI_ACTION_DELAY_MS", "2000")

	cfg, err := parseFlags([]string{
		"-addr", "127.0.0.1:7777",
		"-ai-action-delay", "500",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.addr != "127.0.0.1:7777" {
		t.Errorf("addr got %q, want %q", cfg.addr, "127.0.0.1:7777")
	}
	if cfg.aiActionDelay != 500 {
		t.Errorf("aiActionDelay got %d, want %d", cfg.aiActionDelay, 500)
	}
}

// TestParseFlagsInvalidEnv verifies that invalid environment variable values
// fall back to hardcoded defaults.
func TestParseFlagsInvalidEnv(t *testing.T) {
	t.Setenv("CARDCORE_SERVER_AI_ACTION_DELAY_MS", "not-an-int")
	t.Setenv("CARDCORE_SERVER_DEAL_DISPLAY_DELAY_MS", "-1")
	t.Setenv("CARDCORE_SERVER_SHUTDOWN_TIMEOUT", "-1")

	cfg, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.aiActionDelay != 1000 {
		t.Errorf("aiActionDelay got %d, want %d", cfg.aiActionDelay, 1000)
	}
	if cfg.dealDisplayDelay != 1500 {
		t.Errorf("dealDisplayDelay got %d, want %d", cfg.dealDisplayDelay, 1500)
	}
	if cfg.shutdownTimeout != 10 {
		t.Errorf("shutdownTimeout got %d, want %d", cfg.shutdownTimeout, 10)
	}
}

// TestParseFlagsInvalidFlag verifies validation of invalid explicit flag
// values.
func TestParseFlagsInvalidFlag(t *testing.T) {
	if _, err := parseFlags([]string{"-shutdown-timeout", "0"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for shutdown-timeout=0")
	}
	if _, err := parseFlags([]string{"-ai-action-delay", "-1"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for negative delay")
	}
}

// serverConfigsEqual reports whether two serverConfig values are identical.
func serverConfigsEqual(a, b *serverConfig) bool {
	return a.addr == b.addr &&
		a.logLevel == b.logLevel &&
		a.shutdownTimeout == b.shutdownTimeout &&
		a.aiActionDelay == b.aiActionDelay &&
		a.dealDisplayDelay == b.dealDisplayDelay &&
		a.turnTimeout == b.turnTimeout &&
		a.heartsTrickDisplayDelay == b.heartsTrickDisplayDelay &&
		a.heartsRoundDisplayDelay == b.heartsRoundDisplayDelay
}
