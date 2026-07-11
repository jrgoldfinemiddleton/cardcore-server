package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"syscall"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
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

// GameBuilder constructs commands for a specific game from script entries.
type GameBuilder interface {
	// BuildCommand creates a client.Command for the given script entry and snapshot.
	// The action and selector describe what to build; selectorArgs holds
	// selector-specific parameters. The snapshot is the full JSON snapshot
	// for card resolution. actionID and seq are injected into the command.
	BuildCommand(
		action, selector string,
		selectorArgs json.RawMessage,
		snapshot []byte,
		actionID string,
		seq int,
	) (client.Command, error)
	// TransitionalPhases returns phases that do not require a script entry.
	// The executor skips these silently; actionable phases not present in
	// the script still produce an error so the caller knows the script is
	// incomplete.
	TransitionalPhases() []string
}

// ScriptExecutor evaluates incoming snapshots and produces commands
// according to a script. It is game-agnostic: the core only knows
// phases, actions, and selectors are strings. Game-specific payload
// construction is delegated to the Builder.
type ScriptExecutor struct {
	// script maps phase names to action specifications.
	script Script
	// mySeat is the seat index this executor acts on behalf of.
	mySeat int
	// actionSeq is a monotonic counter for generating unique action IDs.
	actionSeq int
	// builder constructs game-specific commands.
	builder GameBuilder
}

// NewScriptExecutor returns an executor for the given script, seat, and builder.
func NewScriptExecutor(script Script, mySeat int, builder GameBuilder) *ScriptExecutor {
	return &ScriptExecutor{script: script, mySeat: mySeat, builder: builder}
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

	if env.Phase == phaseGameOver {
		return client.Command{}, true, nil
	}

	if env.Turn != e.mySeat {
		return client.Command{}, false, nil
	}

	entry, ok := e.script[env.Phase]
	if !ok {
		if e.isTransitional(env.Phase) {
			return client.Command{}, false, nil
		}
		return client.Command{}, false, fmt.Errorf(
			"no script entry for phase %q (script missing required phase)", env.Phase,
		)
	}

	actionID := e.nextActionID()

	var seqEnv struct {
		Seq int `json:"seq"`
	}
	if err := json.Unmarshal(snapshot, &seqEnv); err != nil {
		return client.Command{}, false, fmt.Errorf("unmarshal seq: %w", err)
	}

	cmd, err := e.builder.BuildCommand(
		entry.Action, entry.Selector, entry.SelectorArgs,
		snapshot, actionID, seqEnv.Seq,
	)
	if err != nil {
		return client.Command{}, false, fmt.Errorf("build %s command: %w", entry.Action, err)
	}
	return cmd, false, nil
}

// isTransitional reports whether phase is a view-only transitional phase
// that does not require a script entry.
func (e *ScriptExecutor) isTransitional(phase string) bool {
	for _, p := range e.builder.TransitionalPhases() {
		if p == phase {
			return true
		}
	}
	return false
}

// nextActionID generates a deterministic action ID for the next command.
// Includes seat number to prevent collisions between multiple CLI clients in
// the same session.
func (e *ScriptExecutor) nextActionID() string {
	e.actionSeq++
	return fmt.Sprintf("cli-%d-%d", e.mySeat, e.actionSeq)
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
func printFinalScores(snapshot []byte) error {
	var snap struct {
		Scores []int `json:"scores"`
	}
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		slog.Warn("unmarshal final scores", "error", err)
		return nil
	}
	if _, err := fmt.Printf("Final scores: %v\n", snap.Scores); err != nil {
		if errors.Is(err, syscall.EPIPE) {
			return errBrokenPipe
		}
		return fmt.Errorf("write stdout: %w", err)
	}
	return nil
}
