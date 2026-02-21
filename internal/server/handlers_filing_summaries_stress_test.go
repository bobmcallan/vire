package server

import (
	"testing"
)

// ============================================================================
// validateTicker — hostile input stress tests
// ============================================================================
// The filing-summaries endpoint extracts the ticker from the URL path.
// We verify that validateTicker rejects injection attempts.

func TestStress_ValidateTicker_HostileInputs(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		reject bool // expect rejection
	}{
		// Valid tickers
		{"standard ASX", "BHP.AU", false},
		{"standard US", "AAPL.US", false},
		{"lowercase normalised", "bhp.au", false},
		{"with dash", "BHP-GROUP.AU", false},
		{"with underscore", "BHP_GROUP.AU", false},
		{"numeric", "360.AU", false},

		// Empty / blank
		{"empty string", "", true},
		{"whitespace only", "   ", true},

		// Missing exchange suffix
		{"no exchange", "BHP", true},
		{"no exchange numeric", "360", true},

		// Injection attempts
		{"XSS script tag", "<script>alert(1)</script>.AU", true},
		{"SQL injection single quote", "'; DROP TABLE stocks;--.AU", true},
		// NOTE: double-dash is valid because '-' is in the allowed charset for tickers
		{"SQL injection double dash", "BHP--comment.AU", false},
		{"path traversal dots", "../../etc/passwd.AU", true},
		{"path traversal encoded", "%2e%2e%2f.AU", true},
		{"newline injection", "BHP\n.AU", true},
		{"carriage return", "BHP\r.AU", true},
		{"null byte", "BHP\x00.AU", true},
		{"space in ticker", "B H P.AU", true},
		{"tab in ticker", "BHP\t.AU", true},
		{"pipe character", "BHP|cat /etc/passwd.AU", true},
		{"semicolon", "BHP;ls.AU", true},
		{"backtick", "BHP`whoami`.AU", true},
		{"dollar expansion", "BHP$(whoami).AU", true},
		{"angle brackets", "BHP<>.AU", true},
		{"curly braces", "BHP{}.AU", true},
		{"square brackets", "BHP[].AU", true},
		{"at sign", "BHP@evil.AU", true},
		{"hash", "BHP#.AU", true},
		{"exclamation", "BHP!.AU", true},
		{"tilde", "BHP~.AU", true},
		{"ampersand", "BHP&.AU", true},
		{"equals", "BHP=.AU", true},
		{"plus", "BHP+.AU", true},
		{"comma", "BHP,.AU", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized, errMsg := validateTicker(tt.input)
			if tt.reject {
				if errMsg == "" {
					t.Errorf("validateTicker(%q) = (%q, %q), expected rejection", tt.input, normalized, errMsg)
				}
			} else {
				if errMsg != "" {
					t.Errorf("validateTicker(%q) rejected with %q, expected valid", tt.input, errMsg)
				}
			}
		})
	}
}

// Test that validateTicker properly uppercases
func TestStress_ValidateTicker_Normalization(t *testing.T) {
	normalized, errMsg := validateTicker("bhp.au")
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if normalized != "BHP.AU" {
		t.Errorf("expected BHP.AU, got %s", normalized)
	}
}

// Test that very long tickers don't cause excessive memory allocation
func TestStress_ValidateTicker_VeryLong(t *testing.T) {
	// 10KB ticker — should still be validated character by character
	long := ""
	for i := 0; i < 10000; i++ {
		long += "A"
	}
	long += ".AU"

	normalized, errMsg := validateTicker(long)
	if errMsg != "" {
		t.Logf("Rejected long ticker (OK): %s", errMsg)
	} else {
		// If it passes, it should be uppercase
		if len(normalized) != 10003 {
			t.Errorf("unexpected length: %d", len(normalized))
		}
	}
}
