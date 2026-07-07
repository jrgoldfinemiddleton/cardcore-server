package heartsclient

// Phase constants for the Hearts game.
const (
	PhaseDeal          = "deal"
	PhasePassing       = "passing"
	PhasePlaying       = "playing"
	PhaseTrickComplete = "trick_complete"
	PhaseRoundComplete = "round_complete"
	PhaseGameOver      = "game_over"
)

// Adapter provides game-specific logic for the Hearts card game. A TUI or
// CLI frontend uses an Adapter to interpret snapshots and construct commands.
type Adapter struct{}

// NewAdapter returns a Hearts game adapter.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// IsActingPhase returns true for phases where the player may send commands.
func (a *Adapter) IsActingPhase(phase string) bool {
	return phase == PhasePassing || phase == PhasePlaying
}

// IsTerminalPhase returns true when the game has ended.
func (a *Adapter) IsTerminalPhase(phase string) bool {
	return phase == PhaseGameOver
}

// PassDirections returns all possible pass directions in Hearts.
func (a *Adapter) PassDirections() []string {
	return []string{"left", "right", "across", "none"}
}

// NumCardsToPass returns the number of cards a player must pass.
func (a *Adapter) NumCardsToPass() int {
	return 3
}

// NumCardsToPlay returns the number of cards a player must play per trick.
func (a *Adapter) NumCardsToPlay() int {
	return 1
}

// IsPassingPhase returns true if the given phase is the passing phase.
func (a *Adapter) IsPassingPhase(phase string) bool {
	return phase == PhasePassing
}

// IsPlayingPhase returns true if the given phase is the playing phase.
func (a *Adapter) IsPlayingPhase(phase string) bool {
	return phase == PhasePlaying
}
