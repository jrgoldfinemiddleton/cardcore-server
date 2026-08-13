package heartscli

import (
	"testing"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/games/hearts"
)

// TestFormatCards verifies slice formatting including empty input.
func TestFormatCards(t *testing.T) {
	cards := []heartsclient.Card{
		{Rank: "two", Suit: "clubs"},
		{Rank: "ace", Suit: "hearts"},
	}
	want := "[2♣ A♥]"
	got := formatCards(cards)
	if got != want {
		t.Errorf("formatCards(...) got %q, want %q", got, want)
	}

	empty := formatCards(nil)
	if empty != "[]" {
		t.Errorf("formatCards(nil) got %q, want %q", empty, "[]")
	}
}

// TestFormatTrick verifies trick entry slice formatting.
func TestFormatTrick(t *testing.T) {
	trick := []heartsclient.TrickEntry{
		{Seat: 0, Card: heartsclient.Card{Rank: "two", Suit: "clubs"}},
		{Seat: 1, Card: heartsclient.Card{Rank: "seven", Suit: "hearts"}},
	}
	want := "[2♣ 7♥]"
	got := formatTrick(trick)
	if got != want {
		t.Errorf("formatTrick(...) got %q, want %q", got, want)
	}

	empty := formatTrick(nil)
	if empty != "[]" {
		t.Errorf("formatTrick(nil) got %q, want %q", empty, "[]")
	}
}

