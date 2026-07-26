package flags

import "testing"

// TestEnvOrDefault verifies environment variable fallback behavior.
func TestEnvOrDefault(t *testing.T) {
	t.Setenv("TEST_ENV_VAR", "from-env")

	if got := EnvOrDefault("TEST_ENV_VAR", "default"); got != "from-env" {
		t.Errorf("EnvOrDefault(TEST_ENV_VAR) = %q, want %q", got, "from-env")
	}
	if got := EnvOrDefault("TEST_ENV_VAR_UNSET", "default"); got != "default" {
		t.Errorf("EnvOrDefault(TEST_ENV_VAR_UNSET) = %q, want %q", got, "default")
	}
	if got := EnvOrDefault("TEST_ENV_VAR", ""); got != "from-env" {
		t.Errorf("EnvOrDefault(TEST_ENV_VAR) = %q, want %q", got, "from-env")
	}
}

// TestIntEnvOrDefault verifies integer environment variable parsing.
func TestIntEnvOrDefault(t *testing.T) {
	t.Setenv("TEST_INT_VAR", "42")

	if got := IntEnvOrDefault("TEST_INT_VAR", 0); got != 42 {
		t.Errorf("IntEnvOrDefault(TEST_INT_VAR) = %d, want %d", got, 42)
	}
	if got := IntEnvOrDefault("TEST_INT_VAR_UNSET", 7); got != 7 {
		t.Errorf("IntEnvOrDefault(TEST_INT_VAR_UNSET) = %d, want %d", got, 7)
	}
	if got := IntEnvOrDefault("TEST_INT_VAR", -1); got != 42 {
		t.Errorf("IntEnvOrDefault(TEST_INT_VAR) = %d, want %d", got, 42)
	}

	t.Setenv("TEST_INT_VAR_NEGATIVE", "-5")
	if got := IntEnvOrDefault("TEST_INT_VAR_NEGATIVE", 10); got != 10 {
		t.Errorf("IntEnvOrDefault(TEST_INT_VAR_NEGATIVE) = %d, want %d", got, 10)
	}

	t.Setenv("TEST_INT_VAR_INVALID", "abc")
	if got := IntEnvOrDefault("TEST_INT_VAR_INVALID", 10); got != 10 {
		t.Errorf("IntEnvOrDefault(TEST_INT_VAR_INVALID) = %d, want %d", got, 10)
	}
}

// TestBoolEnvOrDefault verifies boolean environment variable parsing.
func TestBoolEnvOrDefault(t *testing.T) {
	for _, envValue := range []string{"true", "True", "TRUE", "1", "yes", "on"} {
		t.Run(envValue, func(t *testing.T) {
			t.Setenv("TEST_BOOL_VAR", envValue)
			if got := BoolEnvOrDefault("TEST_BOOL_VAR", false); got != true {
				t.Errorf("BoolEnvOrDefault(TEST_BOOL_VAR, %q) = %v, want %v", envValue, got, true)
			}
		})
	}

	for _, envValue := range []string{"false", "0", "no", "off", "anything"} {
		t.Run(envValue, func(t *testing.T) {
			t.Setenv("TEST_BOOL_VAR", envValue)
			if got := BoolEnvOrDefault("TEST_BOOL_VAR", true); got != false {
				t.Errorf("BoolEnvOrDefault(TEST_BOOL_VAR, %q) = %v, want %v", envValue, got, false)
			}
		})
	}

	if got := BoolEnvOrDefault("TEST_BOOL_VAR_UNSET", true); got != true {
		t.Errorf("BoolEnvOrDefault(TEST_BOOL_VAR_UNSET) = %v, want %v", got, true)
	}
}

// TestEnvOrDefaultEmpty returns default when env var is empty.
func TestEnvOrDefaultEmpty(t *testing.T) {
	t.Setenv("TEST_ENV_EMPTY", "")
	if got := EnvOrDefault("TEST_ENV_EMPTY", "default"); got != "default" {
		t.Errorf("EnvOrDefault(empty) = %q, want %q", got, "default")
	}
}
