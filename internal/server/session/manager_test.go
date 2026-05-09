package session

import (
	"errors"
	"sync"
	"testing"
)

// TestCreateReturnsTokensForHumanSeats verifies that Create issues
// tokens only for human seats.
func TestCreateReturnsTokensForHumanSeats(t *testing.T) {
	m := NewManager()
	cfg := validHeartsCfg()

	id, seats, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if id == "" {
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
	m := NewManager()

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
	m := NewManager()
	cfg := Config{
		Game: "hearts",
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
	}

	id, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	info, err := m.Get(id)
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
	m := NewManager()
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

	id, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	info, err := m.Get(id)
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
	m := NewManager()
	cfg := validHeartsCfg()

	id, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	info, err := m.Get(id)
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
	m := NewManager()
	_, err := m.Get("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestListExcludesExpired verifies that deleted sessions are excluded
// from List results.
func TestListExcludesExpired(t *testing.T) {
	m := NewManager()
	cfg := validHeartsCfg()

	id1, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
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
	m := NewManager()
	cfg := validHeartsCfg()

	id, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	newDelay := 100
	info, err := m.Update(id, PatchConfig{AIDelayMS: &newDelay})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if info.AIDelayMS != 100 {
		t.Errorf("got ai_delay_ms %d, want 100", info.AIDelayMS)
	}
}

// TestUpdateSeatConfigRegeneratesTokens verifies that changing seats
// produces new tokens.
func TestUpdateSeatConfigRegeneratesTokens(t *testing.T) {
	m := NewManager()
	cfg := validHeartsCfg()

	id, originalSeats, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	newSeats := validHeartsCfg().Seats
	if _, err := m.Update(id, PatchConfig{Seats: newSeats}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}

	e := m.sessions[id]
	if e.seats[0].Token == originalSeats[0].Token {
		t.Error("seat 0 token was not regenerated after seat update")
	}
}

// TestUpdateNotFound verifies that Update returns ErrNotFound for
// unknown IDs.
func TestUpdateNotFound(t *testing.T) {
	m := NewManager()
	_, err := m.Update("nonexistent", PatchConfig{})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestUpdateNonDraftFails verifies that Update rejects changes to
// non-draft sessions.
// TODO: switch active subtest to use Start once implemented.
func TestUpdateNonDraftFails(t *testing.T) {
	m := NewManager()
	cfg := validHeartsCfg()

	id, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	t.Run("expired", func(t *testing.T) {
		if err := m.Delete(id); err != nil {
			t.Fatalf("Delete() error: %v", err)
		}
		_, err := m.Update(id, PatchConfig{})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("got error %v, want ErrNotFound", err)
		}
	})

	t.Run("active", func(t *testing.T) {
		id, _, err := m.Create(cfg)
		if err != nil {
			t.Fatalf("Create() error: %v", err)
		}
		m.sessions[id].state = Active
		_, err = m.Update(id, PatchConfig{})
		if !errors.Is(err, ErrNotDraft) {
			t.Errorf("got error %v, want ErrNotDraft", err)
		}
	})
}

// TestDeleteTransitionsToExpired verifies that Delete makes a session
// inaccessible.
func TestDeleteTransitionsToExpired(t *testing.T) {
	m := NewManager()
	cfg := validHeartsCfg()

	id, _, err := m.Create(cfg)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
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
	m := NewManager()
	err := m.Delete("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got error %v, want ErrNotFound", err)
	}
}

// TestManagerConcurrency exercises Create, Get, List, Update, and
// Delete from multiple goroutines to surface race conditions under
// go test -race.
func TestManagerConcurrency(t *testing.T) {
	m := NewManager()
	cfg := validHeartsCfg()

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				id, _, err := m.Create(cfg)
				if err != nil {
					continue
				}
				_, _ = m.Get(id)
				m.List()
				delay := 100
				_, _ = m.Update(id, PatchConfig{AIDelayMS: &delay})
				_ = m.Delete(id)
			}
		})
	}
	wg.Wait()
}

// validHeartsCfg returns a realistic 4-seat Hearts config for tests.
func validHeartsCfg() Config {
	delay := 0
	return Config{
		Game: "hearts",
		Seats: []SeatConfig{
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
			{Type: SeatHuman},
			{Type: SeatAI, AIType: "random"},
		},
		AIDelayMS: &delay,
	}
}
