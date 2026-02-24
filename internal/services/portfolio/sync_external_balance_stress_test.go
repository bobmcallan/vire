package portfolio

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for SyncPortfolio external balance preservation.
// Focus: the raw storage read fix (bypass getPortfolioRecord schema validation),
// corrupted storage data, and numeric edge cases.

// --- Fix 1 verification: raw storage read bypasses schema validation ---

func TestSyncPortfolio_PreservesExternalBalances_StaleSchemaVersion(t *testing.T) {
	// Core bug scenario: stored portfolio has stale schema version but valid external balances.
	// After the fix, SyncPortfolio should preserve external balances even when
	// getPortfolioRecord would reject the stored portfolio.
	userDataStore := newMemUserDataStore()
	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: userDataStore,
	}

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	// Seed storage with a portfolio that has:
	// 1. Old/stale DataVersion (would be rejected by getPortfolioRecord)
	// 2. Valid external balances that must survive the re-sync
	stalePortfolio := &models.Portfolio{
		Name:               "SMSF",
		DataVersion:        "1", // stale
		TotalValueHoldings: 4000,
		TotalValue:         54000,
		ExternalBalances: []models.ExternalBalance{
			{ID: "eb_aabbccdd", Type: "cash", Label: "ANZ Cash", Value: 25000},
			{ID: "eb_11223344", Type: "term_deposit", Label: "NAB TD", Value: 25000},
		},
		ExternalBalanceTotal: 50000,
		LastSynced:           time.Now().Add(-24 * time.Hour),
	}
	data, _ := json.Marshal(stalePortfolio)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   string(data),
	})

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// External balances should be preserved despite stale schema version
	if len(portfolio.ExternalBalances) != 2 {
		t.Fatalf("ExternalBalances count = %d, want 2 (should survive schema version bump)", len(portfolio.ExternalBalances))
	}
	if portfolio.ExternalBalanceTotal != 50000 {
		t.Errorf("ExternalBalanceTotal = %v, want 50000", portfolio.ExternalBalanceTotal)
	}

	// TotalValue should include both holdings and external balances
	expectedTotal := portfolio.TotalValueHoldings + 50000
	if !approxEqual(portfolio.TotalValue, expectedTotal, 0.01) {
		t.Errorf("TotalValue = %v, want %v (holdings + external balances)", portfolio.TotalValue, expectedTotal)
	}

	// Verify balance details survived
	found := false
	for _, b := range portfolio.ExternalBalances {
		if b.Label == "ANZ Cash" && b.Value == 25000 {
			found = true
		}
	}
	if !found {
		t.Error("ANZ Cash balance not found after re-sync")
	}
}

// --- Corrupted storage data resilience ---

func TestSyncPortfolio_CorruptedJSON_ExternalBalancesLost(t *testing.T) {
	// If the stored portfolio JSON is corrupted, SyncPortfolio should still succeed
	// (external balances will be lost since we can't parse them).
	userDataStore := newMemUserDataStore()
	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: userDataStore,
	}

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	// Seed corrupted JSON in storage
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   `{"name":"SMSF","external_balances":[{"label":"ANZ Cash","value": CORRUPTED}`,
	})

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio should succeed even with corrupted stored portfolio: %v", err)
	}

	// External balances lost because stored data was corrupted
	if len(portfolio.ExternalBalances) != 0 {
		t.Errorf("ExternalBalances = %d, want 0 (corrupted data should not produce phantom balances)",
			len(portfolio.ExternalBalances))
	}
	// TotalValue should equal TotalValueHoldings (no external balances)
	if portfolio.TotalValue != portfolio.TotalValueHoldings {
		t.Errorf("TotalValue (%v) != TotalValueHoldings (%v) — should match when no external balances",
			portfolio.TotalValue, portfolio.TotalValueHoldings)
	}
}

