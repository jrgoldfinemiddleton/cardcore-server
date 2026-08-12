package heartssession

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"runtime"
	"time"

	"github.com/jrgoldfinemiddleton/cardcore"
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts/ai"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
	heartsapi "github.com/jrgoldfinemiddleton/cardcore-server/internal/api/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartsview "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/view/games/hearts"
)

// GameAdapter implements [session.Game] for Hearts.
type GameAdapter struct {
	// game is the underlying Hearts engine instance.
	game *hearts.Game
	// players holds the AI player for each seat. Human seats receive a
	// fallback AI player for timeout auto-play.
	players [hearts.NumPlayers]hearts.Player
	// paused tracks which UX pause is active. Nil when not paused.
	paused *pauseState
	// dealDelay is the display delay in milliseconds returned after a
	// fresh deal.
	dealDelay int
	// trickDelay is the display delay in milliseconds returned when a
	// trick completes.
	trickDelay int
	// roundDelay is the display delay in milliseconds returned when a
	// round completes.
	roundDelay int
	// dealPending is true after a fresh Deal(), drives the synthesized deal
	// phase through ViewState and DealPending, and is consumed by DisplayDelay.
	dealPending bool
	// previousScores holds the cumulative scores at the start of the current
	// round. It is used to compute the per-seat score delta shown in the
	// round_complete snapshot.
	previousScores [hearts.NumPlayers]int
	// logger is the per-component structured logger.
	logger *slog.Logger
	// turnDeadline is the authoritative deadline for the current human
	// turn, set by the session goroutine. It is not used by game logic.
	turnDeadline time.Time
	// isPaused is the external UX pause state controlled by the session
	// goroutine. It is not used by game logic.
	isPaused bool
}

// pauseState captures the adapter state during a UX pause.
type pauseState struct {
	// trickComplete is true when the adapter is paused after a trick
	// completes and is waiting for the session to call Resume.
	trickComplete bool
	// roundComplete is true when the adapter is paused after a round
	// completes and is waiting for the session to call Resume.
	roundComplete bool
}

// defaultHumanAIType is the fallback AI type used for human seats when a
// timeout auto-play is needed.
const defaultHumanAIType = "random"

// NewGameAdapter creates a Hearts game adapter. It validates the seat
// configuration, creates AI players for all seats (AI seats use their
// configured ai_type; human seats use their configured ai_type when set,
// falling back to "random" otherwise), and deals the first hand. It marks the
// deal pending so the session emits deal before the first actionable snapshot.
func NewGameAdapter(
	seats []session.SeatConfig, rng *rand.Rand,
	dealDelay, trickDelay, roundDelay int,
) (*GameAdapter, error) {
	cfg := session.Config{Seats: seats}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	a := &GameAdapter{
		dealDelay:   dealDelay,
		trickDelay:  trickDelay,
		roundDelay:  roundDelay,
		dealPending: true,
		logger:      slog.With("component", "hearts_adapter"),
	}
	for i, sc := range seats {
		aiType := sc.AIType
		if sc.Type != session.SeatAI && aiType == "" {
			aiType = defaultHumanAIType
		}
		a.logger.Info("seat configured",
			"seat", i,
			"ai_type", aiType,
			"seat_type", sc.Type,
		)
		p, err := newPlayer(aiType, rng)
		if err != nil {
			return nil, fmt.Errorf("seat %d: %w", i, err)
		}
		a.players[i] = p
	}

	a.game = hearts.New(rng)
	if err := a.game.Deal(); err != nil {
		return nil, fmt.Errorf("initial deal: %w", err)
	}
	a.previousScores = a.game.Scores

	return a, nil
}

// HandleAction processes an inbound player action. It validates turn
// order, phase, and legality, returning a CommandError for rejected
// actions.
func (a *GameAdapter) HandleAction(
	seat int, msg *api.InboundMessage,
) (session.StepResult, *session.CommandError) {
	a.logger.Debug("HandleAction", "seat", seat, "type", msg.Type)

	if a.game.Phase == hearts.PhaseEnd {
		a.logger.Warn("action rejected: game over", "seat", seat, "type", msg.Type)
		return session.StepResult{},
			&session.CommandError{
				Code:    api.ErrGameOver,
				Message: "game is over",
			}
	}

	switch msg.Type {
	case "play_card":
		return a.handlePlayCard(seat, msg.Payload)
	case "pass_cards":
		return a.handlePassCards(seat, msg.Payload)
	default:
		a.logger.Warn("unknown message type", "seat", seat, "type", msg.Type)
		return session.StepResult{},
			&session.CommandError{
				Code: api.ErrMalformedMessage,
				Message: fmt.Sprintf(
					"unknown message type: %q", msg.Type,
				),
			}
	}
}

