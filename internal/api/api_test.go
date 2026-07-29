package api

import (
	"encoding/json"
	"testing"
)

// TestErrorMessageActionIDOmitted verifies empty action IDs are omitted from JSON.
func TestErrorMessageActionIDOmitted(t *testing.T) {
	msg := ErrorMessage{
		Type:       "error",
		ErrorCode:  ErrMalformedMessage,
		Message:    "JSON parse error",
		ActionID:   "",
		CurrentSeq: 5,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal into map failed: %v", err)
	}

	if _, present := raw["action_id"]; present {
		t.Errorf("got action_id present in JSON, want it omitted when empty")
	}
}

// TestErrorMessageActionIDPresent verifies non-empty action IDs are included in JSON.
func TestErrorMessageActionIDPresent(t *testing.T) {
	msg := ErrorMessage{
		Type:       "error",
		ErrorCode:  ErrOutOfTurn,
		Message:    "not your turn",
		ActionID:   "abc-123",
		CurrentSeq: 7,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal into map failed: %v", err)
	}

	if _, present := raw["action_id"]; !present {
		t.Errorf("got action_id absent from JSON, want it present when non-empty")
	}

	var gotID string
	if err := json.Unmarshal(raw["action_id"], &gotID); err != nil {
		t.Fatalf("unmarshal action_id value failed: %v", err)
	}
	if gotID != "abc-123" {
		t.Errorf("got action_id %q, want %q", gotID, "abc-123")
	}
}

// TestValidateInboundMessage verifies envelope field validation.
func TestValidateInboundMessage(t *testing.T) {
	tests := []struct {
		name    string
		msg     *InboundMessage
		wantErr bool
	}{
		{
			name: "valid",
			msg: &InboundMessage{
				Type:     "play_card",
				ActionID: "abc",
				Seq:      0,
				Payload:  json.RawMessage(`{}`),
			},
			wantErr: false,
		},
		{
			name:    "nil message",
			msg:     nil,
			wantErr: true,
		},
		{
			name: "missing type",
			msg: &InboundMessage{
				ActionID: "abc",
				Seq:      0,
			},
			wantErr: true,
		},
		{
			name: "missing action_id",
			msg: &InboundMessage{
				Type: "play_card",
				Seq:  0,
			},
			wantErr: true,
		},
		{
			name: "negative seq",
			msg: &InboundMessage{
				Type:     "play_card",
				ActionID: "abc",
				Seq:      -1,
			},
			wantErr: true,
		},
		{
			name: "missing payload",
			msg: &InboundMessage{
				Type:     "play_card",
				ActionID: "abc",
				Seq:      0,
			},
			wantErr: true,
		},
		{
			name: "action_id too long",
			msg: &InboundMessage{
				Type:     "play_card",
				ActionID: string(make([]byte, MaxActionIDLength+1)),
				Seq:      0,
				Payload:  json.RawMessage(`{}`),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInboundMessage(tt.msg)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("got error %v, want error %v", gotErr, tt.wantErr)
			}
		})
	}
}
