package portfolio

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// --- DividendReturn stress tests ---

// TestDividendReturn_NegativeDividends tests clawback/negative dividend scenarios.
// Navexa can report negative DividendReturn when a dividend is reversed or corrected.
func TestDividendReturn_NegativeDividends(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP Group", Units: 100, CurrentPrice: 50.00,
				MarketValue: 5000, TotalCost: 5000,
				DividendReturn: 200.00,
				LastUpdated:    time.Now(),
			},
			{
				ID: "102", PortfolioID: "1", Ticker: "CBA", Exchange: "AU",
				Name: "CBA", Units: 50, CurrentPrice: 100.00,
				MarketValue: 5000, TotalCost: 4500,
				DividendReturn: -50.00, // clawback / correction
				LastUpdated:    time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{Type: "buy", Units: 100, Price: 50.00, Fees: 0, Date: "2023-01-01"}},
			"102": {{Type: "buy", Units: 50, Price: 90.00, Fees: 0, Date: "2023-01-01"}},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	ctx := common.WithNavexaClient(context.Background(), navexa)

	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// 200 + (-50) = 150
	want := 150.0
	if !approxEqual(portfolio.DividendReturn, want, 0.01) {
		t.Errorf("DividendReturn = %.2f, want %.2f (negative dividends should reduce total)", portfolio.DividendReturn, want)
	}

	// DividendReturn must still be included in NetEquityReturn
	if math.IsNaN(portfolio.NetEquityReturn) || math.IsInf(portfolio.NetEquityReturn, 0) {
		t.Errorf("NetEquityReturn is NaN/Inf with negative dividends")
	}
}

// TestDividendReturn_AllNegative tests when all holdings have negative dividends.
func TestDividendReturn_AllNegative(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP", Units: 100, CurrentPrice: 50.00,
				MarketValue: 5000, TotalCost: 5000,
				DividendReturn: -100.00,
				LastUpdated:    time.Now(),
			},
			{
				ID: "102", PortfolioID: "1", Ticker: "CBA", Exchange: "AU",
				Name: "CBA", Units: 50, CurrentPrice: 100.00,
				MarketValue: 5000, TotalCost: 4500,
				DividendReturn: -75.50,
				LastUpdated:    time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{Type: "buy", Units: 100, Price: 50.00, Fees: 0, Date: "2023-01-01"}},
			"102": {{Type: "buy", Units: 50, Price: 90.00, Fees: 0, Date: "2023-01-01"}},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	ctx := common.WithNavexaClient(context.Background(), navexa)

	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	want := -175.50
	if !approxEqual(portfolio.DividendReturn, want, 0.01) {
		t.Errorf("DividendReturn = %.2f, want %.2f", portfolio.DividendReturn, want)
	}
	if portfolio.DividendReturn >= 0 {
		t.Errorf("DividendReturn should be negative when all dividends are negative")
	}
}