// AIPlay executes the AI player's move for the given seat. It dispatches
// to the Hearts engine's ChoosePass or ChoosePlay depending on phase, and
// advances Turn manually during passing since the engine only
// auto-advances on the 4th pass.
func (a *GameAdapter) AIPlay(seat int) (session.StepResult, error) {
	a.logger.Debug("AIPlay", "seat", seat, "phase", heartsapi.PhaseToWire(a.game.Phase))

	s := hearts.Seat(seat)
	p := a.players[seat]
	if p == nil {
		a.logger.Error("AIPlay on non-AI seat", "seat", seat)
		return session.StepResult{},
			fmt.Errorf("seat %d is not an AI seat", seat)
	}

	switch a.game.Phase {
	case hearts.PhasePass:
		cards := p.ChoosePass(a.game, s)
		if err := a.game.SetPass(s, cards); err != nil {
			a.logger.Error("AI pass failed", "seat", seat, "error", err)
			return session.StepResult{},
				fmt.Errorf("AI pass seat %d: %w", seat, err)
		}
		// SetPass transitions PhasePass→PhasePlay when the 4th player
		// passes. When that happens, the engine sets Turn to the 2♣
		// holder. Only advance Turn manually if passing is still ongoing.
		if a.game.Phase == hearts.PhasePass {
			a.advanceTurn()
		}
		return session.StepResult{Outcome: session.StepContinue}, nil
	case hearts.PhasePlay:
		return a.playCard(seat, p.ChoosePlay(a.game, s))
	default:
		a.logger.Error("AI cannot act in current phase",
			"seat", seat, "phase", heartsapi.PhaseToWire(a.game.Phase),
		)
		return session.StepResult{},
			fmt.Errorf(
				"AI cannot act in phase %q",
				heartsapi.PhaseToWire(a.game.Phase),
			)
	}
}

// Resume advances the game past a pausable state. Only valid when the
// adapter is paused after returning StepPause. Resume can itself return
// StepPause when pauses chain — resolving a completed trick that also ends
// the round pauses again for round completion — so callers must keep
// resuming until the outcome is StepContinue or StepFinished.
func (a *GameAdapter) Resume() (session.StepResult, error) {
	a.logger.Debug("Resume", "paused", a.paused != nil)

	if a.paused == nil {
		a.logger.Warn("Resume called when not paused")
		return session.StepResult{},
			errors.New("Resume called when not paused")
	}

	if a.paused.trickComplete {
		// The engine defers trick resolution until the server explicitly
		// requests it. Resolve the completed trick before checking whether
		// the round ended.
		if err := a.game.ResolveTrick(); err != nil {
			return session.StepResult{}, fmt.Errorf("ResolveTrick: %w", err)
		}
		a.paused = nil
		if a.game.Phase == hearts.PhaseScore {
			a.paused = &pauseState{roundComplete: true}
			return session.StepResult{
				Outcome: session.StepPause,
			}, nil
		}
		return session.StepResult{
			Outcome: session.StepContinue,
		}, nil
	}

	if a.paused.roundComplete {
		a.paused = nil
		if err := a.game.EndRound(); err != nil {
			return session.StepResult{},
				fmt.Errorf("EndRound: %w", err)
		}
		if a.game.Phase == hearts.PhaseEnd {
			return session.StepResult{
				Outcome: session.StepFinished,
			}, nil
		}
		// New round: dealPending makes session.resumePauses broadcast deal
		// and then the actionable transition after the display delay.
		if err := a.game.Deal(); err != nil {
			return session.StepResult{},
				fmt.Errorf("deal: %w", err)
		}
		a.previousScores = a.game.Scores
		a.dealPending = true
		// After Deal, Turn is not updated if PassDir != PassHold.
		// Ensure Turn is set to a valid seat so processTurns can proceed.
		// This applies to subsequent rounds after EndRound; the first
		// round's Turn is implicitly 0 from [hearts.New]().
		if a.game.Phase == hearts.PhasePass {
			a.game.Turn = 0
		}
		return session.StepResult{
			Outcome: session.StepContinue,
		}, nil
	}

	return session.StepResult{}, errors.New("unknown pause state")
}

