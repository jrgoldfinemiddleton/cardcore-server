package testutil

import (
	"testing"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/server/session"
)

// TestHashTestNameDeterministic verifies that HashTestName returns the same
// value for the same input string.
func TestHashTestNameDeterministic(t *testing.T) {
	const name = "TestHashTestNameDeterministic"
	first := HashTestName(name)
	second := HashTestName(name)
	if first != second {
		t.Errorf("got %d, want %d (same input must produce same hash)", second, first)
	}
}

// TestHashTestNameNonEmpty verifies that HashTestName returns a non-zero
// value for a non-empty input.
func TestHashTestNameNonEmpty(t *testing.T) {
	got := HashTestName("TestHashTestNameNonEmpty")
	if got == 0 {
		t.Errorf("got 0, want non-zero hash for non-empty input")
	}
}

// TestHashTestNameDifferentInputs verifies that HashTestName returns
// different values for different input strings.
func TestHashTestNameDifferentInputs(t *testing.T) {
	a := HashTestName("TestHashTestNameDifferentInputs/a")
	b := HashTestName("TestHashTestNameDifferentInputs/b")
	if a == b {
		t.Errorf("got equal hashes %d for distinct inputs, want different", a)
	}
}

// TestHeartsRegistryContainsHearts verifies that HeartsRegistry returns a
// non-nil registry that contains the Hearts game.
func TestHeartsRegistryContainsHearts(t *testing.T) {
	registry := HeartsRegistry(t)
	if registry == nil {
		t.Fatal("HeartsRegistry() returned nil")
	}
	names := registry.Names()
	if len(names) != 1 {
		t.Fatalf("got %d registered games, want 1", len(names))
	}
	if names[0] != "hearts" {
		t.Errorf("got game name %q, want %q", names[0], "hearts")
	}
}

// TestHeartsRegistryValidatesHeartsConfig verifies that HeartsRegistry's
// registry accepts a valid Hearts configuration.
func TestHeartsRegistryValidatesHeartsConfig(t *testing.T) {
	registry := HeartsRegistry(t)
	cfg := HeartsSessionConfigWithPacing(10)
	if err := registry.ValidateConfig(cfg); err != nil {
		t.Errorf("ValidateConfig() error: %v, want nil", err)
	}
}

// TestHeartsRegistryRejectsInvalidConfig verifies that HeartsRegistry's
// registry rejects a configuration with the wrong number of seats.
func TestHeartsRegistryRejectsInvalidConfig(t *testing.T) {
	registry := HeartsRegistry(t)
	cfg := session.Config{
		Game: "hearts",
		Seats: []session.SeatConfig{
			{Type: session.SeatAI, AIType: "random"},
		},
	}
	if err := registry.ValidateConfig(cfg); err == nil {
		t.Error("ValidateConfig() got nil error, want error for wrong seat count")
	}
}