// TestDividendReturn_FXConversion_MatchesSumOfConvertedHoldings verifies that
// DividendReturn == sum(holding.DividendReturn) AFTER FX conversion.
// This catches any scenario where FX is applied to holdings but not to the total.
func TestDividendReturn_FXConversion_MatchesSumOfConvertedHoldings(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP", Units: 100, CurrentPrice: 50.00, MarketValue: 5000,
				TotalCost: 4000, DividendReturn: 300.00, // AUD — no conversion
				Currency: "AUD", LastUpdated: time.Now(),
			},
			{
				ID: "101", PortfolioID: "1", Ticker: "AAPL", Exchange: "US",
				Name: "Apple", Units: 10, CurrentPrice: 200.00, MarketValue: 2000,
				TotalCost: 1500, DividendReturn: 50.00, // USD — will be converted
				Currency: "USD", LastUpdated: time.Now(),
			},
			{
				ID: "102", PortfolioID: "1", Ticker: "MSFT", Exchange: "US",
				Name: "Microsoft", Units: 5, CurrentPrice: 400.00, MarketValue: 2000,
				TotalCost: 1800, DividendReturn: 25.00, // USD — will be converted
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{Type: "buy", Units: 100, Price: 40.00, Fees: 0, Date: "2023-01-01"}},
			"101": {{Type: "buy", Units: 10, Price: 150.00, Fees: 0, Date: "2023-01-01"}},
			"102": {{Type: "buy", Units: 5, Price: 360.00, Fees: 0, Date: "2023-01-01"}},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}
	eodhd := &fxStubEODHDClient{forexRate: 0.6250}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)
	ctx := common.WithNavexaClient(context.Background(), navexa)

	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// Verify DividendReturn matches sum of holding-level DividendReturn
	var holdingDivSum float64
	for _, h := range portfolio.Holdings {
		holdingDivSum += h.DividendReturn
	}

	if !approxEqual(portfolio.DividendReturn, holdingDivSum, 0.01) {
		t.Errorf("DividendReturn (%.2f) != sum of holding DividendReturn (%.2f) — FX conversion inconsistency",
			portfolio.DividendReturn, holdingDivSum)
	}

	// Verify FX conversion was actually applied (USD dividends should be larger in AUD)
	// US$50 / 0.625 = A$80, US$25 / 0.625 = A$40
	// Total = 300 (AUD) + 80 (converted) + 40 (converted) = 420
	expectedTotal := 300.0 + 50.0/0.625 + 25.0/0.625
	if !approxEqual(portfolio.DividendReturn, expectedTotal, 0.01) {
		t.Errorf("DividendReturn = %.2f, want %.2f (300 AUD + 80 + 40 AUD from FX)", portfolio.DividendReturn, expectedTotal)
	}
}

// TestDividendReturn_FXConversionFailed_StaysUSD verifies behavior when FX conversion
// fails (eodhd returns 0 or error). Holdings stay in USD, total should still match sum.
func TestDividendReturn_FXConversionFailed_StaysUSD(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP", Units: 100, CurrentPrice: 50.00, MarketValue: 5000,
				TotalCost: 4000, DividendReturn: 300.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
			{
				ID: "101", PortfolioID: "1", Ticker: "AAPL", Exchange: "US",
				Name: "Apple", Units: 10, CurrentPrice: 200.00, MarketValue: 2000,
				TotalCost: 1500, DividendReturn: 50.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{Type: "buy", Units: 100, Price: 40.00, Fees: 0, Date: "2023-01-01"}},
			"101": {{Type: "buy", Units: 10, Price: 150.00, Fees: 0, Date: "2023-01-01"}},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}
	eodhd := &fxStubEODHDClient{forexRate: 0.0} // zero rate — FX fails
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)
	ctx := common.WithNavexaClient(context.Background(), navexa)

	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// DividendReturn should still equal sum of holding dividends (unconverted)
	var holdingDivSum float64
	for _, h := range portfolio.Holdings {
		holdingDivSum += h.DividendReturn
	}
	if !approxEqual(portfolio.DividendReturn, holdingDivSum, 0.01) {
		t.Errorf("DividendReturn (%.2f) != sum of holding DividendReturn (%.2f) — even when FX fails, total must match holding sum",
			portfolio.DividendReturn, holdingDivSum)
	}
}

