package tm

import "testing"

// devMode decides whether a password-reset token is echoed back in an API
// response, so getting it wrong leaks account-takeover tokens to anyone who
// knows an email address. It must fail safe: only an explicit development ENV
// counts as dev.
func TestDevModeFailsSafe(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{
		{"development", true},
		{"dev", true},
		{"local", true},
		{"test", true},
		{"DEV", true}, // case-insensitive
		{"Development", true},
		{"production", false},
		{"Production", false},
		{"staging", false},
		{"prod", false},       // not a recognised dev value
		{"", false},           // unset: treated as production
		{"developmnt", false}, // typo must not open the gate
	} {
		t.Setenv("ENV", tc.env)
		if got := devMode(); got != tc.want {
			t.Errorf("ENV=%q: devMode() = %v, want %v", tc.env, got, tc.want)
		}
	}
}
