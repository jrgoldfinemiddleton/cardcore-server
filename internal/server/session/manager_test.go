package session

import (
	"errors"
	"sync"
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// TestCreateReturnsTokensForHumanSeats verifies that Create issues
// tokens only for human seats.
func TestCreateReturnsTokensForHumanSeats(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, seats, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if info.SessionID == "" {
		t.Fatal("got empty session ID")
	}
	if got := len(seats); got != 4 {
		t.Fatalf("got %d seats, want 4", got)
	}

	for _, s := range seats {
		if s.Type == SeatHuman && s.Token == "" {
			t.Errorf("seat %d: human seat has no token", s.Index)
		}
		if s.Type == SeatAI && s.Token != "" {
			t.Errorf("seat %d: AI seat has a token", s.Index)
		}
	}
}

// TestCreateInvalidConfig verifies that Create rejects bad configs.
func TestCreateInvalidConfig(t *testing.T) {
	m := NewManager(mockGameFactory())

	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "empty game",
			cfg:  Config{Seats: []SeatConfig{{Type: SeatHuman}}},
		},
		{
			name: "no seats",
			cfg:  Config{Game: "hearts"},
		},
		{
			name: "invalid seat type",
			cfg: Config{
				Game:  "hearts",
				Seats: []SeatConfig{{Type: "bot"}},
			},
		},
		{
			name: "ai seat missing ai_type",
			cfg: Config{
				Game:  "hearts",
				Seats: []SeatConfig{{Type: SeatAI}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := m.Create(tt.cfg)
			if err == nil {
				t.Error("got nil error, want validation error")
			}
		})
	}
}

// TestCreateAIDelayDefault verifies that omitting AIDelayMS uses the
// default value.
func TestCreateAIDelayDefault(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := Config{
		Game: "hearts",
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
	}

	info, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID
	info, err = m.Get(id)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if info.AIDelayMS != defaultAIDelayMS {
		t.Errorf(
			"got ai_delay_ms %d, want %d",
			info.AIDelayMS, defaultAIDelayMS,
		)
	}
}

// TestCreateAIDelayZero verifies that explicitly setting AIDelayMS to 0
// is preserved.
func TestCreateAIDelayZero(t *testing.T) {
	m := NewManager(mockGameFactory())
	delay := 0
	cfg := Config{
		Game: "hearts",
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
		AIDelayMS: &delay,
	}

	info, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID
	info, err = m.Get(id)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if info.AIDelayMS != 0 {
		t.Errorf("got ai_delay_ms %d, want 0", info.AIDelayMS)
	}
}

// TestGetReturnsSessionInfo verifies that Get returns details matching
// the created session.
func TestGetReturnsSessionInfo(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	info, err = m.Get(id)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	if info.State != Draft {
		t.Errorf("got state %q, want %q", info.State, Draft)
	}
	if info.Game != "hearts" {
		t.Errorf("got game %q, want \"hearts\"", info.Game)
	}
	if info.AIDelayMS != 0 {
		t.Errorf("got ai_delay_ms %d, want 0", info.AIDelayMS)
	}
	if got := len(info.Seats); got != 4 {
		t.Fatalf("got %d seats, want 4", got)
	}
	if info.Seats[1].AIType != "random" {
		t.Errorf(
			"got ai_type %q, want \"random\"",
			info.Seats[1].AIType,
		)
	}
}

