package models

import "testing"

func TestExternalBalance_AssetCategory(t *testing.T) {
	types := []string{"cash", "accumulate", "term_deposit", "offset"}
	for _, typ := range types {
		eb := ExternalBalance{Type: typ, Label: "test", Value: 1000}
		got := eb.AssetCategory()
		if got != "cash" {
			t.Errorf("ExternalBalance{Type: %q}.AssetCategory() = %q, want %q", typ, got, "cash")
		}
	}
}

func TestExternalBalance_AssetCategory_AlwaysCash(t *testing.T) {
	// Even for exotic or empty types, AssetCategory always returns "cash"
	eb := ExternalBalance{}
	if eb.AssetCategory() != "cash" {
		t.Errorf("zero-value ExternalBalance.AssetCategory() = %q, want %q", eb.AssetCategory(), "cash")
	}
}

func TestValidateExternalBalanceType(t *testing.T) {
	valid := []string{"cash", "accumulate", "term_deposit", "offset"}
	for _, typ := range valid {
		if !ValidateExternalBalanceType(typ) {
			t.Errorf("ValidateExternalBalanceType(%q) = false, want true", typ)
		}
	}

	invalid := []string{"", "equity", "stock", "savings"}
	for _, typ := range invalid {
		if ValidateExternalBalanceType(typ) {
			t.Errorf("ValidateExternalBalanceType(%q) = true, want false", typ)
		}
	}
}

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
