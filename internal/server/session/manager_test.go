package session

import (
	"errors"
	"sync"
	"testing"
	"time"

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

// TestCreatePacingDelayDefault verifies that omitting PacingDelayMS uses
// the default value.
func TestCreatePacingDelayDefault(t *testing.T) {
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
	if info.PacingDelayMS != defaultPacingDelayMS {
		t.Errorf(
			"got pacing_delay_ms %d, want %d",
			info.PacingDelayMS, defaultPacingDelayMS,
		)
	}
}

// TestCreatePacingDelayZero verifies that explicitly setting PacingDelayMS
// to 0 is preserved.
func TestCreatePacingDelayZero(t *testing.T) {
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
		PacingDelayMS: &delay,
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
	if info.PacingDelayMS != 0 {
		t.Errorf("got pacing_delay_ms %d, want 0", info.PacingDelayMS)
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
	if info.PacingDelayMS != 0 {
		t.Errorf("got pacing_delay_ms %d, want 0", info.PacingDelayMS)
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
	info, updatedSeats, err := m.Update(id, PatchConfig{PacingDelayMS: &newDelay})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if info.PacingDelayMS != 100 {
		t.Errorf("got pacing_delay_ms %d, want 100", info.PacingDelayMS)
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

// TestDeleteIdempotent verifies that deleting the same session twice
// returns ErrNotFound without panicking.
func TestDeleteIdempotent(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	id := info.SessionID
	if err := m.Start(id); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if err := m.Delete(id); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	err = m.Delete(id)
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
				_, _, _ = m.Update(id, PatchConfig{PacingDelayMS: &delay})
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

// TestManagerSubmitActionRejectsFinished verifies that SubmitAction
// rejects commands when the session has finished.
func TestManagerSubmitActionRejectsFinished(t *testing.T) {
	m := NewManager(stepFinishedGameFactory())
	id := mustCreateAndStart(t, m, validHeartsCfg())

	_, err := m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action1",
		Seq:      0,
	})
	if err != nil {
		t.Fatalf("SubmitAction() error: %v", err)
	}

	waitForFinished(t, m, id)

	_, err = m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action2",
		Seq:      1,
	})
	if !errors.Is(err, ErrNotActive) {
		t.Errorf("got error %v, want ErrNotActive", err)
	}
}

// TestManagerSubscribePlayerRejectsFinished verifies that SubscribePlayer
// rejects new subscriptions when the session has finished.
func TestManagerSubscribePlayerRejectsFinished(t *testing.T) {
	m := NewManager(stepFinishedGameFactory())
	id := mustCreateAndStart(t, m, validHeartsCfg())

	_, err := m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action1",
		Seq:      0,
	})
	if err != nil {
		t.Fatalf("SubmitAction() error: %v", err)
	}

	waitForFinished(t, m, id)

	_, err = m.SubscribePlayer(id, 0)
	if !errors.Is(err, ErrNotActive) {
		t.Errorf("got error %v, want ErrNotActive", err)
	}
}

// TestManagerSubscribeObserverRejectsFinished verifies that
// SubscribeObserver rejects new subscriptions when the session has
// finished.
func TestManagerSubscribeObserverRejectsFinished(t *testing.T) {
	m := NewManager(stepFinishedGameFactory())
	id := mustCreateAndStart(t, m, validHeartsCfg())

	_, err := m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action1",
		Seq:      0,
	})
	if err != nil {
		t.Fatalf("SubmitAction() error: %v", err)
	}

	waitForFinished(t, m, id)

	_, err = m.SubscribeObserver(id)
	if !errors.Is(err, ErrNotActive) {
		t.Errorf("got error %v, want ErrNotActive", err)
	}
}

// TestManagerUnsubscribePlayerRejectsFinished verifies that
// UnsubscribePlayer rejects unsubscribes when the session has finished.
func TestManagerUnsubscribePlayerRejectsFinished(t *testing.T) {
	m := NewManager(stepFinishedGameFactory())
	id := mustCreateAndStart(t, m, validHeartsCfg())

	_, err := m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action1",
		Seq:      0,
	})
	if err != nil {
		t.Fatalf("SubmitAction() error: %v", err)
	}

	waitForFinished(t, m, id)

	err = m.UnsubscribePlayer(id, 0)
	if !errors.Is(err, ErrNotActive) {
		t.Errorf("got error %v, want ErrNotActive", err)
	}
}

