package heartsapi

import (
	"fmt"

	"github.com/jrgoldfinemiddleton/cardcore"
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"
)

// rankWireNames maps engine rank values (by index) to their wire-format strings.
var rankWireNames = [cardcore.NumRanks]string{
	"two", "three", "four", "five", "six", "seven",
	"eight", "nine", "ten", "jack", "queen", "king", "ace",
}

// suitWireNames maps engine suit values (by index) to their wire-format strings.
var suitWireNames = [cardcore.NumSuits]string{
	"clubs", "diamonds", "hearts", "spades",
}

// wireRankValues maps wire-format rank strings back to engine rank values.
var wireRankValues = map[string]cardcore.Rank{
	"two":   cardcore.Two,
	"three": cardcore.Three,
	"four":  cardcore.Four,
	"five":  cardcore.Five,
	"six":   cardcore.Six,
	"seven": cardcore.Seven,
	"eight": cardcore.Eight,
	"nine":  cardcore.Nine,
	"ten":   cardcore.Ten,
	"jack":  cardcore.Jack,
	"queen": cardcore.Queen,
	"king":  cardcore.King,
	"ace":   cardcore.Ace,
}

// wireSuitValues maps wire-format suit strings back to engine suit values.
var wireSuitValues = map[string]cardcore.Suit{
	"clubs":    cardcore.Clubs,
	"diamonds": cardcore.Diamonds,
	"hearts":   cardcore.Hearts,
	"spades":   cardcore.Spades,
}

// phaseWireNames maps engine phase values (by index) to their wire-format strings.
var phaseWireNames = [5]string{
	"deal", "passing", "playing", "round_complete", "game_over",
}

// passDirWireNames maps engine pass-direction values (by index) to their wire-format strings.
var passDirWireNames = [4]string{
	"left", "right", "across", "none",
}

// RankToWire converts an engine rank to its wire-format string representation.
func RankToWire(r cardcore.Rank) string {
	if int(r) < len(rankWireNames) {
		return rankWireNames[r]
	}
	return ""
}

// SuitToWire converts an engine suit to its wire-format string representation.
func SuitToWire(s cardcore.Suit) string {
	if int(s) < len(suitWireNames) {
		return suitWireNames[s]
	}
	return ""
}

// RankFromWire converts a wire-format rank string to an engine rank value.
func RankFromWire(s string) (cardcore.Rank, error) {
	r, ok := wireRankValues[s]
	if !ok {
		return 0, fmt.Errorf("unknown rank: %q", s)
	}
	return r, nil
}

// SuitFromWire converts a wire-format suit string to an engine suit value.
func SuitFromWire(s string) (cardcore.Suit, error) {
	su, ok := wireSuitValues[s]
	if !ok {
		return 0, fmt.Errorf("unknown suit: %q", s)
	}
	return su, nil
}

// CardFromEngine converts an engine card to its wire representation.
func CardFromEngine(c cardcore.Card) Card {
	return Card{
		Rank: RankToWire(c.Rank),
		Suit: SuitToWire(c.Suit),
	}
}

// CardToEngine converts a wire card to an engine card.
func CardToEngine(c Card) (cardcore.Card, error) {
	r, err := RankFromWire(c.Rank)
	if err != nil {
		return cardcore.Card{}, err
	}
	s, err := SuitFromWire(c.Suit)
	if err != nil {
		return cardcore.Card{}, err
	}
	return cardcore.Card{Rank: r, Suit: s}, nil
}

// PhaseToWire converts an engine Hearts phase to its wire-format string.
func PhaseToWire(p hearts.Phase) string {
	if int(p) < len(phaseWireNames) {
		return phaseWireNames[p]
	}
	return ""
}

// PassDirToWire converts an engine Hearts pass direction to its wire-format string.
func PassDirToWire(d hearts.PassDirection) string {
	if int(d) < len(passDirWireNames) {
		return passDirWireNames[d]
	}
	return ""
}
