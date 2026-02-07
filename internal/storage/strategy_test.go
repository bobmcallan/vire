package storage

import (
	"context"
	"testing"
	"time"

	"github.com/timshannon/badgerhold/v4"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/models"
)

func newTestStrategyStorage(t *testing.T) *strategyStorage {
	t.Helper()
	opts := badgerhold.DefaultOptions
	opts.Dir = t.TempDir()
	opts.ValueDir = opts.Dir
	opts.Logger = nil

	store, err := badgerhold.Open(opts)
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	logger := common.NewLogger("error")
	db := &BadgerDB{store: store, logger: logger}
	return newStrategyStorage(db, logger)
}

func makeTestStrategy() *models.PortfolioStrategy {
	return &models.PortfolioStrategy{
		PortfolioName:      "SMSF",
		AccountType:        models.AccountTypeSMSF,
		InvestmentUniverse: []string{"AU", "US"},
		RiskAppetite: models.RiskAppetite{
			Level:          "moderate",
			MaxDrawdownPct: 15.0,
			Description:    "Balanced growth",
		},
		TargetReturns: models.TargetReturns{
			AnnualPct: 8.5,
			Timeframe: "3-5 years",
		},
		IncomeRequirements: models.IncomeRequirements{
			DividendYieldPct: 4.0,
			Description:      "Franked dividends",
		},
		SectorPreferences: models.SectorPreferences{
			Preferred: []string{"Financials", "Healthcare"},
			Excluded:  []string{"Gambling"},
		},
		PositionSizing: models.PositionSizing{
			MaxPositionPct: 10.0,
			MaxSectorPct:   30.0,
		},
		ReferenceStrategies: []models.ReferenceStrategy{
			{Name: "Dividend Growth", Description: "Growing dividends"},
		},
		RebalanceFrequency: "quarterly",
		Notes:              "Test strategy",
	}
}

func TestStrategyStorage_SaveAndGet(t *testing.T) {
	s := newTestStrategyStorage(t)
	ctx := context.Background()

	strategy := makeTestStrategy()
	err := s.SaveStrategy(ctx, strategy)
	if err != nil {
		t.Fatalf("SaveStrategy failed: %v", err)
	}

	got, err := s.GetStrategy(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetStrategy failed: %v", err)
	}

	// Verify all fields round-trip
	if got.PortfolioName != "SMSF" {
		t.Errorf("PortfolioName = %q, want %q", got.PortfolioName, "SMSF")
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.AccountType != models.AccountTypeSMSF {
		t.Errorf("AccountType = %q, want %q", got.AccountType, models.AccountTypeSMSF)
	}
	if len(got.InvestmentUniverse) != 2 || got.InvestmentUniverse[0] != "AU" {
		t.Errorf("InvestmentUniverse = %v, want [AU US]", got.InvestmentUniverse)
	}
	if got.RiskAppetite.Level != "moderate" {
		t.Errorf("RiskAppetite.Level = %q, want %q", got.RiskAppetite.Level, "moderate")
	}
	if got.RiskAppetite.MaxDrawdownPct != 15.0 {
		t.Errorf("RiskAppetite.MaxDrawdownPct = %f, want 15.0", got.RiskAppetite.MaxDrawdownPct)
	}
	if got.TargetReturns.AnnualPct != 8.5 {
		t.Errorf("TargetReturns.AnnualPct = %f, want 8.5", got.TargetReturns.AnnualPct)
	}
	if got.IncomeRequirements.DividendYieldPct != 4.0 {
		t.Errorf("IncomeRequirements.DividendYieldPct = %f, want 4.0", got.IncomeRequirements.DividendYieldPct)
	}
	if len(got.SectorPreferences.Preferred) != 2 {
		t.Errorf("SectorPreferences.Preferred len = %d, want 2", len(got.SectorPreferences.Preferred))
	}
	if len(got.SectorPreferences.Excluded) != 1 {
		t.Errorf("SectorPreferences.Excluded len = %d, want 1", len(got.SectorPreferences.Excluded))
	}
	if got.PositionSizing.MaxPositionPct != 10.0 {
		t.Errorf("PositionSizing.MaxPositionPct = %f, want 10.0", got.PositionSizing.MaxPositionPct)
	}
	if got.PositionSizing.MaxSectorPct != 30.0 {
		t.Errorf("PositionSizing.MaxSectorPct = %f, want 30.0", got.PositionSizing.MaxSectorPct)
	}
	if len(got.ReferenceStrategies) != 1 || got.ReferenceStrategies[0].Name != "Dividend Growth" {
		t.Errorf("ReferenceStrategies = %v, want [{Dividend Growth Growing dividends}]", got.ReferenceStrategies)
	}
	if got.RebalanceFrequency != "quarterly" {
		t.Errorf("RebalanceFrequency = %q, want %q", got.RebalanceFrequency, "quarterly")
	}
	if got.Notes != "Test strategy" {
		t.Errorf("Notes = %q, want %q", got.Notes, "Test strategy")
	}
	if got.Disclaimer != models.DefaultDisclaimer {
		t.Errorf("Disclaimer = %q, want DefaultDisclaimer", got.Disclaimer)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set on first save")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set on save")
	}
}