// TestManagerUnsubscribeObserverRejectsFinished verifies that
// UnsubscribeObserver rejects unsubscribes when the session has
// finished.
func TestManagerUnsubscribeObserverRejectsFinished(t *testing.T) {
	m := NewManager(stepFinishedGameFactory())
	id := mustCreateAndStart(t, m, validHeartsCfg())

	_, err := m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action1",
		Seq:      0,
	})
	if err != nil {
		t.Fatalf("SubmitAction() error: %v", err)
	}

	waitForFinished(t, m, id)

	err = m.UnsubscribeObserver(id, nil)
	if !errors.Is(err, ErrNotActive) {
		t.Errorf("got error %v, want ErrNotActive", err)
	}
}

// TestSubmitActionDoesNotBlockAfterGoroutineExits verifies that
// SubmitAction returns ErrNotActive within a timeout after the session
// goroutine has finished naturally, instead of blocking forever on the
// response channel.
func TestSubmitActionDoesNotBlockAfterGoroutineExits(t *testing.T) {
	m := NewManager(stepFinishedGameFactory())
	id := mustCreateAndStart(t, m, validHeartsCfg())

	_, err := m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action1",
		Seq:      0,
	})
	if err != nil {
		t.Fatalf("SubmitAction() error: %v", err)
	}

	waitForFinished(t, m, id)

	done := make(chan struct{})
	var resultErr error
	go func() {
		_, resultErr = m.SubmitAction(id, 0, &api.InboundMessage{
			Type:     "test",
			ActionID: "action2",
			Seq:      1,
		})
		close(done)
	}()

	select {
	case <-done:
		if !errors.Is(resultErr, ErrNotActive) {
			t.Errorf("got error %v, want ErrNotActive", resultErr)
		}
	case <-time.After(time.Second):
		t.Fatal("SubmitAction blocked forever after goroutine exit")
	}
}

// TestOnDoneGuardDoesNotOverwriteExpired verifies that the onDone
// callback only transitions state from Active, preventing a race where
// Delete sets Expired and a late onDone(Finished) overwrites it.
func TestOnDoneGuardDoesNotOverwriteExpired(t *testing.T) {
	m := NewManager(mockGameFactory())

	// Manually create an entry in Expired state.
	m.mu.Lock()
	m.sessions["test"] = &entry{
		state: Expired,
	}
	m.mu.Unlock()

	// Simulate the onDone callback with Finished.
	onDone := func(finalState State) {
		m.mu.Lock()
		if e, ok := m.sessions["test"]; ok && e.state == Active {
			e.state = finalState
		}
		m.mu.Unlock()
	}
	onDone(Finished)

	// Verify state is still Expired.
	m.mu.RLock()
	state := m.sessions["test"].state
	m.mu.RUnlock()
	if state != Expired {
		t.Errorf("got state %q, want Expired", state)
	}
}

// TestLookupTokenValid verifies that LookupToken returns the correct
// session and seat for a valid human seat token.
func TestLookupTokenValid(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, seats, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Find first human seat with token.
	var token string
	var wantSeat int
	for i, s := range seats {
		if s.Type == SeatHuman && s.Token != "" {
			token = s.Token
			wantSeat = i
			break
		}
	}
	if token == "" {
		t.Fatal("no human seat token found")
	}

	gotID, gotSeat, err := m.LookupToken(token)
	if err != nil {
		t.Fatalf("LookupToken() error: %v", err)
	}
	if gotID != info.SessionID {
		t.Errorf("got sessionID %q, want %q", gotID, info.SessionID)
	}
	if gotSeat != wantSeat {
		t.Errorf("got seat %d, want %d", gotSeat, wantSeat)
	}
}

