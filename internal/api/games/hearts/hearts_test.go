package heartsapi

import (
	"encoding/json"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore"
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"
)

// TestAllCardsRoundTrip verifies every card maps to wire format and back.
func TestAllCardsRoundTrip(t *testing.T) {
	for _, suit := range cardcore.AllSuits() {
		for _, rank := range cardcore.AllRanks() {
			engine := cardcore.Card{Rank: rank, Suit: suit}
			wire := CardFromEngine(engine)
			got, err := CardToEngine(wire)
			if err != nil {
				t.Errorf("got error for %v: %v, want nil", engine, err)
				continue
			}
			if got != engine {
				t.Errorf("got %v, want %v after round-trip", got, engine)
			}
		}
	}
}

// TestRankWireMapping verifies rank values map to their wire strings.
func TestRankWireMapping(t *testing.T) {
	cases := []struct {
		rank cardcore.Rank
		want string
	}{
		{cardcore.Two, "two"},
		{cardcore.Three, "three"},
		{cardcore.Four, "four"},
		{cardcore.Five, "five"},
		{cardcore.Six, "six"},
		{cardcore.Seven, "seven"},
		{cardcore.Eight, "eight"},
		{cardcore.Nine, "nine"},
		{cardcore.Ten, "ten"},
		{cardcore.Jack, "jack"},
		{cardcore.Queen, "queen"},
		{cardcore.King, "king"},
		{cardcore.Ace, "ace"},
	}
	for _, tc := range cases {
		got := RankToWire(tc.rank)
		if got != tc.want {
			t.Errorf("got RankToWire(%v) = %q, want %q", tc.rank, got, tc.want)
		}
	}
}

// TestSuitWireMapping verifies suit values map to their wire strings.
func TestSuitWireMapping(t *testing.T) {
	cases := []struct {
		suit cardcore.Suit
		want string
	}{
		{cardcore.Clubs, "clubs"},
		{cardcore.Diamonds, "diamonds"},
		{cardcore.Hearts, "hearts"},
		{cardcore.Spades, "spades"},
	}
	for _, tc := range cases {
		got := SuitToWire(tc.suit)
		if got != tc.want {
			t.Errorf("got SuitToWire(%v) = %q, want %q", tc.suit, got, tc.want)
		}
	}
}

// TestRankFromWireError verifies invalid wire ranks return an error.
func TestRankFromWireError(t *testing.T) {
	_, err := RankFromWire("Q")
	if err == nil {
		t.Errorf("got nil error for invalid rank \"Q\", want non-nil")
	}
}

// TestSuitFromWireError verifies invalid wire suits return an error.
func TestSuitFromWireError(t *testing.T) {
	_, err := SuitFromWire("Clubs")
	if err == nil {
		t.Errorf("got nil error for invalid suit \"Clubs\", want non-nil")
	}
}

// TestPhaseToWire verifies engine phases map to wire strings.
func TestPhaseToWire(t *testing.T) {
	cases := []struct {
		phase hearts.Phase
		want  string
	}{
		{hearts.PhaseDeal, "deal"},
		{hearts.PhasePass, "passing"},
		{hearts.PhasePlay, "playing"},
		{hearts.PhaseScore, "round_complete"},
		{hearts.PhaseEnd, "game_over"},
	}
	for _, tc := range cases {
		got := PhaseToWire(tc.phase)
		if got != tc.want {
			t.Errorf("got PhaseToWire(%v) = %q, want %q", tc.phase, got, tc.want)
		}
	}
}

