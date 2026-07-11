package heartsview

import (
	"math/rand/v2"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore"
	"github.com/jrgoldfinemiddleton/cardcore/games/hearts"

	heartsapi "github.com/jrgoldfinemiddleton/cardcore-server/internal/api/games/hearts"
)

// play records a single seat/card pair for test setup.
type play struct {
	seat hearts.Seat
	card cardcore.Card
}

// TestPlayerViewMasking verifies that PlayerView exposes only the requesting
// seat's own hand, with no card appearing in two different seats' snapshots.
func TestPlayerViewMasking(t *testing.T) {
	g := dealGame(t)
	vs := ViewState{Game: g}

	seen := map[heartsapi.Card]bool{}
	for i := range hearts.NumPlayers {
		seat := hearts.Seat(i)
		snap := PlayerView(vs, seat, 0)

		if got, want := len(snap.Hand), g.Hands[seat].Len(); got != want {
			t.Errorf("seat %d hand len: got %d, want %d", seat, got, want)
		}

		for _, card := range snap.Hand {
			if seen[card] {
				t.Errorf("seat %d: card %v already seen in another hand", seat, card)
			}
			seen[card] = true
		}

		for _, card := range snap.Hand {
			ec, err := heartsapi.CardToEngine(card)
			if err != nil {
				t.Fatalf("seat %d: invalid card %v: %v", seat, card, err)
			}
			if !g.Hands[seat].Contains(ec) {
				t.Errorf("seat %d: card %v in snapshot but not in engine hand", seat, card)
			}
		}
	}

	if got, want := len(seen), 52; got != want {
		t.Errorf("unique cards across all hands: got %d, want %d", got, want)
	}
}

// TestPlayerViewHandSorted verifies that the hand in a PlayerSnapshot is sorted
// by suit then rank in ascending order.
func TestPlayerViewHandSorted(t *testing.T) {
	g := dealGame(t)
	vs := ViewState{Game: g}
	snap := PlayerView(vs, 0, 0)

	for i := 1; i < len(snap.Hand); i++ {
		pc, err1 := heartsapi.CardToEngine(snap.Hand[i-1])
		cc, err2 := heartsapi.CardToEngine(snap.Hand[i])
		if err1 != nil || err2 != nil {
			t.Fatalf("invalid card at index %d or %d", i-1, i)
		}
		if pc.Suit > cc.Suit || (pc.Suit == cc.Suit && pc.Rank > cc.Rank) {
			t.Errorf("hand not sorted at index %d: %v before %v", i, snap.Hand[i-1], snap.Hand[i])
		}
	}
}

// TestPlayerViewScores verifies that scores and round_points map correctly
// from the engine arrays.
func TestPlayerViewScores(t *testing.T) {
	g := hearts.New(rand.New(rand.NewPCG(1, 2)))
	g.Scores = [hearts.NumPlayers]int{10, 3, 0, 13}
	g.RoundPts = [hearts.NumPlayers]int{5, 8, 13, 0}
	vs := ViewState{Game: g}
	snap := PlayerView(vs, 0, 0)

	wantScores := []int{10, 3, 0, 13}
	wantRoundPts := []int{5, 8, 13, 0}

	for i, want := range wantScores {
		if i >= len(snap.Scores) {
			t.Fatalf("snap.Scores too short at index %d", i)
		}
		if got := snap.Scores[i]; got != want {
			t.Errorf("Scores[%d]: got %d, want %d", i, got, want)
		}
	}
	for i, want := range wantRoundPts {
		if i >= len(snap.RoundPoints) {
			t.Fatalf("snap.RoundPoints too short at index %d", i)
		}
		if got := snap.RoundPoints[i]; got != want {
			t.Errorf("RoundPoints[%d]: got %d, want %d", i, got, want)
		}
	}
}

// TestPlayerViewTrickPlayOrder verifies that trick entries in a PlayerSnapshot are
// ordered from the trick leader, not by seat number.
func TestPlayerViewTrickPlayOrder(t *testing.T) {
	g := newGameInPlayPhase(t)
	vs := ViewState{Game: g}

	leader := g.Turn
	twoOfClubs := cardcore.Card{Rank: cardcore.Two, Suit: cardcore.Clubs}
	if err := g.PlayCard(leader, twoOfClubs); err != nil {
		t.Fatalf("PlayCard 2♣: %v", err)
	}

	nextSeat := g.Turn
	moves, err := g.LegalMoves(nextSeat)
	if err != nil || len(moves) == 0 {
		t.Fatal("no legal moves for second player")
	}
	if err := g.PlayCard(nextSeat, moves[0]); err != nil {
		t.Fatalf("PlayCard second card: %v", err)
	}

	snap := PlayerView(vs, leader, 0)

	if got, want := len(snap.Trick), 2; got != want {
		t.Fatalf("trick entry count: got %d, want %d", got, want)
	}
	if got, want := snap.Trick[0].Seat, int(leader); got != want {
		t.Errorf("trick[0].Seat: got %d, want %d", got, want)
	}
	if got, want := snap.Trick[1].Seat, int(nextSeat); got != want {
		t.Errorf("trick[1].Seat: got %d, want %d", got, want)
	}
}

