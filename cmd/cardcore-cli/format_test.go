package main

import (
	"testing"

	heartsclient "github.com/jrgoldfinemiddleton/cardcore-server/internal/client/hearts"
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

// TestFormatSnapshot verifies compact notation for player, observer, and terminal snapshots.
func TestFormatSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		snapshot string
		want     string
	}{
		{
			name: "player passing snapshot",
			snapshot: `{"seq":5,"phase":"passing","turn":0,"hand":[{"rank":"two",` +
				`"suit":"clubs"},{"rank":"three","suit":"diamonds"}],` +
				`"legal_actions":[{"rank":"two","suit":"clubs"},` +
				`{"rank":"three","suit":"diamonds"}],"scores":[0,0,0,0]}`,
			want: "seq=5 phase=passing turn=0 hand=[2♣ 3♦] legal=[2♣ 3♦] scores=[0 0 0 0]",
		},
		{
			name: "player playing snapshot with trick",
			snapshot: `{"seq":12,"phase":"playing","turn":2,"hand":[{"rank":"ace",` +
				`"suit":"spades"}],"legal_actions":[{"rank":"ace","suit":"spades"}],` +
				`"trick":[{"seat":0,"card":{"rank":"two","suit":"clubs"}},` +
				`{"seat":1,"card":{"rank":"seven","suit":"hearts"}}],` +
				`"scores":[0,13,0,0]}`,
			want: "seq=12 phase=playing turn=2 hand=[A♠] legal=[A♠] trick=[2♣ 7♥]" +
				" scores=[0 13 0 0]",
		},
		{
			name: "observer snapshot",
			snapshot: `{"seq":3,"phase":"passing","turn":1,"hands":` +
				`[[{"rank":"two","suit":"clubs"}],[{"rank":"ace","suit":"hearts"}],` +
				`[{"rank":"king","suit":"spades"}],` +
				`[{"rank":"queen","suit":"diamonds"}]],"scores":[0,0,0,0]}`,
			want: "seq=3 phase=passing turn=1 seat0=[2♣] seat1=[A♥] seat2=[K♠]" +
				" seat3=[Q♦] scores=[0 0 0 0]",
		},
		{
			name:     "game over snapshot",
			snapshot: `{"seq":123,"phase":"game_over","turn":0,"scores":[0,26,13,13]}`,
			want:     "seq=123 phase=game_over turn=0 scores=[0 26 13 13]",
		},
		{
			name: "trick_complete snapshot",
			snapshot: `{"seq":89,"phase":"trick_complete","turn":1,"trick":` +
				`[{"seat":0,"card":{"rank":"two","suit":"clubs"}},` +
				`{"seat":1,"card":{"rank":"ace","suit":"spades"}}],` +
				`"scores":[0,1,0,0]}`,
			want: "seq=89 phase=trick_complete turn=1 trick=[2♣ A♠] scores=[0 1 0 0]",
		},
		{
			name:     "malformed snapshot",
			snapshot: `not-json`,
			want:     "malformed: invalid character 'o' in literal null (expecting 'u')",
		},
		{
			name: "round_complete with round_points",
			snapshot: `{"seq":45,"phase":"round_complete","turn":0,` +
				`"round_points":[0,13,0,0],"scores":[0,13,0,0]}`,
			want: "seq=45 phase=round_complete turn=0 round_points=[0 13 0 0]" +
				" scores=[0 13 0 0]",
		},
		{
			name: "empty hand snapshot",
			snapshot: `{"seq":10,"phase":"playing","turn":0,` +
				`"hand":[],"legal_actions":[],"scores":[0,0,0,0]}`,
			want: "seq=10 phase=playing turn=0 hand=[] scores=[0 0 0 0]",
		},
		{
			name: "realistic full hand snapshot midround",
			snapshot: `{"seq":42,"phase":"playing","turn":2,"round_number":1,` +
				`"trick_number":5,"hand":[{"rank":"five","suit":"clubs"},` +
				`{"rank":"jack","suit":"clubs"},{"rank":"ace","suit":"clubs"},` +
				`{"rank":"seven","suit":"diamonds"},{"rank":"queen","suit":"diamonds"},` +
				`{"rank":"ace","suit":"diamonds"},{"rank":"four","suit":"hearts"},` +
				`{"rank":"eight","suit":"hearts"},{"rank":"king","suit":"hearts"},` +
				`{"rank":"two","suit":"spades"},{"rank":"nine","suit":"spades"},` +
				`{"rank":"ten","suit":"spades"}],"legal_actions":[` +
				`{"rank":"seven","suit":"diamonds"},{"rank":"queen","suit":"diamonds"},` +
				`{"rank":"ace","suit":"diamonds"}],"trick":[` +
				`{"seat":0,"card":{"rank":"three","suit":"diamonds"}},` +
				`{"seat":1,"card":{"rank":"six","suit":"diamonds"}}],` +
				`"round_points":[0,13,0,0],"scores":[0,26,13,13]}`,
			want: "seq=42 phase=playing turn=2 round=1 trick_num=5" +
				" hand=[5♣ J♣ A♣ 7♦ Q♦ A♦ 4♥ 8♥ K♥ 2♠ 9♠ 10♠]" +
				" legal=[7♦ Q♦ A♦] trick=[3♦ 6♦]" +
				" round_points=[0 13 0 0] scores=[0 26 13 13]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSnapshot([]byte(tt.snapshot))
			if got != tt.want {
				t.Errorf("formatSnapshot() got\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}
