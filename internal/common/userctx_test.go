package common

import (
	"context"
	"testing"
)

func TestUserContext_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// Absent by default
	if uc := UserContextFromContext(ctx); uc != nil {
		t.Error("Expected nil UserContext from empty context")
	}

	// Store and retrieve
	uc := &UserContext{
		Portfolios:      []string{"SMSF", "Trading"},
		DisplayCurrency: "AUD",
		NavexaAPIKey:    "secret",
	}
	ctx = WithUserContext(ctx, uc)

	got := UserContextFromContext(ctx)
	if got == nil {
		t.Fatal("Expected non-nil UserContext")
	}
	if len(got.Portfolios) != 2 || got.Portfolios[0] != "SMSF" {
		t.Errorf("Expected portfolios [SMSF, Trading], got %v", got.Portfolios)
	}
	if got.DisplayCurrency != "AUD" {
		t.Errorf("Expected AUD, got %s", got.DisplayCurrency)
	}
	if got.NavexaAPIKey != "secret" {
		t.Errorf("Expected secret, got %s", got.NavexaAPIKey)
	}
}

func TestResolvePortfolios_WithUserContext(t *testing.T) {
	ctx := context.Background()
	configPortfolios := []string{"ConfigDefault"}

	// No UserContext: falls back to config
	result := ResolvePortfolios(ctx, configPortfolios)
	if len(result) != 1 || result[0] != "ConfigDefault" {
		t.Errorf("Expected config fallback, got %v", result)
	}

	// With UserContext
	uc := &UserContext{Portfolios: []string{"UserA", "UserB"}}
	ctx = WithUserContext(ctx, uc)
	result = ResolvePortfolios(ctx, configPortfolios)
	if len(result) != 2 || result[0] != "UserA" {
		t.Errorf("Expected user override, got %v", result)
	}
}

func TestResolvePortfolios_EmptyUserPortfolios(t *testing.T) {
	ctx := context.Background()
	uc := &UserContext{Portfolios: []string{}}
	ctx = WithUserContext(ctx, uc)

	result := ResolvePortfolios(ctx, []string{"Fallback"})
	if len(result) != 1 || result[0] != "Fallback" {
		t.Errorf("Expected config fallback for empty user portfolios, got %v", result)
	}
}

func TestResolveDisplayCurrency_WithUserContext(t *testing.T) {
	ctx := context.Background()

	// No UserContext: falls back to config
	result := ResolveDisplayCurrency(ctx, "AUD")
	if result != "AUD" {
		t.Errorf("Expected AUD fallback, got %s", result)
	}

	// With valid UserContext
	uc := &UserContext{DisplayCurrency: "USD"}
	ctx = WithUserContext(ctx, uc)
	result = ResolveDisplayCurrency(ctx, "AUD")
	if result != "USD" {
		t.Errorf("Expected USD override, got %s", result)
	}
}

func TestResolveDisplayCurrency_InvalidCurrency(t *testing.T) {
	ctx := context.Background()
	uc := &UserContext{DisplayCurrency: "EUR"}
	ctx = WithUserContext(ctx, uc)

	result := ResolveDisplayCurrency(ctx, "AUD")
	if result != "AUD" {
		t.Errorf("Expected AUD fallback for invalid EUR, got %s", result)
	}
}

func TestResolveDisplayCurrency_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	uc := &UserContext{DisplayCurrency: "usd"}
	ctx = WithUserContext(ctx, uc)

	result := ResolveDisplayCurrency(ctx, "AUD")
	if result != "USD" {
		t.Errorf("Expected USD (uppercased), got %s", result)
	}
}

func TestNavexaClientContext_RoundTrip(t *testing.T) {
	ctx := context.Background()

	// Absent by default
	if c := NavexaClientFromContext(ctx); c != nil {
		t.Error("Expected nil NavexaClient from empty context")
	}

	// We can't easily test with a real client, but nil-safety is verified
}
