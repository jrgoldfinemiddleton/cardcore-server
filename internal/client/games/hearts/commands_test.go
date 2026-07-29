package heartsclient

import (
	"encoding/json"
	"testing"
)

// TestNewPlayCardMessage verifies that NewPlayCardMessage builds a play_card
// command envelope with the serialized card payload.
func TestNewPlayCardMessage(t *testing.T) {
	card := Card{Rank: "queen", Suit: "spades"}
	cmd, err := NewPlayCardMessage("play-1", 7, card)
	if err != nil {
		t.Fatalf("NewPlayCardMessage: %v", err)
	}
	if cmd.Type != "play_card" {
		t.Errorf("Type = %q, want play_card", cmd.Type)
	}
	if cmd.ActionID != "play-1" {
		t.Errorf("ActionID = %q, want play-1", cmd.ActionID)
	}
	if cmd.Seq != 7 {
		t.Errorf("Seq = %d, want 7", cmd.Seq)
	}

	var payload PlayCardPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Card != card {
		t.Errorf("Card = %+v, want %+v", payload.Card, card)
	}
}

// TestNewPassCardsMessage verifies that NewPassCardsMessage builds a pass_cards
// command envelope with the serialized cards payload.
func TestNewPassCardsMessage(t *testing.T) {
	cards := []Card{
		{Rank: "two", Suit: "clubs"},
		{Rank: "queen", Suit: "spades"},
		{Rank: "ace", Suit: "hearts"},
	}
	cmd, err := NewPassCardsMessage("pass-1", 3, cards)
	if err != nil {
		t.Fatalf("NewPassCardsMessage: %v", err)
	}
	if cmd.Type != "pass_cards" {
		t.Errorf("Type = %q, want pass_cards", cmd.Type)
	}
	if cmd.ActionID != "pass-1" {
		t.Errorf("ActionID = %q, want pass-1", cmd.ActionID)
	}
	if cmd.Seq != 3 {
		t.Errorf("Seq = %d, want 3", cmd.Seq)
	}

	var payload PassCardsPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload.Cards) != len(cards) {
		t.Fatalf("Cards length = %d, want %d", len(payload.Cards), len(cards))
	}
	for i, want := range cards {
		if payload.Cards[i] != want {
			t.Errorf("Card[%d] = %+v, want %+v", i, payload.Cards[i], want)
		}
	}
}
