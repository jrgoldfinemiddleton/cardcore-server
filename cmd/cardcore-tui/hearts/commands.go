package heartstui

import (
	"errors"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// BuildPassCommand builds a pass_cards command from the selected cards.
//
// It returns an error if len(selected) != 3. The seq field is set to 0 because
// the caller's Conn.SendCommand overrides seq with maxSeenSeq.
func BuildPassCommand(actionID string, selected []heartsclient.Card) (client.Command, error) {
	if len(selected) != 3 {
		return client.Command{}, errors.New("pass command requires exactly 3 cards")
	}
	return heartsclient.NewPassCardsMessage(actionID, 0, selected)
}

// BuildPlayCommand builds a play_card command for the given card.
//
// The seq field is set to 0 because the caller's Conn.SendCommand overrides
// seq with maxSeenSeq.
func BuildPlayCommand(actionID string, card heartsclient.Card) (client.Command, error) {
	return heartsclient.NewPlayCardMessage(actionID, 0, card)
}