// TestDividendReturn_JSONRoundTrip verifies the field survives marshal/unmarshal.
func TestDividendReturn_JSONRoundTrip(t *testing.T) {
	original := &models.Portfolio{
		Name:                   "TEST",
		DividendReturn:         1234.56,
		NetEquityReturn:        5000.00,
		RealizedEquityReturn:   2000.00,
		UnrealizedEquityReturn: 1765.44,
		DataVersion:            common.SchemaVersion,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var restored models.Portfolio
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !approxEqual(restored.DividendReturn, original.DividendReturn, 0.001) {
		t.Errorf("DividendReturn after round-trip = %.2f, want %.2f",
			restored.DividendReturn, original.DividendReturn)
	}
}

// TestDividendReturn_JSONRoundTrip_Zero verifies zero value survives round-trip
// (since there is no omitempty on the field).
func TestDividendReturn_JSONRoundTrip_Zero(t *testing.T) {
	original := &models.Portfolio{
		Name:           "TEST",
		DividendReturn: 0.0,
		DataVersion:    common.SchemaVersion,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify zero value is present in JSON (not omitted)
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	if _, exists := raw["dividend_return"]; !exists {
		t.Error("dividend_return not present in JSON output — zero value should not be omitted")
	}

	var restored models.Portfolio
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.DividendReturn != 0.0 {
		t.Errorf("DividendReturn after round-trip = %.4f, want 0.0", restored.DividendReturn)
	}
}

// TestDividendReturn_CachedPortfolio_ConsistencyAfterResync verifies that
// DividendReturn is correctly updated when a cached portfolio is re-synced
// with different dividend values.
func TestDividendReturn_CachedPortfolio_ConsistencyAfterResync(t *testing.T) {
	userDataStore := newMemUserDataStore()
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: userDataStore,
	}

	// First sync: holdings with dividends
	navexa1 := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP", Units: 100, CurrentPrice: 50.00,
				MarketValue: 5000, TotalCost: 5000,
				DividendReturn: 500.00,
				LastUpdated:    time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{Type: "buy", Units: 100, Price: 50.00, Fees: 0, Date: "2023-01-01"}},
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx1 := common.WithNavexaClient(context.Background(), navexa1)
	portfolio1, err := svc.SyncPortfolio(ctx1, "SMSF", true)
	if err != nil {
		t.Fatalf("First SyncPortfolio failed: %v", err)
	}
	if !approxEqual(portfolio1.DividendReturn, 500.00, 0.01) {
		t.Errorf("First sync: DividendReturn = %.2f, want 500.00", portfolio1.DividendReturn)
	}

	// Second sync: dividends changed
	navexa2 := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP", Units: 100, CurrentPrice: 52.00,
				MarketValue: 5200, TotalCost: 5000,
				DividendReturn: 750.00, // increased
				LastUpdated:    time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{Type: "buy", Units: 100, Price: 50.00, Fees: 0, Date: "2023-01-01"}},
		},
	}

	// Backdate LastSynced so the 5-minute cooldown doesn't block the second sync
	existing, err := svc.getPortfolioRecord(context.Background(), "SMSF")
	if err != nil {
		t.Fatalf("getPortfolioRecord failed: %v", err)
	}
	existing.LastSynced = time.Now().Add(-6 * time.Minute)
	if err := svc.savePortfolioRecord(context.Background(), existing); err != nil {
		t.Fatalf("savePortfolioRecord failed: %v", err)
	}

	ctx2 := common.WithNavexaClient(context.Background(), navexa2)
	portfolio2, err := svc.SyncPortfolio(ctx2, "SMSF", true)
	if err != nil {
		t.Fatalf("Second SyncPortfolio failed: %v", err)
	}

	// DividendReturn should reflect the new dividend value, not the cached one
	if !approxEqual(portfolio2.DividendReturn, 750.00, 0.01) {
		t.Errorf("After re-sync: DividendReturn = %.2f, want 750.00 (should not be stale cached value of 500)",
			portfolio2.DividendReturn)
	}

	// Verify it's stored correctly by reading back from the store
	portfolio3, err := svc.GetPortfolio(ctx2, "SMSF")
	if err != nil {
		t.Fatalf("GetPortfolio failed: %v", err)
	}
	if !approxEqual(portfolio3.DividendReturn, 750.00, 0.01) {
		t.Errorf("GetPortfolio after re-sync: DividendReturn = %.2f, want 750.00",
			portfolio3.DividendReturn)
	}
}

// TestDividendReturn_EmptyPortfolio verifies behavior with no holdings.
func TestDividendReturn_EmptyPortfolio(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{},
		trades:   map[string][]*models.NavexaTrade{},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	ctx := common.WithNavexaClient(context.Background(), navexa)

	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	if portfolio.DividendReturn != 0.0 {
		t.Errorf("DividendReturn = %.4f, want 0.0 for empty portfolio", portfolio.DividendReturn)
	}
}