// Turn returns the seat index whose turn it is. It reads directly from
// the Hearts engine's Turn field, which the engine manages during play
// and the adapter manages during passing.
func (a *GameAdapter) Turn() int {
	return int(a.game.Turn)
}

// SetTurnDeadline stores the authoritative deadline for the current
// human turn. It is forwarded to the view layer via ViewState.
func (a *GameAdapter) SetTurnDeadline(deadline time.Time) {
	a.turnDeadline = deadline
}

// SetPaused stores the external UX pause flag so the view layer can
// include it in snapshots. The adapter does not enforce pause logic
// itself — that is the session goroutine's responsibility.
func (a *GameAdapter) SetPaused(paused bool) {
	a.isPaused = paused
}

// DealPending reports whether a freshly dealt hand is awaiting its deal-phase
// display window.
func (a *GameAdapter) DealPending() bool {
	return a.dealPending
}

// DisplayDelay returns phase-aware pacing: deal delay on fresh deals, trick
// delay on trick completion, round delay on round completion, and zero
// otherwise. Consuming a fresh deal clears deal-phase synthesis.
func (a *GameAdapter) DisplayDelay() int {
	if a.dealPending {
		a.dealPending = false
		return a.dealDelay
	}
	if a.paused != nil {
		if a.paused.trickComplete {
			return a.trickDelay
		}
		if a.paused.roundComplete {
			return a.roundDelay
		}
	}
	return 0
}

// PlayerSnapshot delegates to heartsview.PlayerView, which clones the
// engine state and masks other seats' hands so each player sees only
// their own cards.
func (a *GameAdapter) PlayerSnapshot(seat int, seq int) any {
	return heartsview.PlayerView(a.viewState(), hearts.Seat(seat), seq)
}

// ObserverSnapshot delegates to heartsview.ObserverView, which clones the
// engine state and exposes all seats' hands for omniscient viewing.
func (a *GameAdapter) ObserverSnapshot(seq int) any {
	return heartsview.ObserverView(a.viewState(), seq)
}

// handlePlayCard processes a play_card action.
func (a *GameAdapter) handlePlayCard(
	seat int, payload json.RawMessage,
) (session.StepResult, *session.CommandError) {
	var p heartsapi.PlayCardPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return session.StepResult{},
			&session.CommandError{
				Code:    api.ErrMalformedMessage,
				Message: "invalid play_card payload",
			}
	}

	ec, err := heartsapi.CardToEngine(p.Card)
	if err != nil {
		return session.StepResult{},
			&session.CommandError{
				Code:    api.ErrMalformedMessage,
				Message: fmt.Sprintf("invalid card: %v", err),
			}
	}

	res, playErr := a.playCard(seat, ec)
	if playErr != nil {
		return session.StepResult{}, a.engineErrToCommandError(playErr)
	}
	return res, nil
}

// handlePassCards processes a pass_cards action.
func (a *GameAdapter) handlePassCards(
	seat int, payload json.RawMessage,
) (session.StepResult, *session.CommandError) {
	var p heartsapi.PassCardsPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return session.StepResult{},
			&session.CommandError{
				Code:    api.ErrMalformedMessage,
				Message: "invalid pass_cards payload",
			}
	}

	if len(p.Cards) != hearts.PassCount {
		return session.StepResult{},
			&session.CommandError{
				Code: api.ErrMalformedMessage,
				Message: fmt.Sprintf(
					"must pass exactly %d cards, got %d",
					hearts.PassCount, len(p.Cards),
				),
			}
	}

	var cards [hearts.PassCount]cardcore.Card
	for i, wc := range p.Cards {
		ec, err := heartsapi.CardToEngine(wc)
		if err != nil {
			return session.StepResult{},
				&session.CommandError{
					Code: api.ErrMalformedMessage,
					Message: fmt.Sprintf(
						"invalid card at index %d: %v", i, err,
					),
				}
		}
		cards[i] = ec
	}

	if err := a.game.SetPass(hearts.Seat(seat), cards); err != nil {
		return session.StepResult{}, a.engineErrToCommandError(err)
	}

	// SetPass transitions PhasePass→PhasePlay when the 4th player
	// passes. When that happens, the engine sets Turn to the 2♣
	// holder. Only advance Turn manually if passing is still ongoing.
	if a.game.Phase == hearts.PhasePass {
		a.advanceTurn()
	}

	return session.StepResult{Outcome: session.StepContinue}, nil
}

