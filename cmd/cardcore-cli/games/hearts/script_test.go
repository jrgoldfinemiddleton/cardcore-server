package heartscli

import (
	"encoding/json"
	"testing"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/games/hearts"
)

// TestNewBuilder verifies that NewBuilder returns a non-nil Hearts command builder.
func TestNewBuilder(t *testing.T) {
	b := NewBuilder()
	if b == nil {
		t.Fatal("NewBuilder() returned nil")
	}
}

// TestBuilderTransitionalPhases verifies the set of view-only phases between
// actionable turns.
func TestBuilderTransitionalPhases(t *testing.T) {
	b := NewBuilder()
	got := b.TransitionalPhases()
	want := []string{"trick_complete", "round_complete", "deal"}
	if len(got) != len(want) {
		t.Fatalf("TransitionalPhases() length = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("TransitionalPhases()[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// TestBuilderBuildCommandUnknownAction verifies that BuildCommand rejects an
// unsupported action.
func TestBuilderBuildCommandUnknownAction(t *testing.T) {
	b := NewBuilder()
	_, err := b.BuildCommand("invalid", "first_n", nil, []byte(`{}`), "id", 1)
	if err == nil {
		t.Fatal("BuildCommand(...) got nil error, want error for unknown action")
	}
}

// TestBuilderBuildPassCardsFirstN verifies that the first_n selector picks the
// leading cards from the hand.
func TestBuilderBuildPassCardsFirstN(t *testing.T) {
	b := NewBuilder()
	snapshot := []byte(`{"hand":[` +
		`{"rank":"two","suit":"clubs"},` +
		`{"rank":"queen","suit":"spades"},` +
		`{"rank":"ace","suit":"hearts"}]}`)
	args, _ := json.Marshal(map[string]int{"count": 2})

	cmd, err := b.BuildCommand("pass_cards", "first_n", args, snapshot, "pass-1", 5)
	if err != nil {
		t.Fatalf("BuildCommand(...) error: %v", err)
	}
	if cmd.Type != "pass_cards" {
		t.Errorf("Type = %q, want pass_cards", cmd.Type)
	}
	if cmd.ActionID != "pass-1" {
		t.Errorf("ActionID = %q, want pass-1", cmd.ActionID)
	}
	if cmd.Seq != 5 {
		t.Errorf("Seq = %d, want 5", cmd.Seq)
	}

	var payload struct {
		Cards []heartsclient.Card `json:"cards"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload.Cards) != 2 {
		t.Fatalf("Cards length = %d, want 2", len(payload.Cards))
	}
	if payload.Cards[0] != (heartsclient.Card{Rank: "two", Suit: "clubs"}) {
		t.Errorf("Cards[0] = %+v, want {two clubs}", payload.Cards[0])
	}
	if payload.Cards[1] != (heartsclient.Card{Rank: "queen", Suit: "spades"}) {
		t.Errorf("Cards[1] = %+v, want {queen spades}", payload.Cards[1])
	}
}

// TestBuilderBuildPassCardsByIndex verifies that the by_index selector picks the
// requested hand positions.
func TestBuilderBuildPassCardsByIndex(t *testing.T) {
	b := NewBuilder()
	snapshot := []byte(`{"hand":[` +
		`{"rank":"two","suit":"clubs"},` +
		`{"rank":"queen","suit":"spades"},` +
		`{"rank":"ace","suit":"hearts"}]}`)
	args, _ := json.Marshal(map[string][]int{"indices": {2, 0}})

	cmd, err := b.BuildCommand("pass_cards", "by_index", args, snapshot, "pass-2", 6)
	if err != nil {
		t.Fatalf("BuildCommand(...) error: %v", err)
	}

	var payload struct {
		Cards []heartsclient.Card `json:"cards"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(payload.Cards) != 2 {
		t.Fatalf("Cards length = %d, want 2", len(payload.Cards))
	}
	if payload.Cards[0] != (heartsclient.Card{Rank: "ace", Suit: "hearts"}) {
		t.Errorf("Cards[0] = %+v, want {ace hearts}", payload.Cards[0])
	}
	if payload.Cards[1] != (heartsclient.Card{Rank: "two", Suit: "clubs"}) {
		t.Errorf("Cards[1] = %+v, want {two clubs}", payload.Cards[1])
	}
}

// TestBuilderBuildPassCardsErrors verifies error handling for invalid selectors
// and arguments.
func TestBuilderBuildPassCardsErrors(t *testing.T) {
	b := NewBuilder()
	base := []byte(`{"hand":[{"rank":"two","suit":"clubs"},{"rank":"queen","suit":"spades"}]}`)

	tests := []struct {
		name   string
		action string
		sel    string
		args   json.RawMessage
		snap   []byte
	}{
		{
			name:   "unknown selector",
			action: "pass_cards",
			sel:    "unknown",
			args:   nil,
			snap:   base,
		},
		{
			name:   "first_n count zero",
			action: "pass_cards",
			sel:    "first_n",
			args:   mustJSON(t, map[string]int{"count": 0}),
			snap:   base,
		},
		{
			name:   "first_n not enough cards",
			action: "pass_cards",
			sel:    "first_n",
			args:   mustJSON(t, map[string]int{"count": 5}),
			snap:   base,
		},
		{
			name:   "by_index out of range",
			action: "pass_cards",
			sel:    "by_index",
			args:   mustJSON(t, map[string][]int{"indices": {5}}),
			snap:   base,
		},
		{
			name:   "by_index duplicate",
			action: "pass_cards",
			sel:    "by_index",
			args:   mustJSON(t, map[string][]int{"indices": {0, 0}}),
			snap:   base,
		},
		{
			name:   "invalid snapshot",
			action: "pass_cards",
			sel:    "first_n",
			args:   mustJSON(t, map[string]int{"count": 1}),
			snap:   []byte(`not-json`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := b.BuildCommand(tt.action, tt.sel, tt.args, tt.snap, "id", 1)
			if err == nil {
				t.Fatal("BuildCommand(...) got nil error, want error")
			}
		})
	}
}

// TestBuilderBuildPlayCardFirstLegal verifies that the first_legal selector
// picks the first legal action.
func TestBuilderBuildPlayCardFirstLegal(t *testing.T) {
	b := NewBuilder()
	snapshot := []byte(`{"legal_actions":[` +
		`{"rank":"two","suit":"clubs"},` +
		`{"rank":"ace","suit":"hearts"}]}`)

	cmd, err := b.BuildCommand("play_card", "first_legal", nil, snapshot, "play-1", 9)
	if err != nil {
		t.Fatalf("BuildCommand(...) error: %v", err)
	}
	if cmd.Type != "play_card" {
		t.Errorf("Type = %q, want play_card", cmd.Type)
	}
	if cmd.ActionID != "play-1" {
		t.Errorf("ActionID = %q, want play-1", cmd.ActionID)
	}
	if cmd.Seq != 9 {
		t.Errorf("Seq = %d, want 9", cmd.Seq)
	}

	var payload struct {
		Card heartsclient.Card `json:"card"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Card != (heartsclient.Card{Rank: "two", Suit: "clubs"}) {
		t.Errorf("Card = %+v, want {two clubs}", payload.Card)
	}
}

// TestBuilderBuildPlayCardByIndex verifies that the by_index selector picks the
// requested legal action.
func TestBuilderBuildPlayCardByIndex(t *testing.T) {
	b := NewBuilder()
	snapshot := []byte(`{"legal_actions":[` +
		`{"rank":"two","suit":"clubs"},` +
		`{"rank":"ace","suit":"hearts"}]}`)
	args, _ := json.Marshal(map[string]int{"index": 1})

	cmd, err := b.BuildCommand("play_card", "by_index", args, snapshot, "play-2", 10)
	if err != nil {
		t.Fatalf("BuildCommand(...) error: %v", err)
	}

	var payload struct {
		Card heartsclient.Card `json:"card"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Card != (heartsclient.Card{Rank: "ace", Suit: "hearts"}) {
		t.Errorf("Card = %+v, want {ace hearts}", payload.Card)
	}
}

// TestBuilderBuildPlayCardErrors verifies error handling for invalid selectors
// and arguments.
func TestBuilderBuildPlayCardErrors(t *testing.T) {
	b := NewBuilder()
	base := []byte(`{"legal_actions":[{"rank":"two","suit":"clubs"}]}`)

	tests := []struct {
		name string
		sel  string
		args json.RawMessage
		snap []byte
	}{
		{
			name: "unknown selector",
			sel:  "unknown",
			args: nil,
			snap: base,
		},
		{
			name: "no legal actions",
			sel:  "first_legal",
			args: nil,
			snap: []byte(`{"legal_actions":[]}`),
		},
		{
			name: "by_index out of range",
			sel:  "by_index",
			args: mustJSON(t, map[string]int{"index": 5}),
			snap: base,
		},
		{
			name: "invalid snapshot",
			sel:  "first_legal",
			args: nil,
			snap: []byte(`not-json`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := b.BuildCommand("play_card", tt.sel, tt.args, tt.snap, "id", 1)
			if err == nil {
				t.Fatal("BuildCommand(...) got nil error, want error")
			}
		})
	}
}

// mustJSON marshals v to JSON and fails the test on error.
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