// TestGetNotFound verifies that Get returns ErrNotFound for unknown IDs.
func TestGetNotFound(t *testing.T) {
	m := NewManager(mockGameFactory())
	_, err := m.Get("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestListExcludesExpired verifies that deleted sessions are excluded
// from List results.
func TestListExcludesExpired(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info1, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id1 := info1.SessionID
	if _, _, err := m.Create(cfg); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := m.Delete(id1); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	sessions := m.List()
	for _, s := range sessions {
		if s.SessionID == id1 {
			t.Error("deleted session appears in List")
		}
	}
	if got := len(sessions); got != 1 {
		t.Errorf("got %d sessions, want 1", got)
	}
}

// TestUpdateDraftSucceeds verifies that Update modifies config in
// draft state.
func TestUpdateDraftSucceeds(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	newDelay := 100
	info, updatedSeats, err := m.Update(id, PatchConfig{AIDelayMS: &newDelay})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if info.AIDelayMS != 100 {
		t.Errorf("got ai_delay_ms %d, want 100", info.AIDelayMS)
	}
	if updatedSeats != nil {
		t.Errorf("got %d updated seats, want nil", len(updatedSeats))
	}
}

// TestUpdateSeatConfigRegeneratesTokens verifies that changing seats
// produces new tokens and returns them to the caller.
func TestUpdateSeatConfigRegeneratesTokens(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, originalSeats, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	newSeats := validHeartsCfg().Seats
	_, updatedSeats, err := m.Update(id, PatchConfig{Seats: newSeats})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if updatedSeats == nil {
		t.Fatal("got nil seats from Update, want seat info with new tokens")
	}
	if got := len(updatedSeats); got != 4 {
		t.Fatalf("got %d updated seats, want 4", got)
	}
	for _, s := range updatedSeats {
		if s.Type == SeatHuman && s.Token == "" {
			t.Errorf("seat %d: human seat has no token", s.Index)
		}
		if s.Type == SeatAI && s.Token != "" {
			t.Errorf("seat %d: AI seat has a token", s.Index)
		}
	}
	if updatedSeats[0].Token == originalSeats[0].Token {
		t.Error("seat 0 token was not regenerated after seat update")
	}
}

// TestUpdateNotFound verifies that Update returns ErrNotFound for
// unknown IDs.
func TestUpdateNotFound(t *testing.T) {
	m := NewManager(mockGameFactory())
	_, _, err := m.Update("nonexistent", PatchConfig{})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestUpdateNonDraftFails verifies that Update rejects changes to
// non-draft sessions.
func TestUpdateNonDraftFails(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	t.Run("expired", func(t *testing.T) {
		if err := m.Delete(id); err != nil {
			t.Fatalf("Delete() error: %v", err)
		}
		_, _, err := m.Update(id, PatchConfig{})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("got error %v, want ErrNotFound", err)
		}
	})

	t.Run("active", func(t *testing.T) {
		id := mustCreateAndStart(t, m, cfg)
		_, _, err := m.Update(id, PatchConfig{})
		if !errors.Is(err, ErrNotDraft) {
			t.Errorf("got error %v, want ErrNotDraft", err)
		}
		_ = m.Delete(id)
	})
}

// TestDeleteTransitionsToExpired verifies that Delete makes a session
// inaccessible.
func TestDeleteTransitionsToExpired(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID
	if err := m.Delete(id); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = m.Get(id)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestDeleteNotFound verifies that deleting a nonexistent session
// returns ErrNotFound.
func TestDeleteNotFound(t *testing.T) {
	m := NewManager(mockGameFactory())
	err := m.Delete("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestManagerConcurrency exercises Create, Get, List, Update, and
// Delete from multiple goroutines to surface race conditions under
// go test -race.
func TestManagerConcurrency(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				info, _, err := m.Create(cfg)
				if err != nil {
					continue
				}
				id := info.SessionID
				_, _ = m.Get(id)
				m.List()
				delay := 100
				_, _, _ = m.Update(id, PatchConfig{AIDelayMS: &delay})
				_ = m.Delete(id)
			}
		})
	}
	wg.Wait()
}

// TestManagerStartCreatesSession verifies that Start creates a game
// session and transitions to active.
func TestManagerStartCreatesSession(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	id := mustCreateAndStart(t, m, cfg)

	info, err := m.Get(id)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got, want := info.State, Active; got != want {
		t.Errorf("state: got %q, want %q", got, want)
	}
}

// TestManagerSubscribePlayerReturnsChannel verifies that SubscribePlayer
// returns a channel for receiving snapshots.
func TestManagerSubscribePlayerReturnsChannel(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	id := mustCreateAndStart(t, m, cfg)

	ch, err := m.SubscribePlayer(id, 0)
	if err != nil {
		t.Fatalf("SubscribePlayer() error: %v", err)
	}
	if ch == nil {
		t.Fatal("got nil channel, want non-nil")
	}
}

// TestManagerSubscribeObserverReturnsChannel verifies that SubscribeObserver
// returns a channel for receiving snapshots.
func TestManagerSubscribeObserverReturnsChannel(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	id := mustCreateAndStart(t, m, cfg)

	ch, err := m.SubscribeObserver(id)
	if err != nil {
		t.Fatalf("SubscribeObserver() error: %v", err)
	}
	if ch == nil {
		t.Fatal("got nil channel, want non-nil")
	}
}

// TestManagerSubmitActionRejectsNotActive verifies that SubmitAction rejects commands
// when the session is not active.
func TestManagerSubmitActionRejectsNotActive(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID

	_, err = m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action1",
		Seq:      0,
	})
	if !errors.Is(err, ErrNotActive) {
		t.Errorf("got error %v, want ErrNotActive", err)
	}
}

// TestManagerSubmitActionActiveSucceeds verifies that SubmitAction accepts
// a command when the session is active and returns a successful result.
func TestManagerSubmitActionActiveSucceeds(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	id := mustCreateAndStart(t, m, cfg)

	result, err := m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action1",
		Seq:      0,
	})
	if err != nil {
		t.Fatalf("SubmitAction() transport error: %v", err)
	}
	if result.Err != nil {
		t.Errorf("got result.Err %v, want nil", result.Err)
	}
}

// TestManagerUnsubscribePlayerSendsCommand verifies that UnsubscribePlayer
// sends an unsubscribe command to the session goroutine.
func TestManagerUnsubscribePlayerSendsCommand(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	id := mustCreateAndStart(t, m, cfg)

	_, err := m.SubscribePlayer(id, 0)
	if err != nil {
		t.Fatalf("SubscribePlayer() error: %v", err)
	}

	if err := m.UnsubscribePlayer(id, 0); err != nil {
		t.Fatalf("UnsubscribePlayer() error: %v", err)
	}
}

// TestManagerUnsubscribeObserverSendsCommand verifies that UnsubscribeObserver
// sends an unsubscribe command to the session goroutine.
func TestManagerUnsubscribeObserverSendsCommand(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	id := mustCreateAndStart(t, m, cfg)

	ch, err := m.SubscribeObserver(id)
	if err != nil {
		t.Fatalf("SubscribeObserver() error: %v", err)
	}

	if err := m.UnsubscribeObserver(id, ch); err != nil {
		t.Fatalf("UnsubscribeObserver() error: %v", err)
	}
}
