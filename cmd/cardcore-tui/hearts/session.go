package heartstui

import (
	"context"
	"fmt"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

const gameNameHearts = "hearts"

// validAITypes is the set of AI player types accepted by the Hearts server
// adapter. CreateSession rejects any aiType not in this set before issuing
// any HTTP request.
var validAITypes = map[string]bool{
	"random":    true,
	"heuristic": true,
	"pimc":      true,
}

// CreateSession creates and starts a Hearts session suitable for the TUI.
//
// When observer is false the session has one human seat at index 0 and three
// AI seats; the returned token is the human seat's bearer credential. When
// observer is true the session has four AI seats and the returned token is
// empty (observers connect without a seat token). The returned seat is always
// 0.
//
// aiType must be one of "random", "heuristic", or "pimc"; any other value
// returns an error before any HTTP request is made. aiActionDelayMS and
// dealDisplayDelayMS are optional overrides: nil means use the server default.
func CreateSession(
	ctx context.Context,
	sc *client.SessionClient,
	aiType string,
	observer bool,
	aiActionDelayMS, dealDisplayDelayMS *int,
) (sessionID, token string, seat int, err error) {
	if !validAITypes[aiType] {
		return "", "", 0, fmt.Errorf(
			"invalid ai_type: %q (want random, heuristic, or pimc)", aiType)
	}

	seats := make([]client.SeatConfig, 0, 4)
	if !observer {
		seats = append(seats, client.SeatConfig{Type: "human"})
	}
	for range 4 - len(seats) {
		seats = append(seats, client.SeatConfig{Type: "ai", AIType: aiType})
	}

	cfg := client.Config{
		Game:               gameNameHearts,
		Seats:              seats,
		AIActionDelayMS:    aiActionDelayMS,
		DealDisplayDelayMS: dealDisplayDelayMS,
	}

	id, seatInfos, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		return "", "", 0, fmt.Errorf("create session: %w", err)
	}

	if !observer {
		const humanSeat = 0
		token = seatInfos[humanSeat].Token
		if token == "" {
			return "", "", 0, fmt.Errorf("no human seat token in create response")
		}
	}

	if err := sc.StartSession(ctx, id); err != nil {
		return "", "", 0, fmt.Errorf("start session: %w", err)
	}

	return id, token, 0, nil
}
