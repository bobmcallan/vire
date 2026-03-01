package portfolio

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Per-holding FX conversion tests ---

func TestSyncPortfolio_USDHoldingsConvertedToAUD(t *testing.T) {
	// AUDUSD = 0.6250 means 1 AUD = 0.625 USD
	// So USD→AUD = value / 0.625
	// AAPL: US$200 current price → A$320
	// AAPL: US$2,000 market value → A$3,200

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
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
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

	holdingMap := make(map[string]*models.Holding)
	for i := range portfolio.Holdings {
		holdingMap[portfolio.Holdings[i].Ticker] = &portfolio.Holdings[i]
	}

	// BHP (AUD) should be unchanged
	bhp := holdingMap["BHP"]
	if bhp == nil {
		t.Fatal("BHP holding not found")
	}
	if bhp.Currency != "AUD" {
		t.Errorf("BHP.Currency = %q, want AUD", bhp.Currency)
	}
	if bhp.OriginalCurrency != "" {
		t.Errorf("BHP.OriginalCurrency = %q, want empty (not converted)", bhp.OriginalCurrency)
	}
	if !approxEqual(bhp.CurrentPrice, 45.00, 0.01) {
		t.Errorf("BHP.CurrentPrice = %.2f, want 45.00 (unchanged)", bhp.CurrentPrice)
	}

	// CBOE (originally USD) should be converted to AUD
	cboe := holdingMap["CBOE"]
	if cboe == nil {
		t.Fatal("CBOE holding not found")
	}
	if cboe.Currency != "AUD" {
		t.Errorf("CBOE.Currency = %q, want AUD (converted)", cboe.Currency)
	}
	if cboe.OriginalCurrency != "USD" {
		t.Errorf("CBOE.OriginalCurrency = %q, want USD", cboe.OriginalCurrency)
	}

	// US$200 / 0.625 = A$320
	expectedPrice := 200.00 / 0.6250
	if !approxEqual(cboe.CurrentPrice, expectedPrice, 0.01) {
		t.Errorf("CBOE.CurrentPrice = %.2f, want %.2f (USD→AUD converted)", cboe.CurrentPrice, expectedPrice)
	}

	// US$2,000 / 0.625 = A$3,200
	expectedMV := 2000.00 / 0.6250
	if !approxEqual(cboe.MarketValue, expectedMV, 0.01) {
		t.Errorf("CBOE.MarketValue = %.2f, want %.2f (USD→AUD converted)", cboe.MarketValue, expectedMV)
	}

	// AvgCost should also be converted: US$150.50 (150*10+5)/10 / 0.625
	expectedAvgCost := ((150.0*10 + 5) / 10) / 0.6250
	if !approxEqual(cboe.AvgCost, expectedAvgCost, 0.01) {
		t.Errorf("CBOE.AvgCost = %.2f, want %.2f (USD→AUD converted)", cboe.AvgCost, expectedAvgCost)
	}
}

func TestSyncPortfolio_AUDHoldingsUnchangedByFXConversion(t *testing.T) {
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
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{marketStore: marketStore}
	logger := common.NewLogger("error")
	// No EODHD client needed — all AUD
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	bhp := portfolio.Holdings[0]
	if bhp.Currency != "AUD" {
		t.Errorf("Currency = %q, want AUD", bhp.Currency)
	}
	if bhp.OriginalCurrency != "" {
		t.Errorf("OriginalCurrency = %q, want empty", bhp.OriginalCurrency)
	}
	if !approxEqual(bhp.CurrentPrice, 45.00, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want 45.00", bhp.CurrentPrice)
	}
	if !approxEqual(bhp.MarketValue, 4500.00, 0.01) {
		t.Errorf("MarketValue = %.2f, want 4500.00", bhp.MarketValue)
	}
}

