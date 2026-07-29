package heartsclient

import "testing"

// TestRankSymbol verifies rank string to symbol mapping.
func TestRankSymbol(t *testing.T) {
	tests := []struct {
		rank string
		want string
	}{
		{"two", "2"},
		{"three", "3"},
		{"four", "4"},
		{"five", "5"},
		{"six", "6"},
		{"seven", "7"},
		{"eight", "8"},
		{"nine", "9"},
		{"ten", "10"},
		{"jack", "J"},
		{"queen", "Q"},
		{"king", "K"},
		{"ace", "A"},
		{"unknown", "?"},
	}
	for _, tt := range tests {
		if got := RankSymbol(tt.rank); got != tt.want {
			t.Errorf("RankSymbol(%q) = %q, want %q", tt.rank, got, tt.want)
		}
	}
}

// TestSuitSymbol verifies suit string to Unicode symbol mapping.
func TestSuitSymbol(t *testing.T) {
	tests := []struct {
		suit string
		want string
	}{
		{"clubs", "♣"},
		{"diamonds", "♦"},
		{"hearts", "♥"},
		{"spades", "♠"},
		{"unknown", "?"},
	}
	for _, tt := range tests {
		if got := SuitSymbol(tt.suit); got != tt.want {
			t.Errorf("SuitSymbol(%q) = %q, want %q", tt.suit, got, tt.want)
		}
	}
}

// TestRankValue verifies rank string to numeric value mapping.
func TestRankValue(t *testing.T) {
	tests := []struct {
		rank string
		want int
	}{
		{"two", 2},
		{"ten", 10},
		{"jack", 11},
		{"queen", 12},
		{"king", 13},
		{"ace", 14},
		{"unknown", 0},
	}
	for _, tt := range tests {
		if got := RankValue(tt.rank); got != tt.want {
			t.Errorf("RankValue(%q) = %d, want %d", tt.rank, got, tt.want)
		}
	}
}

// TestGameName verifies the canonical Hearts game name.
func TestGameName(t *testing.T) {
	if GameName != "hearts" {
		t.Errorf("GameName = %q, want %q", GameName, "hearts")
	}
}