// TestFormatSnapshot verifies compact notation for player, observer, and
// terminal snapshots. Every snapshot below is a complete wire message as the
// server sends it: all fields are always present, hand sizes match the number
// of cards played for the phase, legal actions follow Hearts rules, and
// scores sum to the penalty points actually awarded across completed rounds
// (26 per round, or 78 in a round where the moon was shot).
func TestFormatSnapshot(t *testing.T) {
	f := NewFormatter()

	tests := []struct {
		name     string
		snapshot string
		want     string
	}{
		{
			// Round 1, first actionable snapshot: seat 0 holds a freshly
			// passed hand of 13 and may pass any of them.
			name: "player passing snapshot",
			snapshot: `{"type":"snapshot","seq":2,"phase":"passing",` +
				`"round_number":1,"trick_number":1,"pass_direction":"left",` +
				`"turn":0,"trick_winner":-1,"hearts_broken":false,` +
				`"hand":[{"rank":"two","suit":"clubs"},` +
				`{"rank":"five","suit":"clubs"},{"rank":"nine","suit":"clubs"},` +
				`{"rank":"jack","suit":"clubs"},{"rank":"three","suit":"diamonds"},` +
				`{"rank":"eight","suit":"diamonds"},` +
				`{"rank":"queen","suit":"diamonds"},` +
				`{"rank":"four","suit":"hearts"},{"rank":"seven","suit":"hearts"},` +
				`{"rank":"king","suit":"hearts"},{"rank":"six","suit":"spades"},` +
				`{"rank":"ten","suit":"spades"},{"rank":"ace","suit":"spades"}],` +
				`"hand_counts":[13,13,13,13],"trick":[],"scores":[0,0,0,0],` +
				`"round_points":[0,0,0,0],` +
				`"legal_actions":[{"rank":"two","suit":"clubs"},` +
				`{"rank":"five","suit":"clubs"},{"rank":"nine","suit":"clubs"},` +
				`{"rank":"jack","suit":"clubs"},{"rank":"three","suit":"diamonds"},` +
				`{"rank":"eight","suit":"diamonds"},` +
				`{"rank":"queen","suit":"diamonds"},` +
				`{"rank":"four","suit":"hearts"},{"rank":"seven","suit":"hearts"},` +
				`{"rank":"king","suit":"hearts"},{"rank":"six","suit":"spades"},` +
				`{"rank":"ten","suit":"spades"},{"rank":"ace","suit":"spades"}],` +
				`"turn_deadline_ms":1728451200000,"paused":false}`,
			want: "seq=2 phase=passing turn=0 round=1 trick_num=1" +
				" hand=[2♣ 5♣ 9♣ J♣ 3♦ 8♦ Q♦ 4♥ 7♥ K♥ 6♠ 10♠ A♠]" +
				" legal=[2♣ 5♣ 9♣ J♣ 3♦ 8♦ Q♦ 4♥ 7♥ K♥ 6♠ 10♠ A♠]" +
				" round_points=[0 0 0 0] scores=[0 0 0 0]",
		},
		{
			// Round 4 is a hold round (pass_direction "none"), so play
			// begins immediately after the deal: seat 0 held 2♣ and led
			// it, and seat 1 must follow suit with a club.
			name: "player playing snapshot with trick",
			snapshot: `{"type":"snapshot","seq":216,"phase":"playing",` +
				`"round_number":4,"trick_number":1,"pass_direction":"none",` +
				`"turn":1,"trick_winner":-1,"hearts_broken":false,` +
				`"hand":[{"rank":"three","suit":"clubs"},` +
				`{"rank":"seven","suit":"clubs"},{"rank":"jack","suit":"clubs"},` +
				`{"rank":"five","suit":"diamonds"},` +
				`{"rank":"nine","suit":"diamonds"},{"rank":"king","suit":"diamonds"},` +
				`{"rank":"ace","suit":"diamonds"},{"rank":"six","suit":"hearts"},` +
				`{"rank":"eight","suit":"hearts"},{"rank":"queen","suit":"hearts"},` +
				`{"rank":"four","suit":"spades"},{"rank":"seven","suit":"spades"},` +
				`{"rank":"queen","suit":"spades"}],` +
				`"hand_counts":[12,13,13,13],` +
				`"trick":[{"seat":0,"card":{"rank":"two","suit":"clubs"}}],` +
				`"scores":[20,17,25,16],"round_points":[0,0,0,0],` +
				`"legal_actions":[{"rank":"three","suit":"clubs"},` +
				`{"rank":"seven","suit":"clubs"},` +
				`{"rank":"jack","suit":"clubs"}],` +
				`"turn_deadline_ms":1728451400000,"paused":false}`,
			want: "seq=216 phase=playing turn=1 round=4 trick_num=1" +
				" hand=[3♣ 7♣ J♣ 5♦ 9♦ K♦ A♦ 6♥ 8♥ Q♥ 4♠ 7♠ Q♠]" +
				" legal=[3♣ 7♣ J♣] trick=[2♣]" +
				" round_points=[0 0 0 0] scores=[20 17 25 16]",
		},
		{
			// Observer of an all-AI game (no turn deadline) at the round 1
			// passing transition: a full 52-card deal, 13 cards per seat.
			name: "observer snapshot",
			snapshot: `{"type":"snapshot","seq":2,"phase":"passing",` +
				`"round_number":1,"trick_number":1,"pass_direction":"left",` +
				`"turn":0,"trick_winner":-1,"hearts_broken":false,` +
				`"hands":[[{"rank":"two","suit":"clubs"},` +
				`{"rank":"five","suit":"clubs"},{"rank":"nine","suit":"clubs"},` +
				`{"rank":"jack","suit":"clubs"},{"rank":"three","suit":"diamonds"},` +
				`{"rank":"eight","suit":"diamonds"},` +
				`{"rank":"queen","suit":"diamonds"},` +
				`{"rank":"four","suit":"hearts"},{"rank":"seven","suit":"hearts"},` +
				`{"rank":"king","suit":"hearts"},{"rank":"six","suit":"spades"},` +
				`{"rank":"ten","suit":"spades"},{"rank":"ace","suit":"spades"}],` +
				`[{"rank":"three","suit":"clubs"},{"rank":"six","suit":"clubs"},` +
				`{"rank":"ten","suit":"clubs"},{"rank":"queen","suit":"clubs"},` +
				`{"rank":"four","suit":"diamonds"},{"rank":"nine","suit":"diamonds"},` +
				`{"rank":"king","suit":"diamonds"},{"rank":"two","suit":"hearts"},` +
				`{"rank":"eight","suit":"hearts"},{"rank":"queen","suit":"hearts"},` +
				`{"rank":"two","suit":"spades"},{"rank":"seven","suit":"spades"},` +
				`{"rank":"jack","suit":"spades"}],` +
				`[{"rank":"four","suit":"clubs"},{"rank":"seven","suit":"clubs"},` +
				`{"rank":"king","suit":"clubs"},{"rank":"ace","suit":"clubs"},` +
				`{"rank":"five","suit":"diamonds"},{"rank":"ten","suit":"diamonds"},` +
				`{"rank":"ace","suit":"diamonds"},{"rank":"three","suit":"hearts"},` +
				`{"rank":"nine","suit":"hearts"},{"rank":"jack","suit":"hearts"},` +
				`{"rank":"four","suit":"spades"},{"rank":"eight","suit":"spades"},` +
				`{"rank":"queen","suit":"spades"}],` +
				`[{"rank":"eight","suit":"clubs"},{"rank":"two","suit":"diamonds"},` +
				`{"rank":"six","suit":"diamonds"},` +
				`{"rank":"seven","suit":"diamonds"},` +
				`{"rank":"jack","suit":"diamonds"},{"rank":"five","suit":"hearts"},` +
				`{"rank":"six","suit":"hearts"},{"rank":"ten","suit":"hearts"},` +
				`{"rank":"ace","suit":"hearts"},{"rank":"three","suit":"spades"},` +
				`{"rank":"five","suit":"spades"},{"rank":"nine","suit":"spades"},` +
				`{"rank":"king","suit":"spades"}]],` +
				`"hand_counts":[13,13,13,13],"trick":[],"trick_history":[],` +
				`"scores":[0,0,0,0],"round_points":[0,0,0,0],` +
				`"legal_actions":[{"rank":"two","suit":"clubs"},` +
				`{"rank":"five","suit":"clubs"},{"rank":"nine","suit":"clubs"},` +
				`{"rank":"jack","suit":"clubs"},{"rank":"three","suit":"diamonds"},` +
				`{"rank":"eight","suit":"diamonds"},` +
				`{"rank":"queen","suit":"diamonds"},` +
				`{"rank":"four","suit":"hearts"},{"rank":"seven","suit":"hearts"},` +
				`{"rank":"king","suit":"hearts"},{"rank":"six","suit":"spades"},` +
				`{"rank":"ten","suit":"spades"},{"rank":"ace","suit":"spades"}],` +
				`"turn_deadline_ms":0,"paused":false}`,
			want: "seq=2 phase=passing turn=0 round=1 trick_num=1" +
				" seat0=[2♣ 5♣ 9♣ J♣ 3♦ 8♦ Q♦ 4♥ 7♥ K♥ 6♠ 10♠ A♠]" +
				" seat1=[3♣ 6♣ 10♣ Q♣ 4♦ 9♦ K♦ 2♥ 8♥ Q♥ 2♠ 7♠ J♠]" +
				" seat2=[4♣ 7♣ K♣ A♣ 5♦ 10♦ A♦ 3♥ 9♥ J♥ 4♠ 8♠ Q♠]" +
				" seat3=[8♣ 2♦ 6♦ 7♦ J♦ 5♥ 6♥ 10♥ A♥ 3♠ 5♠ 9♠ K♠]" +
				" round_points=[0 0 0 0] scores=[0 0 0 0]",
		},
		{
			// The game's opening snapshot: the freshly dealt hand with no
			// legal actions and no turn deadline.
			name: "player deal snapshot",
			snapshot: `{"type":"snapshot","seq":1,"phase":"deal",` +
				`"round_number":1,"trick_number":1,"pass_direction":"left",` +
				`"turn":0,"trick_winner":-1,"hearts_broken":false,` +
				`"hand":[{"rank":"two","suit":"clubs"},` +
				`{"rank":"five","suit":"clubs"},{"rank":"nine","suit":"clubs"},` +
				`{"rank":"jack","suit":"clubs"},{"rank":"three","suit":"diamonds"},` +
				`{"rank":"eight","suit":"diamonds"},` +
				`{"rank":"queen","suit":"diamonds"},` +
				`{"rank":"four","suit":"hearts"},{"rank":"seven","suit":"hearts"},` +
				`{"rank":"king","suit":"hearts"},{"rank":"six","suit":"spades"},` +
				`{"rank":"ten","suit":"spades"},{"rank":"ace","suit":"spades"}],` +
				`"hand_counts":[13,13,13,13],"trick":[],"scores":[0,0,0,0],` +
				`"round_points":[0,0,0,0],"legal_actions":[],` +
				`"turn_deadline_ms":0,"paused":false}`,
			want: "seq=1 phase=deal turn=0 round=1 trick_num=1" +
				" hand=[2♣ 5♣ 9♣ J♣ 3♦ 8♦ Q♦ 4♥ 7♥ K♥ 6♠ 10♠ A♠]" +
				" round_points=[0 0 0 0] scores=[0 0 0 0]",
		},
		{
			// Round 2 deal at the round boundary: round 1 ended with seat
			// 1 taking the queen of spades, and the pass direction rotates
			// to "right".
			name: "observer deal snapshot",
			snapshot: `{"type":"snapshot","seq":72,"phase":"deal",` +
				`"round_number":2,"trick_number":1,"pass_direction":"right",` +
				`"turn":0,"trick_winner":-1,"hearts_broken":false,` +
				`"hands":[[{"rank":"three","suit":"clubs"},` +
				`{"rank":"six","suit":"clubs"},{"rank":"ten","suit":"clubs"},` +
				`{"rank":"queen","suit":"clubs"},{"rank":"four","suit":"diamonds"},` +
				`{"rank":"nine","suit":"diamonds"},{"rank":"king","suit":"diamonds"},` +
				`{"rank":"two","suit":"hearts"},{"rank":"eight","suit":"hearts"},` +
				`{"rank":"queen","suit":"hearts"},{"rank":"two","suit":"spades"},` +
				`{"rank":"seven","suit":"spades"},{"rank":"jack","suit":"spades"}],` +
				`[{"rank":"four","suit":"clubs"},{"rank":"seven","suit":"clubs"},` +
				`{"rank":"king","suit":"clubs"},{"rank":"ace","suit":"clubs"},` +
				`{"rank":"five","suit":"diamonds"},{"rank":"ten","suit":"diamonds"},` +
				`{"rank":"ace","suit":"diamonds"},{"rank":"three","suit":"hearts"},` +
				`{"rank":"nine","suit":"hearts"},{"rank":"jack","suit":"hearts"},` +
				`{"rank":"four","suit":"spades"},{"rank":"eight","suit":"spades"},` +
				`{"rank":"queen","suit":"spades"}],` +
				`[{"rank":"eight","suit":"clubs"},{"rank":"two","suit":"diamonds"},` +
				`{"rank":"six","suit":"diamonds"},` +
				`{"rank":"seven","suit":"diamonds"},` +
				`{"rank":"jack","suit":"diamonds"},{"rank":"five","suit":"hearts"},` +
				`{"rank":"six","suit":"hearts"},{"rank":"ten","suit":"hearts"},` +
				`{"rank":"ace","suit":"hearts"},{"rank":"three","suit":"spades"},` +
				`{"rank":"five","suit":"spades"},{"rank":"nine","suit":"spades"},` +
				`{"rank":"king","suit":"spades"}],` +
				`[{"rank":"two","suit":"clubs"},{"rank":"five","suit":"clubs"},` +
				`{"rank":"nine","suit":"clubs"},{"rank":"jack","suit":"clubs"},` +
				`{"rank":"three","suit":"diamonds"},` +
				`{"rank":"eight","suit":"diamonds"},` +
				`{"rank":"queen","suit":"diamonds"},` +
				`{"rank":"four","suit":"hearts"},{"rank":"seven","suit":"hearts"},` +
				`{"rank":"king","suit":"hearts"},{"rank":"six","suit":"spades"},` +
				`{"rank":"ten","suit":"spades"},{"rank":"ace","suit":"spades"}]],` +
				`"hand_counts":[13,13,13,13],"trick":[],"trick_history":[],` +
				`"scores":[4,15,2,5],"round_points":[0,0,0,0],` +
				`"legal_actions":[],"turn_deadline_ms":0,"paused":false}`,
			want: "seq=72 phase=deal turn=0 round=2 trick_num=1" +
				" seat0=[3♣ 6♣ 10♣ Q♣ 4♦ 9♦ K♦ 2♥ 8♥ Q♥ 2♠ 7♠ J♠]" +
				" seat1=[4♣ 7♣ K♣ A♣ 5♦ 10♦ A♦ 3♥ 9♥ J♥ 4♠ 8♠ Q♠]" +
				" seat2=[8♣ 2♦ 6♦ 7♦ J♦ 5♥ 6♥ 10♥ A♥ 3♠ 5♠ 9♠ K♠]" +
				" seat3=[2♣ 5♣ 9♣ J♣ 3♦ 8♦ Q♦ 4♥ 7♥ K♥ 6♠ 10♠ A♠]" +
				" round_points=[0 0 0 0] scores=[4 15 2 5]",
		},
		{
			// Thirteen rounds played; seat 0 crossed 100 points at the
			// final round's scoring, ending the game. The final round's
			// round_points include seat 0's queen of spades (14 = Q♠ plus
			// one heart); the last trick shown is point-free. Hands are
			// empty, and trick_winner is -1: the field is only meaningful
			// during trick_complete, two transitions back.
			name: "game over snapshot",
			snapshot: `{"type":"snapshot","seq":912,"phase":"game_over",` +
				`"round_number":13,"trick_number":13,"pass_direction":"left",` +
				`"turn":2,"trick_winner":-1,"hearts_broken":true,` +
				`"hand":[],"hand_counts":[0,0,0,0],` +
				`"trick":[{"seat":3,"card":{"rank":"ace","suit":"diamonds"}},` +
				`{"seat":0,"card":{"rank":"ten","suit":"diamonds"}},` +
				`{"seat":1,"card":{"rank":"queen","suit":"diamonds"}},` +
				`{"seat":2,"card":{"rank":"king","suit":"diamonds"}}],` +
				`"scores":[104,98,72,64],"round_points":[14,4,5,3],` +
				`"legal_actions":[],"turn_deadline_ms":0,"paused":false}`,
			want: "seq=912 phase=game_over turn=2 round=13 trick_num=13" +
				" scores=[104 98 72 64]",
		},
		{
			// Trick 7 of round 3: hearts are broken, so seat 2 leads a
			// heart; seat 1 takes the trick with the king. The trick's 4
			// points are not yet in round_points because the trick is
			// resolved only after this snapshot.
			name: "trick_complete snapshot",
			snapshot: `{"type":"snapshot","seq":182,"phase":"trick_complete",` +
				`"round_number":3,"trick_number":7,"pass_direction":"across",` +
				`"turn":1,"trick_winner":1,"hearts_broken":true,` +
				`"hand":[{"rank":"four","suit":"clubs"},` +
				`{"rank":"ten","suit":"clubs"},{"rank":"five","suit":"diamonds"},` +
				`{"rank":"ace","suit":"diamonds"},` +
				`{"rank":"eight","suit":"spades"},` +
				`{"rank":"queen","suit":"spades"}],` +
				`"hand_counts":[6,6,6,6],` +
				`"trick":[{"seat":2,"card":{"rank":"nine","suit":"hearts"}},` +
				`{"seat":3,"card":{"rank":"jack","suit":"hearts"}},` +
				`{"seat":0,"card":{"rank":"queen","suit":"hearts"}},` +
				`{"seat":1,"card":{"rank":"king","suit":"hearts"}}],` +
				`"scores":[18,9,24,1],"round_points":[0,6,0,2],` +
				`"legal_actions":[],"turn_deadline_ms":0,"paused":false}`,
			want: "seq=182 phase=trick_complete turn=1 round=3 trick_num=7" +
				" hand=[4♣ 10♣ 5♦ A♦ 8♠ Q♠] trick=[9♥ J♥ Q♥ K♥]" +
				" trick_winner=1 round_points=[0 6 0 2] scores=[18 9 24 1]",
		},
		{
			name:     "malformed snapshot",
			snapshot: `not-json`,
			want:     "malformed: invalid character 'o' in literal null (expecting 'u')",
		},
		{
			// End of round 1: every hand is empty and the final trick is
			// still shown — a point-free clubs trick won by seat 0's ace
			// (its winner is not reflected in trick_winner, which is only
			// meaningful during trick_complete). round_points carries the
			// round's score delta: 26 points total, seat 1's 15 being the
			// queen of spades plus two hearts.
			name: "round_complete with round_points",
			snapshot: `{"type":"snapshot","seq":71,"phase":"round_complete",` +
				`"round_number":1,"trick_number":13,"pass_direction":"left",` +
				`"turn":3,"trick_winner":-1,"hearts_broken":true,` +
				`"hand":[],"hand_counts":[0,0,0,0],` +
				`"trick":[{"seat":0,"card":{"rank":"ace","suit":"clubs"}},` +
				`{"seat":1,"card":{"rank":"ten","suit":"clubs"}},` +
				`{"seat":2,"card":{"rank":"jack","suit":"clubs"}},` +
				`{"seat":3,"card":{"rank":"queen","suit":"clubs"}}],` +
				`"scores":[4,15,2,5],"round_points":[4,15,2,5],` +
				`"legal_actions":[],"turn_deadline_ms":0,"paused":false}`,
			want: "seq=71 phase=round_complete turn=3 round=1 trick_num=13" +
				" hand=[] trick=[A♣ 10♣ J♣ Q♣]" +
				" round_points=[4 15 2 5] scores=[4 15 2 5]",
		},
		{
			// The last trick of round 2 is complete: all hands are empty.
			// Seat 0 shot the moon in round 1, which is why every other
			// seat carries 26 points. round_points is missing this trick's
			// 4 hearts (scored only on resolution) but already includes
			// seat 1's queen of spades from an earlier trick.
			name: "final trick_complete snapshot with empty hands",
			snapshot: `{"type":"snapshot","seq":141,"phase":"trick_complete",` +
				`"round_number":2,"trick_number":13,"pass_direction":"right",` +
				`"turn":2,"trick_winner":2,"hearts_broken":true,` +
				`"hand":[],"hand_counts":[0,0,0,0],` +
				`"trick":[{"seat":2,"card":{"rank":"ace","suit":"hearts"}},` +
				`{"seat":3,"card":{"rank":"two","suit":"hearts"}},` +
				`{"seat":0,"card":{"rank":"five","suit":"hearts"}},` +
				`{"seat":1,"card":{"rank":"ten","suit":"hearts"}}],` +
				`"scores":[0,26,26,26],"round_points":[3,15,2,2],` +
				`"legal_actions":[],"turn_deadline_ms":0,"paused":false}`,
			want: "seq=141 phase=trick_complete turn=2 round=2 trick_num=13" +
				" hand=[] trick=[A♥ 2♥ 5♥ 10♥] trick_winner=2" +
				" round_points=[3 15 2 2] scores=[0 26 26 26]",
		},
		{
			// Round 2, trick 5: seats 0 and 1 have played to the trick,
			// and seat 2 must follow the led diamonds. Round 1 scores sum
			// to 26 with seat 1 holding the queen of spades; round_points
			// are the points taken in tricks 1-4.
			name: "realistic full hand snapshot midround",
			snapshot: `{"type":"snapshot","seq":99,"phase":"playing",` +
				`"round_number":2,"trick_number":5,"pass_direction":"right",` +
				`"turn":2,"trick_winner":-1,"hearts_broken":true,` +
				`"hand":[{"rank":"four","suit":"clubs"},` +
				`{"rank":"eight","suit":"clubs"},{"rank":"ace","suit":"clubs"},` +
				`{"rank":"two","suit":"diamonds"},` +
				`{"rank":"queen","suit":"diamonds"},` +
				`{"rank":"five","suit":"hearts"},{"rank":"nine","suit":"hearts"},` +
				`{"rank":"six","suit":"spades"},{"rank":"king","suit":"spades"}],` +
				`"hand_counts":[8,8,9,9],` +
				`"trick":[{"seat":0,"card":{"rank":"three","suit":"diamonds"}},` +
				`{"seat":1,"card":{"rank":"six","suit":"diamonds"}}],` +
				`"scores":[3,15,6,2],"round_points":[1,0,4,2],` +
				`"legal_actions":[{"rank":"two","suit":"diamonds"},` +
				`{"rank":"queen","suit":"diamonds"}],` +
				`"turn_deadline_ms":1728451600000,"paused":false}`,
			want: "seq=99 phase=playing turn=2 round=2 trick_num=5" +
				" hand=[4♣ 8♣ A♣ 2♦ Q♦ 5♥ 9♥ 6♠ K♠]" +
				" legal=[2♦ Q♦] trick=[3♦ 6♦]" +
				" round_points=[1 0 4 2] scores=[3 15 6 2]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.FormatSnapshot([]byte(tt.snapshot))
			if got != tt.want {
				t.Errorf("FormatSnapshot() got\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}
