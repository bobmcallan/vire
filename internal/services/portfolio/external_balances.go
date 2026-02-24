package portfolio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/bobmcallan/vire/internal/models"
)

// generateExternalBalanceID returns a unique ID with "eb_" prefix + 8 hex chars.
func generateExternalBalanceID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback should never happen in practice
		return "eb_00000000"
	}
	return "eb_" + hex.EncodeToString(b)
}

// validateExternalBalance checks that a balance has a valid type, label, and non-negative finite value/rate.
func validateExternalBalance(b models.ExternalBalance) error {
	if !models.ValidateExternalBalanceType(b.Type) {
		return fmt.Errorf("invalid external balance type %q: must be cash, accumulate, term_deposit, or offset", b.Type)
	}
	if strings.TrimSpace(b.Label) == "" {
		return fmt.Errorf("external balance label is required")
	}
	if len(b.Label) > 200 {
		return fmt.Errorf("external balance label exceeds maximum length of 200 characters")
	}
	if len(b.Notes) > 1000 {
		return fmt.Errorf("external balance notes exceeds maximum length of 1000 characters")
	}
	if math.IsNaN(b.Value) || math.IsInf(b.Value, 0) {
		return fmt.Errorf("external balance value must be a finite number")
	}
	if b.Value < 0 {
		return fmt.Errorf("external balance value must be non-negative, got %.2f", b.Value)
	}
	if b.Value > 1e15 {
		return fmt.Errorf("external balance value exceeds maximum (1e15)")
	}
	if math.IsNaN(b.Rate) || math.IsInf(b.Rate, 0) {
		return fmt.Errorf("external balance rate must be a finite number")
	}
	if b.Rate < 0 {
		return fmt.Errorf("external balance rate must be non-negative, got %.4f", b.Rate)
	}
	return nil
}

// recomputeExternalBalanceTotal sums ExternalBalances[].Value into ExternalBalanceTotal
// and updates TotalValue to reflect holdings + external balances.
func recomputeExternalBalanceTotal(p *models.Portfolio) {
	total := 0.0
	for _, b := range p.ExternalBalances {
		total += b.Value
	}
	p.ExternalBalanceTotal = total
	p.TotalValue = p.TotalValueHoldings + p.ExternalBalanceTotal
}

// recomputeHoldingWeights recalculates holding weights using total market value + external balance total
// as the denominator, so weights reflect true portfolio allocation.
func recomputeHoldingWeights(p *models.Portfolio) {
	totalMarketValue := 0.0
	for _, h := range p.Holdings {
		totalMarketValue += h.MarketValue
	}
	weightDenom := totalMarketValue + p.ExternalBalanceTotal
	for i := range p.Holdings {
		if weightDenom > 0 {
			p.Holdings[i].Weight = (p.Holdings[i].MarketValue / weightDenom) * 100
		} else {
			p.Holdings[i].Weight = 0
		}
	}
}

// GetExternalBalances returns the external balances for a portfolio.
func (s *Service) GetExternalBalances(ctx context.Context, portfolioName string) ([]models.ExternalBalance, error) {
	portfolio, err := s.getPortfolioRecord(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to load portfolio: %w", err)
	}
	if portfolio.ExternalBalances == nil {
		return []models.ExternalBalance{}, nil
	}
	return portfolio.ExternalBalances, nil
}

// SetExternalBalances replaces all external balances, recomputes totals and weights, and saves.
func (s *Service) SetExternalBalances(ctx context.Context, portfolioName string, balances []models.ExternalBalance) (*models.Portfolio, error) {
	portfolio, err := s.getPortfolioRecord(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to load portfolio: %w", err)
	}

	// Validate and assign IDs
	for i := range balances {
		if err := validateExternalBalance(balances[i]); err != nil {
			return nil, err
		}
		if balances[i].ID == "" {
			balances[i].ID = generateExternalBalanceID()
		}
	}

	portfolio.ExternalBalances = balances
	recomputeExternalBalanceTotal(portfolio)
	recomputeHoldingWeights(portfolio)

	if err := s.savePortfolioRecord(ctx, portfolio); err != nil {
		return nil, fmt.Errorf("failed to save portfolio: %w", err)
	}

	s.logger.Info().
		Str("portfolio", portfolioName).
		Int("balances", len(balances)).
		Float64("total", portfolio.ExternalBalanceTotal).
		Msg("External balances set")

	return portfolio, nil
}

// AddExternalBalance appends a single external balance, recomputes totals and weights, and saves.
func (s *Service) AddExternalBalance(ctx context.Context, portfolioName string, balance models.ExternalBalance) (*models.Portfolio, error) {
	if err := validateExternalBalance(balance); err != nil {
		return nil, err
	}

	portfolio, err := s.getPortfolioRecord(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to load portfolio: %w", err)
	}

	balance.ID = generateExternalBalanceID()
	portfolio.ExternalBalances = append(portfolio.ExternalBalances, balance)
	recomputeExternalBalanceTotal(portfolio)
	recomputeHoldingWeights(portfolio)

	if err := s.savePortfolioRecord(ctx, portfolio); err != nil {
		return nil, fmt.Errorf("failed to save portfolio: %w", err)
	}

	s.logger.Info().
		Str("portfolio", portfolioName).
		Str("id", balance.ID).
		Str("type", balance.Type).
		Str("label", balance.Label).
		Float64("value", balance.Value).
		Msg("External balance added")

	return portfolio, nil
}

// RemoveExternalBalance removes an external balance by ID, recomputes totals and weights, and saves.
func (s *Service) RemoveExternalBalance(ctx context.Context, portfolioName string, balanceID string) (*models.Portfolio, error) {
	portfolio, err := s.getPortfolioRecord(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to load portfolio: %w", err)
	}

	found := false
	filtered := make([]models.ExternalBalance, 0, len(portfolio.ExternalBalances))
	for _, b := range portfolio.ExternalBalances {
		if b.ID == balanceID {
			found = true
			continue
		}
		filtered = append(filtered, b)
	}

	if !found {
		return nil, fmt.Errorf("external balance %q not found in portfolio %q", balanceID, portfolioName)
	}

	portfolio.ExternalBalances = filtered
	recomputeExternalBalanceTotal(portfolio)
	recomputeHoldingWeights(portfolio)

	if err := s.savePortfolioRecord(ctx, portfolio); err != nil {
		return nil, fmt.Errorf("failed to save portfolio: %w", err)
	}

	s.logger.Info().
		Str("portfolio", portfolioName).
		Str("id", balanceID).
		Msg("External balance removed")

	return portfolio, nil
}
