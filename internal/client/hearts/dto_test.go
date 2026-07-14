package heartsclient

import (
	"encoding/json"
	"slices"
	"testing"
)

// playerSnapshotJSON is the example snapshot from doc/games/hearts/protocol.md.
const playerSnapshotJSON = `{
  "type": "snapshot",
  "seq": 12,
  "phase": "playing",
  "round_number": 1,
  "trick_number": 3,
  "pass_direction": "left",
  "turn": 0,
  "trick_winner": -1,
  "hearts_broken": false,
  "hand": [
    {"rank": "four", "suit": "clubs"},
    {"rank": "jack", "suit": "clubs"},
    {"rank": "seven", "suit": "diamonds"},
    {"rank": "queen", "suit": "diamonds"},
    {"rank": "two", "suit": "hearts"},
    {"rank": "nine", "suit": "hearts"},
    {"rank": "five", "suit": "spades"},
    {"rank": "ten", "suit": "spades"},
    {"rank": "king", "suit": "spades"},
    {"rank": "ace", "suit": "spades"},
    {"rank": "three", "suit": "spades"}
  ],
  "hand_counts": [11, 11, 10, 10],
  "trick": [
    {"seat": 2, "card": {"rank": "king", "suit": "diamonds"}},
    {"seat": 3, "card": {"rank": "five", "suit": "diamonds"}}
  ],
  "scores": [0, 0, 0, 0],
  "round_points": [0, 1, 0, 3],
  "legal_actions": [
    {"rank": "seven", "suit": "diamonds"},
    {"rank": "queen", "suit": "diamonds"}
  ]
}`

// realisticTrickHistory is tricks 1–11 from the cardcore shoot-the-moon
// integration test (games/hearts/hearts_test.go). South (seat 0) wins every
// trick from trick 3 onward and leads the next. Hearts break in trick 5
// when North (seat 2) throws off 2♥.
var realisticTrickHistory = [][]TrickEntry{
	// Trick 1: South leads 2♣. West wins with A♣.
	{
		{Seat: 0, Card: Card{Rank: "two", Suit: "clubs"}},
		{Seat: 1, Card: Card{Rank: "ace", Suit: "clubs"}},
		{Seat: 2, Card: Card{Rank: "three", Suit: "clubs"}},
		{Seat: 3, Card: Card{Rank: "four", Suit: "clubs"}},
	},
	// Trick 2: West leads 2♦. South wins with A♦.
	{
		{Seat: 1, Card: Card{Rank: "two", Suit: "diamonds"}},
		{Seat: 2, Card: Card{Rank: "four", Suit: "diamonds"}},
		{Seat: 3, Card: Card{Rank: "three", Suit: "diamonds"}},
		{Seat: 0, Card: Card{Rank: "ace", Suit: "diamonds"}},
	},
	// Trick 3: South leads K♣. South wins.
	{
		{Seat: 0, Card: Card{Rank: "king", Suit: "clubs"}},
		{Seat: 1, Card: Card{Rank: "ten", Suit: "clubs"}},
		{Seat: 2, Card: Card{Rank: "five", Suit: "clubs"}},
		{Seat: 3, Card: Card{Rank: "six", Suit: "clubs"}},
	},
	// Trick 4: South leads Q♣. South wins.
	{
		{Seat: 0, Card: Card{Rank: "queen", Suit: "clubs"}},
		{Seat: 1, Card: Card{Rank: "nine", Suit: "clubs"}},
		{Seat: 2, Card: Card{Rank: "eight", Suit: "clubs"}},
		{Seat: 3, Card: Card{Rank: "seven", Suit: "clubs"}},
	},
	// Trick 5: South leads J♣. South wins (North sloughs 2♥, breaking hearts).
	{
		{Seat: 0, Card: Card{Rank: "jack", Suit: "clubs"}},
		{Seat: 1, Card: Card{Rank: "jack", Suit: "diamonds"}},
		{Seat: 2, Card: Card{Rank: "two", Suit: "hearts"}},
		{Seat: 3, Card: Card{Rank: "queen", Suit: "spades"}},
	},
	// Trick 6: South leads K♦. South wins.
	{
		{Seat: 0, Card: Card{Rank: "king", Suit: "diamonds"}},
		{Seat: 1, Card: Card{Rank: "five", Suit: "diamonds"}},
		{Seat: 2, Card: Card{Rank: "seven", Suit: "diamonds"}},
		{Seat: 3, Card: Card{Rank: "six", Suit: "diamonds"}},
	},
	// Trick 7: South leads Q♦. South wins.
	{
		{Seat: 0, Card: Card{Rank: "queen", Suit: "diamonds"}},
		{Seat: 1, Card: Card{Rank: "eight", Suit: "diamonds"}},
		{Seat: 2, Card: Card{Rank: "ten", Suit: "diamonds"}},
		{Seat: 3, Card: Card{Rank: "nine", Suit: "diamonds"}},
	},
	// Trick 8: South leads A♠. South wins.
	{
		{Seat: 0, Card: Card{Rank: "ace", Suit: "spades"}},
		{Seat: 1, Card: Card{Rank: "four", Suit: "spades"}},
		{Seat: 2, Card: Card{Rank: "three", Suit: "spades"}},
		{Seat: 3, Card: Card{Rank: "two", Suit: "spades"}},
	},
	// Trick 9: South leads K♠. South wins.
	{
		{Seat: 0, Card: Card{Rank: "king", Suit: "spades"}},
		{Seat: 1, Card: Card{Rank: "seven", Suit: "spades"}},
		{Seat: 2, Card: Card{Rank: "six", Suit: "spades"}},
		{Seat: 3, Card: Card{Rank: "five", Suit: "spades"}},
	},
	// Trick 10: South leads J♠. South wins.
	{
		{Seat: 0, Card: Card{Rank: "jack", Suit: "spades"}},
		{Seat: 1, Card: Card{Rank: "ten", Suit: "spades"}},
		{Seat: 2, Card: Card{Rank: "nine", Suit: "spades"}},
		{Seat: 3, Card: Card{Rank: "eight", Suit: "spades"}},
	},
	// Trick 11: South leads A♥. South wins.
	{
		{Seat: 0, Card: Card{Rank: "ace", Suit: "hearts"}},
		{Seat: 1, Card: Card{Rank: "four", Suit: "hearts"}},
		{Seat: 2, Card: Card{Rank: "five", Suit: "hearts"}},
		{Seat: 3, Card: Card{Rank: "three", Suit: "hearts"}},
	},
}

