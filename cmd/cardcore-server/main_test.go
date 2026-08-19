package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartssession "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session/games/hearts"
)

// TestParseFlagsDefaults verifies default flag values when no arguments or
// environment variables are provided.
func TestParseFlagsDefaults(t *testing.T) {
	cfg, err := parseFlags([]string{}, testRegistry())
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	want := &serverConfig{
		addr:             "127.0.0.1:8080",
		logLevel:         "info",
		shutdownTimeout:  10,
		aiActionDelay:    1000,
		dealDisplayDelay: 1500,
		turnTimeout:      30000,
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
	t.Setenv("CARDCORE_SERVER_SHUTDOWN_TIMEOUT_SECS", "30")
	t.Setenv("CARDCORE_SERVER_AI_ACTION_DELAY_MS", "2000")
	t.Setenv("CARDCORE_SERVER_DEAL_DISPLAY_DELAY_MS", "2500")
	t.Setenv("CARDCORE_SERVER_TURN_TIMEOUT_MS", "60000")

	cfg, err := parseFlags([]string{}, testRegistry())
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	want := &serverConfig{
		addr:             "0.0.0.0:9090",
		logLevel:         "debug",
		shutdownTimeout:  30,
		aiActionDelay:    2000,
		dealDisplayDelay: 2500,
		turnTimeout:      60000,
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
		"-ai-action-delay-ms", "500",
	}, testRegistry())
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
	t.Setenv("CARDCORE_SERVER_SHUTDOWN_TIMEOUT_SECS", "-1")

	cfg, err := parseFlags([]string{}, testRegistry())
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
	if _, err := parseFlags([]string{"-shutdown-timeout-secs", "0"}, testRegistry()); err == nil {
		t.Errorf("parseFlags got nil error, want error for shutdown-timeout-secs=0")
	}
	if _, err := parseFlags([]string{"-ai-action-delay-ms", "-1"}, testRegistry()); err == nil {
		t.Errorf("parseFlags got nil error, want error for negative delay")
	}
}

// TestParseFlagsLogFile verifies that the -log-file flag and its environment
// variable are parsed correctly.
func TestParseFlagsLogFile(t *testing.T) {
	cfg, err := parseFlags([]string{"-log-file", "/tmp/server.log"}, testRegistry())
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.logFile != "/tmp/server.log" {
		t.Errorf("logFile got %q, want %q", cfg.logFile, "/tmp/server.log")
	}

	t.Setenv("CARDCORE_SERVER_LOG_FILE", "/env/server.log")
	cfg, err = parseFlags([]string{}, testRegistry())
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.logFile != "/env/server.log" {
		t.Errorf("logFile got %q, want %q", cfg.logFile, "/env/server.log")
	}
}

// TestParseFlagsVersion verifies that -version is accepted and bypasses
// unrelated flag validation.
func TestParseFlagsVersion(t *testing.T) {
	cfg, err := parseFlags([]string{"-version"}, testRegistry())
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if !cfg.showVersion {
		t.Errorf("showVersion got %v, want true", cfg.showVersion)
	}
}

// TestRegistryContainsHearts verifies that the default registry used by the
// server contains the Hearts game.
func TestRegistryContainsHearts(t *testing.T) {
	r := testRegistry()
	names := r.Names()
	if len(names) != 1 {
		t.Fatalf("got %d registered games, want 1", len(names))
	}
	if got, want := names[0], "hearts"; got != want {
		t.Errorf("game name got %q, want %q", got, want)
	}
}

// TestRunWithArgsAndSignalStartsAndShutsDown verifies that the server starts
// on a free port and shuts down cleanly when signaled.
func TestRunWithArgsAndSignalStartsAndShutsDown(t *testing.T) {
	sigCh := make(chan os.Signal, 1)
	done := make(chan int, 1)

	go func() {
		done <- runWithArgsAndSignal([]string{"-addr", "127.0.0.1:0", "-log-level", "warn"}, sigCh)
	}()

	time.Sleep(100 * time.Millisecond)
	sigCh <- os.Interrupt

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code got %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5s")
	}
}

// TestRunWithArgsAndSignalLogFile verifies that -log-file creates a log file
// and writes server output to it.
func TestRunWithArgsAndSignalLogFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "server.log")

	sigCh := make(chan os.Signal, 1)
	done := make(chan int, 1)
	go func() {
		done <- runWithArgsAndSignal([]string{
			"-addr", "127.0.0.1:0",
			"-log-file", logFile,
		}, sigCh)
	}()

	time.Sleep(100 * time.Millisecond)
	sigCh <- os.Interrupt

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code got %d, want 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within 5s")
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Error("log file is empty")
	}
}

// TestRunWithArgsAndSignalVersion verifies that -version prints build
// information to stdout and returns 0 without starting the server.
func TestRunWithArgsAndSignalVersion(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	code := runWithArgsAndSignal([]string{"-version"}, make(chan os.Signal, 1))

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}

	if code != 0 {
		t.Errorf("exit code got %d, want 0", code)
	}
	if !strings.Contains(string(out), "cardcore-server") {
		t.Errorf("version output got %q, want it to contain %q", string(out), "cardcore-server")
	}
}

// serverConfigsEqual reports whether two serverConfig values are identical.
func serverConfigsEqual(a, b *serverConfig) bool {
	return a.addr == b.addr &&
		a.logLevel == b.logLevel &&
		a.logFile == b.logFile &&
		a.shutdownTimeout == b.shutdownTimeout &&
		a.aiActionDelay == b.aiActionDelay &&
		a.dealDisplayDelay == b.dealDisplayDelay &&
		a.turnTimeout == b.turnTimeout
}

// testRegistry returns a registry with the Hearts game config for tests.
func testRegistry() *session.Registry {
	r := session.NewRegistry()
	r.Register(heartssession.NewGameConfig())
	return r
}
