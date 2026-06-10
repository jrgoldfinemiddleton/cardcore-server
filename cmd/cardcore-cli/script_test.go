package main

import (
	"encoding/json"
	"testing"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// TestParseScriptValid verifies that a well-formed JSON script is parsed
// into a phase-lookup map.
func TestParseScriptValid(t *testing.T) {
	data := []byte(`[
		{
			"phase": "passing",
			"action": "pass_cards",
			"selector": "first_n",
			"selector_args": {"count": 3}
		},
		{
			"phase": "playing",
			"action": "play_card",
			"selector": "first_legal"
		}
	]`)

	s, err := parseScript(data)
	if err != nil {
		t.Fatalf("ParseScript error: %v", err)
	}

	if len(s) != 2 {
		t.Fatalf("got %d entries, want 2", len(s))
	}

	pass, ok := s["passing"]
	if !ok {
		t.Fatal("missing passing entry")
	}
	if got, want := pass.Action, "pass_cards"; got != want {
		t.Errorf("passing action got %q, want %q", got, want)
	}
	if got, want := pass.Selector, "first_n"; got != want {
		t.Errorf("passing selector got %q, want %q", got, want)
	}

	play, ok := s["playing"]
	if !ok {
		t.Fatal("missing playing entry")
	}
	if got, want := play.Action, "play_card"; got != want {
		t.Errorf("playing action got %q, want %q", got, want)
	}
	if got, want := play.Selector, "first_legal"; got != want {
		t.Errorf("playing selector got %q, want %q", got, want)
	}
}

// TestParseScriptInvalidJSON verifies that malformed JSON returns an error.
func TestParseScriptInvalidJSON(t *testing.T) {
	data := []byte(`[invalid`)
	_, err := parseScript(data)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// TestParseScriptDuplicatePhase verifies that duplicate phase entries
// return an error.
func TestParseScriptDuplicatePhase(t *testing.T) {
	data := []byte(`[
		{
			"phase": "playing",
			"action": "play_card",
			"selector": "first_legal"
		},
		{
			"phase": "playing",
			"action": "pass_cards",
			"selector": "first_n",
			"selector_args": {"count": 3}
		}
	]`)
	_, err := parseScript(data)
	if err == nil {
		t.Fatal("expected error for duplicate phase, got nil")
	}
}

// TestScriptExecutorGameOver verifies that a game_over snapshot signals
// completion.
func TestScriptExecutorGameOver(t *testing.T) {
	script := Script{"playing": {Phase: "playing", Action: "play_card", Selector: "first_legal"}}
	exec := NewScriptExecutor(script, 0)

	snapshot := []byte(`{"phase": "game_over", "seq": 100, "scores": [0, 26, 13, 13]}`)
	cmd, done, err := exec.Step(snapshot)
	if err != nil {
		t.Fatalf("Step error: %v", err)
	}
	if !done {
		t.Fatal("expected done=true for game_over")
	}
	if cmd.Type != "" {
		t.Errorf("expected zero command for game_over, got %+v", cmd)
	}
}

// TestScriptExecutorNotMyTurn verifies that no command is produced when
// it is another seat's turn.
func TestScriptExecutorNotMyTurn(t *testing.T) {
	script := Script{"playing": {Phase: "playing", Action: "play_card", Selector: "first_legal"}}
	exec := NewScriptExecutor(script, 0)

	snapshot := []byte(`{
		"phase": "playing",
		"seq": 5,
		"turn": 1,
		"legal_actions": [{"rank": "two", "suit": "clubs"}]
	}`)
	cmd, done, err := exec.Step(snapshot)
	if err != nil {
		t.Fatalf("Step error: %v", err)
	}
	if done {
		t.Fatal("expected done=false")
	}
	if cmd.Type != "" {
		t.Errorf("expected zero command when not my turn, got %+v", cmd)
	}
}

// TestScriptExecutorWrongPhase verifies that no command is produced when
// the current phase has no script entry.
func TestScriptExecutorWrongPhase(t *testing.T) {
	script := Script{"playing": {Phase: "playing", Action: "play_card", Selector: "first_legal"}}
	exec := NewScriptExecutor(script, 0)

	// trick_complete has no script entry.
	snapshot := []byte(`{"phase": "trick_complete", "seq": 5, "turn": 0}`)
	_, _, err := exec.Step(snapshot)
	if err == nil {
		t.Fatal("expected error for unscripted phase")
	}
}

// TestScriptExecutorPassingPhase verifies that the first_n selector
// selects the first N cards from the hand in the passing phase.
func TestScriptExecutorPassingPhase(t *testing.T) {
	script := Script{
		"passing": {
			Phase:        "passing",
			Action:       "pass_cards",
			Selector:     "first_n",
			SelectorArgs: []byte(`{"count": 3}`),
		},
	}
	exec := NewScriptExecutor(script, 0)

	snapshot := []byte(`{
		"phase": "passing",
		"seq": 5,
		"turn": 0,
		"hand": [
			{"rank": "two", "suit": "clubs"},
			{"rank": "three", "suit": "clubs"},
			{"rank": "four", "suit": "clubs"},
			{"rank": "five", "suit": "clubs"}
		]
	}`)

	cmd, done, err := exec.Step(snapshot)
	if err != nil {
		t.Fatalf("Step error: %v", err)
	}
	if done {
		t.Fatal("expected done=false")
	}
	if cmd.Type != "pass_cards" {
		t.Errorf("got command type %q, want pass_cards", cmd.Type)
	}

	var payload heartsclient.PassCardsPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	want := []heartsclient.Card{
		{Rank: "two", Suit: "clubs"},
		{Rank: "three", Suit: "clubs"},
		{Rank: "four", Suit: "clubs"},
	}
	if len(payload.Cards) != len(want) {
		t.Fatalf("got %d cards, want %d", len(payload.Cards), len(want))
	}
	for i, c := range payload.Cards {
		if c != want[i] {
			t.Errorf("card %d: got %+v, want %+v", i, c, want[i])
		}
	}
}

// TestScriptExecutorPlayingPhase verifies that the first_legal selector
// selects the first legal action in the playing phase.
func TestScriptExecutorPlayingPhase(t *testing.T) {
	script := Script{
		"playing": {
			Phase:    "playing",
			Action:   "play_card",
			Selector: "first_legal",
		},
	}
	exec := NewScriptExecutor(script, 0)

	snapshot := []byte(`{
		"phase": "playing",
		"seq": 10,
		"turn": 0,
		"legal_actions": [
			{"rank": "two", "suit": "clubs"},
			{"rank": "three", "suit": "diamonds"}
		]
	}`)

	cmd, done, err := exec.Step(snapshot)
	if err != nil {
		t.Fatalf("Step error: %v", err)
	}
	if done {
		t.Fatal("expected done=false")
	}
	if cmd.Type != "play_card" {
		t.Errorf("got command type %q, want play_card", cmd.Type)
	}

	var payload heartsclient.PlayCardPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	want := heartsclient.Card{Rank: "two", Suit: "clubs"}
	if payload.Card != want {
		t.Errorf("got card %+v, want %+v", payload.Card, want)
	}
}

// TestScriptExecutorUnknownAction verifies that an unknown action in the
// script returns an error.
func TestScriptExecutorUnknownAction(t *testing.T) {
	script := Script{
		"playing": {
			Phase:    "playing",
			Action:   "invalid_action",
			Selector: "first_legal",
		},
	}
	exec := NewScriptExecutor(script, 0)

	snapshot := []byte(`{
		"phase": "playing",
		"seq": 10,
		"turn": 0,
		"legal_actions": [
			{"rank": "two", "suit": "clubs"}
		]
	}`)

	_, _, err := exec.Step(snapshot)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

// TestScriptExecutorFirstNInsufficientCards verifies that first_n fails
// when the hand is too small.
func TestScriptExecutorFirstNInsufficientCards(t *testing.T) {
	script := Script{
		"passing": {
			Phase:        "passing",
			Action:       "pass_cards",
			Selector:     "first_n",
			SelectorArgs: []byte(`{"count": 5}`),
		},
	}
	exec := NewScriptExecutor(script, 0)

	snapshot := []byte(`{
		"phase": "passing",
		"seq": 5,
		"turn": 0,
		"hand": [
			{"rank": "two", "suit": "clubs"},
			{"rank": "three", "suit": "clubs"}
		]
	}`)

	_, _, err := exec.Step(snapshot)
	if err == nil {
		t.Fatal("expected error for insufficient cards")
	}
}

// TestScriptExecutorFirstLegalNoActions verifies that first_legal fails
// when no legal actions are available.
func TestScriptExecutorFirstLegalNoActions(t *testing.T) {
	script := Script{
		"playing": {
			Phase:    "playing",
			Action:   "play_card",
			Selector: "first_legal",
		},
	}
	exec := NewScriptExecutor(script, 0)

	snapshot := []byte(`{
		"phase": "playing",
		"seq": 10,
		"turn": 0,
		"legal_actions": []
	}`)

	_, _, err := exec.Step(snapshot)
	if err == nil {
		t.Fatal("expected error for empty legal_actions")
	}
}

// TestScriptExecutorByIndex verifies that the by_index selector
// selects cards at the specified indices from the hand.
func TestScriptExecutorByIndex(t *testing.T) {
	script := Script{
		"passing": {
			Phase:        "passing",
			Action:       "pass_cards",
			Selector:     "by_index",
			SelectorArgs: []byte(`{"indices": [0, 2]}`),
		},
	}
	exec := NewScriptExecutor(script, 0)

	snapshot := []byte(`{
		"phase": "passing",
		"seq": 5,
		"turn": 0,
		"hand": [
			{"rank": "two", "suit": "clubs"},
			{"rank": "three", "suit": "clubs"},
			{"rank": "four", "suit": "clubs"}
		]
	}`)

	cmd, done, err := exec.Step(snapshot)
	if err != nil {
		t.Fatalf("Step error: %v", err)
	}
	if done {
		t.Fatal("expected done=false")
	}
	if cmd.Type != "pass_cards" {
		t.Errorf("got command type %q, want pass_cards", cmd.Type)
	}

	var payload heartsclient.PassCardsPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	want := []heartsclient.Card{
		{Rank: "two", Suit: "clubs"},
		{Rank: "four", Suit: "clubs"},
	}
	if len(payload.Cards) != len(want) {
		t.Fatalf("got %d cards, want %d", len(payload.Cards), len(want))
	}
	for i, c := range payload.Cards {
		if c != want[i] {
			t.Errorf("card %d: got %+v, want %+v", i, c, want[i])
		}
	}
}

// TestScriptExecutorByIndexOutOfRange verifies that by_index returns an
// error when an index is out of range.
func TestScriptExecutorByIndexOutOfRange(t *testing.T) {
	script := Script{
		"passing": {
			Phase:        "passing",
			Action:       "pass_cards",
			Selector:     "by_index",
			SelectorArgs: []byte(`{"indices": [0, 5]}`),
		},
	}
	exec := NewScriptExecutor(script, 0)

	snapshot := []byte(`{
		"phase": "passing",
		"seq": 5,
		"turn": 0,
		"hand": [
			{"rank": "two", "suit": "clubs"},
			{"rank": "three", "suit": "clubs"},
			{"rank": "four", "suit": "clubs"}
		]
	}`)

	_, _, err := exec.Step(snapshot)
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}
