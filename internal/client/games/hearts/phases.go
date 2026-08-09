package heartsclient

// Phase constants for the Hearts game.
const (
	// PhaseDeal indicates that cards have been dealt; a brief pause before
	// passing or playing begins.
	PhaseDeal = "deal"
	// PhasePassing indicates that players are selecting cards to pass.
	PhasePassing = "passing"
	// PhasePlaying indicates that trick-taking is in progress.
	PhasePlaying = "playing"
	// PhaseTrickComplete indicates that a trick has been won; a
	// server-synthesized pause displays the completed trick.
	PhaseTrickComplete = "trick_complete"
	// PhaseRoundComplete indicates that a round has ended and scores have been
	// updated.
	PhaseRoundComplete = "round_complete"
	// PhaseGameOver indicates that the game has ended.
	PhaseGameOver = "game_over"
	// PhasePaused indicates the Hearts game is paused.
	PhasePaused = "paused"
)
