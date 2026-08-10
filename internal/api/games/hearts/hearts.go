package heartsapi

// Card represents a playing card in the wire format.
type Card struct {
	// Rank is the rank name: one of "two", "three", "four", "five", "six",
	// "seven", "eight", "nine", "ten", "jack", "queen", "king", or "ace".
	Rank string `json:"rank"`
	// Suit is the suit name: one of "clubs", "diamonds", "hearts", or
	// "spades".
	Suit string `json:"suit"`
}

// TrickEntry represents one card played in a trick, in play order.
type TrickEntry struct {
	// Seat is the index of the seat that played this card.
	Seat int `json:"seat"`
	// Card is the card that was played.
	Card Card `json:"card"`
}

// PlayerSnapshot is the game state snapshot sent to a seated player.
type PlayerSnapshot struct {
	// Type is the message type identifier, always "snapshot".
	Type string `json:"type"`
	// Seq is the monotonic snapshot counter; it increments on every state
	// change.
	Seq int `json:"seq"`
	// Phase is the current game phase: one of "deal", "passing",
	// "playing", "trick_complete", "round_complete", "game_over", or
	// "paused".
	Phase string `json:"phase"`
	// RoundNumber is the current round, 1-indexed.
	RoundNumber int `json:"round_number"`
	// TrickNumber is the current trick within the round, 1-indexed; it is
	// only meaningful during the "playing" and "trick_complete" phases.
	TrickNumber int `json:"trick_number"`
	// PassDirection indicates which direction cards are passed this round:
	// "left", "right", "across", or "none".
	PassDirection string `json:"pass_direction"`
	// Turn is the seat index of the player who must act next.
	Turn int `json:"turn"`
	// TrickWinner is the seat index of the winner of the completed trick;
	// it is only meaningful during the "trick_complete" phase and is -1 in
	// other phases.
	TrickWinner int `json:"trick_winner"`
	// HeartsBroken indicates whether hearts have been played to any trick
	// this round.
	HeartsBroken bool `json:"hearts_broken"`
	// Hand is the receiving player's current hand, sorted.
	Hand []Card `json:"hand"`
	// HandCounts holds the number of cards in each seat's hand, indexed by
	// seat.
	HandCounts []int `json:"hand_counts"`
	// Trick holds the cards played to the current trick so far, in play
	// order.
	Trick []TrickEntry `json:"trick"`
	// Scores holds the cumulative score per seat across all completed
	// rounds.
	Scores []int `json:"scores"`
	// RoundPoints holds the penalty points accumulated this round per
	// seat; it resets to zero at the start of each round. During the
	// round_complete phase it instead carries the score delta applied for
	// the round, which differs from the raw penalty points on a successful
	// moon shot (0 for the shooter, 26 for each other seat).
	RoundPoints []int `json:"round_points"`
	// LegalActions lists the cards the player may legally play or pass.
	// During the passing phase it holds the player's full hand; during the
	// playing phase it is empty when it is not the player's turn. A paused
	// snapshot keeps the legal actions of the underlying phase.
	LegalActions []Card `json:"legal_actions"`
	// TurnDeadlineMS is the server-side deadline for the current human
	// turn as Unix milliseconds since the epoch, or 0 when no deadline is
	// active.
	TurnDeadlineMS int64 `json:"turn_deadline_ms"`
	// Paused indicates whether the game is currently paused.
	Paused bool `json:"paused"`
}

// ObserverSnapshot is the game state snapshot sent to an observer connection.
type ObserverSnapshot struct {
	// Type is the message type identifier, always "snapshot".
	Type string `json:"type"`
	// Seq is the monotonic snapshot counter; it increments on every state
	// change.
	Seq int `json:"seq"`
	// Phase is the current game phase: one of "deal", "passing",
	// "playing", "trick_complete", "round_complete", "game_over", or
	// "paused".
	Phase string `json:"phase"`
	// RoundNumber is the current round, 1-indexed.
	RoundNumber int `json:"round_number"`
	// TrickNumber is the current trick within the round, 1-indexed; it is
	// only meaningful during the "playing" and "trick_complete" phases.
	TrickNumber int `json:"trick_number"`
	// PassDirection indicates which direction cards are passed this round:
	// "left", "right", "across", or "none".
	PassDirection string `json:"pass_direction"`
	// Turn is the seat index of the player who must act next.
	Turn int `json:"turn"`
	// TrickWinner is the seat index of the winner of the completed trick;
	// it is only meaningful during the "trick_complete" phase and is -1 in
	// other phases.
	TrickWinner int `json:"trick_winner"`
	// HeartsBroken indicates whether hearts have been played to any trick
	// this round.
	HeartsBroken bool `json:"hearts_broken"`
	// Hands holds every seat's hand, indexed by seat; all cards are
	// visible.
	Hands [][]Card `json:"hands"`
	// HandCounts holds the number of cards in each seat's hand, indexed by
	// seat.
	HandCounts []int `json:"hand_counts"`
	// Trick holds the cards played to the current trick so far, in play
	// order.
	Trick []TrickEntry `json:"trick"`
	// TrickHistory holds the completed tricks this round; each trick is a
	// list of trick entries in play order.
	TrickHistory [][]TrickEntry `json:"trick_history"`
	// Scores holds the cumulative score per seat across all completed
	// rounds.
	Scores []int `json:"scores"`
	// RoundPoints holds the penalty points accumulated this round per
	// seat; it resets to zero at the start of each round. During the
	// round_complete phase it instead carries the score delta applied for
	// the round, which differs from the raw penalty points on a successful
	// moon shot (0 for the shooter, 26 for each other seat).
	RoundPoints []int `json:"round_points"`
	// LegalActions lists the cards the seat indicated by Turn may legally
	// play or pass.
	LegalActions []Card `json:"legal_actions"`
	// TurnDeadlineMS is the server-side deadline for the current human
	// turn as Unix milliseconds since the epoch, or 0 when no deadline is
	// active.
	TurnDeadlineMS int64 `json:"turn_deadline_ms"`
	// Paused indicates whether the game is currently paused.
	Paused bool `json:"paused"`
}

// PlayCardPayload is the payload for a play_card inbound message.
type PlayCardPayload struct {
	// Card is the card to play.
	Card Card `json:"card"`
}

// PassCardsPayload is the payload for a pass_cards inbound message.
type PassCardsPayload struct {
	// Cards lists exactly 3 cards to pass during the passing phase.
	Cards []Card `json:"cards"`
}
