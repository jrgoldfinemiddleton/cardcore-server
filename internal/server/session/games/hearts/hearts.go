package heartssession

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/jrgoldfinemiddleton/cardcore"
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts/ai"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
	heartsapi "github.com/jrgoldfinemiddleton/cardcore-server/internal/api/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
	heartsview "github.com/jrgoldfinemiddleton/cardcore-server/internal/server/view/hearts"
)

// Adapter implements [session.Game] for Hearts.
type Adapter struct {
	game    *hearts.Game
	players [hearts.NumPlayers]hearts.Player

	// paused tracks which UX pause is active. Nil when not paused.
	paused *pauseState
}

// pauseState captures the adapter state during a UX pause.
type pauseState struct {
	trickComplete bool
	roundComplete bool
}

// NewAdapter creates a Hearts game adapter. It validates the seat
// configuration, creates AI players for AI seats, and deals the first
// hand.
func NewAdapter(
	seats []session.SeatConfig, rng *rand.Rand,
) (*Adapter, error) {
	if len(seats) != hearts.NumPlayers {
		return nil, fmt.Errorf(
			"%w: hearts requires %d seats, got %d",
			session.ErrInvalidConfig, hearts.NumPlayers, len(seats),
		)
	}

	a := &Adapter{}
	for i, sc := range seats {
		if sc.Type != session.SeatAI {
			continue
		}
		p, err := newPlayer(sc.AIType, rng)
		if err != nil {
			return nil, fmt.Errorf("seat %d: %w", i, err)
		}
		a.players[i] = p
	}

	a.game = hearts.New()
	if err := a.game.Deal(); err != nil {
		return nil, fmt.Errorf("initial deal: %w", err)
	}

	return a, nil
}

// HandleAction processes an inbound player action. It validates turn
// order, phase, and legality, returning a CommandError for rejected
// actions.
func (a *Adapter) HandleAction(
	seat int, msg *api.InboundMessage,
) (session.StepResult, *session.CommandError) {
	if a.game.Phase == hearts.PhaseEnd {
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
		return session.StepResult{},
			&session.CommandError{
				Code: api.ErrMalformedMessage,
				Message: fmt.Sprintf(
					"unknown message type: %q", msg.Type,
				),
			}
	}
}

// AIPlay executes the AI player's move for the given seat.
func (a *Adapter) AIPlay(seat int) (session.StepResult, error) {
	s := hearts.Seat(seat)
	p := a.players[seat]
	if p == nil {
		return session.StepResult{},
			fmt.Errorf("seat %d is not an AI seat", seat)
	}

	switch a.game.Phase {
	case hearts.PhasePass:
		cards := p.ChoosePass(a.game, s)
		if err := a.game.SetPass(s, cards); err != nil {
			return session.StepResult{},
				fmt.Errorf("AI pass seat %d: %w", seat, err)
		}
		return session.StepResult{Outcome: session.StepContinue}, nil
	case hearts.PhasePlay:
		return a.playCard(seat, p.ChoosePlay(a.game, s))
	default:
		return session.StepResult{},
			fmt.Errorf(
				"AI cannot act in phase %q",
				heartsapi.PhaseToWire(a.game.Phase),
			)
	}
}

// Resume advances the game past a pausable state. Only valid when the
// adapter is paused after returning StepPause.
func (a *Adapter) Resume() (session.StepResult, error) {
	if a.paused == nil {
		return session.StepResult{},
			errors.New("Resume called when not paused")
	}

	if a.paused.trickComplete {
		a.paused = nil
		// The engine already resolved the trick during PlayCard.
		// Check if the round ended (PhaseScore).
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
		// New round: deal and continue.
		if err := a.game.Deal(); err != nil {
			return session.StepResult{},
				fmt.Errorf("deal: %w", err)
		}
		return session.StepResult{
			Outcome: session.StepContinue,
		}, nil
	}

	return session.StepResult{}, errors.New("unknown pause state")
}

// Turn returns the seat index whose turn it is.
func (a *Adapter) Turn() int {
	return int(a.game.Turn)
}

// PlayerSnapshot builds a seat-filtered snapshot for the given player.
func (a *Adapter) PlayerSnapshot(seat int, seq int) any {
	return heartsview.PlayerView(a.viewState(), hearts.Seat(seat), seq)
}

// ObserverSnapshot builds a full-information snapshot.
func (a *Adapter) ObserverSnapshot(seq int) any {
	return heartsview.ObserverView(a.viewState(), seq)
}

// handlePlayCard processes a play_card action.
func (a *Adapter) handlePlayCard(
	seat int, payload json.RawMessage,
) (session.StepResult, *session.CommandError) {
	s := hearts.Seat(seat)

	if a.game.Phase != hearts.PhasePlay {
		return session.StepResult{},
			&session.CommandError{
				Code:    api.ErrWrongPhase,
				Message: "cannot play a card during this phase",
			}
	}
	if s != a.game.Turn {
		return session.StepResult{},
			&session.CommandError{
				Code: api.ErrOutOfTurn,
				Message: fmt.Sprintf(
					"not your turn (current: seat %d)", a.game.Turn,
				),
			}
	}

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
		return session.StepResult{},
			&session.CommandError{
				Code:    api.ErrIllegalMove,
				Message: playErr.Error(),
			}
	}
	return res, nil
}

// handlePassCards processes a pass_cards action.
func (a *Adapter) handlePassCards(
	seat int, payload json.RawMessage,
) (session.StepResult, *session.CommandError) {
	s := hearts.Seat(seat)

	if a.game.Phase != hearts.PhasePass {
		return session.StepResult{},
			&session.CommandError{
				Code:    api.ErrWrongPhase,
				Message: "cannot pass cards during this phase",
			}
	}

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

	if err := a.game.SetPass(s, cards); err != nil {
		return session.StepResult{},
			&session.CommandError{
				Code:    api.ErrIllegalMove,
				Message: err.Error(),
			}
	}

	return session.StepResult{Outcome: session.StepContinue}, nil
}

// playCard applies a card play and determines the step outcome. If the
// play completes a trick, the adapter enters a paused state.
func (a *Adapter) playCard(
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

// viewState builds the ViewState for snapshot generation, reflecting
// the current pause state.
func (a *Adapter) viewState() heartsview.ViewState {
	vs := heartsview.ViewState{Game: a.game}
	if a.paused != nil {
		vs.TrickComplete = a.paused.trickComplete
		vs.RoundComplete = a.paused.roundComplete
	}
	return vs
}

// newPlayer creates an AI player from the ai_type string.
func newPlayer(
	aiType string, rng *rand.Rand,
) (hearts.Player, error) {
	switch aiType {
	case "random":
		return ai.NewRandom(rng), nil
	case "heuristic":
		return ai.NewHeuristic(rng), nil
	default:
		return nil, fmt.Errorf(
			"%w: unknown ai_type: %q", session.ErrInvalidConfig, aiType,
		)
	}
}