// TestPlayerViewOneIndexed verifies that a fresh game produces 1-indexed
// round and trick numbers on the wire.
func TestPlayerViewOneIndexed(t *testing.T) {
	g := newGameInPlayPhase(t)
	vs := ViewState{Game: g}
	snap := PlayerView(vs, 0, 0)

	if got, want := snap.RoundNumber, 1; got != want {
		t.Errorf("RoundNumber: got %d, want %d", got, want)
	}
	if got, want := snap.TrickNumber, 1; got != want {
		t.Errorf("TrickNumber: got %d, want %d", got, want)
	}
}

// TestPlayerViewPhasePriority verifies the override hierarchy:
// TrickComplete > RoundComplete > engine phase.
func TestPlayerViewPhasePriority(t *testing.T) {
	phases := []hearts.Phase{hearts.PhaseDeal, hearts.PhaseScore}
	for _, phase := range phases {
		g := hearts.New(rand.New(rand.NewPCG(1, 2)))
		g.Phase = phase

		// Engine phase wins when no overrides are set.
		vs := ViewState{Game: g}
		snap := PlayerView(vs, 0, 0)
		if got, want := snap.Phase, heartsapi.PhaseToWire(phase); got != want {
			t.Errorf("phase %v, no overrides: got %q, want %q", phase, got, want)
		}

		// RoundComplete overrides engine phase.
		vs = ViewState{Game: g, RoundComplete: true}
		snap = PlayerView(vs, 0, 0)
		if got, want := snap.Phase, "round_complete"; got != want {
			t.Errorf("phase %v, RoundComplete: got %q, want %q", phase, got, want)
		}

		// TrickComplete overrides RoundComplete.
		vs = ViewState{Game: g, TrickComplete: true, RoundComplete: true}
		snap = PlayerView(vs, 0, 0)
		if got, want := snap.Phase, "trick_complete"; got != want {
			t.Errorf("phase %v, both overrides: got %q, want %q", phase, got, want)
		}
	}
}

// TestObserverViewAllHands verifies that an ObserverSnapshot exposes all four
// hands with the correct cards in sorted order.
func TestObserverViewAllHands(t *testing.T) {
	g := dealGame(t)
	vs := ViewState{Game: g}
	snap := ObserverView(vs, 0)

	if got, want := len(snap.Hands), hearts.NumPlayers; got != want {
		t.Fatalf("Hands length: got %d, want %d", got, want)
	}
	for i := range hearts.NumPlayers {
		wantCards := engineHandToWire(g.Hands[i])
		if got, want := len(snap.Hands[i]), len(wantCards); got != want {
			t.Fatalf("Hands[%d] length: got %d, want %d", i, got, want)
		}
		for j, want := range wantCards {
			if got := snap.Hands[i][j]; got != want {
				t.Errorf("Hands[%d][%d]: got %v, want %v", i, j, got, want)
			}
		}
	}
}

// TestPlayerViewTrickCompleteUsesHistory verifies that when the server sets
// TrickComplete, the snapshot renders the completed trick from TrickHistory
// instead of the empty g.Trick that the engine has already reset.
func TestPlayerViewTrickCompleteUsesHistory(t *testing.T) {
	g := newGameInPlayPhase(t)

	plays := make([]play, 0, hearts.NumPlayers)
	twoOfClubs := cardcore.Card{Rank: cardcore.Two, Suit: cardcore.Clubs}
	leader := g.Turn
	plays = append(plays, play{seat: leader, card: twoOfClubs})
	if err := g.PlayCard(leader, twoOfClubs); err != nil {
		t.Fatalf("PlayCard 2♣: %v", err)
	}
	for range hearts.NumPlayers - 1 {
		seat := g.Turn
		moves, err := g.LegalMoves(seat)
		if err != nil || len(moves) == 0 {
			t.Fatal("no legal moves")
		}
		plays = append(plays, play{seat: seat, card: moves[0]})
		if err := g.PlayCard(seat, moves[0]); err != nil {
			t.Fatalf("PlayCard: %v", err)
		}
	}

	vs := ViewState{Game: g, TrickComplete: true}
	snap := PlayerView(vs, leader, 0)

	if got, want := len(snap.Trick), hearts.NumPlayers; got != want {
		t.Fatalf("trick entry count: got %d, want %d", got, want)
	}
	for i, p := range plays {
		if got, want := snap.Trick[i].Seat, int(p.seat); got != want {
			t.Errorf("Trick[%d].Seat: got %d, want %d", i, got, want)
		}
		wantCard := heartsapi.CardFromEngine(p.card)
		if got := snap.Trick[i].Card; got != wantCard {
			t.Errorf("Trick[%d].Card: got %v, want %v", i, got, wantCard)
		}
	}
}