// TestPlayerSnapshotParsesProtocolExample verifies that the example snapshot
// from the protocol documentation unmarshals correctly with all fields populated.
func TestPlayerSnapshotParsesProtocolExample(t *testing.T) {
	var got PlayerSnapshot
	if err := json.Unmarshal([]byte(playerSnapshotJSON), &got); err != nil {
		t.Fatalf("unmarshal player snapshot: %v", err)
	}

	if got.Type != "snapshot" {
		t.Errorf("got Type %q, want %q", got.Type, "snapshot")
	}
	if got.Seq != 12 {
		t.Errorf("got Seq %d, want %d", got.Seq, 12)
	}
	if got.Phase != "playing" {
		t.Errorf("got Phase %q, want %q", got.Phase, "playing")
	}
	if got.RoundNumber != 1 {
		t.Errorf("got RoundNumber %d, want %d", got.RoundNumber, 1)
	}
	if got.TrickNumber != 3 {
		t.Errorf("got TrickNumber %d, want %d", got.TrickNumber, 3)
	}
	if got.PassDirection != "left" {
		t.Errorf("got PassDirection %q, want %q", got.PassDirection, "left")
	}
	if got.Turn != 0 {
		t.Errorf("got Turn %d, want %d", got.Turn, 0)
	}
	if got.TrickWinner != -1 {
		t.Errorf("got TrickWinner %d, want %d", got.TrickWinner, -1)
	}
	if got.HeartsBroken != false {
		t.Errorf("got HeartsBroken %v, want %v", got.HeartsBroken, false)
	}
	if wantHand := 11; len(got.Hand) != wantHand {
		t.Errorf("got Hand length %d, want %d", len(got.Hand), wantHand)
	}
	if wantCounts := []int{11, 11, 10, 10}; !slices.Equal(got.HandCounts, wantCounts) {
		t.Errorf("got HandCounts %v, want %v", got.HandCounts, wantCounts)
	}
	if wantTrick := 2; len(got.Trick) != wantTrick {
		t.Errorf("got Trick length %d, want %d", len(got.Trick), wantTrick)
	}
	if wantScores := []int{0, 0, 0, 0}; !slices.Equal(got.Scores, wantScores) {
		t.Errorf("got Scores %v, want %v", got.Scores, wantScores)
	}
	if wantRoundPoints := []int{0, 1, 0, 3}; !slices.Equal(got.RoundPoints, wantRoundPoints) {
		t.Errorf("got RoundPoints %v, want %v", got.RoundPoints, wantRoundPoints)
	}
	if wantLegal := 2; len(got.LegalActions) != wantLegal {
		t.Errorf("got LegalActions length %d, want %d", len(got.LegalActions), wantLegal)
	}
}