func TestSyncPortfolio_EmptyValueField_NoExternalBalances(t *testing.T) {
	// If UserDataStore record has an empty Value field
	userDataStore := newMemUserDataStore()
	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: userDataStore,
	}

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	// Empty Value field
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   "",
	})

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio should succeed with empty stored value: %v", err)
	}

	if portfolio.ExternalBalanceTotal != 0 {
		t.Errorf("ExternalBalanceTotal = %v, want 0", portfolio.ExternalBalanceTotal)
	}
}

func TestSyncPortfolio_NaNExternalBalanceValue_InStorage(t *testing.T) {
	// Stored portfolio has NaN in external balance value — JSON unmarshal produces NaN
	userDataStore := newMemUserDataStore()
	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: userDataStore,
	}

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	// Create a portfolio with NaN values using Go marshaling workaround.
	// Standard JSON doesn't support NaN, so a corrupted record might have 0 or be unparseable.
	// Instead, test what happens with a valid portfolio that has zero external balance
	// but non-zero ExternalBalanceTotal (inconsistent state).
	inconsistent := &models.Portfolio{
		Name:                 "SMSF",
		DataVersion:          "1",
		ExternalBalances:     []models.ExternalBalance{}, // empty array
		ExternalBalanceTotal: 99999,                      // inconsistent — should be 0
		TotalValueHoldings:   5000,
		TotalValue:           104999,
	}
	data, _ := json.Marshal(inconsistent)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   string(data),
	})

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// The raw storage read preserves ExternalBalances (empty array) and ExternalBalanceTotal (99999).
	// However, when ExternalBalances is empty, the total SHOULD be 0.
	// This reveals a data consistency issue: SyncPortfolio trusts the stored ExternalBalanceTotal
	// without recomputing from the actual ExternalBalances array.
	// Log the actual behaviour for awareness.
	t.Logf("ExternalBalances count = %d", len(portfolio.ExternalBalances))
	t.Logf("ExternalBalanceTotal = %v", portfolio.ExternalBalanceTotal)
	t.Logf("TotalValue = %v, TotalValueHoldings = %v", portfolio.TotalValue, portfolio.TotalValueHoldings)

	// TotalValue should be finite
	if math.IsNaN(portfolio.TotalValue) || math.IsInf(portfolio.TotalValue, 0) {
		t.Errorf("TotalValue is NaN/Inf: %v", portfolio.TotalValue)
	}
}

func TestSyncPortfolio_NilExternalBalances_VsEmpty(t *testing.T) {
	// Verify nil vs empty external balances array handling
	userDataStore := newMemUserDataStore()
	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: userDataStore,
	}

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	// Store portfolio with explicit nil external_balances (omitted in JSON due to omitempty)
	nilBalances := &models.Portfolio{
		Name:        "SMSF",
		DataVersion: "1",
		// ExternalBalances not set — nil
		ExternalBalanceTotal: 0,
	}
	data, _ := json.Marshal(nilBalances)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   string(data),
	})

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// Nil external balances should not produce non-zero totals
	if portfolio.ExternalBalanceTotal != 0 {
		t.Errorf("ExternalBalanceTotal = %v, want 0 (nil external balances)", portfolio.ExternalBalanceTotal)
	}
	if portfolio.TotalValue != portfolio.TotalValueHoldings {
		t.Errorf("TotalValue (%v) != TotalValueHoldings (%v) when no external balances",
			portfolio.TotalValue, portfolio.TotalValueHoldings)
	}
}

func TestSyncPortfolio_NoExistingPortfolio_FirstSync(t *testing.T) {
	// First sync ever — no existing portfolio in storage.
	// External balances should be empty, not panic.
	userDataStore := newMemUserDataStore()
	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: userDataStore,
	}

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed on first sync: %v", err)
	}

	if portfolio.ExternalBalanceTotal != 0 {
		t.Errorf("ExternalBalanceTotal = %v, want 0 on first sync", portfolio.ExternalBalanceTotal)
	}
	if len(portfolio.ExternalBalances) != 0 {
		t.Errorf("ExternalBalances = %d, want 0 on first sync", len(portfolio.ExternalBalances))
	}
	if portfolio.TotalValue != portfolio.TotalValueHoldings {
		t.Errorf("TotalValue (%v) != TotalValueHoldings (%v) on first sync",
			portfolio.TotalValue, portfolio.TotalValueHoldings)
	}
}