// TestLookupTokenInvalid verifies that LookupToken returns ErrNotFound
// for a non-existent token.
func TestLookupTokenInvalid(t *testing.T) {
	m := NewManager(mockGameFactory())

	_, _, err := m.LookupToken("invalid-token")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestLookupTokenAfterDelete verifies that LookupToken returns
// ErrNotFound after the session is deleted.
func TestLookupTokenAfterDelete(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := validHeartsCfg()

	info, seats, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Find first human seat token.
	var token string
	for _, s := range seats {
		if s.Type == SeatHuman && s.Token != "" {
			token = s.Token
			break
		}
	}
	if token == "" {
		t.Fatal("no human seat token found")
	}

	if err := m.Delete(info.SessionID); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, _, err = m.LookupToken(token)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestLookupTokenAfterUpdate verifies that old tokens become invalid
// and new tokens are valid after updating seat configuration.
func TestLookupTokenAfterUpdate(t *testing.T) {
	m := NewManager(mockGameFactory())
	cfg := Config{
		Game: "hearts",
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
	}

	info, seats, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	oldToken := seats[0].Token
	if oldToken == "" {
		t.Fatal("no human seat token found")
	}

	// Verify old token works.
	_, _, err = m.LookupToken(oldToken)
	if err != nil {
		t.Fatalf("LookupToken(oldToken) error: %v", err)
	}

	newSeats := []SeatConfig{
		{Type: SeatHuman},
		{Type: SeatHuman},
	}
	_, newSeatInfos, err := m.Update(info.SessionID, PatchConfig{Seats: newSeats})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	// Old token should be invalid.
	_, _, err = m.LookupToken(oldToken)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("LookupToken(oldToken) got error %v, want ErrNotFound", err)
	}

	// New tokens should be valid.
	var newToken string
	for _, s := range newSeatInfos {
		if s.Type == SeatHuman && s.Token != "" {
			newToken = s.Token
			break
		}
	}
	if newToken == "" {
		t.Fatal("no new human seat token found")
	}

	_, _, err = m.LookupToken(newToken)
	if err != nil {
		t.Errorf("LookupToken(newToken) error: %v", err)
	}
}

// TestManagerMarshalFailureTransitionsToFinished verifies that when a
// session terminates due to snapshot marshal failure, the Manager
// state transitions to Finished so that subsequent commands are
// rejected with ErrNotActive.
func TestManagerMarshalFailureTransitionsToFinished(t *testing.T) {
	m := NewManager(unmarshalableGameFactory())
	id := mustCreateAndStart(t, m, validHeartsCfg())

	_, err := m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action1",
		Seq:      0,
	})
	if err != nil {
		t.Fatalf("SubmitAction() error: %v", err)
	}

	waitForFinished(t, m, id)

	_, err = m.SubmitAction(id, 0, &api.InboundMessage{
		Type:     "test",
		ActionID: "action2",
		Seq:      1,
	})
	if !errors.Is(err, ErrNotActive) {
		t.Errorf("got error %v, want ErrNotActive", err)
	}
}

// TestBuildSeatInfoTokens verifies token generation for mixed human/AI
// seat configurations.
func TestBuildSeatInfoTokens(t *testing.T) {
	tests := []struct {
		name       string
		configs    []SeatConfig
		wantHumans int
	}{
		{
			name: "all human",
			configs: []SeatConfig{
				{Type: SeatHuman},
				{Type: SeatHuman},
			},
			wantHumans: 2,
		},
		{
			name: "all AI",
			configs: []SeatConfig{
				{Type: SeatAI, AIType: "random"},
				{Type: SeatAI, AIType: "random"},
			},
			wantHumans: 0,
		},
		{
			name: "mixed",
			configs: []SeatConfig{
				{Type: SeatHuman},
				{Type: SeatAI, AIType: "random"},
				{Type: SeatHuman},
			},
			wantHumans: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seats, err := buildSeatInfo(tc.configs)
			if err != nil {
				t.Fatalf("buildSeatInfo error: %v", err)
			}
			if len(seats) != len(tc.configs) {
				t.Fatalf("got %d seats, want %d", len(seats), len(tc.configs))
			}

			var humanCount int
			for i, s := range seats {
				if s.Index != i {
					t.Errorf("seat %d: got index %d, want %d", i, s.Index, i)
				}
				if s.Type == SeatHuman {
					humanCount++
					if s.Token == "" {
						t.Errorf("seat %d: human seat has no token", i)
					}
					// Token should be 64 hex chars (32 bytes).
					if len(s.Token) != 64 {
						t.Errorf("seat %d: got token length %d, want 64", i, len(s.Token))
					}
				}
				if s.Type == SeatAI && s.Token != "" {
					t.Errorf("seat %d: AI seat has token %q", i, s.Token)
				}
			}
			if humanCount != tc.wantHumans {
				t.Errorf("got %d human seats, want %d", humanCount, tc.wantHumans)
			}
		})
	}
}
