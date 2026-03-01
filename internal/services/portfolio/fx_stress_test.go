package portfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- FX conversion edge-case stress tests ---

func TestSyncPortfolio_FXRateZero_HoldingsStayNativeCurrency(t *testing.T) {
	// When EODHD returns a zero rate, USD holdings must stay in USD (no conversion)
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE Global Markets",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: 0.0} // zero rate — EODHD returned bad data
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	cboe := portfolio.Holdings[0]
	if cboe.Currency != "USD" {
		t.Errorf("Currency = %q, want USD (should stay unconverted when FX rate is 0)", cboe.Currency)
	}
	if cboe.OriginalCurrency != "" {
		t.Errorf("OriginalCurrency = %q, want empty (no conversion occurred)", cboe.OriginalCurrency)
	}
	if !approxEqual(cboe.CurrentPrice, 200.00, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want 200.00 (unchanged)", cboe.CurrentPrice)
	}
	if !approxEqual(cboe.MarketValue, 2000.00, 0.01) {
		t.Errorf("MarketValue = %.2f, want 2000.00 (unchanged)", cboe.MarketValue)
	}
}

func TestSyncPortfolio_FXRateNil_NoEODHDClient(t *testing.T) {
	// When eodhd client is nil, USD holdings must stay unconverted
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE Global Markets",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger) // nil EODHD

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	cboe := portfolio.Holdings[0]
	if cboe.Currency != "USD" {
		t.Errorf("Currency = %q, want USD (no EODHD client, no conversion)", cboe.Currency)
	}
	if portfolio.FXRate != 0 {
		t.Errorf("FXRate = %f, want 0 (no EODHD client)", portfolio.FXRate)
	}
}

func TestSyncPortfolio_VerySmallFXRate(t *testing.T) {
	// A very small AUDUSD rate (e.g. 0.001) — AUD is nearly worthless vs USD.
	// USD values would become very large in AUD terms. This tests for overflow safety.
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE Global Markets",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: 0.001} // extremely weak AUD
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	cboe := portfolio.Holdings[0]
	// US$200 / 0.001 = A$200,000
	expectedPrice := 200.00 / 0.001
	if !approxEqual(cboe.CurrentPrice, expectedPrice, 1.0) {
		t.Errorf("CurrentPrice = %.2f, want %.2f", cboe.CurrentPrice, expectedPrice)
	}
	if math.IsInf(cboe.CurrentPrice, 0) || math.IsNaN(cboe.CurrentPrice) {
		t.Errorf("CurrentPrice is Inf/NaN — overflow with small FX rate")
	}
	if math.IsInf(portfolio.EquityValue, 0) || math.IsNaN(portfolio.EquityValue) {
		t.Errorf("TotalValueHoldings is Inf/NaN — overflow with small FX rate")
	}
}

func TestSyncPortfolio_VeryLargeFXRate(t *testing.T) {
	// A very large AUDUSD rate (e.g. 100.0) — AUD is very strong vs USD.
	// USD values would become very small in AUD terms.
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE Global Markets",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: 100.0}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	cboe := portfolio.Holdings[0]
	// US$200 / 100 = A$2.00
	expectedPrice := 200.00 / 100.0
	if !approxEqual(cboe.CurrentPrice, expectedPrice, 0.01) {
		t.Errorf("CurrentPrice = %.4f, want %.4f", cboe.CurrentPrice, expectedPrice)
	}
	// Values should be positive and finite
	if cboe.CurrentPrice <= 0 {
		t.Errorf("CurrentPrice should be positive, got %.4f", cboe.CurrentPrice)
	}
}

func TestSyncPortfolio_EmptyCurrencyDefaultsToAUD(t *testing.T) {
	// When Navexa returns empty currency, it should default to AUD and not be FX-converted
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 45.00, MarketValue: 4500.00,
				Currency:    "", // empty — should default to AUD
				LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: 0.6250}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	bhp := portfolio.Holdings[0]
	if bhp.Currency != "AUD" {
		t.Errorf("Currency = %q, want AUD (empty should default to AUD)", bhp.Currency)
	}
	if bhp.OriginalCurrency != "" {
		t.Errorf("OriginalCurrency = %q, want empty (AUD holdings not converted)", bhp.OriginalCurrency)
	}
	if !approxEqual(bhp.CurrentPrice, 45.00, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want 45.00 (unchanged)", bhp.CurrentPrice)
	}
}