func TestSyncPortfolio_PortfolioTotalsCorrectAfterFXConversion(t *testing.T) {
	// BHP: A$4,500 market value, A$4,010 cost
	// CBOE: US$2,000 market value, US$1,505 cost
	// FX rate 0.625 → CBOE in AUD: A$3,200 market value, A$2,408 cost
	// Expected totals: value=7700, cost=6418

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
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
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

	// BHP: TotalInvested = 100*40 + 10 = 4010 AUD
	// CBOE: TotalInvested = (10*150 + 5) / 0.625 = 1505/0.625 = 2408 AUD
	// totalCost = 4010 + 2408 = 6418
	// No cashflow ledger → totalCash = 0, availableCash = 0 - 6418 = -6418
	// TotalValue = equity(7700) + availableCash(-6418) = 1282
	expectedTotalCost := 4010.00 + (1505.00 / 0.6250)
	if !approxEqual(portfolio.TotalCost, expectedTotalCost, 1.0) {
		t.Errorf("TotalCost = %.2f, want ~%.2f (net equity capital from trades)", portfolio.TotalCost, expectedTotalCost)
	}

	expectedAvailableCash := 0.0 - expectedTotalCost // totalCash=0 minus totalCost
	if !approxEqual(portfolio.AvailableCash, expectedAvailableCash, 1.0) {
		t.Errorf("AvailableCash = %.2f, want ~%.2f", portfolio.AvailableCash, expectedAvailableCash)
	}

	// TotalValueHoldings should still equal the sum of MarketValues (equity only)
	holdingSum := 0.0
	for _, h := range portfolio.Holdings {
		holdingSum += h.MarketValue
	}
	equityTotal := 4500.00 + (2000.00 / 0.6250) // 4500 + 3200 = 7700
	if !approxEqual(portfolio.TotalValueHoldings, equityTotal, 1.0) {
		t.Errorf("TotalValueHoldings = %.2f, want ~%.2f", portfolio.TotalValueHoldings, equityTotal)
	}
	if !approxEqual(holdingSum, portfolio.TotalValueHoldings, 1.0) {
		t.Errorf("sum of holding MarketValues = %.2f, want ~%.2f (should match TotalValueHoldings)", holdingSum, portfolio.TotalValueHoldings)
	}
}

func TestSyncPortfolio_TrueBreakevenConvertedForUSD(t *testing.T) {
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
	if cboe.TrueBreakevenPrice == nil {
		t.Fatal("TrueBreakevenPrice is nil for open position")
	}

	// Before FX: totalCost = 10*150.5 = 1505, realizedNetReturn = 0
	// breakeven = (1505 - 0) / 10 = 150.5 (in USD)
	// After FX: 150.5 / 0.625 = 240.8
	expectedBreakeven := 150.5 / 0.6250
	if !approxEqual(*cboe.TrueBreakevenPrice, expectedBreakeven, 0.1) {
		t.Errorf("TrueBreakevenPrice = %.2f, want ~%.2f (USD→AUD)", *cboe.TrueBreakevenPrice, expectedBreakeven)
	}
}

func TestSyncPortfolio_NetReturnConvertedForUSD(t *testing.T) {
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

	// NetReturn in USD: 2000 - 1505 = 495, in AUD: 495 / 0.625 = 792
	expectedNetReturn := 495.0 / 0.6250
	if !approxEqual(cboe.NetReturn, expectedNetReturn, 1.0) {
		t.Errorf("NetReturn = %.2f, want ~%.2f (USD→AUD)", cboe.NetReturn, expectedNetReturn)
	}

	// NetReturnPct should be unchanged (percentage is currency-independent)
	// totalInvested = 1505, gainLoss = 495, pct = 495/1505*100 = 32.89%
	expectedPct := (495.0 / 1505.0) * 100
	if !approxEqual(cboe.NetReturnPct, expectedPct, 0.1) {
		t.Errorf("NetReturnPct = %.2f, want ~%.2f", cboe.NetReturnPct, expectedPct)
	}
}

// --- Cache invalidation / DataVersion tests ---

