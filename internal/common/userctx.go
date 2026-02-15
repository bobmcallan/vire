package common

import (
	"context"
	"strings"

	"github.com/bobmcallan/vire/internal/interfaces"
)

// UserContext holds per-request user configuration injected via X-Vire-* headers.
// When present, values override server-side config defaults. When absent (nil),
// the server operates in single-tenant mode using config file values.
type UserContext struct {
	Portfolios      []string
	DisplayCurrency string
	NavexaAPIKey    string
	UserID          string
}

type contextKey int

const (
	userContextKey       contextKey = iota
	navexaClientOverride contextKey = iota
)

// WithUserContext stores a UserContext in the request context.
func WithUserContext(ctx context.Context, uc *UserContext) context.Context {
	return context.WithValue(ctx, userContextKey, uc)
}

// UserContextFromContext retrieves the UserContext from context, or nil if absent.
func UserContextFromContext(ctx context.Context) *UserContext {
	uc, _ := ctx.Value(userContextKey).(*UserContext)
	return uc
}

// WithNavexaClient stores a per-request NavexaClient override in context.
func WithNavexaClient(ctx context.Context, client interfaces.NavexaClient) context.Context {
	return context.WithValue(ctx, navexaClientOverride, client)
}

// NavexaClientFromContext retrieves a per-request NavexaClient override, or nil.
func NavexaClientFromContext(ctx context.Context) interfaces.NavexaClient {
	c, _ := ctx.Value(navexaClientOverride).(interfaces.NavexaClient)
	return c
}

// ResolvePortfolios returns user-context portfolios if present, otherwise config fallback.
func ResolvePortfolios(ctx context.Context, configPortfolios []string) []string {
	if uc := UserContextFromContext(ctx); uc != nil && len(uc.Portfolios) > 0 {
		return uc.Portfolios
	}
	return configPortfolios
}

// ResolveDisplayCurrency returns user-context display currency if present and valid,
// otherwise config fallback. Validates AUD/USD only.
func ResolveDisplayCurrency(ctx context.Context, configCurrency string) string {
	if uc := UserContextFromContext(ctx); uc != nil && uc.DisplayCurrency != "" {
		dc := strings.ToUpper(uc.DisplayCurrency)
		if dc == "AUD" || dc == "USD" {
			return dc
		}
	}
	return configCurrency
}
