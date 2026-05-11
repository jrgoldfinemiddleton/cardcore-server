package heartssession

import (
	"encoding/json"
	"math/rand/v2"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
	heartsapi "github.com/jrgoldfinemiddleton/cardcore-server/internal/api/games/hearts"
	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

var _ session.Game = (*Adapter)(nil)

// TestNewAdapterValid verifies that a valid 4-seat config creates an
// adapter in the pass or play phase.
func TestNewAdapterValid(t *testing.T) {
	a, err := NewAdapter(validSeats(), testRNG())
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	phase := a.game.Phase
	if phase != hearts.PhasePass && phase != hearts.PhasePlay {
		t.Errorf("got phase %d, want PhasePass or PhasePlay", phase)
	}
}

// TestNewAdapterWrongSeatCount verifies that non-4-seat configs are
// rejected.
func TestNewAdapterWrongSeatCount(t *testing.T) {
	seats := []session.SeatConfig{
		{Type: session.SeatHuman},
		{Type: session.SeatAI, AIType: "random"},
	}
	_, err := NewAdapter(seats, testRNG())
	if err == nil {
		t.Fatal("got nil error, want seat count error")
	}
}

// TestNewAdapterUnknownAIType verifies that an unknown ai_type is
// rejected.
func TestNewAdapterUnknownAIType(t *testing.T) {
	seats := []session.SeatConfig{
		{Type: session.SeatHuman},
		{Type: session.SeatAI, AIType: "unknown"},
		{Type: session.SeatAI, AIType: "random"},
		{Type: session.SeatAI, AIType: "random"},
	}
	_, err := NewAdapter(seats, testRNG())
	if err == nil {
		t.Fatal("got nil error, want unknown ai_type error")
	}
}

// TestHandlePlayCard verifies that a valid play_card action advances
// the game.
func TestHandlePlayCard(t *testing.T) {
	a := adapterInPlayPhase(t)
	seat := int(a.game.Turn)
	moves, err := a.game.LegalMoves(a.game.Turn)
	if err != nil {
		t.Fatalf("LegalMoves: %v", err)
	}

	msg := playCardMsg(t, heartsapi.CardFromEngine(moves[0]))
	res, cmdErr := a.HandleAction(seat, msg)
	if cmdErr != nil {
		t.Fatalf("got CommandError: %v", cmdErr)
	}
	if res.Outcome != session.StepContinue &&
		res.Outcome != session.StepPause {
		t.Errorf("got outcome %d, want StepContinue or StepPause",
			res.Outcome)
	}
}

// TestHandlePlayCardWrongPhase verifies that play_card during the pass
// phase returns a wrong_phase error.
func TestHandlePlayCardWrongPhase(t *testing.T) {
	a := adapterInPassPhase(t)
	msg := playCardMsg(t, heartsapi.Card{Rank: "two", Suit: "clubs"})
	_, cmdErr := a.HandleAction(0, msg)
	if cmdErr == nil {
		t.Fatal("got nil CommandError, want wrong_phase")
	}
	if got, want := cmdErr.Code, api.ErrWrongPhase; got != want {
		t.Errorf("got code %q, want %q", got, want)
	}
}

// TestHandlePlayCardOutOfTurn verifies that playing out of turn returns
// an out_of_turn error.
func TestHandlePlayCardOutOfTurn(t *testing.T) {
	a := adapterInPlayPhase(t)
	wrongSeat := (int(a.game.Turn) + 1) % hearts.NumPlayers

	// Use a card the wrong seat actually holds so the only possible
	// error is out_of_turn, not illegal_move.
	msg := playCardMsg(t,
		heartsapi.CardFromEngine(a.game.Hands[wrongSeat].Cards[0]),
	)
	_, cmdErr := a.HandleAction(wrongSeat, msg)
	if cmdErr == nil {
		t.Fatal("got nil CommandError, want out_of_turn")
	}
	if got, want := cmdErr.Code, api.ErrOutOfTurn; got != want {
		t.Errorf("got code %q, want %q", got, want)
	}
}

// TestHandlePlayCardIllegalMove verifies that an illegal play returns
// an illegal_move error.
func TestHandlePlayCardIllegalMove(t *testing.T) {
	a := adapterInPlayPhase(t)
	seat := int(a.game.Turn)

	// Find a card NOT in the player's hand by picking one from
	// a different seat's hand.
	otherSeat := (seat + 1) % hearts.NumPlayers
	absentCard := heartsapi.CardFromEngine(
		a.game.Hands[otherSeat].Cards[0],
	)
	msg := playCardMsg(t, absentCard)
	_, cmdErr := a.HandleAction(seat, msg)
	if cmdErr == nil {
		t.Fatal("got nil CommandError, want illegal_move")
	}
	if got, want := cmdErr.Code, api.ErrIllegalMove; got != want {
		t.Errorf("got code %q, want %q", got, want)
	}
}

// TestHandlePassCards verifies that a valid pass_cards action is
// accepted.
func TestHandlePassCards(t *testing.T) {
	a := adapterInPassPhase(t)
	cards := firstThreeCards(a, 0)
	msg := passCardsMsg(t, cards)
	res, cmdErr := a.HandleAction(0, msg)
	if cmdErr != nil {
		t.Fatalf("got CommandError: %v", cmdErr)
	}
	if res.Outcome != session.StepContinue {
		t.Errorf("got outcome %d, want StepContinue", res.Outcome)
	}
}

// TestHandlePassCardsWrongCount verifies that passing the wrong number
// of cards returns a malformed_message error.
func TestHandlePassCardsWrongCount(t *testing.T) {
	a := adapterInPassPhase(t)
	msg := passCardsMsg(t, []heartsapi.Card{
		{Rank: "two", Suit: "clubs"},
		{Rank: "three", Suit: "clubs"},
	})
	_, cmdErr := a.HandleAction(0, msg)
	if cmdErr == nil {
		t.Fatal("got nil CommandError, want malformed_message")
	}
	if got, want := cmdErr.Code, api.ErrMalformedMessage; got != want {
		t.Errorf("got code %q, want %q", got, want)
	}
}

// TestHandleActionUnknownType verifies that an unknown message type
// returns a malformed_message error.
func TestHandleActionUnknownType(t *testing.T) {
	a := adapterInPlayPhase(t)
	msg := &api.InboundMessage{Type: "unknown", Payload: []byte("{}")}
	_, cmdErr := a.HandleAction(0, msg)
	if cmdErr == nil {
		t.Fatal("got nil CommandError, want malformed_message")
	}
	if got, want := cmdErr.Code, api.ErrMalformedMessage; got != want {
		t.Errorf("got code %q, want %q", got, want)
	}
}

// TestHandleActionGameOver verifies that actions after game over return
// a game_over error.
func TestHandleActionGameOver(t *testing.T) {
	a := adapterInPlayPhase(t)
	a.game.Phase = hearts.PhaseEnd
	msg := playCardMsg(t, heartsapi.Card{Rank: "two", Suit: "clubs"})
	_, cmdErr := a.HandleAction(0, msg)
	if cmdErr == nil {
		t.Fatal("got nil CommandError, want game_over")
	}
	if got, want := cmdErr.Code, api.ErrGameOver; got != want {
		t.Errorf("got code %q, want %q", got, want)
	}
}

// TestAIPlay verifies that an AI seat can make a play without error
// and the adapter processes it (hand size decreases). Card legality is
// enforced by the engine, not verified here.
func TestAIPlay(t *testing.T) {
	a := allAIAdapter(t)
	advanceToPlayPhase(t, a)
	seat := int(a.game.Turn)
	handBefore := a.game.Hands[seat].Len()

	res, err := a.AIPlay(seat)
	if err != nil {
		t.Fatalf("AIPlay: %v", err)
	}
	if res.Outcome != session.StepContinue &&
		res.Outcome != session.StepPause {
		t.Errorf("got outcome %d, want StepContinue or StepPause",
			res.Outcome)
	}

	handAfter := a.game.Hands[seat].Len()
	if got, want := handAfter, handBefore-1; got != want {
		t.Errorf("hand size after play: got %d, want %d", got, want)
	}
}

// TestAIPlayPass verifies that an AI seat can pass cards.
func TestAIPlayPass(t *testing.T) {
	a := allAIAdapter(t)
	if a.game.Phase != hearts.PhasePass {
		t.Skip("deal produced hold round, no pass phase")
	}
	seat := 0
	res, err := a.AIPlay(seat)
	if err != nil {
		t.Fatalf("AIPlay pass: %v", err)
	}
	if res.Outcome != session.StepContinue {
		t.Errorf("got outcome %d, want StepContinue", res.Outcome)
	}
}

// TestAIPlayHumanSeat verifies that AIPlay on a human seat returns an
// error.
func TestAIPlayHumanSeat(t *testing.T) {
	a := adapterInPlayPhase(t)
	_, err := a.AIPlay(0) // seat 0 is human in validSeats()
	if err == nil {
		t.Fatal("got nil error, want human seat error")
	}
}

// TestResumeAfterTrickComplete verifies the pause-resume cycle after a
// trick completes.
func TestResumeAfterTrickComplete(t *testing.T) {
	a := allAIAdapter(t)
	advanceToPlayPhase(t, a)

	// Play a full trick (4 cards).
	var lastRes session.StepResult
	for range hearts.NumPlayers {
		seat := int(a.game.Turn)
		res, err := a.AIPlay(seat)
		if err != nil {
			t.Fatalf("AIPlay seat %d: %v", seat, err)
		}
		lastRes = res
	}

	if got, want := lastRes.Outcome, session.StepPause; got != want {
		t.Fatalf("after trick: got outcome %d, want StepPause", got)
	}

	res, err := a.Resume()
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	// After first trick, should continue or pause again (round
	// complete). Should not be finished.
	if res.Outcome == session.StepFinished {
		t.Error("got StepFinished after first trick resume")
	}
}

// TestResumeNotPaused verifies that Resume returns an error when not
// paused.
func TestResumeNotPaused(t *testing.T) {
	a := adapterInPlayPhase(t)
	_, err := a.Resume()
	if err == nil {
		t.Fatal("got nil error, want not-paused error")
	}
}

// TestPlayerSnapshot verifies that PlayerSnapshot returns a non-nil
// value with the correct type and seq.
func TestPlayerSnapshot(t *testing.T) {
	a := adapterInPlayPhase(t)
	snap := a.PlayerSnapshot(0, 42)
	ps, ok := snap.(*heartsapi.PlayerSnapshot)
	if !ok {
		t.Fatalf("got type %T, want *heartsapi.PlayerSnapshot", snap)
	}
	if got, want := ps.Seq, 42; got != want {
		t.Errorf("Seq: got %d, want %d", got, want)
	}
}

// TestObserverSnapshot verifies that ObserverSnapshot returns a
// non-nil value with the correct type and seq.
func TestObserverSnapshot(t *testing.T) {
	a := adapterInPlayPhase(t)
	snap := a.ObserverSnapshot(7)
	os, ok := snap.(*heartsapi.ObserverSnapshot)
	if !ok {
		t.Fatalf("got type %T, want *heartsapi.ObserverSnapshot", snap)
	}
	if got, want := os.Seq, 7; got != want {
		t.Errorf("Seq: got %d, want %d", got, want)
	}
	if got, want := len(os.Hands), hearts.NumPlayers; got != want {
		t.Errorf("Hands length: got %d, want %d", got, want)
	}
}

// TestSnapshotSerializable verifies that snapshots can be marshaled to
// JSON without error.
func TestSnapshotSerializable(t *testing.T) {
	a := adapterInPlayPhase(t)
	ps := a.PlayerSnapshot(0, 1)
	if _, err := json.Marshal(ps); err != nil {
		t.Errorf("PlayerSnapshot marshal: %v", err)
	}
	os := a.ObserverSnapshot(1)
	if _, err := json.Marshal(os); err != nil {
		t.Errorf("ObserverSnapshot marshal: %v", err)
	}
}

// TestTrickCompletePauseShowsCompletedTrick verifies that during a
// trick_complete pause, the snapshot phase is "trick_complete".
func TestTrickCompletePauseShowsCompletedTrick(t *testing.T) {
	a := allAIAdapter(t)
	advanceToPlayPhase(t, a)

	// Play a full trick.
	for range hearts.NumPlayers {
		seat := int(a.game.Turn)
		_, err := a.AIPlay(seat)
		if err != nil {
			t.Fatalf("AIPlay: %v", err)
		}
	}

	// Snapshot while paused shows trick_complete.
	ps := a.PlayerSnapshot(0, 1)
	snap, ok := ps.(*heartsapi.PlayerSnapshot)
	if !ok {
		t.Fatalf("got type %T, want *heartsapi.PlayerSnapshot", ps)
	}
	if got, want := snap.Phase, "trick_complete"; got != want {
		t.Errorf("phase during pause: got %q, want %q", got, want)
	}

	// Verify the adapter was actually paused: Resume should succeed.
	if _, err := a.Resume(); err != nil {
		t.Fatalf("Resume failed, adapter not paused: %v", err)
	}
}

// TestFullGameThroughAdapter plays a complete game through the adapter
// using all-AI players, verifying that it terminates with StepFinished.
func TestFullGameThroughAdapter(t *testing.T) {
	a := allAIAdapter(t)

	const maxSteps = 10000
	steps := 0
	for steps < maxSteps {
		steps++
		switch a.game.Phase {
		case hearts.PhasePass:
			for i := range hearts.NumPlayers {
				if a.game.Phase != hearts.PhasePass {
					break
				}
				_, err := a.AIPlay(i)
				if err != nil {
					t.Fatalf("step %d: AIPlay pass seat %d: %v",
						steps, i, err)
				}
			}
		case hearts.PhasePlay:
			seat := int(a.game.Turn)
			res, err := a.AIPlay(seat)
			if err != nil {
				t.Fatalf("step %d: AIPlay seat %d: %v",
					steps, seat, err)
			}
			if res.Outcome == session.StepPause {
				for {
					r, rErr := a.Resume()
					if rErr != nil {
						t.Fatalf("step %d: Resume: %v",
							steps, rErr)
					}
					if r.Outcome == session.StepFinished {
						return
					}
					if r.Outcome != session.StepPause {
						break
					}
				}
			}
			if res.Outcome == session.StepFinished {
				return
			}
		case hearts.PhaseEnd:
			t.Fatal("reached PhaseEnd without StepFinished")
		default:
			t.Fatalf("step %d: unexpected phase %d",
				steps, a.game.Phase)
		}
	}
	t.Fatalf("game did not finish within %d steps", maxSteps)
}

// playCardMsg builds an InboundMessage for a play_card action.
func playCardMsg(t *testing.T, card heartsapi.Card) *api.InboundMessage {
	t.Helper()
	payload, err := json.Marshal(heartsapi.PlayCardPayload{Card: card})
	if err != nil {
		t.Fatalf("marshal play_card payload: %v", err)
	}
	return &api.InboundMessage{
		Type:    "play_card",
		Payload: payload,
	}
}

// passCardsMsg builds an InboundMessage for a pass_cards action.
func passCardsMsg(
	t *testing.T, cards []heartsapi.Card,
) *api.InboundMessage {
	t.Helper()
	payload, err := json.Marshal(heartsapi.PassCardsPayload{Cards: cards})
	if err != nil {
		t.Fatalf("marshal pass_cards payload: %v", err)
	}
	return &api.InboundMessage{
		Type:    "pass_cards",
		Payload: payload,
	}
}

// firstThreeCards returns the first three cards from a seat's hand as
// wire-format cards.
func firstThreeCards(a *Adapter, seat int) []heartsapi.Card {
	h := a.game.Hands[seat]
	return []heartsapi.Card{
		heartsapi.CardFromEngine(h.Cards[0]),
		heartsapi.CardFromEngine(h.Cards[1]),
		heartsapi.CardFromEngine(h.Cards[2]),
	}
}

// validSeats returns a standard 1-human + 3-AI seat configuration.
func validSeats() []session.SeatConfig {
	return []session.SeatConfig{
		{Type: session.SeatHuman},
		{Type: session.SeatAI, AIType: "random"},
		{Type: session.SeatAI, AIType: "random"},
		{Type: session.SeatAI, AIType: "random"},
	}
}

// allAISeats returns a 4-AI seat configuration.
func allAISeats() []session.SeatConfig {
	return []session.SeatConfig{
		{Type: session.SeatAI, AIType: "random"},
		{Type: session.SeatAI, AIType: "random"},
		{Type: session.SeatAI, AIType: "random"},
		{Type: session.SeatAI, AIType: "random"},
	}
}

// testRNG returns a deterministic RNG for reproducible tests.
func testRNG() *rand.Rand {
	return rand.New(rand.NewPCG(42, 43))
}

// allAIAdapter creates an adapter with 4 AI players.
func allAIAdapter(t *testing.T) *Adapter {
	t.Helper()
	a, err := NewAdapter(allAISeats(), testRNG())
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	return a
}

// adapterInPlayPhase creates a 1-human + 3-AI adapter and advances
// past the pass phase using AI players for AI seats and manual passes
// for the human seat.
func adapterInPlayPhase(t *testing.T) *Adapter {
	t.Helper()
	a, err := NewAdapter(validSeats(), testRNG())
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	advanceToPlayPhase(t, a)
	return a
}

// adapterInPassPhase creates a 1-human + 3-AI adapter that is in the
// pass phase. Skips the test if the deal produced a hold round.
func adapterInPassPhase(t *testing.T) *Adapter {
	t.Helper()
	a, err := NewAdapter(validSeats(), testRNG())
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if a.game.Phase != hearts.PhasePass {
		t.Skip("deal produced hold round, no pass phase")
	}
	return a
}

// advanceToPlayPhase advances the adapter past the pass phase. For
// human seats, it submits the first three cards. For AI seats, it
// calls AIPlay.
func advanceToPlayPhase(t *testing.T, a *Adapter) {
	t.Helper()
	if a.game.Phase == hearts.PhasePlay {
		return
	}
	if a.game.Phase != hearts.PhasePass {
		t.Fatalf("expected PhasePass, got %d", a.game.Phase)
	}
	for i := range hearts.NumPlayers {
		if a.game.Phase != hearts.PhasePass {
			break
		}
		if a.players[i] != nil {
			if _, err := a.AIPlay(i); err != nil {
				t.Fatalf("AIPlay pass seat %d: %v", i, err)
			}
		} else {
			cards := firstThreeCards(a, i)
			msg := passCardsMsg(t, cards)
			if _, cmdErr := a.HandleAction(i, msg); cmdErr != nil {
				t.Fatalf("pass seat %d: %v", i, cmdErr)
			}
		}
	}
	if a.game.Phase != hearts.PhasePlay {
		t.Fatalf("after passing: got phase %d, want PhasePlay",
			a.game.Phase)
	}
}