// playCard applies a card play and determines the step outcome. If the
// play completes a trick, the adapter enters a paused state.
func (a *GameAdapter) playCard(
	seat int, card cardcore.Card,
) (session.StepResult, error) {
	willCompleteTrick := a.game.Trick.Count == hearts.NumPlayers-1

	if err := a.game.PlayCard(hearts.Seat(seat), card); err != nil {
		return session.StepResult{}, err
	}

	if willCompleteTrick {
		a.paused = &pauseState{trickComplete: true}
		return session.StepResult{
			Outcome: session.StepPause,
		}, nil
	}

	return session.StepResult{Outcome: session.StepContinue}, nil
}

// advanceTurn moves Turn to the next seat in cyclic order.
func (a *GameAdapter) advanceTurn() {
	a.game.Turn = (a.game.Turn + 1) % hearts.NumPlayers
}

// viewState builds the ViewState for snapshot generation, reflecting
// the current pause state.
func (a *GameAdapter) viewState() heartsview.ViewState {
	vs := heartsview.ViewState{
		Game:           a.game,
		DealPending:    a.dealPending,
		TurnDeadline:   a.turnDeadline,
		PreviousScores: a.previousScores,
	}
	if a.paused != nil {
		vs.TrickComplete = a.paused.trickComplete
		vs.RoundComplete = a.paused.roundComplete
	}
	vs.Paused = a.isPaused
	return vs
}

// engineErrToCommandError maps a Hearts engine error to a wire
// CommandError using errors.Is against the engine's typed sentinels.
// Known sentinels return their engine message to the client; unknown
// errors are masked as a generic "internal error" while the full
// engine error is logged server-side for debugging.
func (a *GameAdapter) engineErrToCommandError(err error) *session.CommandError {
	switch {
	case errors.Is(err, hearts.ErrWrongPhase):
		return &session.CommandError{
			Code:    api.ErrWrongPhase,
			Message: err.Error(),
		}
	case errors.Is(err, hearts.ErrOutOfTurn):
		return &session.CommandError{
			Code:    api.ErrOutOfTurn,
			Message: err.Error(),
		}
	case errors.Is(err, hearts.ErrIllegalMove):
		return &session.CommandError{
			Code:    api.ErrIllegalMove,
			Message: err.Error(),
		}
	default:
		a.logger.Error("internal engine error", "error", err)
		return &session.CommandError{
			Code:    api.ErrInternal,
			Message: "internal error",
		}
	}
}

// validateConfig checks game-specific constraints for a Hearts session.
// It validates that exactly four seats are configured and that every seat
// has a supported ai_type. For human seats an empty ai_type falls back to
// the default fallback AI ("random"). This matches the validation
// performed by NewGameAdapter without mutating engine state.
func validateConfig(cfg session.Config) error {
	if len(cfg.Seats) != hearts.NumPlayers {
		return fmt.Errorf(
			"%w: hearts requires %d seats, got %d",
			session.ErrInvalidConfig, hearts.NumPlayers, len(cfg.Seats),
		)
	}
	for i, sc := range cfg.Seats {
		aiType := sc.AIType
		if sc.Type != session.SeatAI && aiType == "" {
			aiType = defaultHumanAIType
		}
		if err := validateAIType(aiType); err != nil {
			return fmt.Errorf("seat %d: %w", i, err)
		}
	}
	return nil
}

// validateAIType returns an error if aiType is not a supported Hearts AI type.
func validateAIType(aiType string) error {
	switch aiType {
	case "random", "heuristic", "pimc":
		return nil
	default:
		return fmt.Errorf(
			"%w: unknown ai_type: %q", session.ErrInvalidConfig, aiType,
		)
	}
}

// newPlayer creates an AI player from the ai_type string.
func newPlayer(
	aiType string, rng *rand.Rand,
) (hearts.Player, error) {
	if err := validateAIType(aiType); err != nil {
		return nil, err
	}
	switch aiType {
	case "random":
		return ai.NewRandom(rng), nil
	case "heuristic":
		return ai.NewHeuristic(rng), nil
	case "pimc":
		rollout := func(r *rand.Rand) hearts.Player { return ai.NewRandom(r) }
		return ai.NewPIMC(rng, 100, rollout, runtime.GOMAXPROCS(0)), nil
	default:
		return nil, fmt.Errorf(
			"%w: unknown ai_type: %q", session.ErrInvalidConfig, aiType,
		)
	}
}