func TestGetPortfolioRecord_RejectsStaleSchemaVersion(t *testing.T) {
	// Save a portfolio with an old schema version, verify getPortfolioRecord rejects it
	userDataStore := newMemUserDataStore()
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: userDataStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	// Create a portfolio with an old DataVersion
	oldPortfolio := &models.Portfolio{
		Name:        "TEST",
		DataVersion: "5", // intentionally old
		LastSynced:  time.Now(),
	}
	data, _ := json.Marshal(oldPortfolio)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "TEST",
		Value:   string(data),
	})

	// getPortfolioRecord should reject this
	_, err := svc.getPortfolioRecord(context.Background(), "TEST")
	if err == nil {
		t.Fatal("expected error for stale schema version, got nil")
	}
	if !containsSubstr(err.Error(), "stale schema version") {
		t.Errorf("error = %q, want to contain 'stale schema version'", err.Error())
	}
}

func TestGetPortfolioRecord_AcceptsCurrentSchemaVersion(t *testing.T) {
	userDataStore := newMemUserDataStore()
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: userDataStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	// Create a portfolio with the current DataVersion
	currentPortfolio := &models.Portfolio{
		Name:        "TEST",
		DataVersion: common.SchemaVersion,
		LastSynced:  time.Now(),
	}
	data, _ := json.Marshal(currentPortfolio)
	_ = userDataStore.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     "TEST",
		Value:   string(data),
	})

	portfolio, err := svc.getPortfolioRecord(context.Background(), "TEST")
	if err != nil {
		t.Fatalf("expected no error for current schema version, got: %v", err)
	}
	if portfolio.Name != "TEST" {
		t.Errorf("portfolio.Name = %q, want TEST", portfolio.Name)
	}
}

func TestSavePortfolioRecord_SetsDataVersion(t *testing.T) {
	userDataStore := newMemUserDataStore()
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: userDataStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	portfolio := &models.Portfolio{
		Name: "TEST",
	}

	err := svc.savePortfolioRecord(context.Background(), portfolio)
	if err != nil {
		t.Fatalf("savePortfolioRecord failed: %v", err)
	}

	if portfolio.DataVersion != common.SchemaVersion {
		t.Errorf("DataVersion = %q, want %q", portfolio.DataVersion, common.SchemaVersion)
	}

	// Verify the stored record has the version
	rec, err := userDataStore.Get(context.Background(), "default", "portfolio", "TEST")
	if err != nil {
		t.Fatalf("failed to read back record: %v", err)
	}
	var stored models.Portfolio
	if err := json.Unmarshal([]byte(rec.Value), &stored); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if stored.DataVersion != common.SchemaVersion {
		t.Errorf("stored DataVersion = %q, want %q", stored.DataVersion, common.SchemaVersion)
	}
}

// --- calculateGainLossFromTrades sanity test ---

func TestCalculateGainLossFromTrades_NonZeroForBuyAndHold(t *testing.T) {
	// Buy 100 shares at $10, current market value = 100 * $15 = $1500
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.0, Fees: 5.0},
	}
	currentMV := 1500.0

	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, currentMV)

	// totalInvested = 100*10 + 5 = 1005
	if !approxEqual(totalInvested, 1005.0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 1005.00", totalInvested)
	}
	// totalProceeds = 0 (no sells)
	if totalProceeds != 0 {
		t.Errorf("totalProceeds = %.2f, want 0", totalProceeds)
	}
	// gainLoss = 0 + 1500 - 1005 = 495
	if !approxEqual(gainLoss, 495.0, 0.01) {
		t.Errorf("gainLoss = %.2f, want 495.00", gainLoss)
	}
	if gainLoss == 0 {
		t.Error("gainLoss should be non-zero for a profitable buy-and-hold")
	}
}

// containsSubstr checks if s contains substr
func containsSubstr(s, substr string) bool {
	return strings.Contains(s, substr)
}
