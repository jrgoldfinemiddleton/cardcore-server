package menu

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	heartstui "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-tui/hearts"
)

// TestDefaultConfigReturnedOnStart verifies that selecting Start Game
// immediately (without changing any option) returns the initial config
// unchanged.
func TestDefaultConfigReturnedOnStart(t *testing.T) {
	initial := defaultTestConfig()
	m := newModel(initial, heartstui.NewDarkTheme())

	// Navigate down to Start Game (item index 5).
	for range itemStart {
		m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.cursor != itemStart {
		t.Fatalf("got cursor %d, want %d", m.cursor, itemStart)
	}

	// Press Enter to start the game.
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, ok := model.(*menuModel)
	if !ok {
		t.Fatalf("got model type %T, want *menuModel", model)
	}

	isQuitCmd(t, cmd)
	if m.err != nil {
		t.Fatalf("got err %v, want nil", m.err)
	}
	if m.result == nil {
		t.Fatalf("got nil result, want non-nil")
	}

	got := *m.result
	if got.Server != initial.Server {
		t.Errorf("Server got %q, want %q", got.Server, initial.Server)
	}
	if got.Game != initial.Game {
		t.Errorf("Game got %q, want %q", got.Game, initial.Game)
	}
	if got.AIType != initial.AIType {
		t.Errorf("AIType got %q, want %q", got.AIType, initial.AIType)
	}
	if got.Observer != initial.Observer {
		t.Errorf("Observer got %v, want %v", got.Observer, initial.Observer)
	}
	if got.Theme != initial.Theme {
		t.Errorf("Theme got %q, want %q", got.Theme, initial.Theme)
	}
	if got.Debug != initial.Debug {
		t.Errorf("Debug got %v, want %v", got.Debug, initial.Debug)
	}
}

// TestAIDifficultyCycling verifies that Enter on the AI Difficulty item
// cycles through Easy, Medium, Hard, and back to Easy.
func TestAIDifficultyCycling(t *testing.T) {
	m := newModel(defaultTestConfig(), heartstui.NewDarkTheme())

	// Navigate to AI Difficulty (item index 2).
	for range itemAIDifficulty {
		m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.cursor != itemAIDifficulty {
		t.Fatalf("got cursor %d, want %d", m.cursor, itemAIDifficulty)
	}

	// Initial state: Easy (index 0).
	if m.aiDiffIdx != 0 {
		t.Fatalf("got aiDiffIdx %d, want 0", m.aiDiffIdx)
	}

	// Press Enter: Easy -> Medium.
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.aiDiffIdx != 1 {
		t.Errorf("got aiDiffIdx %d after 1 Enter, want 1", m.aiDiffIdx)
	}

	// Press Enter: Medium -> Hard.
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.aiDiffIdx != 2 {
		t.Errorf("got aiDiffIdx %d after 2 Enter, want 2", m.aiDiffIdx)
	}

	// Press Enter: Hard -> Easy (wraps around).
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.aiDiffIdx != 0 {
		t.Errorf("got aiDiffIdx %d after 3 Enter, want 0", m.aiDiffIdx)
	}
}

// TestObserverAndThemeToggle verifies that Enter toggles the Observer and
// Theme items.
func TestObserverAndThemeToggle(t *testing.T) {
	m := newModel(defaultTestConfig(), heartstui.NewDarkTheme())

	// Navigate to Observer (item index 3).
	for range itemObserver {
		m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.cursor != itemObserver {
		t.Fatalf("got cursor %d, want %d", m.cursor, itemObserver)
	}

	// Initial state: observer false.
	if m.observer {
		t.Fatalf("got observer true, want false")
	}

	// Press Enter: No -> Yes.
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.observer {
		t.Errorf("got observer false after Enter, want true")
	}

	// Navigate to Theme (item index 4).
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor != itemTheme {
		t.Fatalf("got cursor %d, want %d", m.cursor, itemTheme)
	}

	// Initial state: theme dark (index 0).
	if m.themeIdx != 0 {
		t.Fatalf("got themeIdx %d, want 0", m.themeIdx)
	}

	// Press Enter: dark -> light.
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.themeIdx != 1 {
		t.Errorf("got themeIdx %d after Enter, want 1", m.themeIdx)
	}
}

// TestEscReturnsCancelled verifies that pressing Esc sets err to ErrCancelled
// and returns a quit command.
func TestEscReturnsCancelled(t *testing.T) {
	m := newModel(defaultTestConfig(), heartstui.NewDarkTheme())

	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	result, ok := model.(*menuModel)
	if !ok {
		t.Fatalf("got model type %T, want *menuModel", model)
	}

	isQuitCmd(t, cmd)
	if !errors.Is(result.err, ErrCancelled) {
		t.Errorf("got err %v, want ErrCancelled", result.err)
	}
	if result.result != nil {
		t.Errorf("got result %v, want nil", result.result)
	}
}

// TestRenderContainsLabelsAndValues verifies that the rendered output
// contains all item labels and their current values.
func TestRenderContainsLabelsAndValues(t *testing.T) {
	cfg := defaultTestConfig()
	m := newModel(cfg, heartstui.NewDarkTheme())

	out := m.render()

	labels := []string{"Game", "Server", "AI Difficulty", "Observer", "Theme", "Start Game"}
	for _, label := range labels {
		if !strings.Contains(out, label) {
			t.Errorf("render output missing label %q", label)
		}
	}

	values := []string{"Hearts", cfg.Server, "Easy (random)", "No", "dark"}
	for _, value := range values {
		if !strings.Contains(out, value) {
			t.Errorf("render output missing value %q", value)
		}
	}

	if !strings.Contains(out, "Cardcore") {
		t.Errorf("render output missing title %q", "Cardcore")
	}
}

// TestThemeUpdatesInRealTime verifies that toggling the Theme item updates
// the model's palette immediately so the rendered view honors the new theme.
func TestThemeUpdatesInRealTime(t *testing.T) {
	m := newModel(defaultTestConfig(), heartstui.NewDarkTheme())

	// Navigate to Theme (item index 4).
	for range itemTheme {
		m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.cursor != itemTheme {
		t.Fatalf("got cursor %d, want %d", m.cursor, itemTheme)
	}

	// Initial state: dark theme.
	if !reflect.DeepEqual(m.theme, heartstui.NewDarkTheme()) {
		t.Fatalf("got theme %v, want dark theme", m.theme)
	}

	// Press Enter: dark -> light.
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !reflect.DeepEqual(m.theme, heartstui.NewLightTheme()) {
		t.Errorf("got theme %v, want light theme", m.theme)
	}

	// Press Enter: light -> dark.
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !reflect.DeepEqual(m.theme, heartstui.NewDarkTheme()) {
		t.Errorf("got theme %v, want dark theme", m.theme)
	}
}

// TestServerEditMode verifies that pressing Enter on the Server item enters
// inline editing, printable keys update the buffer, Backspace removes the last
// rune, Enter confirms, and Esc cancels.
func TestServerEditMode(t *testing.T) {
	m := newModel(defaultTestConfig(), heartstui.NewDarkTheme())

	// Navigate to Server (item index 1).
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor != itemServer {
		t.Fatalf("got cursor %d, want %d", m.cursor, itemServer)
	}

	// Enter edit mode.
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.editingServer {
		t.Fatalf("got editingServer false, want true")
	}
	if m.serverBuffer != m.config.Server {
		t.Fatalf("got serverBuffer %q, want %q", m.serverBuffer, m.config.Server)
	}

	// Type a character.
	m = sendKey(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.serverBuffer != m.config.Server+"x" {
		t.Errorf("got serverBuffer %q, want %q", m.serverBuffer, m.config.Server+"x")
	}

	// Backspace removes the typed character.
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.serverBuffer != m.config.Server {
		t.Errorf("got serverBuffer %q, want %q", m.serverBuffer, m.config.Server)
	}

	// Type a new URL and confirm.
	wantServer := "http://localhost:9999"
	m.serverBuffer = ""
	for _, r := range wantServer {
		m = sendKey(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if m.serverBuffer != wantServer {
		t.Errorf("got serverBuffer %q, want %q", m.serverBuffer, wantServer)
	}

	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editingServer {
		t.Errorf("got editingServer true after confirm, want false")
	}
	if m.config.Server != wantServer {
		t.Errorf("got config.Server %q, want %q", m.config.Server, wantServer)
	}

	// Enter edit mode again and cancel with Esc.
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.serverBuffer = "changed"
	m = sendKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.editingServer {
		t.Errorf("got editingServer true after cancel, want false")
	}
	if m.config.Server != wantServer {
		t.Errorf("got config.Server %q, want %q", m.config.Server, wantServer)
	}
}

// TestAIDifficultyMapping verifies that the aiDifficulties table maps
// Easy to random, Medium to heuristic, and Hard to pimc.
func TestAIDifficultyMapping(t *testing.T) {
	want := []struct {
		label  string
		aiType string
	}{
		{"Easy", "random"},
		{"Medium", "heuristic"},
		{"Hard", "pimc"},
	}

	if len(aiDifficulties) != len(want) {
		t.Fatalf("got %d difficulties, want %d", len(aiDifficulties), len(want))
	}

	for i, w := range want {
		got := aiDifficulties[i]
		if got.label != w.label {
			t.Errorf("aiDifficulties[%d].label got %q, want %q", i, got.label, w.label)
		}
		if got.aiType != w.aiType {
			t.Errorf("aiDifficulties[%d].aiType got %q, want %q", i, got.aiType, w.aiType)
		}
	}
}

// defaultTestConfig returns a Config with standard default values for tests.
func defaultTestConfig() Config {
	return Config{
		Server:   "http://localhost:8080",
		Game:     "hearts",
		AIType:   "random",
		Observer: false,
		Theme:    "dark",
		Debug:    false,
	}
}

// sendKey sends a key press to the model and returns the updated model. It
// fails the test if the returned model is not a *menuModel.
func sendKey(t *testing.T, m *menuModel, key tea.KeyPressMsg) *menuModel {
	t.Helper()
	model, _ := m.Update(key)
	result, ok := model.(*menuModel)
	if !ok {
		t.Fatalf("sendKey: got model type %T, want *menuModel", model)
	}
	return result
}

// isQuitCmd verifies that cmd is a tea.Quit command by calling it and
// checking the returned message type.
func isQuitCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Errorf("got nil cmd, want tea.Quit")
		return
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("got msg %T, want tea.QuitMsg", msg)
	}
}
