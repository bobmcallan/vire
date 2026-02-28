package models

import "testing"

func TestEodhExchange(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ASX", "AU"},
		{"AU", "AU"},
		{"NYSE", "US"},
		{"NASDAQ", "US"},
		{"LSE", "LSE"},
		{"", "AU"},
	}
	for _, tt := range tests {
		got := EodhExchange(tt.input)
		if got != tt.want {
			t.Errorf("EodhExchange(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