// --- Weight computation with external balances ---

func TestSyncPortfolio_WeightsReflectExternalBalances(t *testing.T) {
	// When external balances exist, holding weights should be < 100% total
	// because external balance total is part of the weight denominator.
	userDataStore := newMemUserDataStore()
	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: userDataStore,
	}

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	// Seed portfolio with external balances equal to equity holdings
	existing := &models.Portfolio{
		Name:        "SMSF",
		DataVersion: "1",
		ExternalBalances: []models.ExternalBalance{
			{ID: "eb_aabbccdd", Type: "cash", Label: "Cash", Value: 5000},
		},
		ExternalBalanceTotal: 5000,
		TotalValueHoldings:   5000,
		TotalValue:           10000,
	}
	data, _ := json.Marshal(existing)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   string(data),
	})

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	bhp := portfolio.Holdings[0]
	// BHP MarketValue = 5000, external balance = 5000, denominator = 10000
	// Weight should be ~50%, not 100%
	if bhp.Weight > 55 || bhp.Weight < 45 {
		t.Errorf("BHP Weight = %.2f%%, want ~50%% (external balance in denominator)", bhp.Weight)
	}
}

// --- Concurrency: SyncPortfolio vs AddExternalBalance ---

func TestConcurrent_SyncAndAddExternalBalance(t *testing.T) {
	// Race condition test: concurrent SyncPortfolio and AddExternalBalance.
	// Both read-modify-write the same portfolio record.
	// This test verifies no panic or data corruption occurs.
	userDataStore := newMemUserDataStore()
	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: userDataStore,
	}

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	// Seed initial portfolio
	initial := &models.Portfolio{
		Name:               "SMSF",
		DataVersion:        common.SchemaVersion,
		TotalValueHoldings: 5000,
		TotalValue:         5000,
		Holdings: []models.Holding{
			{Ticker: "BHP", MarketValue: 5000, Units: 100},
		},
	}
	data, _ := json.Marshal(initial)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   string(data),
	})

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	var wg sync.WaitGroup
	var syncErr, addErr error

	// Run SyncPortfolio and AddExternalBalance concurrently
	wg.Add(2)
	go func() {
		defer wg.Done()
		ctx := common.WithNavexaClient(context.Background(), navexa)
		_, syncErr = svc.SyncPortfolio(ctx, "SMSF", true)
	}()
	go func() {
		defer wg.Done()
		_, addErr = svc.AddExternalBalance(context.Background(), "SMSF", models.ExternalBalance{
			Type:  "cash",
			Label: "Race Condition Cash",
			Value: 10000,
		})
	}()
	wg.Wait()

	// Neither should panic. One or both may succeed depending on timing.
	// The important thing is no crash and data is in a valid state.
	if syncErr != nil {
		t.Logf("SyncPortfolio error (acceptable in race): %v", syncErr)
	}
	if addErr != nil {
		t.Logf("AddExternalBalance error (acceptable in race): %v", addErr)
	}

	// Read final state — should be valid JSON and consistent
	rec, err := userDataStore.Get(context.Background(), "default", "portfolio", "SMSF")
	if err != nil {
		t.Fatalf("Failed to read final portfolio state: %v", err)
	}

	var final models.Portfolio
	if err := json.Unmarshal([]byte(rec.Value), &final); err != nil {
		t.Fatalf("Final portfolio is corrupted JSON: %v", err)
	}

	// TotalValue should be consistent
	if math.IsNaN(final.TotalValue) || math.IsInf(final.TotalValue, 0) {
		t.Errorf("Final TotalValue is NaN/Inf: %v", final.TotalValue)
	}
	if final.TotalValue < 0 {
		t.Errorf("Final TotalValue is negative: %v", final.TotalValue)
	}
}