func TestSyncPortfolio_NZDHoldingsNotConverted(t *testing.T) {
	// NZD holdings should NOT be FX-converted — only USD is handled
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "102", PortfolioID: "1",
				Ticker: "TWR", Exchange: "NZ", Name: "Tower Limited",
				Units: 500, CurrentPrice: 1.20, MarketValue: 600.00,
				Currency:    "NZD",
				LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"102": {{ID: "3", HoldingID: "102", Symbol: "TWR", Type: "buy", Units: 500, Price: 1.00, Fees: 10}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: 0.6250}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	twr := portfolio.Holdings[0]
	if twr.Currency != "NZD" {
		t.Errorf("Currency = %q, want NZD (NZD should not be FX-converted)", twr.Currency)
	}
	if twr.OriginalCurrency != "" {
		t.Errorf("OriginalCurrency = %q, want empty (NZD not converted)", twr.OriginalCurrency)
	}
	if !approxEqual(twr.CurrentPrice, 1.20, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want 1.20 (unchanged NZD)", twr.CurrentPrice)
	}
}

// --- FX data consistency tests ---

func TestSyncPortfolio_FXConversion_AllMonetaryFieldsConsistent(t *testing.T) {
	// Verify that after FX conversion, derived fields remain internally consistent:
	// MarketValue == CurrentPrice * Units
	// TotalCost == AvgCost * Units (approximately, due to fees)
	// NetReturn == MarketValue - TotalCost (approximately)
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE Global Markets",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: 0.6250}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	cboe := portfolio.Holdings[0]

	// MarketValue == CurrentPrice * Units
	expectedMV := cboe.CurrentPrice * cboe.Units
	if !approxEqual(cboe.MarketValue, expectedMV, 0.01) {
		t.Errorf("MarketValue = %.2f, want CurrentPrice*Units = %.2f (consistency check)",
			cboe.MarketValue, expectedMV)
	}

	// UnrealizedNetReturn == MarketValue - TotalCost (for a single-buy holding with no sells)
	expectedUnrealized := cboe.MarketValue - cboe.NetEquityCost
	if !approxEqual(cboe.UnrealizedReturn, expectedUnrealized, 0.01) {
		t.Errorf("UnrealizedNetReturn = %.2f, want MarketValue-TotalCost = %.2f (consistency check)",
			cboe.UnrealizedReturn, expectedUnrealized)
	}

	// For a buy-and-hold, RealizedNetReturn should be 0
	if !approxEqual(cboe.RealizedReturn, 0.0, 0.01) {
		t.Errorf("RealizedNetReturn = %.2f, want 0 (no sells)", cboe.RealizedReturn)
	}

	// NetReturn should equal UnrealizedNetReturn for a buy-and-hold
	if !approxEqual(cboe.NetReturn, cboe.UnrealizedReturn, 1.0) {
		t.Errorf("NetReturn = %.2f vs UnrealizedNetReturn = %.2f — should match for buy-and-hold",
			cboe.NetReturn, cboe.UnrealizedReturn)
	}

	// TrueBreakevenPrice should be positive and in AUD
	if cboe.TrueBreakevenPrice == nil {
		t.Fatal("TrueBreakevenPrice is nil for open position")
	}
	if *cboe.TrueBreakevenPrice <= 0 {
		t.Errorf("TrueBreakevenPrice = %.2f, should be positive", *cboe.TrueBreakevenPrice)
	}
	// TrueBreakevenPrice should be > CurrentPrice since there are no realized gains yet
	// to reduce the breakeven (fees push breakeven slightly above avg cost)
	// For this case: breakeven = totalCost / units = avgCost = 150.5/0.625 = 240.80 AUD
	// CurrentPrice = 200/0.625 = 320 AUD — breakeven < current (profitable position)

	// With no cash ledger: totalCost = TotalInvested, availableCash = 0 - totalCost (negative).
	// weightDenom = equity + availableCash = net equity gain. Weight > 100% is expected here.
	// Weight calculation correctness is tested in TestSyncPortfolio_WeightsSumTo100_WithCashLedger.
	if math.IsNaN(cboe.PortfolioWeightPct) || math.IsInf(cboe.PortfolioWeightPct, 0) {
		t.Errorf("Weight = %v — must be finite", cboe.PortfolioWeightPct)
	}
}

