package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
)

// ScriptEntry describes a single scripted action tied to a game phase.
type ScriptEntry struct {
	// Phase is the game phase that triggers this entry (e.g., "passing",
	// "playing").
	Phase string `json:"phase"`
	// Action is the command type to send (e.g., "pass_cards", "play_card").
	Action string `json:"action"`
	// Selector describes how to choose cards or actions from the snapshot
	// (e.g., "first_n", "first_legal", "by_index").
	Selector string `json:"selector"`
	// SelectorArgs holds selector-specific parameters. For "first_n" this
	// contains {"count": N}; for "by_index" this contains {"index": N}.
	SelectorArgs json.RawMessage `json:"selector_args,omitempty"`
}

// Script maps each phase to the action to perform when it is the player's
// turn in that phase. This is a lookup table, not a sequence: the same
// entry is reused every time the phase recurs.
type Script map[string]ScriptEntry

// ScriptExecutor evaluates incoming snapshots and produces commands
// according to a script. It is game-agnostic: the core only knows
// phases, actions, and selectors are strings. Hearts-specific payload
// construction lives in the build methods.
type ScriptExecutor struct {
	// script maps phase names to action specifications.
	script Script
	// mySeat is the seat index this executor acts on behalf of.
	mySeat int
	// actionSeq is a monotonic counter for generating unique action IDs.
	actionSeq int
}

// NewScriptExecutor returns an executor for the given script and seat.
func NewScriptExecutor(script Script, mySeat int) *ScriptExecutor {
	return &ScriptExecutor{script: script, mySeat: mySeat}
}

// Step evaluates a snapshot. It returns a command if the snapshot
// indicates it is this seat's turn in a scripted phase. The second
// return value is true when the snapshot's phase is "game_over".
// If it is not this seat's turn, or the phase has no script entry,
// it returns the zero value, false, nil.
func (e *ScriptExecutor) Step(snapshot []byte) (client.Command, bool, error) {
	var env struct {
		Phase string `json:"phase"`
		Turn  int    `json:"turn"`
	}
	if err := json.Unmarshal(snapshot, &env); err != nil {
		return client.Command{}, false, fmt.Errorf("unmarshal snapshot envelope: %w", err)
	}

	if env.Phase == "game_over" {
		return client.Command{}, true, nil
	}

	if env.Turn != e.mySeat {
		return client.Command{}, false, nil
	}

	entry, ok := e.script[env.Phase]
	if !ok {
		return client.Command{}, false, fmt.Errorf(
			"no script entry for phase %q (script missing required phase)", env.Phase,
		)
	}

	cmd, err := e.buildCommand(entry, snapshot)
	if err != nil {
		return client.Command{}, false, fmt.Errorf("build %s command: %w", entry.Action, err)
	}
	return cmd, false, nil
}

// nextActionID generates a deterministic action ID for the next command.
func (e *ScriptExecutor) nextActionID() string {
	e.actionSeq++
	return fmt.Sprintf("cli-%d", e.actionSeq)
}

// buildCommand dispatches to the appropriate builder based on the action.
func (e *ScriptExecutor) buildCommand(entry ScriptEntry, snapshot []byte) (client.Command, error) {
	actionID := e.nextActionID()

	var env struct {
		Seq int `json:"seq"`
	}
	if err := json.Unmarshal(snapshot, &env); err != nil {
		return client.Command{}, fmt.Errorf("unmarshal seq: %w", err)
	}

	switch entry.Action {
	case "pass_cards":
		return e.buildPassCards(entry, snapshot, actionID, env.Seq)
	case "play_card":
		return e.buildPlayCard(entry, snapshot, actionID, env.Seq)
	default:
		return client.Command{}, fmt.Errorf("unknown action %q", entry.Action)
	}
}

// buildPassCards resolves the selector against the snapshot's hand and
// builds a pass_cards command.
func (e *ScriptExecutor) buildPassCards(
	entry ScriptEntry,
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
	switch entry.Selector {
	case "first_n":
		var args struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal(entry.SelectorArgs, &args); err != nil {
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
		if err := json.Unmarshal(entry.SelectorArgs, &args); err != nil {
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
		return client.Command{}, fmt.Errorf("unknown selector %q for pass_cards", entry.Selector)
	}

	return heartsclient.NewPassCardsMessage(actionID, seq, cards)
}

// buildPlayCard resolves the selector against the snapshot's legal_actions
// and builds a play_card command.
func (e *ScriptExecutor) buildPlayCard(
	entry ScriptEntry,
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
	switch entry.Selector {
	case "first_legal":
		card = snap.LegalActions[0]
	case "last_legal":
		// Not yet implemented; reserved selector.
		return client.Command{}, fmt.Errorf("selector %q not implemented", entry.Selector)
	case "random_legal":
		// Not yet implemented; reserved selector.
		return client.Command{}, fmt.Errorf("selector %q not implemented", entry.Selector)
	case "by_index":
		var args struct {
			Index int `json:"index"`
		}
		if err := json.Unmarshal(entry.SelectorArgs, &args); err != nil {
			return client.Command{}, fmt.Errorf("parse by_index args: %w", err)
		}
		if args.Index < 0 || args.Index >= len(snap.LegalActions) {
			return client.Command{}, fmt.Errorf(
				"index %d out of range [0,%d)", args.Index, len(snap.LegalActions),
			)
		}
		card = snap.LegalActions[args.Index]
	default:
		return client.Command{}, fmt.Errorf("unknown selector %q for play_card", entry.Selector)
	}

	return heartsclient.NewPlayCardMessage(actionID, seq, card)
}

// parseScript unmarshals a JSON array of ScriptEntry values into a
// phase-lookup Script. It returns an error if the JSON is invalid or if
// the same phase appears more than once.
func parseScript(data []byte) (Script, error) {
	var entries []ScriptEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse script: %w", err)
	}

	s := make(Script, len(entries))
	for _, e := range entries {
		if _, ok := s[e.Phase]; ok {
			return nil, fmt.Errorf("duplicate phase %q in script", e.Phase)
		}
		s[e.Phase] = e
	}
	return s, nil
}

// printFinalScores extracts and prints the scores from a game_over snapshot.
func printFinalScores(snapshot []byte) {
	var snap struct {
		Scores []int `json:"scores"`
	}
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		slog.Warn("unmarshal final scores", "error", err)
		return
	}
	fmt.Printf("Final scores: %v\n", snap.Scores)
}

// wsURL converts an HTTP base URL to a WebSocket URL for the given
// session and path.
func wsURL(baseURL, sessionID, path string) string {
	u := strings.TrimSuffix(baseURL, "/")
	u = strings.Replace(u, "http://", "ws://", 1)
	u = strings.Replace(u, "https://", "wss://", 1)
	return fmt.Sprintf("%s/sessions/%s%s", u, sessionID, path)
}
