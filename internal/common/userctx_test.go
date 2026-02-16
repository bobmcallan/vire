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
		UserID:          "user-123",
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
	if got.UserID != "user-123" {
		t.Errorf("Expected user-123, got %s", got.UserID)
	}
}

func TestResolvePortfolios_WithUserContext(t *testing.T) {
	ctx := context.Background()

	// No UserContext: returns nil
	result := ResolvePortfolios(ctx)
	if result != nil {
		t.Errorf("Expected nil, got %v", result)
	}

	// With UserContext
	uc := &UserContext{Portfolios: []string{"UserA", "UserB"}}
	ctx = WithUserContext(ctx, uc)
	result = ResolvePortfolios(ctx)
	if len(result) != 2 || result[0] != "UserA" {
		t.Errorf("Expected user override, got %v", result)
	}
}

func TestResolvePortfolios_EmptyUserPortfolios(t *testing.T) {
	ctx := context.Background()
	uc := &UserContext{Portfolios: []string{}}
	ctx = WithUserContext(ctx, uc)

	result := ResolvePortfolios(ctx)
	if result != nil {
		t.Errorf("Expected nil for empty user portfolios, got %v", result)
	}
}

func TestResolveDisplayCurrency_WithUserContext(t *testing.T) {
	ctx := context.Background()

	// No UserContext: defaults to AUD
	result := ResolveDisplayCurrency(ctx)
	if result != "AUD" {
		t.Errorf("Expected AUD default, got %s", result)
	}

	// With valid UserContext
	uc := &UserContext{DisplayCurrency: "USD"}
	ctx = WithUserContext(ctx, uc)
	result = ResolveDisplayCurrency(ctx)
	if result != "USD" {
		t.Errorf("Expected USD override, got %s", result)
	}
}

func TestResolveDisplayCurrency_InvalidCurrency(t *testing.T) {
	ctx := context.Background()
	uc := &UserContext{DisplayCurrency: "EUR"}
	ctx = WithUserContext(ctx, uc)

	result := ResolveDisplayCurrency(ctx)
	if result != "AUD" {
		t.Errorf("Expected AUD fallback for invalid EUR, got %s", result)
	}
}

func TestResolveDisplayCurrency_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	uc := &UserContext{DisplayCurrency: "usd"}
	ctx = WithUserContext(ctx, uc)

	result := ResolveDisplayCurrency(ctx)
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