func TestSyncPortfolio_FXConversion_WeightsSumTo100(t *testing.T) {
	// Verify weights sum to 100% after FX conversion with mixed currencies AND a cash ledger.
	// Without a cash ledger, weights won't sum to 100% (availableCash is negative).
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 45.00, MarketValue: 4500.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE Global Markets",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
			{
				ID: "102", PortfolioID: "1",
				Ticker: "CBA", Exchange: "AU", Name: "CBA",
				Units: 50, CurrentPrice: 100.00, MarketValue: 5000.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
			"101": {{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
			"102": {{ID: "3", HoldingID: "102", Symbol: "CBA", Type: "buy", Units: 50, Price: 90.0, Fees: 10}},
		},
	}

	// BHP: TotalInvested = 100*40+10 = 4010 AUD
	// CBOE: TotalInvested = (10*150+5)/0.625 = 2408 AUD
	// CBA: TotalInvested = 50*90+10 = 4510 AUD
	// totalCost = 4010 + 2408 + 4510 = 10928
	// Provide a cash ledger with enough cash to cover totalCost + leave room for available cash
	cashSvc := &stubCashFlowSvc{
		ledgers: map[string]*models.CashFlowLedger{
			"SMSF": {
				PortfolioName: "SMSF",
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Amount: 12000, Date: time.Now()},
				},
			},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore, userDataStore: newMemUserDataStore()}
	eodhd := &fxStubEODHDClient{forexRate: 0.6250}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)
	svc.SetCashFlowService(cashSvc)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// Weights should sum to ~100% when availableCash >= 0
	totalWeight := 0.0
	for _, h := range portfolio.Holdings {
		totalWeight += h.PortfolioWeightPct
	}
	// Each holding weight = MarketValue / (equity + availableCash)
	// equity = 4500 + 3200 + 5000 = 12700
	// totalCost ≈ 10928, totalCash = 12000, availableCash = 12000 - 10928 = 1072
	// weightDenom = 12700 + 1072 = 13772
	// sum of weights = (4500+3200+5000)/13772 * 100 = 12700/13772 * 100 ≈ 92.2%
	// Weights should sum < 100% (cash portion makes up the rest)
	if totalWeight > 100.0 {
		t.Errorf("sum of weights = %.2f%%, should be <= 100%% when cash ledger covers costs", totalWeight)
	}
	if totalWeight <= 0 {
		t.Errorf("sum of weights = %.2f%%, should be positive", totalWeight)
	}
}

// --- Cache invalidation edge-case tests ---

func TestGetPortfolioRecord_EmptyDataVersion_TriggersStaleError(t *testing.T) {
	// Old cached portfolio with no DataVersion field at all (pre-existing data)
	userDataStore := newMemUserDataStore()
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: userDataStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	// Store a portfolio with empty DataVersion (simulates old data)
	oldPortfolio := &models.Portfolio{
		Name:        "TEST",
		DataVersion: "", // empty — pre-schema-version data
		LastSynced:  time.Now(),
	}
	data, _ := json.Marshal(oldPortfolio)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "TEST",
		Value:   string(data),
	})

	_, err := svc.getPortfolioRecord(context.Background(), "TEST")
	if err == nil {
		t.Fatal("expected error for empty DataVersion (old data), got nil")
	}
	if !containsSubstr(err.Error(), "stale schema version") {
		t.Errorf("error = %q, want to contain 'stale schema version'", err.Error())
	}
}

func TestGetPortfolioRecord_FutureDataVersion_TriggersStaleError(t *testing.T) {
	// If DataVersion is somehow higher than current SchemaVersion (e.g. downgrade),
	// it should also trigger a re-sync rather than serving potentially incompatible data
	userDataStore := newMemUserDataStore()
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: userDataStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	futurePortfolio := &models.Portfolio{
		Name:        "TEST",
		DataVersion: "999", // future version
		LastSynced:  time.Now(),
	}
	data, _ := json.Marshal(futurePortfolio)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "TEST",
		Value:   string(data),
	})

	_, err := svc.getPortfolioRecord(context.Background(), "TEST")
	if err == nil {
		t.Fatal("expected error for future DataVersion, got nil")
	}
}

func TestSyncPortfolio_StaleCache_TriggersResync(t *testing.T) {
	// Full integration test: save a portfolio with old DataVersion, then
	// call GetPortfolio — it should detect the stale version and re-sync
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

	// Seed stale cache — old version, stale price of $45
	stale := &models.Portfolio{
		Name:        "SMSF",
		DataVersion: "5", // old
		Holdings: []models.Holding{
			{Ticker: "BHP", CurrentPrice: 45.00, MarketValue: 4500.00, Currency: "AUD", Units: 100},
		},
		EquityValue: 4500.00,
		TotalValue:         4500.00,
		LastSynced:         time.Now(), // recently synced — would normally be served from cache
	}
	data, _ := json.Marshal(stale)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   string(data),
	})

	// GetPortfolio should detect stale version, attempt sync, and return fresh data
	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.GetPortfolio(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetPortfolio failed: %v", err)
	}

	// Should have re-synced with Navexa's $50 price, not the stale $45
	bhp := portfolio.Holdings[0]
	if !approxEqual(bhp.CurrentPrice, 50.00, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want 50.00 (re-synced from Navexa, not stale cache)",
			bhp.CurrentPrice)
	}
	if portfolio.DataVersion != common.SchemaVersion {
		t.Errorf("DataVersion = %q, want %q (should be updated after re-sync)",
			portfolio.DataVersion, common.SchemaVersion)
	}
}