// TestDividendReturn_VeryLargeDividends verifies no overflow with large values.
func TestDividendReturn_VeryLargeDividends(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP", Units: 1000000, CurrentPrice: 50.00,
				MarketValue: 50000000, TotalCost: 40000000,
				DividendReturn: 9999999999.99,
				LastUpdated:    time.Now(),
			},
			{
				ID: "102", PortfolioID: "1", Ticker: "CBA", Exchange: "AU",
				Name: "CBA", Units: 1000000, CurrentPrice: 100.00,
				MarketValue: 100000000, TotalCost: 80000000,
				DividendReturn: 8888888888.88,
				LastUpdated:    time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{Type: "buy", Units: 1000000, Price: 40.00, Fees: 0, Date: "2023-01-01"}},
			"102": {{Type: "buy", Units: 1000000, Price: 80.00, Fees: 0, Date: "2023-01-01"}},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	ctx := common.WithNavexaClient(context.Background(), navexa)

	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	if math.IsInf(portfolio.DividendReturn, 0) || math.IsNaN(portfolio.DividendReturn) {
		t.Errorf("DividendReturn is Inf/NaN with large dividend values")
	}

	expected := 9999999999.99 + 8888888888.88
	if !approxEqual(portfolio.DividendReturn, expected, 1.0) {
		t.Errorf("DividendReturn = %.2f, want %.2f", portfolio.DividendReturn, expected)
	}
}

// TestDividendReturn_ClosedPositions verifies that closed positions' dividends
// are included in DividendReturn.
func TestDividendReturn_ClosedPositions(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP", Units: 100, CurrentPrice: 50.00,
				MarketValue: 5000, TotalCost: 5000,
				DividendReturn: 200.00, // open position
				LastUpdated:    time.Now(),
			},
			{
				ID: "102", PortfolioID: "1", Ticker: "WBC", Exchange: "AU",
				Name: "WBC", Units: 0, CurrentPrice: 0, // closed position
				MarketValue: 0, TotalCost: 0,
				DividendReturn: 150.00, // dividends from when it was open
				LastUpdated:    time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{Type: "buy", Units: 100, Price: 50.00, Fees: 0, Date: "2023-01-01"}},
			"102": {
				{Type: "buy", Units: 50, Price: 30.00, Fees: 0, Date: "2023-01-01"},
				{Type: "sell", Units: 50, Price: 35.00, Fees: 0, Date: "2024-01-01"},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	ctx := common.WithNavexaClient(context.Background(), navexa)

	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// Both open and closed position dividends should be included
	want := 350.0
	if !approxEqual(portfolio.DividendReturn, want, 0.01) {
		t.Errorf("DividendReturn = %.2f, want %.2f (closed position dividends must be included)",
			portfolio.DividendReturn, want)
	}
}

// TestDividendReturn_IdentityRelationship verifies the algebraic relationship:
// NetEquityReturn = sum(h.NetReturn) + DividendReturn
// This catches any scenario where the field gets out of sync with the main return calculation.
func TestDividendReturn_IdentityRelationship(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP", Units: 100, CurrentPrice: 55.00,
				MarketValue: 5500, TotalCost: 5000,
				GainLoss: 500, DividendReturn: 200.00,
				LastUpdated: time.Now(),
			},
			{
				ID: "102", PortfolioID: "1", Ticker: "CBA", Exchange: "AU",
				Name: "CBA", Units: 50, CurrentPrice: 110.00,
				MarketValue: 5500, TotalCost: 4500,
				GainLoss: 1000, DividendReturn: 100.00,
				LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{Type: "buy", Units: 100, Price: 50.00, Fees: 0, Date: "2023-01-01"}},
			"102": {{Type: "buy", Units: 50, Price: 90.00, Fees: 0, Date: "2023-01-01"}},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	ctx := common.WithNavexaClient(context.Background(), navexa)

	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// The identity: NetEquityReturn = sum(h.NetReturn) + DividendReturn
	var sumHoldingNetReturn float64
	for _, h := range portfolio.Holdings {
		sumHoldingNetReturn += h.NetReturn
	}

	expectedNetEquityReturn := sumHoldingNetReturn + portfolio.DividendReturn
	if !approxEqual(portfolio.NetEquityReturn, expectedNetEquityReturn, 0.01) {
		t.Errorf("NetEquityReturn (%.2f) != sum(h.NetReturn) (%.2f) + DividendReturn (%.2f) = %.2f",
			portfolio.NetEquityReturn, sumHoldingNetReturn, portfolio.DividendReturn, expectedNetEquityReturn)
	}
}
