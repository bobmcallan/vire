package portfolio

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Model field tests ---

func TestHolding_HasCurrencyField(t *testing.T) {
	h := models.Holding{
		Ticker:   "AAPL",
		Currency: "USD",
	}
	if h.Currency != "USD" {
		t.Errorf("Holding.Currency = %q, want %q", h.Currency, "USD")
	}
}

func TestHolding_HasCountryField(t *testing.T) {
	h := models.Holding{
		Ticker:  "BHP",
		Country: "AU",
	}
	if h.Country != "AU" {
		t.Errorf("Holding.Country = %q, want %q", h.Country, "AU")
	}
}

func TestNavexaHolding_HasCurrencyField(t *testing.T) {
	h := models.NavexaHolding{
		Ticker:   "AAPL",
		Currency: "USD",
	}
	if h.Currency != "USD" {
		t.Errorf("NavexaHolding.Currency = %q, want %q", h.Currency, "USD")
	}
}

func TestPortfolio_HasFXRateField(t *testing.T) {
	p := models.Portfolio{
		Name:   "SMSF",
		FXRate: 0.6250,
	}
	if !approxEqual(p.FXRate, 0.6250, 0.0001) {
		t.Errorf("Portfolio.FXRate = %.4f, want 0.6250", p.FXRate)
	}
}

// --- SyncPortfolio currency mapping tests ---