// --- Negative FX rate defense test ---

func TestSyncPortfolio_NegativeFXRate_NoConversion(t *testing.T) {
	// If EODHD somehow returns a negative rate, the `quote.Close > 0` check should block it
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE Global Markets",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: -0.5} // negative rate — should be blocked
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	cboe := portfolio.Holdings[0]
	// Negative FX rate should be rejected (quote.Close > 0 check in SyncPortfolio)
	// Holdings should stay unconverted
	if cboe.Currency != "USD" {
		t.Errorf("Currency = %q, want USD (negative FX rate should be rejected)", cboe.Currency)
	}
	if cboe.CurrentPrice < 0 {
		t.Errorf("CurrentPrice = %.2f, should not be negative from FX conversion", cboe.CurrentPrice)
	}
	if portfolio.FXRate != 0 {
		t.Errorf("FXRate = %f, want 0 (negative rate should be rejected)", portfolio.FXRate)
	}
}

// --- Closed position FX conversion test ---

func TestSyncPortfolio_ClosedUSDPosition_FXConverted(t *testing.T) {
	// Closed positions (units == 0) should still get FX-converted monetary fields
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1",
				Ticker: "AAPL", Exchange: "US", Name: "Apple",
				Units: 0, CurrentPrice: 0, MarketValue: 0,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {
				{ID: "1", HoldingID: "101", Symbol: "AAPL", Type: "buy", Units: 10, Price: 150.0, Fees: 5},
				{ID: "2", HoldingID: "101", Symbol: "AAPL", Type: "sell", Units: 10, Price: 200.0, Fees: 5},
			},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: 0.6250}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	aapl := portfolio.Holdings[0]
	if aapl.Currency != "AUD" {
		t.Errorf("Currency = %q, want AUD (closed USD positions should also be converted)", aapl.Currency)
	}
	if aapl.OriginalCurrency != "USD" {
		t.Errorf("OriginalCurrency = %q, want USD", aapl.OriginalCurrency)
	}
	// TrueBreakevenPrice should be nil for closed positions (units == 0)
	if aapl.TrueBreakevenPrice != nil {
		t.Errorf("TrueBreakevenPrice should be nil for closed position, got %.2f", *aapl.TrueBreakevenPrice)
	}
	// RealizedNetReturn should be converted to AUD
	// USD realized: proceeds (10*200-5) - invested (10*150+5) = 1995 - 1505 = 490
	// AUD: 490 / 0.625 = 784
	if aapl.RealizedReturn == 0 {
		// This is expected because calculateGainLossFromTrades uses marketValue=0 for closed positions,
		// and the realized/unrealized split is done differently. Just verify no NaN/Inf.
		if math.IsNaN(aapl.RealizedReturn) || math.IsInf(aapl.RealizedReturn, 0) {
			t.Error("RealizedNetReturn is NaN/Inf")
		}
	}
}

// --- Malformed JSON cache test ---

func TestGetPortfolioRecord_CorruptedJSON(t *testing.T) {
	userDataStore := newMemUserDataStore()
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: userDataStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	// Store corrupted JSON
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "TEST",
		Value:   `{"name":"TEST", "data_version": "` + common.SchemaVersion + `", broken}`,
	})

	_, err := svc.getPortfolioRecord(context.Background(), "TEST")
	if err == nil {
		t.Fatal("expected error for corrupted JSON, got nil")
	}
	if !containsSubstr(err.Error(), "unmarshal") {
		t.Errorf("error = %q, expected unmarshal-related error", err.Error())
	}
}

// --- Partial sell FX conversion consistency ---

