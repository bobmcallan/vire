package server

import (
	"strings"
	"testing"
)

func TestValidateQuoteTicker_Valid(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"BHP.AU", "BHP.AU"},
		{"AAPL.US", "AAPL.US"},
		{"XAGUSD.FOREX", "XAGUSD.FOREX"},
		{"AUDUSD.FOREX", "AUDUSD.FOREX"},
		{"xagusd.forex", "XAGUSD.FOREX"}, // lowercase normalized
		{" BHP.AU ", "BHP.AU"},            // whitespace trimmed
		{"X-Y_Z.FOREX", "X-Y_Z.FOREX"},   // hyphens and underscores allowed
	}

	for _, tt := range tests {
		result, errMsg := validateQuoteTicker(tt.input)
		if errMsg != "" {
			t.Errorf("validateQuoteTicker(%q) returned error: %s", tt.input, errMsg)
		}
		if result != tt.expected {
			t.Errorf("validateQuoteTicker(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestValidateQuoteTicker_Invalid(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"", "empty string"},
		{"BHP", "no exchange suffix"},
		{"../etc/passwd", "path traversal"},
		{"BHP;DROP.AU", "semicolon injection"},
		{"BHP AU.FOREX", "space in ticker"},
		{"BHP$.AU", "dollar sign"},
		{"BHP/.AU", "forward slash"},
	}

	for _, tt := range tests {
		_, errMsg := validateQuoteTicker(tt.input)
		if errMsg == "" {
			t.Errorf("validateQuoteTicker(%q) should reject %s", tt.input, tt.desc)
		}
	}
}

func TestValidateQuoteTicker_PathTraversal(t *testing.T) {
	// Specifically test path traversal attempts
	attacks := []string{
		"../../../etc/passwd",
		"..%2F..%2Fetc%2Fpasswd",
		"AAPL/../../../etc/passwd.US",
	}

	for _, input := range attacks {
		_, errMsg := validateQuoteTicker(input)
		if errMsg == "" {
			t.Errorf("validateQuoteTicker(%q) should reject path traversal attempt", input)
		}
	}
}

// Tests for the hardened validateTicker() used by stock endpoints

func TestValidateTicker_Valid(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"BHP.AU", "BHP.AU"},
		{"AAPL.US", "AAPL.US"},
		{"aapl.us", "AAPL.US"},
		{" BHP.AU ", "BHP.AU"},
	}

	for _, tt := range tests {
		result, errMsg := validateTicker(tt.input)
		if errMsg != "" {
			t.Errorf("validateTicker(%q) returned error: %s", tt.input, errMsg)
		}
		if result != tt.expected {
			t.Errorf("validateTicker(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestValidateTicker_RejectsPathTraversal(t *testing.T) {
	attacks := []struct {
		input string
		desc  string
	}{
		{"../../../etc/passwd", "path traversal with dots and slashes"},
		{"BHP/.AU", "forward slash"},
		{"BHP;DROP.AU", "semicolon injection"},
		{"BHP$.AU", "dollar sign"},
		{"AAPL/../../../etc/passwd.US", "embedded path traversal"},
	}

	for _, tt := range attacks {
		_, errMsg := validateTicker(tt.input)
		if errMsg == "" {
			t.Errorf("validateTicker(%q) should reject %s", tt.input, tt.desc)
		}
	}
}

func TestValidateTicker_RejectsNoSuffix(t *testing.T) {
	_, errMsg := validateTicker("BHP")
	if errMsg == "" {
		t.Error("validateTicker should reject ticker without exchange suffix")
	}
	// Error message should suggest exchanges
	if !strings.Contains(errMsg, ".AU") || !strings.Contains(errMsg, ".US") {
		t.Errorf("validateTicker error should suggest exchange suffixes, got: %s", errMsg)
	}
}
