package heartscli

import (
	"context"
	"fmt"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/client"
)

const gameNameHearts = "hearts"

// CreateHumanSession creates a Hearts session with one human seat and
// three AI seats, returning the session ID and the human seat token.
// The human seat is always index 0.
func CreateHumanSession(
	ctx context.Context,
	sc *client.SessionClient,
	aiType string,
	pacing int,
) (string, string, error) {
	zero := 0
	cfg := client.Config{
		Game: gameNameHearts,
		Seats: []client.SeatConfig{
			{Type: "human"},
			{Type: "ai", AIType: aiType},
			{Type: "ai", AIType: aiType},
			{Type: "ai", AIType: aiType},
		},
		AIActionDelayMS:    &pacing,
		DealDisplayDelayMS: &zero,
	}

	id, seats, err := sc.CreateSession(ctx, cfg)
	if err != nil {
		return "", "", fmt.Errorf("create session: %w", err)
	}

	const humanSeat = 0
	token := seats[humanSeat].Token
	if token == "" {
		return "", "", fmt.Errorf("no human seat token in create response")
	}

	return id, token, nil
}

// CreateObserverSession creates a 4-AI Hearts session for observation.
func CreateObserverSession(
	ctx context.Context,
	sc *client.SessionClient,
	aiType string,
	pacing int,
) (string, []client.SeatInfo, error) {
	zero := 0
	cfg := client.Config{
		Game: gameNameHearts,
		Seats: []client.SeatConfig{
			{Type: "ai", AIType: aiType},
			{Type: "ai", AIType: aiType},
			{Type: "ai", AIType: aiType},
			{Type: "ai", AIType: aiType},
		},
		AIActionDelayMS:    &pacing,
		DealDisplayDelayMS: &zero,
	}

	return sc.CreateSession(ctx, cfg)
}