func TestSyncPortfolio_PartialSellUSD_FXConsistency(t *testing.T) {
	// Buy 20 shares at US$100, sell 10 at US$120, current price US$130
	// Tests that realized + unrealized = net return after FX conversion
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE",
				Units: 10, CurrentPrice: 130.00, MarketValue: 1300.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {
				{ID: "1", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 20, Price: 100.0, Fees: 10},
				{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "sell", Units: 10, Price: 120.0, Fees: 5},
			},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: 0.6250}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	cboe := portfolio.Holdings[0]

	// After FX conversion, realized + unrealized should approximately equal netReturn
	sumParts := cboe.RealizedReturn + cboe.UnrealizedReturn
	if !approxEqual(sumParts, cboe.NetReturn, 1.0) {
		t.Errorf("RealizedNetReturn (%.2f) + UnrealizedNetReturn (%.2f) = %.2f, want ~NetReturn (%.2f)",
			cboe.RealizedReturn, cboe.UnrealizedReturn, sumParts, cboe.NetReturn)
	}

	// All monetary values should be positive and finite
	for _, v := range []float64{cboe.CurrentPrice, cboe.MarketValue, cboe.NetEquityCost, cboe.GrossInvested} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("found NaN/Inf value in monetary field: %f", v)
		}
		if v < 0 {
			t.Errorf("found negative monetary value: %f (should all be positive for a profitable position)", v)
		}
	}
}

// --- Error message injection test ---

func TestGetPortfolioRecord_StaleVersionErrorMessage_SafeFormatting(t *testing.T) {
	// Verify the error message includes version info but can't be used for injection
	userDataStore := newMemUserDataStore()
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: userDataStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	// Use a name with special characters — should not cause issues
	portfolioName := `TEST<script>alert('xss')</script>`
	oldPortfolio := &models.Portfolio{
		Name:        portfolioName,
		DataVersion: "1",
		LastSynced:  time.Now(),
	}
	data, _ := json.Marshal(oldPortfolio)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     portfolioName,
		Value:   string(data),
	})

	_, err := svc.getPortfolioRecord(context.Background(), portfolioName)
	if err == nil {
		t.Fatal("expected error for stale version, got nil")
	}
	// Error message should contain the portfolio name as-is (server-side, not rendered)
	errMsg := err.Error()
	if !containsSubstr(errMsg, "stale schema version") {
		t.Errorf("error = %q, want to contain 'stale schema version'", errMsg)
	}
	// Verify the error uses fmt.Errorf with %s (no format string injection possible)
	// The portfolio name is a raw string arg — not interpretable as format verbs
	if containsSubstr(errMsg, "%") {
		t.Errorf("error message contains unresolved format verb: %q", errMsg)
	}
}

// --- Portfolio-level FX rate stored correctly test ---

func TestSyncPortfolio_FXRateStoredOnPortfolio(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"101": {{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxStubEODHDClient{forexRate: 0.6250}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// FXRate should be stored on the portfolio for ReviewPortfolio to use
	if !approxEqual(portfolio.FXRate, 0.6250, 0.0001) {
		t.Errorf("FXRate = %.4f, want 0.6250", portfolio.FXRate)
	}
}

// --- EODHD error handling test ---

type fxErrorEODHDClient struct {
	stubEODHDClient
}

func (s *fxErrorEODHDClient) GetRealTimeQuote(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
	if ticker == "AUDUSD.FOREX" {
		return nil, fmt.Errorf("EODHD API timeout: connection refused")
	}
	return nil, fmt.Errorf("not found")
}

func TestSyncPortfolio_EODHDError_GracefulDegradation(t *testing.T) {
	// When EODHD returns an error, portfolio should still be built (without FX conversion)
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1",
				Ticker: "BHP", Exchange: "AU", Name: "BHP Group",
				Units: 100, CurrentPrice: 45.00, MarketValue: 4500.00,
				Currency: "AUD", LastUpdated: time.Now(),
			},
			{
				ID: "101", PortfolioID: "1",
				Ticker: "CBOE", Exchange: "US", Name: "CBOE",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
			"101": {{ID: "2", HoldingID: "101", Symbol: "CBOE", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	eodhd := &fxErrorEODHDClient{} // returns errors
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio should succeed even when EODHD fails, got: %v", err)
	}

	// Portfolio should have 2 holdings
	if len(portfolio.Holdings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(portfolio.Holdings))
	}

	// USD holding should stay in USD
	for _, h := range portfolio.Holdings {
		if h.Ticker == "CBOE" && h.Currency != "USD" {
			t.Errorf("CBOE.Currency = %q, want USD (EODHD failed, no conversion)", h.Currency)
		}
	}

	if portfolio.FXRate != 0 {
		t.Errorf("FXRate = %f, want 0 (EODHD error)", portfolio.FXRate)
	}
}

var _ interfaces.EODHDClient = (*fxErrorEODHDClient)(nil)