// TestPassDirToWire verifies pass directions map to wire strings.
func TestPassDirToWire(t *testing.T) {
	cases := []struct {
		dir  hearts.PassDirection
		want string
	}{
		{hearts.PassLeft, "left"},
		{hearts.PassRight, "right"},
		{hearts.PassAcross, "across"},
		{hearts.PassHold, "none"},
	}
	for _, tc := range cases {
		got := PassDirToWire(tc.dir)
		if got != tc.want {
			t.Errorf("got PassDirToWire(%v) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

// TestPlayerSnapshotJSON verifies player snapshots marshal and unmarshal correctly.
func TestPlayerSnapshotJSON(t *testing.T) {
	snap := PlayerSnapshot{
		Type:          "snapshot",
		Seq:           12,
		Phase:         "playing",
		RoundNumber:   1,
		TrickNumber:   3,
		PassDirection: "left",
		Turn:          0,
		TrickWinner:   -1,
		HeartsBroken:  false,
		Hand: []Card{
			{Rank: "four", Suit: "clubs"},
			{Rank: "jack", Suit: "clubs"},
			{Rank: "seven", Suit: "diamonds"},
			{Rank: "queen", Suit: "diamonds"},
			{Rank: "two", Suit: "hearts"},
			{Rank: "nine", Suit: "hearts"},
			{Rank: "five", Suit: "spades"},
			{Rank: "ten", Suit: "spades"},
			{Rank: "king", Suit: "spades"},
			{Rank: "ace", Suit: "spades"},
			{Rank: "three", Suit: "spades"},
		},
		HandCounts: []int{11, 11, 10, 10},
		Trick: []TrickEntry{
			{Seat: 2, Card: Card{Rank: "king", Suit: "diamonds"}},
			{Seat: 3, Card: Card{Rank: "five", Suit: "diamonds"}},
		},
		Scores:      []int{0, 0, 0, 0},
		RoundPoints: []int{0, 1, 0, 3},
		LegalActions: []Card{
			{Rank: "seven", Suit: "diamonds"},
			{Rank: "queen", Suit: "diamonds"},
		},
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got PlayerSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
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
	if len(got.Hand) != 11 {
		t.Errorf("got Hand length %d, want %d", len(got.Hand), 11)
	}
	if got.Hand[0] != (Card{Rank: "four", Suit: "clubs"}) {
		t.Errorf("got Hand[0] %v, want %v", got.Hand[0], Card{Rank: "four", Suit: "clubs"})
	}
	if len(got.HandCounts) != 4 {
		t.Errorf("got HandCounts length %d, want %d", len(got.HandCounts), 4)
	}
	if got.HandCounts[2] != 10 {
		t.Errorf("got HandCounts[2] %d, want %d", got.HandCounts[2], 10)
	}
	if len(got.Trick) != 2 {
		t.Errorf("got Trick length %d, want %d", len(got.Trick), 2)
	}
	if got.Trick[0].Seat != 2 {
		t.Errorf("got Trick[0].Seat %d, want %d", got.Trick[0].Seat, 2)
	}
	wantTrick0Card := Card{Rank: "king", Suit: "diamonds"}
	if got.Trick[0].Card != wantTrick0Card {
		t.Errorf("got Trick[0].Card %v, want %v", got.Trick[0].Card, wantTrick0Card)
	}
	if got.Scores[0] != 0 {
		t.Errorf("got Scores[0] %d, want %d", got.Scores[0], 0)
	}
	if got.RoundPoints[3] != 3 {
		t.Errorf("got RoundPoints[3] %d, want %d", got.RoundPoints[3], 3)
	}
	if len(got.LegalActions) != 2 {
		t.Errorf("got LegalActions length %d, want %d", len(got.LegalActions), 2)
	}
	if got.LegalActions[1] != (Card{Rank: "queen", Suit: "diamonds"}) {
		t.Errorf(
			"got LegalActions[1] %v, want %v",
			got.LegalActions[1],
			Card{Rank: "queen", Suit: "diamonds"},
		)
	}
}

// TestPassCardsPayloadRoundTrip verifies pass-card payloads round-trip through JSON.
func TestPassCardsPayloadRoundTrip(t *testing.T) {
	want := PassCardsPayload{
		Cards: []Card{
			{Rank: "queen", Suit: "spades"},
			{Rank: "king", Suit: "spades"},
			{Rank: "ace", Suit: "spades"},
		},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got PassCardsPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(got.Cards) != 3 {
		t.Errorf("got Cards length %d, want %d", len(got.Cards), 3)
	}
	for i, wantCard := range want.Cards {
		if got.Cards[i] != wantCard {
			t.Errorf("got Cards[%d] %v, want %v", i, got.Cards[i], wantCard)
		}
	}
}

// TestPlayCardPayloadRoundTrip verifies play-card payloads round-trip through JSON.
func TestPlayCardPayloadRoundTrip(t *testing.T) {
	want := PlayCardPayload{
		Card: Card{Rank: "queen", Suit: "spades"},
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got PlayCardPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Card != want.Card {
		t.Errorf("got Card %v, want %v", got.Card, want.Card)
	}
}