func TestStrategyStorage_SaveNewDefaults(t *testing.T) {
	s := newTestStrategyStorage(t)
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{
		PortfolioName: "Personal",
		AccountType:   models.AccountTypeTrading,
	}
	err := s.SaveStrategy(ctx, strategy)
	if err != nil {
		t.Fatalf("SaveStrategy failed: %v", err)
	}

	got, err := s.GetStrategy(ctx, "Personal")
	if err != nil {
		t.Fatalf("GetStrategy failed: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("Version = %d, want 1 for new strategy", got.Version)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be auto-set for new strategy")
	}
	if got.Disclaimer != models.DefaultDisclaimer {
		t.Errorf("Disclaimer should default to DefaultDisclaimer, got %q", got.Disclaimer)
	}
}

func TestStrategyStorage_UpdatePreservesCreatedAt(t *testing.T) {
	s := newTestStrategyStorage(t)
	ctx := context.Background()

	strategy := makeTestStrategy()
	err := s.SaveStrategy(ctx, strategy)
	if err != nil {
		t.Fatalf("first SaveStrategy failed: %v", err)
	}

	got, err := s.GetStrategy(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetStrategy failed: %v", err)
	}
	originalCreatedAt := got.CreatedAt
	originalUpdatedAt := got.UpdatedAt

	// Wait a moment so timestamps differ
	time.Sleep(10 * time.Millisecond)

	// Update the strategy
	updated := makeTestStrategy()
	updated.RiskAppetite.Level = "aggressive"
	updated.Notes = "Updated notes"
	err = s.SaveStrategy(ctx, updated)
	if err != nil {
		t.Fatalf("second SaveStrategy failed: %v", err)
	}

	got, err = s.GetStrategy(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetStrategy after update failed: %v", err)
	}

	if got.Version != 2 {
		t.Errorf("Version = %d, want 2 after update", got.Version)
	}
	if !got.CreatedAt.Equal(originalCreatedAt) {
		t.Errorf("CreatedAt changed on update: was %v, now %v", originalCreatedAt, got.CreatedAt)
	}
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("UpdatedAt should advance on update: was %v, now %v", originalUpdatedAt, got.UpdatedAt)
	}
	if got.RiskAppetite.Level != "aggressive" {
		t.Errorf("RiskAppetite.Level = %q, want %q after update", got.RiskAppetite.Level, "aggressive")
	}
	if got.Notes != "Updated notes" {
		t.Errorf("Notes = %q, want %q after update", got.Notes, "Updated notes")
	}
}

func TestStrategyStorage_GetNotFound(t *testing.T) {
	s := newTestStrategyStorage(t)
	ctx := context.Background()

	_, err := s.GetStrategy(ctx, "nonexistent")
	if err == nil {
		t.Fatal("GetStrategy should return error for nonexistent strategy")
	}
}

func TestStrategyStorage_Delete(t *testing.T) {
	s := newTestStrategyStorage(t)
	ctx := context.Background()

	strategy := makeTestStrategy()
	err := s.SaveStrategy(ctx, strategy)
	if err != nil {
		t.Fatalf("SaveStrategy failed: %v", err)
	}

	err = s.DeleteStrategy(ctx, "SMSF")
	if err != nil {
		t.Fatalf("DeleteStrategy failed: %v", err)
	}

	_, err = s.GetStrategy(ctx, "SMSF")
	if err == nil {
		t.Fatal("GetStrategy should return error after delete")
	}
}

func TestStrategyStorage_ListStrategies(t *testing.T) {
	s := newTestStrategyStorage(t)
	ctx := context.Background()

	// Empty list
	names, err := s.ListStrategies(ctx)
	if err != nil {
		t.Fatalf("ListStrategies failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("ListStrategies should return empty for no strategies, got %v", names)
	}

	// Add two strategies
	s1 := makeTestStrategy()
	s1.PortfolioName = "SMSF"
	err = s.SaveStrategy(ctx, s1)
	if err != nil {
		t.Fatalf("SaveStrategy SMSF failed: %v", err)
	}

	s2 := &models.PortfolioStrategy{
		PortfolioName: "Personal",
		AccountType:   models.AccountTypeTrading,
	}
	err = s.SaveStrategy(ctx, s2)
	if err != nil {
		t.Fatalf("SaveStrategy Personal failed: %v", err)
	}

	names, err = s.ListStrategies(ctx)
	if err != nil {
		t.Fatalf("ListStrategies failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("ListStrategies len = %d, want 2", len(names))
	}

	// Check both names are present (order not guaranteed)
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["SMSF"] || !nameSet["Personal"] {
		t.Errorf("ListStrategies = %v, want [SMSF Personal]", names)
	}
}
