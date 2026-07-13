package heartsclient

// Card represents a playing card in the wire format.
type Card struct {
	// Rank is the card rank name (e.g., "ace", "king", "queen").
	Rank string `json:"rank"`
	// Suit is the card suit name (e.g., "spades", "hearts", "diamonds", "clubs").
	Suit string `json:"suit"`
}

// TrickEntry represents one card played in a trick, in play order.
type TrickEntry struct {
	// Seat is the index of the player who played this card.
	Seat int `json:"seat"`
	// Card is the card that was played.
	Card Card `json:"card"`
}

// PlayerSnapshot is the game state snapshot sent to a seated player.
type PlayerSnapshot struct {
	// Type is always "snapshot".
	Type string `json:"type"`
	// Seq is the monotonic state change counter.
	Seq int `json:"seq"`
	// Phase is the current game phase (e.g., "passing", "playing").
	Phase string `json:"phase"`
	// RoundNumber is the current round (1-indexed).
	RoundNumber int `json:"round_number"`
	// TrickNumber is the current trick within the round (1-indexed).
	TrickNumber int `json:"trick_number"`
	// PassDirection indicates which direction cards are passed this round.
	PassDirection string `json:"pass_direction"`
	// Turn is the seat index of the player who must act next.
	Turn int `json:"turn"`
	// TrickWinner is the seat index of the winner of the completed trick.
	// Only meaningful during the trick_complete phase; -1 in other phases.
	TrickWinner int `json:"trick_winner"`
	// HeartsBroken is true when hearts have been played this round.
	HeartsBroken bool `json:"hearts_broken"`
	// Hand is the receiving player's current hand, sorted.
	Hand []Card `json:"hand"`
	// HandCounts is the number of cards in each seat's hand, indexed by seat.
	HandCounts []int `json:"hand_counts"`
	// Trick is the cards played to the current trick so far, in play order.
	Trick []TrickEntry `json:"trick"`
	// Scores is the cumulative scores per seat across all completed rounds.
	Scores []int `json:"scores"`
	// RoundPoints is the penalty points accumulated this round per seat.
	RoundPoints []int `json:"round_points"`
	// LegalActions is the cards the player may legally play or pass. Empty when
	// it is not the player's turn.
	LegalActions []Card `json:"legal_actions"`
	// TurnDeadlineMS is the server-side deadline for the current human turn as
	// Unix milliseconds. It is zero when no deadline is active.
	TurnDeadlineMS int64 `json:"turn_deadline_ms"`
}

// ObserverSnapshot is the game state snapshot sent to an observer connection.
type ObserverSnapshot struct {
	// Type is always "snapshot".
	Type string `json:"type"`
	// Seq is the monotonic state change counter.
	Seq int `json:"seq"`
	// Phase is the current game phase (e.g., "passing", "playing").
	Phase string `json:"phase"`
	// RoundNumber is the current round (1-indexed).
	RoundNumber int `json:"round_number"`
	// TrickNumber is the current trick within the round (1-indexed).
	TrickNumber int `json:"trick_number"`
	// PassDirection indicates which direction cards are passed this round.
	PassDirection string `json:"pass_direction"`
	// Turn is the seat index of the player who must act next.
	Turn int `json:"turn"`
	// TrickWinner is the seat index of the winner of the completed trick.
	// Only meaningful during the trick_complete phase; -1 in other phases.
	TrickWinner int `json:"trick_winner"`
	// HeartsBroken is true when hearts have been played this round.
	HeartsBroken bool `json:"hearts_broken"`
	// Hands is all four seats' hands, indexed by seat. All cards are visible.
	Hands [][]Card `json:"hands"`
	// HandCounts is the number of cards in each seat's hand, indexed by seat.
	HandCounts []int `json:"hand_counts"`
	// Trick is the cards played to the current trick so far, in play order.
	Trick []TrickEntry `json:"trick"`
	// TrickHistory is the completed tricks this round. Each trick is an array
	// of trick entries in play order.
	TrickHistory [][]TrickEntry `json:"trick_history"`
	// Scores is the cumulative scores per seat across all completed rounds.
	Scores []int `json:"scores"`
	// RoundPoints is the penalty points accumulated this round per seat.
	RoundPoints []int `json:"round_points"`
	// LegalActions shows legal actions for the seat indicated by Turn.
	LegalActions []Card `json:"legal_actions"`
	// TurnDeadlineMS is the server-side deadline for the current human turn as
	// Unix milliseconds. It is zero when no deadline is active.
	TurnDeadlineMS int64 `json:"turn_deadline_ms"`
}

// PlayCardPayload is the payload for a play_card inbound message.
type PlayCardPayload struct {
	// Card is the card to play.
	Card Card `json:"card"`
}

// PassCardsPayload is the payload for a pass_cards inbound message.
type PassCardsPayload struct {
	// Cards is the exactly 3 cards to pass.
	Cards []Card `json:"cards"`
}
