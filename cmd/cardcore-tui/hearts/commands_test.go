package heartstui

import (
	"encoding/json"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// TestBuildPassCommandError verifies that BuildPassCommand returns an error
// for 2 cards and for 4 cards.
func TestBuildPassCommandError(t *testing.T) {
	twoCards := []heartsclient.Card{
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}
	fourCards := []heartsclient.Card{
		{Rank: "jack", Suit: "clubs"},
		{Rank: "queen", Suit: "diamonds"},
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}

	_, err := BuildPassCommand("tui-0-1", twoCards)
	if err == nil {
		t.Errorf("BuildPassCommand with 2 cards: got nil error, want error")
	}

	_, err = BuildPassCommand("tui-0-1", fourCards)
	if err == nil {
		t.Errorf("BuildPassCommand with 4 cards: got nil error, want error")
	}
}

// TestBuildPassCommandSuccess verifies that BuildPassCommand succeeds for
// exactly 3 cards and produces a command with Type=="pass_cards" and a payload
// that unmarshals to 3 cards.
func TestBuildPassCommandSuccess(t *testing.T) {
	cards := []heartsclient.Card{
		{Rank: "queen", Suit: "diamonds"},
		{Rank: "king", Suit: "hearts"},
		{Rank: "ace", Suit: "spades"},
	}

	cmd, err := BuildPassCommand("tui-0-1", cards)
	if err != nil {
		t.Fatalf("BuildPassCommand: got error %v, want nil", err)
	}

	if cmd.Type != "pass_cards" {
		t.Errorf("cmd.Type = %q, want %q", cmd.Type, "pass_cards")
	}

	var payload heartsclient.PassCardsPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload.Cards) != 3 {
		t.Errorf("len(payload.Cards) = %d, want 3", len(payload.Cards))
	}
}

// TestBuildPlayCommand verifies that BuildPlayCommand returns a command with
// Type=="play_card" and a payload whose card matches the input.
func TestBuildPlayCommand(t *testing.T) {
	card := heartsclient.Card{Rank: "ace", Suit: "spades"}

	cmd, err := BuildPlayCommand("tui-0-2", card)
	if err != nil {
		t.Fatalf("BuildPlayCommand: got error %v, want nil", err)
	}

	if cmd.Type != "play_card" {
		t.Errorf("cmd.Type = %q, want %q", cmd.Type, "play_card")
	}

	var payload heartsclient.PlayCardPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Card != card {
		t.Errorf("payload.Card = %+v, want %+v", payload.Card, card)
	}
}
