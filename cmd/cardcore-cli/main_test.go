package main

import (
	"path/filepath"
	"testing"
)

// TestParseFlagsDefaults verifies default flag values when using observer mode
// and no environment variables.
func TestParseFlagsDefaults(t *testing.T) {
	cfg, err := parseFlags([]string{"-observe"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.script != "" {
		t.Errorf("script got %q, want empty", cfg.script)
	}
	if cfg.addr != "http://127.0.0.1:8080" {
		t.Errorf("addr got %q, want %q", cfg.addr, "http://127.0.0.1:8080")
	}
	if cfg.game != "hearts" {
		t.Errorf("game got %q, want %q", cfg.game, "hearts")
	}
	if cfg.observe != true {
		t.Errorf("observe got %v, want true", cfg.observe)
	}
	if cfg.seat != 0 {
		t.Errorf("seat got %d, want 0", cfg.seat)
	}
	if cfg.pacing != 500 {
		t.Errorf("pacing got %d, want %d", cfg.pacing, 500)
	}
	if cfg.aiType != "random" {
		t.Errorf("aiType got %q, want %q", cfg.aiType, "random")
	}
	if cfg.exitDelay != 1000 {
		t.Errorf("exitDelay got %d, want %d", cfg.exitDelay, 1000)
	}
	if cfg.deleteOnExit != false {
		t.Errorf("deleteOnExit got %v, want false", cfg.deleteOnExit)
	}
}

// TestParseFlagsEnvFallback verifies that environment variables are used as
// defaults when no flags are provided.
func TestParseFlagsEnvFallback(t *testing.T) {
	t.Setenv("CARDCORE_CLI_SCRIPT", "script.json")
	t.Setenv("CARDCORE_CLI_ADDR", "http://localhost:9090")
	t.Setenv("CARDCORE_CLI_GAME", "hearts")
	t.Setenv("CARDCORE_CLI_OBSERVE", "false")
	t.Setenv("CARDCORE_CLI_SESSION_ID", "session-123")
	t.Setenv("CARDCORE_CLI_TOKEN", "token-123")
	t.Setenv("CARDCORE_CLI_SEAT", "2")
	t.Setenv("CARDCORE_CLI_DELETE_ON_EXIT", "true")
	t.Setenv("CARDCORE_CLI_PACING_MS", "100")
	t.Setenv("CARDCORE_CLI_AI_TYPE", "pimc")
	t.Setenv("CARDCORE_CLI_EXIT_DELAY_MS", "2000")

	cfg, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.script != "script.json" {
		t.Errorf("script got %q, want %q", cfg.script, "script.json")
	}
	if cfg.addr != "http://localhost:9090" {
		t.Errorf("addr got %q, want %q", cfg.addr, "http://localhost:9090")
	}
	if cfg.game != "hearts" {
		t.Errorf("game got %q, want %q", cfg.game, "hearts")
	}
	if cfg.observe != false {
		t.Errorf("observe got %v, want false", cfg.observe)
	}
	if cfg.sessionID != "session-123" {
		t.Errorf("sessionID got %q, want %q", cfg.sessionID, "session-123")
	}
	if cfg.token != "token-123" {
		t.Errorf("token got %q, want %q", cfg.token, "token-123")
	}
	if cfg.seat != 2 {
		t.Errorf("seat got %d, want 2", cfg.seat)
	}
	if cfg.deleteOnExit != true {
		t.Errorf("deleteOnExit got %v, want true", cfg.deleteOnExit)
	}
	if cfg.pacing != 100 {
		t.Errorf("pacing got %d, want 100", cfg.pacing)
	}
	if cfg.aiType != "pimc" {
		t.Errorf("aiType got %q, want %q", cfg.aiType, "pimc")
	}
	if cfg.exitDelay != 2000 {
		t.Errorf("exitDelay got %d, want 2000", cfg.exitDelay)
	}
}

// TestParseFlagsFlagOverride verifies that explicit flags take precedence over
// environment variables.
func TestParseFlagsFlagOverride(t *testing.T) {
	t.Setenv("CARDCORE_CLI_ADDR", "http://localhost:9090")
	t.Setenv("CARDCORE_CLI_PACING_MS", "100")
	t.Setenv("CARDCORE_CLI_AI_TYPE", "pimc")

	cfg, err := parseFlags([]string{
		"-observe",
		"-addr", "http://localhost:1111",
		"-pacing-ms", "0",
		"-ai-type", "random",
	})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.addr != "http://localhost:1111" {
		t.Errorf("addr got %q, want %q", cfg.addr, "http://localhost:1111")
	}
	if cfg.pacing != 0 {
		t.Errorf("pacing got %d, want 0", cfg.pacing)
	}
	if cfg.aiType != "random" {
		t.Errorf("aiType got %q, want %q", cfg.aiType, "random")
	}
	if cfg.observe != true {
		t.Errorf("observe got %v, want true", cfg.observe)
	}
}

// TestParseFlagsInvalidEnv verifies that invalid environment variable values
// fall back to hardcoded defaults.
func TestParseFlagsInvalidEnv(t *testing.T) {
	t.Setenv("CARDCORE_CLI_PACING_MS", "not-an-int")
	t.Setenv("CARDCORE_CLI_EXIT_DELAY_MS", "-1")
	t.Setenv("CARDCORE_CLI_SEAT", "-5")

	cfg, err := parseFlags([]string{"-observe"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.pacing != 500 {
		t.Errorf("pacing got %d, want %d", cfg.pacing, 500)
	}
	if cfg.exitDelay != 1000 {
		t.Errorf("exitDelay got %d, want %d", cfg.exitDelay, 1000)
	}
	if cfg.seat != 0 {
		t.Errorf("seat got %d, want 0", cfg.seat)
	}
}

// TestParseFlagsValidation verifies flag validation rules.
func TestParseFlagsValidation(t *testing.T) {
	if _, err := parseFlags([]string{}); err == nil {
		t.Errorf("parseFlags got nil error, want error for missing script")
	}
	if _, err := parseFlags([]string{"-observe", "-session-id", "x"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for observe with session-id")
	}
	if _, err := parseFlags([]string{"-session-id", "x"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for session-id without token")
	}
	if _, err := parseFlags([]string{"-token", "x"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for token without session-id")
	}
	if _, err := parseFlags([]string{"-observe", "-seat", "-1"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for negative seat")
	}
	if _, err := parseFlags([]string{"-observe", "-pacing-ms", "-1"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for negative pacing-ms")
	}
	if _, err := parseFlags([]string{"-observe", "-exit-delay-ms", "-1"}); err == nil {
		t.Errorf("parseFlags got nil error, want error for negative exit-delay-ms")
	}
}

// TestRunMissingScriptFile verifies that run() returns an error when the
// configured script file does not exist.
func TestRunMissingScriptFile(t *testing.T) {
	cfg := &cliConfig{
		script: filepath.Join(t.TempDir(), "missing.json"),
		addr:   "http://127.0.0.1:8080",
		game:   "hearts",
		pacing: 10,
	}
	if err := run(cfg); err == nil {
		t.Fatal("run() got nil error, want error for missing script file")
	}
}

// TestRunInvalidServerAddress verifies that run() returns an error when the
// server cannot be reached.
func TestRunInvalidServerAddress(t *testing.T) {
	cfg := &cliConfig{
		observe: true,
		addr:    "http://127.0.0.1:1",
		game:    "hearts",
		pacing:  10,
	}
	if err := run(cfg); err == nil {
		t.Fatal("run() got nil error, want error for invalid server address")
	}
}

// TestRunUnsupportedGame verifies that run() returns an error for an
// unsupported game in observer mode.
func TestRunUnsupportedGame(t *testing.T) {
	cfg := &cliConfig{
		observe: true,
		addr:    "http://127.0.0.1:8080",
		game:    "chess",
		pacing:  10,
	}
	if err := run(cfg); err == nil {
		t.Fatal("run() got nil error, want error for unsupported game")
	}
}

// TestRunJoinModeInvalidSession verifies that run() returns an error when
// joining a session that does not exist.
func TestRunJoinModeInvalidSession(t *testing.T) {
	cfg := &cliConfig{
		sessionID: "nonexistent-session",
		token:     "invalid-token",
		addr:      "http://127.0.0.1:1",
		game:      "hearts",
		pacing:    10,
		seat:      0,
	}
	if err := run(cfg); err == nil {
		t.Fatal("run() got nil error, want error for invalid session join")
	}
}

// TestNewGameFormatter verifies that the formatter factory returns an error
// for unsupported games and succeeds for Hearts.
func TestNewGameFormatter(t *testing.T) {
	if _, err := newGameFormatter("hearts"); err != nil {
		t.Errorf("newGameFormatter(hearts) got error %v, want nil", err)
	}
	if _, err := newGameFormatter("chess"); err == nil {
		t.Error("newGameFormatter(chess) got nil error, want error")
	}
}

// TestNewGameBuilder verifies that the builder factory returns an error for
// unsupported games and succeeds for Hearts.
func TestNewGameBuilder(t *testing.T) {
	if _, err := newGameBuilder("hearts"); err != nil {
		t.Errorf("newGameBuilder(hearts) got error %v, want nil", err)
	}
	if _, err := newGameBuilder("chess"); err == nil {
		t.Error("newGameBuilder(chess) got nil error, want error")
	}
}