// TestObserverViewTrickCompleteUsesHistory verifies the observer view also uses
// the last TrickHistory entry when the server flags a completed trick.
func TestObserverViewTrickCompleteUsesHistory(t *testing.T) {
	g := newGameInPlayPhase(t)

	plays := make([]play, 0, hearts.NumPlayers)
	twoOfClubs := cardcore.Card{Rank: cardcore.Two, Suit: cardcore.Clubs}
	leader := g.Turn
	plays = append(plays, play{seat: leader, card: twoOfClubs})
	if err := g.PlayCard(leader, twoOfClubs); err != nil {
		t.Fatalf("PlayCard 2♣: %v", err)
	}
	for range hearts.NumPlayers - 1 {
		seat := g.Turn
		moves, err := g.LegalMoves(seat)
		if err != nil || len(moves) == 0 {
			t.Fatal("no legal moves")
		}
		plays = append(plays, play{seat: seat, card: moves[0]})
		if err := g.PlayCard(seat, moves[0]); err != nil {
			t.Fatalf("PlayCard: %v", err)
		}
	}

	vs := ViewState{Game: g, TrickComplete: true}
	snap := ObserverView(vs, 0)

	if got, want := len(snap.Trick), hearts.NumPlayers; got != want {
		t.Fatalf("trick entry count: got %d, want %d", got, want)
	}
	for i, p := range plays {
		if got, want := snap.Trick[i].Seat, int(p.seat); got != want {
			t.Errorf("Trick[%d].Seat: got %d, want %d", i, got, want)
		}
		wantCard := heartsapi.CardFromEngine(p.card)
		if got := snap.Trick[i].Card; got != wantCard {
			t.Errorf("Trick[%d].Card: got %v, want %v", i, got, wantCard)
		}
	}
}

// TestObserverViewTrickHistory verifies that after completing a trick the
// trick_history in an ObserverSnapshot contains the correct seats and cards
// in leader order.
func TestObserverViewTrickHistory(t *testing.T) {
	g := newGameInPlayPhase(t)
	vs := ViewState{Game: g}

	plays := make([]play, 0, hearts.NumPlayers)

	twoOfClubs := cardcore.Card{Rank: cardcore.Two, Suit: cardcore.Clubs}
	leader := g.Turn
	plays = append(plays, play{seat: leader, card: twoOfClubs})
	if err := g.PlayCard(leader, twoOfClubs); err != nil {
		t.Fatalf("PlayCard 2♣: %v", err)
	}
	for range hearts.NumPlayers - 1 {
		seat := g.Turn
		moves, err := g.LegalMoves(seat)
		if err != nil || len(moves) == 0 {
			t.Fatal("no legal moves")
		}
		plays = append(plays, play{seat: seat, card: moves[0]})
		if err := g.PlayCard(seat, moves[0]); err != nil {
			t.Fatalf("PlayCard: %v", err)
		}
	}

	snap := ObserverView(vs, 0)

	if got, want := len(snap.TrickHistory), 1; got != want {
		t.Fatalf("TrickHistory length: got %d, want %d", got, want)
	}
	trick := snap.TrickHistory[0]
	if got, want := len(trick), hearts.NumPlayers; got != want {
		t.Fatalf("TrickHistory[0] entry count: got %d, want %d", got, want)
	}
	for i, p := range plays {
		if got, want := trick[i].Seat, int(p.seat); got != want {
			t.Errorf("TrickHistory[0][%d].Seat: got %d, want %d", i, got, want)
		}
		wantCard := heartsapi.CardFromEngine(p.card)
		if got := trick[i].Card; got != wantCard {
			t.Errorf("TrickHistory[0][%d].Card: got %v, want %v", i, got, wantCard)
		}
	}
}

