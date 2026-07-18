package menu

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	heartstui "github.com/jrgoldfinemiddleton/cardcore-server/cmd/cardcore-tui/hearts"
)

// Theme is the color palette for the menu. It is an alias for
// heartstui.Theme so the menu shares the same palette as the rest of the
// TUI without importing the hearts package's render functions.
type Theme = heartstui.Theme

// Config holds the resolved menu selections. The caller translates this into
// the authoritative tuiConfig in cmd/cardcore-tui/main.go.
type Config struct {
	// Server is the base URL of the cardcore server (e.g., "http://localhost:8080").
	Server string
	// Game selects which game to play (e.g., "hearts"). Display-only in the menu.
	Game string
	// AIType selects the AI player type: "random", "heuristic", or "pimc".
	AIType string
	// Observer enables receive-only mode where all hands are visible.
	Observer bool
	// Theme selects the color palette: "dark" or "light".
	Theme string
	// Debug enables logging to a file for troubleshooting.
	Debug bool
}

// aiDifficulty pairs a display label with its server-side AI type identifier.
type aiDifficulty struct {
	// label is the human-readable difficulty name shown in the menu.
	label string
	// aiType is the server-side AI type identifier passed to the session.
	aiType string
}

// ErrCancelled is returned by Run when the user presses Esc to dismiss the
// menu without starting a game.
var ErrCancelled = errors.New("menu cancelled")

// aiDifficulties is the ordered list of AI difficulty options shown in the
// menu. The index cycles on Enter.
var aiDifficulties = []aiDifficulty{
	{label: "Easy", aiType: "random"},
	{label: "Medium", aiType: "heuristic"},
	{label: "Hard", aiType: "pimc"},
}

// themeOptions is the ordered list of theme names shown in the menu. The
// index cycles on Enter.
var themeOptions = []string{"dark", "light"}

// Run starts the menu wizard and blocks until the user either starts a game
// or cancels. It returns the resolved Config on success, or ErrCancelled when
// the user presses Esc. The theme parameter supplies the color palette for
// rendering.
func Run(initial Config, theme Theme) (*Config, error) {
	m := newModel(initial, theme)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("menu: %w", err)
	}
	result, ok := final.(*menuModel)
	if !ok {
		return nil, fmt.Errorf("menu: unexpected model type %T", final)
	}
	if result.err != nil {
		return nil, result.err
	}
	if result.result == nil {
		return nil, ErrCancelled
	}
	return result.result, nil
}

// newModel constructs the initial menu model from the given Config and theme.
// The AI difficulty index, observer flag, and theme index are derived from the
// initial config so the menu opens showing the current values.
func newModel(initial Config, theme Theme) *menuModel {
	return &menuModel{
		config:    initial,
		theme:     theme,
		aiDiffIdx: initialAIDifficulty(initial.AIType),
		observer:  initial.Observer,
		themeIdx:  initialThemeIdx(initial.Theme),
	}
}

// initialAIDifficulty returns the index into aiDifficulties matching the given
// AI type, or 0 if no match is found.
func initialAIDifficulty(aiType string) int {
	for i, d := range aiDifficulties {
		if d.aiType == aiType {
			return i
		}
	}
	return 0
}

// initialThemeIdx returns the index into themeOptions matching the given
// theme name, or 0 if no match is found.
func initialThemeIdx(theme string) int {
	for i, t := range themeOptions {
		if t == theme {
			return i
		}
	}
	return 0
}

// themeAtIndex returns the Theme palette for the theme option at the given
// index.
func themeAtIndex(idx int) Theme {
	if idx >= 0 && idx < len(themeOptions) && themeOptions[idx] == "light" {
		return heartstui.NewLightTheme()
	}
	return heartstui.NewDarkTheme()
}

// gameDisplayName returns a pretty label for a supported game name.
func gameDisplayName(game string) string {
	if game == "hearts" {
		return "Hearts"
	}
	return game
}