// TestObserverSnapshotHasObserverFields verifies that ObserverSnapshot has
// the game-specific observer fields: Hands (replacing Hand) and TrickHistory.
// The snapshot represents a realistic game state at trick 12, round 1, with
// seat 0 having led and seat 1 next to play.
func TestObserverSnapshotHasObserverFields(t *testing.T) {
	original := ObserverSnapshot{
		Type:          "snapshot",
		Seq:           48,
		Phase:         "playing",
		RoundNumber:   1,
		TrickNumber:   12,
		PassDirection: "left",
		Turn:          1,
		TrickWinner:   -1,
		HeartsBroken:  true,
		Hands: [][]Card{
			{{Rank: "queen", Suit: "hearts"}},
			{{Rank: "seven", Suit: "hearts"}, {Rank: "ten", Suit: "hearts"}},
			{{Rank: "eight", Suit: "hearts"}, {Rank: "jack", Suit: "hearts"}},
			{{Rank: "six", Suit: "hearts"}, {Rank: "nine", Suit: "hearts"}},
		},
		HandCounts:   []int{1, 2, 2, 2},
		Trick:        []TrickEntry{{Seat: 0, Card: Card{Rank: "king", Suit: "hearts"}}},
		TrickHistory: realisticTrickHistory,
		Scores:       []int{0, 0, 0, 0},
		RoundPoints:  []int{18, 0, 0, 0},
		LegalActions: []Card{{Rank: "seven", Suit: "hearts"}, {Rank: "ten", Suit: "hearts"}},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to raw: %v", err)
	}

	if _, ok := raw["hands"]; !ok {
		t.Error("missing hands field in observer snapshot")
	}
	if _, ok := raw["hand"]; ok {
		t.Error("got hand field in observer snapshot, want hands")
	}
	if _, ok := raw["trick_history"]; !ok {
		t.Error("missing trick_history field in observer snapshot")
	}

	var got ObserverSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Hands) != 4 {
		t.Errorf("got Hands length %d, want %d", len(got.Hands), 4)
	}
	if len(got.Trick) != 1 {
		t.Errorf("got Trick length %d, want %d", len(got.Trick), 1)
	}
	if len(got.TrickHistory) != 11 {
		t.Errorf("got TrickHistory length %d, want %d", len(got.TrickHistory), 11)
	}
	if len(got.TrickHistory[10]) != 4 {
		t.Errorf("got TrickHistory[10] length %d, want %d",
			len(got.TrickHistory[10]), 4)
	}
	if got.TrickWinner != -1 {
		t.Errorf("got TrickWinner %d, want %d", got.TrickWinner, -1)
	}
}

// TestPlayerSnapshotPausedRoundTrip verifies that a PlayerSnapshot with Paused
// set to true round-trips through JSON encoding/decoding correctly.
func TestPlayerSnapshotPausedRoundTrip(t *testing.T) {
	want := PlayerSnapshot{
		Type:           "snapshot",
		Seq:            1,
		Phase:          "playing",
		TurnDeadlineMS: 0,
		Paused:         true,
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var got PlayerSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got.Paused != true {
		t.Errorf("got Paused %v, want %v", got.Paused, true)
	}
}