// TestLegalActions verifies legal action content across view types and game
// states: pass phase (full hand), play phase active seat (engine legal moves),
// and play phase inactive seat (empty).
func TestLegalActions(t *testing.T) {
	// Pass phase: every seat's player view gets their full hand as legal actions.
	g := dealGame(t)
	vs := ViewState{Game: g}
	for i := range hearts.NumPlayers {
		seat := hearts.Seat(i)
		snap := PlayerView(vs, seat, 0)
		wantCards := engineHandToWire(g.Hands[seat])
		if got, want := len(snap.LegalActions), len(wantCards); got != want {
			t.Fatalf("pass phase seat %d: legal action count: got %d, want %d",
				seat, got, want)
		}
		for j, want := range wantCards {
			if got := snap.LegalActions[j]; got != want {
				t.Errorf("pass phase seat %d: LegalActions[%d]: got %v, want %v",
					seat, j, got, want)
			}
		}
	}

	// Pass phase: observer sees active seat's full hand as legal actions.
	oSnap := ObserverView(vs, 0)
	wantObsCards := engineHandToWire(g.Hands[g.Turn])
	if got, want := len(oSnap.LegalActions), len(wantObsCards); got != want {
		t.Fatalf("pass phase observer: legal action count: got %d, want %d",
			got, want)
	}
	for j, want := range wantObsCards {
		if got := oSnap.LegalActions[j]; got != want {
			t.Errorf("pass phase observer: LegalActions[%d]: got %v, want %v",
				j, got, want)
		}
	}

	// Play phase: active seat gets engine legal moves.
	g = newGameInPlayPhase(t)
	vs = ViewState{Game: g}
	activeSeat := g.Turn
	moves, err := g.LegalMoves(activeSeat)
	if err != nil {
		t.Fatalf("LegalMoves: %v", err)
	}
	wantMoves := make([]heartsapi.Card, len(moves))
	for i, c := range moves {
		wantMoves[i] = heartsapi.CardFromEngine(c)
	}

	snap := PlayerView(vs, activeSeat, 0)
	if got, want := len(snap.LegalActions), len(wantMoves); got != want {
		t.Fatalf("play phase active seat: legal action count: got %d, want %d",
			got, want)
	}
	for j, want := range wantMoves {
		if got := snap.LegalActions[j]; got != want {
			t.Errorf("play phase active seat: LegalActions[%d]: got %v, want %v",
				j, got, want)
		}
	}

	// Play phase: observer sees the same legal moves as the active seat.
	oSnap = ObserverView(vs, 0)
	if got, want := len(oSnap.LegalActions), len(wantMoves); got != want {
		t.Fatalf("play phase observer: legal action count: got %d, want %d",
			got, want)
	}
	for j, want := range wantMoves {
		if got := oSnap.LegalActions[j]; got != want {
			t.Errorf("play phase observer: LegalActions[%d]: got %v, want %v",
				j, got, want)
		}
	}

	// Play phase: inactive seat gets empty legal actions.
	inactiveSeat := hearts.Seat((int(activeSeat) + 1) % hearts.NumPlayers)
	snap = PlayerView(vs, inactiveSeat, 0)
	if snap.LegalActions == nil {
		t.Fatal("play phase inactive seat: got nil legal_actions, want empty slice")
	}
	if got := len(snap.LegalActions); got != 0 {
		t.Errorf("play phase inactive seat: legal action count: got %d, want 0",
			got)
	}
}

// dealGame creates and deals a new Hearts game, returning it in the pass phase.
func dealGame(t *testing.T) *hearts.Game {
	t.Helper()
	g := hearts.New(rand.New(rand.NewPCG(1, 2)))
	if err := g.Deal(); err != nil {
		t.Fatalf("Deal: %v", err)
	}
	return g
}

// passAllSeats submits the first three cards from each seat's hand as their pass.
func passAllSeats(t *testing.T, g *hearts.Game) {
	t.Helper()
	for i := range hearts.NumPlayers {
		seat := hearts.Seat(i)
		pass := [hearts.PassCount]cardcore.Card{
			g.Hands[seat].Cards[0],
			g.Hands[seat].Cards[1],
			g.Hands[seat].Cards[2],
		}
		if err := g.SetPass(seat, pass); err != nil {
			t.Fatalf("SetPass seat %d: %v", seat, err)
		}
	}
}

// newGameInPlayPhase deals a game and completes all passes, returning the game
// in the play phase.
func newGameInPlayPhase(t *testing.T) *hearts.Game {
	t.Helper()
	g := dealGame(t)
	passAllSeats(t, g)
	return g
}

// engineHandToWire converts a sorted engine hand to a slice of wire-format cards.
func engineHandToWire(h *cardcore.Hand) []heartsapi.Card {
	h.Sort()
	cards := make([]heartsapi.Card, 0, h.Len())
	for _, c := range h.Cards {
		cards = append(cards, heartsapi.CardFromEngine(c))
	}
	return cards
}
