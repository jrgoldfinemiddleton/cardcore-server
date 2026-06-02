package hearts

import (
	"encoding/json"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

// NewPlayCardMessage builds a play_card command envelope with the given card
// in the payload.
func NewPlayCardMessage(actionID string, seq int, card Card) (client.Command, error) {
	payload := PlayCardPayload{Card: card}
	data, err := json.Marshal(payload)
	if err != nil {
		return client.Command{}, err
	}
	return client.Command{
		Type:     "play_card",
		ActionID: actionID,
		Seq:      seq,
		Payload:  data,
	}, nil
}

// NewPassCardsMessage builds a pass_cards command envelope with the given
// cards in the payload.
func NewPassCardsMessage(actionID string, seq int, cards []Card) (client.Command, error) {
	payload := PassCardsPayload{Cards: cards}
	data, err := json.Marshal(payload)
	if err != nil {
		return client.Command{}, err
	}
	return client.Command{
		Type:     "pass_cards",
		ActionID: actionID,
		Seq:      seq,
		Payload:  data,
	}, nil
}
