package heartscli

import (
	"encoding/json"
	"fmt"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// Builder implements game-specific command construction for Hearts.
type Builder struct{}

// NewBuilder returns a Hearts game builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// TransitionalPhases returns the Hearts phases that do not require a script
// entry. These are view-only snapshots displayed between actionable phases.
func (b *Builder) TransitionalPhases() []string {
	return []string{"trick_complete", "round_complete", "deal"}
}

// BuildCommand creates a client.Command for the given script entry and snapshot.
// It dispatches to the appropriate builder based on the action.
func (b *Builder) BuildCommand(
	action, selector string,
	selectorArgs json.RawMessage,
	snapshot []byte,
	actionID string,
	seq int,
) (client.Command, error) {
	switch action {
	case "pass_cards":
		return b.buildPassCards(selector, selectorArgs, snapshot, actionID, seq)
	case "play_card":
		return b.buildPlayCard(selector, selectorArgs, snapshot, actionID, seq)
	default:
		return client.Command{}, fmt.Errorf("unknown action %q", action)
	}
}

// buildPassCards resolves the selector against the snapshot's hand and
// builds a pass_cards command.
func (b *Builder) buildPassCards(
	selector string,
	selectorArgs json.RawMessage,
	snapshot []byte,
	actionID string,
	seq int,
) (client.Command, error) {
	var snap struct {
		Hand []heartsclient.Card `json:"hand"`
	}
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		return client.Command{}, fmt.Errorf("unmarshal hand: %w", err)
	}

	var cards []heartsclient.Card
	switch selector {
	case "first_n":
		var args struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal(selectorArgs, &args); err != nil {
			return client.Command{}, fmt.Errorf("parse selector_args: %w", err)
		}
		if args.Count <= 0 {
			return client.Command{}, fmt.Errorf("count must be > 0, got %d", args.Count)
		}
		if len(snap.Hand) < args.Count {
			return client.Command{}, fmt.Errorf(
				"hand has %d cards, need %d", len(snap.Hand), args.Count,
			)
		}
		cards = snap.Hand[:args.Count]
	case "by_index":
		var args struct {
			Indices []int `json:"indices"`
		}
		if err := json.Unmarshal(selectorArgs, &args); err != nil {
			return client.Command{}, fmt.Errorf("parse by_index args: %w", err)
		}
		if len(args.Indices) == 0 {
			return client.Command{}, fmt.Errorf("by_index requires at least one index")
		}
		seen := make(map[int]struct{}, len(args.Indices))
		for _, idx := range args.Indices {
			if idx < 0 || idx >= len(snap.Hand) {
				return client.Command{}, fmt.Errorf(
					"index %d out of range [0,%d)", idx, len(snap.Hand),
				)
			}
			if _, ok := seen[idx]; ok {
				return client.Command{}, fmt.Errorf("duplicate index %d", idx)
			}
			seen[idx] = struct{}{}
			cards = append(cards, snap.Hand[idx])
		}
	default:
		return client.Command{}, fmt.Errorf("unknown selector %q for pass_cards", selector)
	}

	return heartsclient.NewPassCardsMessage(actionID, seq, cards)
}

// buildPlayCard resolves the selector against the snapshot's legal_actions
// and builds a play_card command.
func (b *Builder) buildPlayCard(
	selector string,
	selectorArgs json.RawMessage,
	snapshot []byte,
	actionID string,
	seq int,
) (client.Command, error) {
	var snap struct {
		LegalActions []heartsclient.Card `json:"legal_actions"`
	}
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		return client.Command{}, fmt.Errorf("unmarshal legal_actions: %w", err)
	}
	if len(snap.LegalActions) == 0 {
		return client.Command{}, fmt.Errorf("no legal actions available")
	}

	var card heartsclient.Card
	switch selector {
	case "first_legal":
		card = snap.LegalActions[0]
	case "last_legal":
		return client.Command{}, fmt.Errorf("selector %q not implemented", selector)
	case "random_legal":
		return client.Command{}, fmt.Errorf("selector %q not implemented", selector)
	case "by_index":
		var args struct {
			Index int `json:"index"`
		}
		if err := json.Unmarshal(selectorArgs, &args); err != nil {
			return client.Command{}, fmt.Errorf("parse by_index args: %w", err)
		}
		if args.Index < 0 || args.Index >= len(snap.LegalActions) {
			return client.Command{}, fmt.Errorf(
				"index %d out of range [0,%d)", args.Index, len(snap.LegalActions),
			)
		}
		card = snap.LegalActions[args.Index]
	default:
		return client.Command{}, fmt.Errorf("unknown selector %q for play_card", selector)
	}

	return heartsclient.NewPlayCardMessage(actionID, seq, card)
}
