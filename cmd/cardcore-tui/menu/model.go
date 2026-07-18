package menu

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Menu item indices. These are cursor positions in the vertical list and must
// remain in the order the items are rendered.
const (
	itemGame         = iota // 0: display-only
	itemServer              // 1: display-only
	itemAIDifficulty        // 2: cycles on Enter
	itemObserver            // 3: cycles on Enter
	itemTheme               // 4: cycles on Enter
	itemStart               // 5: action — starts the game
	itemCount               // 6: total number of items
)

// menuModel is the Bubble Tea model for the menu wizard. It holds the
// mutable selection state (cursor, AI difficulty index, observer flag, theme
// index) and the resolved result or error after the program quits.
type menuModel struct {
	// config is the initial configuration; Game is display-only.
	config Config
	// theme is the color palette used by the render method.
	theme Theme
	// cursor is the index of the currently highlighted menu item.
	cursor int
	// aiDiffIdx is the index into aiDifficulties for the current selection.
	aiDiffIdx int
	// observer is true when observer mode is selected.
	observer bool
	// themeIdx is the index into themeOptions for the current selection.
	themeIdx int
	// editingServer is true when the Server value is being edited inline.
	editingServer bool
	// serverBuffer holds the in-progress Server value during inline editing.
	serverBuffer string
	// result holds the resolved Config when the user starts the game, or nil
	// if the menu was cancelled or has not yet completed.
	result *Config
	// err holds ErrCancelled when the user presses Esc, or nil otherwise.
	err error
}

// Init is the first function called by the Bubble Tea framework. The menu
// has no initial I/O to perform, so it returns nil.
func (m *menuModel) Init() tea.Cmd {
	return nil
}

// Update handles incoming messages. Key presses are delegated to
// handleKeyPress; all other messages are ignored.
func (m *menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleKeyPress(key)
	}
	return m, nil
}

// View renders the current menu state as a terminal screen. The rendering
// logic lives in the pure render method; View wraps it in a tea.View. The
// view's background color is set to the theme background so the entire menu
// area fills with the correct palette.
func (m *menuModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.BackgroundColor = m.theme.Background
	return v
}

// handleKeyPress processes Up/Down navigation, Enter to cycle or start, and
// Esc to cancel. When editing the Server value, printable keys update the
// buffer, Backspace removes the last character, Enter confirms, and Esc
// cancels the edit.
func (m *menuModel) handleKeyPress(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.editingServer {
		return m.handleEditKey(key)
	}

	switch key.Code {
	case tea.KeyUp:
		m.moveCursor(-1)
		return m, nil
	case tea.KeyDown:
		m.moveCursor(1)
		return m, nil
	case tea.KeyEnter:
		return m.handleEnter()
	case tea.KeyEscape:
		m.err = ErrCancelled
		return m, tea.Quit
	default:
		return m, nil
	}
}

// handleEnter processes an Enter key press on the current item. Cycling items
// advance their index; Server enters inline edit mode; Start Game resolves the
// config and quits.
func (m *menuModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case itemServer:
		m.editingServer = true
		m.serverBuffer = m.config.Server
		return m, nil
	case itemAIDifficulty:
		m.aiDiffIdx = (m.aiDiffIdx + 1) % len(aiDifficulties)
		return m, nil
	case itemObserver:
		m.observer = !m.observer
		return m, nil
	case itemTheme:
		m.themeIdx = (m.themeIdx + 1) % len(themeOptions)
		m.theme = themeAtIndex(m.themeIdx)
		return m, nil
	case itemStart:
		cfg := m.resolve()
		m.result = &cfg
		return m, tea.Quit
	default:
		return m, nil
	}
}

// handleEditKey processes key presses while editing the Server value.
// Enter confirms the edit, Esc cancels it, Backspace removes the last rune,
// and printable characters append to the buffer.
func (m *menuModel) handleEditKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEnter:
		m.config.Server = m.serverBuffer
		m.editingServer = false
		m.serverBuffer = ""
		return m, nil
	case tea.KeyEscape:
		m.editingServer = false
		m.serverBuffer = ""
		return m, nil
	case tea.KeyBackspace:
		runes := []rune(m.serverBuffer)
		if len(runes) > 0 {
			m.serverBuffer = string(runes[:len(runes)-1])
		}
		return m, nil
	default:
		if key.Text != "" {
			m.serverBuffer += key.Text
		}
		return m, nil
	}
}

// moveCursor moves the cursor by delta, clamping to the valid item range.
func (m *menuModel) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= itemCount {
		m.cursor = itemCount - 1
	}
}

// render produces the menu view as a styled string. The title and Start Game
// action use the accent color; item values use the dimmed color. The cursor
// is a ">" prefix on the highlighted line. Output is kept within 80 columns.
func (m *menuModel) render() string {
	baseStyle := lipgloss.NewStyle().Background(m.theme.Background)

	titleStyle := baseStyle.Bold(true).Foreground(m.theme.Accent)
	labelStyle := baseStyle.Foreground(m.theme.Text)
	valueStyle := baseStyle.Foreground(m.theme.Text)
	cursorStyle := baseStyle.Foreground(m.theme.Accent).Bold(true)
	startStyle := baseStyle.Bold(true).Foreground(m.theme.Accent)

	var b strings.Builder
	b.WriteString(titleStyle.Render("Cardcore"))
	b.WriteString("\n\n")

	for i := range itemCount {
		if i == itemStart {
			b.WriteString("\n")
		}

		cursor := " "
		if m.cursor == i {
			cursor = cursorStyle.Render(">")
		}

		label := m.itemLabel(i)

		if i == itemStart {
			b.WriteString(cursor)
			b.WriteString(" ")
			b.WriteString(startStyle.Render(label))
		} else {
			value := m.itemValue(i)
			b.WriteString(cursor)
			b.WriteString(" ")
			b.WriteString(labelStyle.Render(fmt.Sprintf("%-15s", label)))
			b.WriteString(" ")
			b.WriteString(valueStyle.Render(value))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// itemLabel returns the display label for the item at the given index.
func (m *menuModel) itemLabel(idx int) string {
	switch idx {
	case itemGame:
		return "Game"
	case itemServer:
		return "Server"
	case itemAIDifficulty:
		return "AI Difficulty"
	case itemObserver:
		return "Observer"
	case itemTheme:
		return "Theme"
	case itemStart:
		return "Start Game"
	default:
		return ""
	}
}

// itemValue returns the current value string for the item at the given index.
// Display-only items (Game, Server) return the config value; cycling items
// return their current selection; Start Game returns an empty string.
func (m *menuModel) itemValue(idx int) string {
	switch idx {
	case itemGame:
		return gameDisplayName(m.config.Game)
	case itemServer:
		if m.editingServer {
			return m.serverBuffer + "▋"
		}
		return m.config.Server
	case itemAIDifficulty:
		d := aiDifficulties[m.aiDiffIdx]
		return fmt.Sprintf("%s (%s)", d.label, d.aiType)
	case itemObserver:
		if m.observer {
			return "Yes"
		}
		return "No"
	case itemTheme:
		return themeOptions[m.themeIdx]
	default:
		return ""
	}
}

// resolve builds a Config from the current menu state. Game, Server, and
// Debug are carried over from the initial config; AIType, Observer, and Theme
// are updated from the menu selections.
func (m *menuModel) resolve() Config {
	cfg := m.config
	cfg.AIType = aiDifficulties[m.aiDiffIdx].aiType
	cfg.Observer = m.observer
	cfg.Theme = themeOptions[m.themeIdx]
	return cfg
}