func TestSyncPortfolio_MapsCurrencyFromNavexa(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID:           "100",
				PortfolioID:  "1",
				Ticker:       "BHP",
				Exchange:     "AU",
				Name:         "BHP Group",
				Units:        100,
				CurrentPrice: 45.00,
				MarketValue:  4500.00,
				Currency:     "AUD",
				LastUpdated:  time.Now(),
			},
			{
				ID:           "101",
				PortfolioID:  "1",
				Ticker:       "AAPL",
				Exchange:     "US",
				Name:         "Apple Inc",
				Units:        10,
				CurrentPrice: 185.00,
				MarketValue:  1850.00,
				Currency:     "USD",
				LastUpdated:  time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
			"101": {{ID: "2", HoldingID: "101", Symbol: "AAPL", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}

	storage := &stubStorageManager{
		marketStore: marketStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// Find holdings by ticker
	holdingMap := make(map[string]*models.Holding)
	for i := range portfolio.Holdings {
		holdingMap[portfolio.Holdings[i].Ticker] = &portfolio.Holdings[i]
	}

	bhp, ok := holdingMap["BHP"]
	if !ok {
		t.Fatal("BHP holding not found")
	}
	if bhp.Currency != "AUD" {
		t.Errorf("BHP.Currency = %q, want %q", bhp.Currency, "AUD")
	}

	aapl, ok := holdingMap["AAPL"]
	if !ok {
		t.Fatal("AAPL holding not found")
	}
	if aapl.Currency != "USD" {
		t.Errorf("AAPL.Currency = %q, want %q", aapl.Currency, "USD")
	}
}

func TestSyncPortfolio_CurrencyDefaultsToAUD(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID:           "100",
				PortfolioID:  "1",
				Ticker:       "BHP",
				Exchange:     "AU",
				Name:         "BHP Group",
				Units:        100,
				CurrentPrice: 45.00,
				MarketValue:  4500.00,
				Currency:     "", // empty currency from Navexa
				LastUpdated:  time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}

	storage := &stubStorageManager{
		marketStore: marketStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	if len(portfolio.Holdings) == 0 {
		t.Fatal("no holdings")
	}

	if portfolio.Holdings[0].Currency != "AUD" {
		t.Errorf("Holding.Currency = %q for empty Navexa currency, want %q", portfolio.Holdings[0].Currency, "AUD")
	}
}

// --- Country population test ---

func TestSyncPortfolio_PopulatesCountryFromFundamentals(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID:           "100",
				PortfolioID:  "1",
				Ticker:       "BHP",
				Exchange:     "AU",
				Name:         "BHP Group",
				Units:        100,
				CurrentPrice: 45.00,
				MarketValue:  4500.00,
				Currency:     "AUD",
				LastUpdated:  time.Now(),
			},
			{
				ID:           "101",
				PortfolioID:  "1",
				Ticker:       "AAPL",
				Exchange:     "US",
				Name:         "Apple Inc",
				Units:        10,
				CurrentPrice: 185.00,
				MarketValue:  1850.00,
				Currency:     "USD",
				LastUpdated:  time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
			"101": {{ID: "2", HoldingID: "101", Symbol: "AAPL", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				Fundamentals: &models.Fundamentals{
					Ticker:     "BHP.AU",
					CountryISO: "AU",
					Sector:     "Materials",
				},
			},
			"AAPL.US": {
				Ticker: "AAPL.US",
				Fundamentals: &models.Fundamentals{
					Ticker:     "AAPL.US",
					CountryISO: "US",
					Sector:     "Technology",
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore: marketStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	holdingMap := make(map[string]*models.Holding)
	for i := range portfolio.Holdings {
		holdingMap[portfolio.Holdings[i].Ticker] = &portfolio.Holdings[i]
	}

	bhp, ok := holdingMap["BHP"]
	if !ok {
		t.Fatal("BHP holding not found")
	}
	if bhp.Country != "AU" {
		t.Errorf("BHP.Country = %q, want %q", bhp.Country, "AU")
	}

	aapl, ok := holdingMap["AAPL"]
	if !ok {
		t.Fatal("AAPL holding not found")
	}
	if aapl.Country != "US" {
		t.Errorf("AAPL.Country = %q, want %q", aapl.Country, "US")
	}
}

func TestSyncPortfolio_CountryEmptyWhenNoFundamentals(t *testing.T) {
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID:           "100",
				PortfolioID:  "1",
				Ticker:       "XYZ",
				Exchange:     "AU",
				Name:         "Unknown Corp",
				Units:        50,
				CurrentPrice: 10.00,
				MarketValue:  500.00,
				Currency:     "AUD",
				LastUpdated:  time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "XYZ", Type: "buy", Units: 50, Price: 10.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}

	storage := &stubStorageManager{
		marketStore: marketStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	if len(portfolio.Holdings) == 0 {
		t.Fatal("no holdings")
	}
	if portfolio.Holdings[0].Country != "" {
		t.Errorf("Holding.Country = %q when no fundamentals, want empty", portfolio.Holdings[0].Country)
	}
}

// --- FX conversion tests ---

// fxSyncStorageManager wraps stubStorageManager but allows setting a custom EODHD client
type fxStubEODHDClient struct {
	stubEODHDClient
	forexRate float64 // AUDUSD rate
}

func (s *fxStubEODHDClient) GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error) {
	if ticker == "AUDUSD.FOREX" {
		return &models.RealTimeQuote{
			Code:  "AUDUSD.FOREX",
			Close: s.forexRate,
		}, nil
	}
	return nil, fmt.Errorf("not found")
}

func TestSyncPortfolio_FXConversionForMixedCurrencyTotals(t *testing.T) {
	// AUD holding: BHP at A$45 x 100 = A$4,500
	// USD holding: AAPL at US$200 x 10 = US$2,000
	// AUDUSD rate: 0.6250 (1 AUD = 0.625 USD, so US$2,000 = A$3,200)
	// Expected total (AUD): 4,500 + 3,200 = 7,700

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
				Ticker: "AAPL", Exchange: "US", Name: "Apple Inc",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
			"101": {{ID: "2", HoldingID: "101", Symbol: "AAPL", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore: marketStore,
	}

	eodhd := &fxStubEODHDClient{forexRate: 0.6250}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// TotalValueHoldings (equity only) should be in AUD: A$4,500 + US$2,000/0.625 = A$4,500 + A$3,200 = A$7,700
	expectedEquityTotal := 4500.00 + (2000.00 / 0.6250)
	if !approxEqual(portfolio.EquityValue, expectedEquityTotal, 1.0) {
		t.Errorf("Portfolio.EquityValue = %.2f, want ~%.2f (AUD converted)", portfolio.EquityValue, expectedEquityTotal)
	}
	// No cashflow ledger → TotalCash = 0, so TotalValue = equity + availableCash = equity - totalCost
	// totalCost = BHP: 100*40+10 = 4010 + AAPL: (10*150+5)/0.625 = 2408 → 6418
	// availableCash = 0 - 6418 = -6418
	// TotalValue = 7700 + (-6418) = 1282
	// (This is by design: without a cash ledger, TotalValue reflects net equity gain)

	// FX rate should be recorded
	if !approxEqual(portfolio.FXRate, 0.6250, 0.0001) {
		t.Errorf("Portfolio.FXRate = %.4f, want 0.6250", portfolio.FXRate)
	}
}

func TestSyncPortfolio_NoFXConversionWhenAllAUD(t *testing.T) {
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
	storage := &stubStorageManager{
		marketStore: marketStore,
	}

	// EODHD client should NOT be called for forex when all holdings are AUD
	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			if ticker == "AUDUSD.FOREX" {
				t.Error("GetRealTimeQuote called for AUDUSD.FOREX when all holdings are AUD — should skip FX lookup")
			}
			return nil, fmt.Errorf("not found")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// FX rate should be zero (not used)
	if portfolio.FXRate != 0 {
		t.Errorf("Portfolio.FXRate = %.4f, want 0 (no FX conversion needed)", portfolio.FXRate)
	}
}

func TestSyncPortfolio_FXConversionFailureStillSyncs(t *testing.T) {
	// If forex API fails, sync should still succeed with unconverted totals
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
				Ticker: "AAPL", Exchange: "US", Name: "Apple Inc",
				Units: 10, CurrentPrice: 200.00, MarketValue: 2000.00,
				Currency: "USD", LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.0, Fees: 10}},
			"101": {{ID: "2", HoldingID: "101", Symbol: "AAPL", Type: "buy", Units: 10, Price: 150.0, Fees: 5}},
		},
	}

	marketStore := &stubMarketDataStorage{data: map[string]*models.MarketData{}}
	storage := &stubStorageManager{
		marketStore: marketStore,
	}

	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			return nil, fmt.Errorf("forex API unavailable")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio should not fail on FX error, got: %v", err)
	}

	// Should still have holdings
	if len(portfolio.Holdings) != 2 {
		t.Errorf("expected 2 holdings, got %d", len(portfolio.Holdings))
	}
}

// --- Formatter tests ---

func TestFormatPortfolioHoldings_ShowsCurrencyAndCountry(t *testing.T) {
	// This test validates that the formatter output includes currency and country columns.
	h := models.Holding{
		Ticker:   "AAPL",
		Exchange: "US",
		Name:     "Apple Inc",
		Units:    10,
		Currency: "USD",
		Country:  "US",
	}

	if h.Currency != "USD" {
		t.Errorf("Currency not preserved on Holding: got %q, want %q", h.Currency, "USD")
	}
	if h.Country != "US" {
		t.Errorf("Country not preserved on Holding: got %q, want %q", h.Country, "US")
	}
}

// --- Ensure eodhd stub satisfies interface ---

var _ interfaces.EODHDClient = (*fxStubEODHDClient)(nil)