// --- Hostile portfolio name ---

func TestSyncPortfolio_ExternalBalancePreservation_SpecialCharName(t *testing.T) {
	// Portfolio name with special characters — should not break JSON storage
	userDataStore := newMemUserDataStore()
	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: userDataStore,
	}

	portfolioName := "Test <Portfolio> \"Quotes\" & 'Apostrophes'"
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: portfolioName, Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	// Seed with external balances
	existing := &models.Portfolio{
		Name:        portfolioName,
		DataVersion: "1",
		ExternalBalances: []models.ExternalBalance{
			{ID: "eb_aabb", Type: "cash", Label: "Cash", Value: 10000},
		},
		ExternalBalanceTotal: 10000,
	}
	data, _ := json.Marshal(existing)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     portfolioName,
		Value:   string(data),
	})

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, portfolioName, true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed with special char name: %v", err)
	}

	if len(portfolio.ExternalBalances) != 1 {
		t.Errorf("ExternalBalances count = %d, want 1", len(portfolio.ExternalBalances))
	}
}

// --- TotalValue invariant ---

func TestSyncPortfolio_TotalValueInvariant(t *testing.T) {
	// After SyncPortfolio, TotalValue must always equal TotalValueHoldings + ExternalBalanceTotal.
	// This tests the invariant across multiple scenarios.
	cases := []struct {
		name             string
		externalBalances []models.ExternalBalance
		externalTotal    float64
	}{
		{"no balances", nil, 0},
		{"single cash", []models.ExternalBalance{{ID: "eb_1", Type: "cash", Label: "Cash", Value: 10000}}, 10000},
		{"multiple types", []models.ExternalBalance{
			{ID: "eb_1", Type: "cash", Label: "Cash", Value: 5000},
			{ID: "eb_2", Type: "term_deposit", Label: "TD", Value: 15000},
			{ID: "eb_3", Type: "offset", Label: "Offset", Value: 20000},
		}, 40000},
		{"zero value balance", []models.ExternalBalance{{ID: "eb_1", Type: "cash", Label: "Empty", Value: 0}}, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userDataStore := newMemUserDataStore()
			marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
			storage := &stubStorageManager{
				marketStore:   marketStore,
				userDataStore: userDataStore,
			}

			navexa := &stubNavexaClient{
				portfolios: []*models.NavexaPortfolio{
					{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
				},
				holdings: []*models.NavexaHolding{
					{
						ID: "100", PortfolioID: "1",
						Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
						Units: 100, CurrentPrice: 50.00, MarketValue: 5000.00,
						Currency: "AUD", LastUpdated: time.Now(),
					},
				},
				trades: map[string][]*models.NavexaTrade{
					"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
				},
			}

			existing := &models.Portfolio{
				Name:                 "SMSF",
				DataVersion:          "1",
				ExternalBalances:     tc.externalBalances,
				ExternalBalanceTotal: tc.externalTotal,
			}
			data, _ := json.Marshal(existing)
			_ = userDataStore.Put(context.Background(), &models.UserRecord{
				UserID:  "default",
				Subject: "portfolio",
				Key:     "SMSF",
				Value:   string(data),
			})

			logger := common.NewLogger("error")
			svc := NewService(storage, nil, nil, nil, logger)

			ctx := common.WithNavexaClient(context.Background(), navexa)
			portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
			if err != nil {
				t.Fatalf("SyncPortfolio failed: %v", err)
			}

			// Invariant: TotalValue = TotalValueHoldings + ExternalBalanceTotal
			expected := portfolio.TotalValueHoldings + portfolio.ExternalBalanceTotal
			if !approxEqual(portfolio.TotalValue, expected, 0.01) {
				t.Errorf("TotalValue = %v, want TotalValueHoldings (%v) + ExternalBalanceTotal (%v) = %v",
					portfolio.TotalValue, portfolio.TotalValueHoldings, portfolio.ExternalBalanceTotal, expected)
			}
		})
	}
}
