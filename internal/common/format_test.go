package common

import (
	"testing"
)

func TestFormatMoney_ExistingBehavior(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{1234.56, "$1,234.56"},
		{0, "$0.00"},
		{-500.00, "-$500.00"},
		{1000000.99, "$1,000,000.99"},
	}

	for _, tt := range tests {
		got := FormatMoney(tt.value)
		if got != tt.want {
			t.Errorf("FormatMoney(%.2f) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestFormatMoneyWithCurrency_AUD(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{1234.56, "A$1,234.56"},
		{0, "A$0.00"},
		{-500.00, "-A$500.00"},
	}

	for _, tt := range tests {
		got := FormatMoneyWithCurrency(tt.value, "AUD")
		if got != tt.want {
			t.Errorf("FormatMoneyWithCurrency(%.2f, AUD) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestFormatMoneyWithCurrency_USD(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{1234.56, "US$1,234.56"},
		{0, "US$0.00"},
		{-500.00, "-US$500.00"},
	}

	for _, tt := range tests {
		got := FormatMoneyWithCurrency(tt.value, "USD")
		if got != tt.want {
			t.Errorf("FormatMoneyWithCurrency(%.2f, USD) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestFormatMoneyWithCurrency_UnknownDefaultsToDollar(t *testing.T) {
	got := FormatMoneyWithCurrency(100.00, "GBP")
	if got != "$100.00" {
		t.Errorf("FormatMoneyWithCurrency(100, GBP) = %q, want %q (unknown currency defaults to $)", got, "$100.00")
	}
}

func TestFormatSignedMoneyWithCurrency(t *testing.T) {
	tests := []struct {
		value    float64
		currency string
		want     string
	}{
		{500.00, "AUD", "+A$500.00"},
		{-300.00, "AUD", "-A$300.00"},
		{500.00, "USD", "+US$500.00"},
		{-300.00, "USD", "-US$300.00"},
	}

	for _, tt := range tests {
		got := FormatSignedMoneyWithCurrency(tt.value, tt.currency)
		if got != tt.want {
			t.Errorf("FormatSignedMoneyWithCurrency(%.2f, %s) = %q, want %q", tt.value, tt.currency, got, tt.want)
		}
	}
}
