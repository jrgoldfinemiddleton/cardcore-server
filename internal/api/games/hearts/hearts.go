package heartsapi

// Card represents a playing card in the wire format.
type Card struct {
	Rank string `json:"rank"`
	Suit string `json:"suit"`
}

// TrickEntry represents one card played in a trick, in play order.
type TrickEntry struct {
	Seat int  `json:"seat"`
	Card Card `json:"card"`
}

// PlayerSnapshot is the game state snapshot sent to a seated player.
type PlayerSnapshot struct {
	Type          string       `json:"type"`
	Seq           int          `json:"seq"`
	Phase         string       `json:"phase"`
	RoundNumber   int          `json:"round_number"`
	TrickNumber   int          `json:"trick_number"`
	PassDirection string       `json:"pass_direction"`
	Turn          int          `json:"turn"`
	HeartsBroken  bool         `json:"hearts_broken"`
	Hand          []Card       `json:"hand"`
	HandCounts    []int        `json:"hand_counts"`
	Trick         []TrickEntry `json:"trick"`
	Scores        []int        `json:"scores"`
	RoundPoints   []int        `json:"round_points"`
	LegalActions  []Card       `json:"legal_actions"`
}

// ObserverSnapshot is the game state snapshot sent to an observer connection.
type ObserverSnapshot struct {
	Type          string         `json:"type"`
	Seq           int            `json:"seq"`
	Phase         string         `json:"phase"`
	RoundNumber   int            `json:"round_number"`
	TrickNumber   int            `json:"trick_number"`
	PassDirection string         `json:"pass_direction"`
	Turn          int            `json:"turn"`
	HeartsBroken  bool           `json:"hearts_broken"`
	Hands         [][]Card       `json:"hands"`
	HandCounts    []int          `json:"hand_counts"`
	Trick         []TrickEntry   `json:"trick"`
	TrickHistory  [][]TrickEntry `json:"trick_history"`
	Scores        []int          `json:"scores"`
	RoundPoints   []int          `json:"round_points"`
	LegalActions  []Card         `json:"legal_actions"`
}

// PlayCardPayload is the payload for a play_card inbound message.
type PlayCardPayload struct {
	Card Card `json:"card"`
}

// PassCardsPayload is the payload for a pass_cards inbound message.
type PassCardsPayload struct {
	Cards []Card `json:"cards"`
}
