package heartsclient

// GameName is the canonical name for the Hearts game on the server and in
// client configuration.
const GameName = "hearts"

// RankSymbol maps a rank string to its display symbol.
//
// Known ranks: "two".."ten", "jack", "queen", "king", "ace".
// Unknown ranks return "?".
func RankSymbol(rank string) string {
	switch rank {
	case "two":
		return "2"
	case "three":
		return "3"
	case "four":
		return "4"
	case "five":
		return "5"
	case "six":
		return "6"
	case "seven":
		return "7"
	case "eight":
		return "8"
	case "nine":
		return "9"
	case "ten":
		return "10"
	case "jack":
		return "J"
	case "queen":
		return "Q"
	case "king":
		return "K"
	case "ace":
		return "A"
	default:
		return "?"
	}
}

// SuitSymbol maps a suit string to its Unicode symbol.
//
// Known suits: "clubs", "diamonds", "hearts", "spades".
// Unknown suits return "?".
func SuitSymbol(suit string) string {
	switch suit {
	case "clubs":
		return "♣"
	case "diamonds":
		return "♦"
	case "hearts":
		return "♥"
	case "spades":
		return "♠"
	default:
		return "?"
	}
}

// RankValue returns a numeric value for a rank string, used for sorting.
// Unknown ranks return 0.
func RankValue(rank string) int {
	switch rank {
	case "two":
		return 2
	case "three":
		return 3
	case "four":
		return 4
	case "five":
		return 5
	case "six":
		return 6
	case "seven":
		return 7
	case "eight":
		return 8
	case "nine":
		return 9
	case "ten":
		return 10
	case "jack":
		return 11
	case "queen":
		return 12
	case "king":
		return 13
	case "ace":
		return 14
	default:
		return 0
	}
}
