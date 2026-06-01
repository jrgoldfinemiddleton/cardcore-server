package client

import (
	"encoding/json"
	"testing"
)

// TestCommandJSONRoundTrip verifies that Command marshals and unmarshals
// correctly, preserving all envelope fields.
func TestCommandJSONRoundTrip(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"card": map[string]string{
			"rank": "queen",
			"suit": "spades",
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	original := Command{
		Type:     "play_card",
		ActionID: "test-action-123",
		Seq:      42,
		Payload:  payload,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}

	var got Command
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal command: %v", err)
	}

	if got.Type != original.Type {
		t.Errorf("got Type %q, want %q", got.Type, original.Type)
	}
	if got.ActionID != original.ActionID {
		t.Errorf("got ActionID %q, want %q", got.ActionID, original.ActionID)
	}
	if got.Seq != original.Seq {
		t.Errorf("got Seq %d, want %d", got.Seq, original.Seq)
	}
	if string(got.Payload) != string(original.Payload) {
		t.Errorf("got Payload %s, want %s", got.Payload, original.Payload)
	}
}

// TestErrorMessageJSONRoundTrip verifies that ErrorMessage marshals and
// unmarshals correctly, preserving all fields.
func TestErrorMessageJSONRoundTrip(t *testing.T) {
	original := ErrorMessage{
		Type:       "error",
		ErrorCode:  "illegal_move",
		Message:    "Must follow suit: diamonds was led",
		ActionID:   "test-action-456",
		CurrentSeq: 7,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error message: %v", err)
	}

	var got ErrorMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal error message: %v", err)
	}

	if got.Type != original.Type {
		t.Errorf("got Type %q, want %q", got.Type, original.Type)
	}
	if got.ErrorCode != original.ErrorCode {
		t.Errorf("got ErrorCode %q, want %q", got.ErrorCode, original.ErrorCode)
	}
	if got.Message != original.Message {
		t.Errorf("got Message %q, want %q", got.Message, original.Message)
	}
	if got.ActionID != original.ActionID {
		t.Errorf("got ActionID %q, want %q", got.ActionID, original.ActionID)
	}
	if got.CurrentSeq != original.CurrentSeq {
		t.Errorf("got CurrentSeq %d, want %d", got.CurrentSeq, original.CurrentSeq)
	}
}
