package portfolio

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Adversarial stress tests for the portfolio value fix (TotalCost from trades,
// AvailableCash, TotalValue = equity + availableCash).
//
// Covers edge cases:
// 1. totalCost > totalCash (negative availableCash — equity appreciated beyond cash)
// 2. No trades at all (TotalInvested=0, TotalProceeds=0)
// 3. All positions closed (totalCost includes closed positions' net capital)
// 4. Division by zero: totalGainPct when totalCost=0
// 5. FX rate = 0 with USD holdings — TotalProceeds must not divide by zero
// 6. Negative totalCost from cost base decreases exceeding invested
// 7. Large portfolio with many holdings — numerical stability
// 8. Holding with no metrics entry — TotalInvested and TotalProceeds both 0
// 9. Mixed open and closed positions — totalCost sums both
// 10. Weight denominator with negative availableCash

// =============================================================================
// stubCashFlowSvc implements a minimal CashFlowService for testing totalCash
// =============================================================================

type stubCashFlowSvc struct {
	ledgers map[string]*models.CashFlowLedger
}

func (s *stubCashFlowSvc) GetLedger(_ context.Context, portfolioName string) (*models.CashFlowLedger, error) {
	if l, ok := s.ledgers[portfolioName]; ok {
		return l, nil
	}
	return nil, nil
}

// Satisfy the interface — these are not used in SyncPortfolio tests.
func (s *stubCashFlowSvc) AddTransaction(_ context.Context, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (s *stubCashFlowSvc) AddTransfer(_ context.Context, _, _, _ string, _ float64, _ time.Time, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (s *stubCashFlowSvc) UpdateTransaction(_ context.Context, _, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (s *stubCashFlowSvc) RemoveTransaction(_ context.Context, _, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (s *stubCashFlowSvc) SetTransactions(_ context.Context, _ string, _ []models.CashTransaction, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (s *stubCashFlowSvc) UpdateAccount(_ context.Context, _, _ string, _ models.CashAccountUpdate) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (s *stubCashFlowSvc) CalculatePerformance(_ context.Context, _ string) (*models.CapitalPerformance, error) {
	return nil, nil
}
func (s *stubCashFlowSvc) ClearLedger(_ context.Context, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}

// =============================================================================
// 1. totalCost > totalCash — negative availableCash
//
// Scenario: Portfolio gained value. $500k invested via trades, only $400k in
// cash ledger (user deposited $400k, bought stocks that appreciated to $500k
// market value). totalCost = $500k (net invested), totalCash = $400k.
// availableCash = -$100k. TotalValue = equity + (-100k).
//
// This is valid: the "missing" $100k is unrealized gain in equities. The
// portfolio has more equity value than cash deposited.
// =============================================================================

func TestSyncPortfolio_NegativeAvailableCash_EquityAppreciation(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP Group", Units: 1000, CurrentPrice: 50.00,
				MarketValue: 50000, TotalCost: 40000, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				// Bought $40k worth (500 units at $40 + 500 at $40)
				{ID: "t1", HoldingID: "1", Symbol: "BHP", Type: "buy", Units: 500, Price: 40.00, Fees: 0},
				{ID: "t2", HoldingID: "1", Symbol: "BHP", Type: "buy", Units: 500, Price: 40.00, Fees: 0},
			},
		},
	}

	// Cash ledger: only $30k deposited (totalCost from trades = $40k)
	cashSvc := &stubCashFlowSvc{
		ledgers: map[string]*models.CashFlowLedger{
			"SMSF": {
				PortfolioName: "SMSF",
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Amount: 30000, Date: today},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// totalCost = TotalInvested(40000) - TotalProceeds(0) = 40000
	if !approxEqual(portfolio.NetEquityCost, 40000, 0.01) {
		t.Errorf("TotalCost = %.2f, want 40000", portfolio.NetEquityCost)
	}

	// totalCash = 30000 from ledger
	if !approxEqual(portfolio.GrossCashBalance, 30000, 0.01) {
		t.Errorf("TotalCash = %.2f, want 30000", portfolio.GrossCashBalance)
	}

	// availableCash = 30000 - 40000 = -10000
	if !approxEqual(portfolio.NetCashBalance, -10000, 0.01) {
		t.Errorf("AvailableCash = %.2f, want -10000 (negative is valid — equity appreciated beyond cash)", portfolio.NetCashBalance)
	}

	// TotalValue = equity(50000) + availableCash(-10000) = 40000
	expectedTotalValue := 50000.0 + (-10000.0)
	if !approxEqual(portfolio.PortfolioValue, expectedTotalValue, 0.01) {
		t.Errorf("TotalValue = %.2f, want %.2f", portfolio.PortfolioValue, expectedTotalValue)
	}

	// Must not be NaN or Inf
	if math.IsNaN(portfolio.EquityValue) || math.IsInf(portfolio.EquityValue, 0) {
		t.Errorf("TotalValue = %v — must be finite", portfolio.PortfolioValue)
	}
}

// =============================================================================
// 2. No trades at all — TotalInvested=0, TotalProceeds=0, totalCost=0
//
// If holdings have no trade history, holdingMetrics will be empty for that
// ticker. TotalInvested and TotalProceeds default to 0, so totalCost = 0.
// availableCash = totalCash - 0 = totalCash.
// TotalValue = equity + totalCash (same as old behavior).
// totalGainPct must be 0 (not NaN from 0/0).
// =============================================================================

func TestSyncPortfolio_NoTrades_TotalCostZero(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP Group", Units: 100, CurrentPrice: 50.00,
				MarketValue: 5000, TotalCost: 4500, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{}, // no trades at all
	}

	cashSvc := &stubCashFlowSvc{
		ledgers: map[string]*models.CashFlowLedger{
			"SMSF": {
				PortfolioName: "SMSF",
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Amount: 10000, Date: today},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// No trades → TotalInvested=0, TotalProceeds=0 → totalCost=0
	if !approxEqual(portfolio.NetEquityCost, 0, 0.01) {
		t.Errorf("TotalCost = %.2f, want 0 (no trades)", portfolio.NetEquityCost)
	}

	// availableCash = 10000 - 0 = 10000
	if !approxEqual(portfolio.NetCashBalance, 10000, 0.01) {
		t.Errorf("AvailableCash = %.2f, want 10000", portfolio.NetCashBalance)
	}

	// TotalValue = equity(5000) + availableCash(10000) = 15000
	if !approxEqual(portfolio.PortfolioValue, 15000, 0.01) {
		t.Errorf("TotalValue = %.2f, want 15000", portfolio.PortfolioValue)
	}

	// totalGainPct must be 0 (totalCost=0, division guard)
	if portfolio.NetEquityReturnPct != 0 {
		t.Errorf("TotalNetReturnPct = %.2f, want 0 (totalCost=0, no division)", portfolio.NetEquityReturnPct)
	}

	// Must not be NaN
	if math.IsNaN(portfolio.NetEquityReturnPct) {
		t.Error("TotalNetReturnPct is NaN — division by zero when totalCost=0")
	}
}

// =============================================================================
// 3. All positions closed — totalCost includes closed positions' net capital
//
// When all positions are sold, totalCost = sum(TotalInvested - TotalProceeds).
// If the sells returned more than invested (profit), totalCost is reduced.
// availableCash = totalCash - totalCost.
// TotalValue = equity(0) + availableCash.
// =============================================================================

func TestSyncPortfolio_AllPositionsClosed_TotalCostFromClosedTrades(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "SOLD", Exchange: "AU",
				Name: "Sold Co", Units: 0, CurrentPrice: 0, MarketValue: 0,
				TotalCost: 0, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				{ID: "t1", HoldingID: "1", Symbol: "SOLD", Type: "buy", Units: 100, Price: 10.00, Fees: 0},
				{ID: "t2", HoldingID: "1", Symbol: "SOLD", Type: "sell", Units: 100, Price: 12.00, Fees: 0},
			},
		},
	}

	// Cash: deposited $1000, then stock sold for $1200 (proceeds return to cash conceptually)
	cashSvc := &stubCashFlowSvc{
		ledgers: map[string]*models.CashFlowLedger{
			"SMSF": {
				PortfolioName: "SMSF",
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Amount: 1000, Date: today},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// totalCost = TotalInvested(1000) - TotalProceeds(1200) = -200
	// Negative totalCost means we got back more than we invested.
	if !approxEqual(portfolio.NetEquityCost, -200, 0.01) {
		t.Errorf("TotalCost = %.2f, want -200 (sold at profit)", portfolio.NetEquityCost)
	}

	// availableCash = 1000 - (-200) = 1200
	if !approxEqual(portfolio.NetCashBalance, 1200, 0.01) {
		t.Errorf("AvailableCash = %.2f, want 1200", portfolio.NetCashBalance)
	}

	// TotalValue = equity(0) + availableCash(1200) = 1200
	if !approxEqual(portfolio.PortfolioValue, 1200, 0.01) {
		t.Errorf("TotalValue = %.2f, want 1200", portfolio.PortfolioValue)
	}

	// totalGainPct: totalCost is -200 which is NOT > 0, so guard triggers, should be 0
	if portfolio.NetEquityReturnPct != 0 {
		t.Errorf("TotalNetReturnPct = %.2f, want 0 (totalCost <= 0, guard triggers)", portfolio.NetEquityReturnPct)
	}
}

// =============================================================================
// 4. Division by zero: totalCost = 0 (no trades or all proceeds equal invested)
//
// totalGainPct has `if totalCost > 0` guard.
// Verify the guard works and we get 0, not NaN/Inf.
// =============================================================================

func TestSyncPortfolio_ZeroTotalCost_GainPctZero(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "EVEN", Exchange: "AU",
				Name: "Break Even Co", Units: 0, CurrentPrice: 0, MarketValue: 0,
				TotalCost: 0, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				// Buy $1000, sell $1000 → TotalInvested = TotalProceeds → totalCost = 0
				{ID: "t1", HoldingID: "1", Symbol: "EVEN", Type: "buy", Units: 100, Price: 10.00, Fees: 0},
				{ID: "t2", HoldingID: "1", Symbol: "EVEN", Type: "sell", Units: 100, Price: 10.00, Fees: 0},
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

	if portfolio.NetEquityCost != 0 {
		t.Errorf("TotalCost = %.2f, want 0 (buy = sell)", portfolio.NetEquityCost)
	}

	if portfolio.NetEquityReturnPct != 0 {
		t.Errorf("TotalNetReturnPct = %.2f, want 0 (guard: totalCost <= 0)", portfolio.NetEquityReturnPct)
	}

	if math.IsNaN(portfolio.NetEquityReturnPct) || math.IsInf(portfolio.NetEquityReturnPct, 0) {
		t.Errorf("TotalNetReturnPct = %v — division by zero guard failed", portfolio.NetEquityReturnPct)
	}
}

// =============================================================================
// 5. FX rate = 0 with USD holdings — TotalProceeds must not cause div-by-zero
//
// When EODHD returns fxRate=0 (or call fails), the code skips FX conversion
// entirely (the `if fxRate > 0` guard). USD holdings stay in USD.
// TotalProceeds is populated from trades, but since FX conversion is skipped,
// values remain in original currency. No division by zero should occur.
// =============================================================================

func TestSyncPortfolio_FXRateZero_NoDivisionByZero(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "AAPL", Exchange: "NASDAQ",
				Name: "Apple", Units: 10, CurrentPrice: 200.00,
				MarketValue: 2000, TotalCost: 1500, Currency: "USD",
				LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				{ID: "t1", HoldingID: "1", Symbol: "AAPL", Type: "buy", Units: 20, Price: 150.00, Fees: 0},
				{ID: "t2", HoldingID: "1", Symbol: "AAPL", Type: "sell", Units: 10, Price: 180.00, Fees: 0},
			},
		},
	}

	// EODHD client returns 0 for FX rate (simulating failure)
	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			if ticker == "AUDUSD.FOREX" {
				return &models.RealTimeQuote{Close: 0}, nil // zero rate
			}
			return nil, errNotFound
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// FX rate is 0, so conversion should be skipped. Holding stays in USD.
	var aapl *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "AAPL" {
			aapl = &portfolio.Holdings[i]
			break
		}
	}
	if aapl == nil {
		t.Fatal("AAPL holding not found")
	}

	// Currency should still be USD (conversion skipped)
	if aapl.Currency != "USD" {
		t.Errorf("Currency = %s, want USD (FX conversion skipped when rate=0)", aapl.Currency)
	}

	// TotalProceeds should be populated from trades (in USD, not converted)
	// sell: 10 * 180 - 0 = 1800
	if !approxEqual(aapl.GrossProceeds, 1800, 0.01) {
		t.Errorf("TotalProceeds = %.2f, want 1800 (not FX-converted)", aapl.GrossProceeds)
	}

	// No NaN or Inf anywhere
	if math.IsNaN(portfolio.EquityValue) || math.IsInf(portfolio.EquityValue, 0) {
		t.Errorf("TotalValue = %v — must be finite even with FX rate 0", portfolio.PortfolioValue)
	}
	if math.IsNaN(portfolio.NetEquityCost) || math.IsInf(portfolio.NetEquityCost, 0) {
		t.Errorf("TotalCost = %v — must be finite", portfolio.NetEquityCost)
	}
}

// =============================================================================
// 6. Negative totalCost from cost base decrease exceeding invested
//
// If a holding has a "cost base decrease" that exceeds its buy cost,
// totalInvested goes negative. This creates a negative totalCost.
// availableCash = totalCash - (-N) = totalCash + N (more cash available).
// =============================================================================

func TestSyncPortfolio_NegativeTotalCost_CostBaseDecreaseExceedsInvested(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "CBD", Exchange: "AU",
				Name: "Cost Base Decrease Co", Units: 100, CurrentPrice: 10.00,
				MarketValue: 1000, TotalCost: 500, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				{ID: "t1", HoldingID: "1", Symbol: "CBD", Type: "buy", Units: 100, Price: 5.00, Fees: 0},
				// Cost base decrease of $600 exceeds the $500 invested
				{ID: "t2", HoldingID: "1", Symbol: "CBD", Type: "cost base decrease", Value: 600},
			},
		},
	}

	cashSvc := &stubCashFlowSvc{
		ledgers: map[string]*models.CashFlowLedger{
			"SMSF": {
				PortfolioName: "SMSF",
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Amount: 1000, Date: today},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// totalInvested = 500 (buy) - 600 (cost base decrease) = -100
	// totalCost = totalInvested(-100) - totalProceeds(0) = -100
	if !approxEqual(portfolio.NetEquityCost, -100, 0.01) {
		t.Errorf("TotalCost = %.2f, want -100", portfolio.NetEquityCost)
	}

	// availableCash = 1000 - (-100) = 1100
	if !approxEqual(portfolio.NetCashBalance, 1100, 0.01) {
		t.Errorf("AvailableCash = %.2f, want 1100", portfolio.NetCashBalance)
	}

	// totalGainPct guard: totalCost=-100 is NOT > 0, so should be 0
	if portfolio.NetEquityReturnPct != 0 {
		t.Errorf("TotalNetReturnPct = %.2f, want 0 (totalCost <= 0)", portfolio.NetEquityReturnPct)
	}

	// Must not be NaN or Inf
	if math.IsNaN(portfolio.EquityValue) || math.IsInf(portfolio.EquityValue, 0) {
		t.Errorf("TotalValue = %v — must be finite", portfolio.PortfolioValue)
	}
}

// =============================================================================
// 7. Weight denominator with negative availableCash
//
// When availableCash is negative, weightDenom = equity + availableCash could
// be small or even negative. If weightDenom <= 0, the `if weightDenom > 0`
// guard means no weights are computed (all 0). This is correct: if the
// denominator is nonsensical, skip weight computation.
// =============================================================================

func TestSyncPortfolio_NegativeAvailableCash_WeightDenomGuard(t *testing.T) {
	today := time.Now()

	// Equity = 100, totalCost = 200, totalCash = 50
	// availableCash = 50 - 200 = -150
	// weightDenom = 100 + (-150) = -50 → guard triggers, weights = 0
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "SML", Exchange: "AU",
				Name: "Small Co", Units: 10, CurrentPrice: 10.00,
				MarketValue: 100, TotalCost: 100, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				{ID: "t1", HoldingID: "1", Symbol: "SML", Type: "buy", Units: 10, Price: 20.00, Fees: 0},
				// No sells, so totalInvested=200, totalProceeds=0, totalCost=200
			},
		},
	}

	cashSvc := &stubCashFlowSvc{
		ledgers: map[string]*models.CashFlowLedger{
			"SMSF": {
				PortfolioName: "SMSF",
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Amount: 50, Date: today},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// weightDenom = 100 + (-150) = -50
	// Guard: if weightDenom > 0 → false → weights stay 0
	for _, h := range portfolio.Holdings {
		if h.PortfolioWeightPct != 0 {
			t.Errorf("Weight for %s = %.2f, want 0 (weightDenom <= 0)", h.Ticker, h.PortfolioWeightPct)
		}
	}

	// Must not be NaN
	for _, h := range portfolio.Holdings {
		if math.IsNaN(h.PortfolioWeightPct) || math.IsInf(h.PortfolioWeightPct, 0) {
			t.Errorf("Weight for %s = %v — must be finite", h.Ticker, h.PortfolioWeightPct)
		}
	}
}

// =============================================================================
// 8. Mixed open and closed positions — totalCost sums both
//
// Open position: TotalInvested=5000, TotalProceeds=0 → contribution = 5000
// Closed position: TotalInvested=3000, TotalProceeds=4000 → contribution = -1000
// totalCost = 5000 + (-1000) = 4000
// =============================================================================

func TestSyncPortfolio_MixedOpenClosed_TotalCostSumsBoth(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "OPEN", Exchange: "AU",
				Name: "Open Co", Units: 100, CurrentPrice: 60.00,
				MarketValue: 6000, TotalCost: 5000, LastUpdated: today,
			},
			{
				ID: "2", PortfolioID: "1", Ticker: "CLOSED", Exchange: "AU",
				Name: "Closed Co", Units: 0, CurrentPrice: 0,
				MarketValue: 0, TotalCost: 0, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				{ID: "t1", HoldingID: "1", Symbol: "OPEN", Type: "buy", Units: 100, Price: 50.00, Fees: 0},
			},
			"2": {
				{ID: "t3", HoldingID: "2", Symbol: "CLOSED", Type: "buy", Units: 50, Price: 60.00, Fees: 0},
				{ID: "t4", HoldingID: "2", Symbol: "CLOSED", Type: "sell", Units: 50, Price: 80.00, Fees: 0},
			},
		},
	}

	cashSvc := &stubCashFlowSvc{
		ledgers: map[string]*models.CashFlowLedger{
			"SMSF": {
				PortfolioName: "SMSF",
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Amount: 10000, Date: today},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// OPEN: TotalInvested=5000, TotalProceeds=0 → net = 5000
	// CLOSED: TotalInvested=3000, TotalProceeds=4000 → net = -1000
	// totalCost = 5000 + (-1000) = 4000
	if !approxEqual(portfolio.NetEquityCost, 4000, 0.01) {
		t.Errorf("TotalCost = %.2f, want 4000 (open+closed net capital)", portfolio.NetEquityCost)
	}

	// availableCash = 10000 - 4000 = 6000
	if !approxEqual(portfolio.NetCashBalance, 6000, 0.01) {
		t.Errorf("AvailableCash = %.2f, want 6000", portfolio.NetCashBalance)
	}

	// TotalValue = equity(6000) + availableCash(6000) = 12000
	if !approxEqual(portfolio.PortfolioValue, 12000, 0.01) {
		t.Errorf("TotalValue = %.2f, want 12000", portfolio.PortfolioValue)
	}
}

// =============================================================================
// 9. No cashflow service (nil) — totalCash=0, availableCash=0
//
// When cashflowSvc is nil, totalCash defaults to 0.
// availableCash = 0 - totalCost.
// If totalCost > 0, availableCash is negative.
// TotalValue = equity + negative availableCash.
// =============================================================================

func TestSyncPortfolio_NoCashflowService_TotalCashZero(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP Group", Units: 100, CurrentPrice: 50.00,
				MarketValue: 5000, TotalCost: 4000, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				{ID: "t1", HoldingID: "1", Symbol: "BHP", Type: "buy", Units: 100, Price: 40.00, Fees: 0},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	// NOT calling svc.SetCashFlowService — cashflowSvc remains nil

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// totalCash = 0 (no cashflow service)
	if portfolio.GrossCashBalance != 0 {
		t.Errorf("TotalCash = %.2f, want 0 (no cashflow service)", portfolio.GrossCashBalance)
	}

	// availableCash = 0 - 4000 = -4000
	if !approxEqual(portfolio.NetCashBalance, -4000, 0.01) {
		t.Errorf("AvailableCash = %.2f, want -4000 (no cash, all in equities)", portfolio.NetCashBalance)
	}

	// TotalValue = 5000 + (-4000) = 1000
	if !approxEqual(portfolio.PortfolioValue, 1000, 0.01) {
		t.Errorf("TotalValue = %.2f, want 1000", portfolio.PortfolioValue)
	}
}

// =============================================================================
// 10. TotalProceeds FX conversion for USD holding
//
// When AUDUSD rate is available (e.g., 0.65), all USD holding values are
// divided by 0.65 to convert to AUD. TotalProceeds must be converted too.
// =============================================================================

func TestSyncPortfolio_TotalProceeds_FXConverted(t *testing.T) {
	today := time.Now()
	fxRate := 0.65 // AUDUSD rate

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "AAPL", Exchange: "NASDAQ",
				Name: "Apple", Units: 10, CurrentPrice: 200.00,
				MarketValue: 2000, TotalCost: 1500, Currency: "USD",
				LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				{ID: "t1", HoldingID: "1", Symbol: "AAPL", Type: "buy", Units: 20, Price: 150.00, Fees: 0},
				{ID: "t2", HoldingID: "1", Symbol: "AAPL", Type: "sell", Units: 10, Price: 180.00, Fees: 0},
			},
		},
	}

	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			if ticker == "AUDUSD.FOREX" {
				return &models.RealTimeQuote{Close: fxRate}, nil
			}
			return nil, errNotFound
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	var aapl *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "AAPL" {
			aapl = &portfolio.Holdings[i]
			break
		}
	}
	if aapl == nil {
		t.Fatal("AAPL holding not found")
	}

	// TotalProceeds in USD: sell 10 * 180 = 1800
	// After FX: 1800 / 0.65 = 2769.23
	expectedProceedsAUD := 1800.0 / fxRate
	if !approxEqual(aapl.GrossProceeds, expectedProceedsAUD, 1.0) {
		t.Errorf("TotalProceeds = %.2f, want ~%.2f (FX-converted from USD)", aapl.GrossProceeds, expectedProceedsAUD)
	}

	// Currency should be AUD (converted)
	if aapl.Currency != "AUD" {
		t.Errorf("Currency = %s, want AUD (after FX conversion)", aapl.Currency)
	}

	// OriginalCurrency should be USD
	if aapl.OriginalCurrency != "USD" {
		t.Errorf("OriginalCurrency = %s, want USD", aapl.OriginalCurrency)
	}
}

// =============================================================================
// 11. Large portfolio — numerical stability with many holdings
//
// 50 holdings each with $10k invested, $2k proceeds.
// totalCost = 50 * (10000 - 2000) = 400000
// Verifies no floating point accumulation errors.
// =============================================================================

func TestSyncPortfolio_LargePortfolio_NumericalStability(t *testing.T) {
	today := time.Now()
	numHoldings := 50

	holdings := make([]*models.NavexaHolding, numHoldings)
	trades := make(map[string][]*models.NavexaTrade)
	for i := 0; i < numHoldings; i++ {
		id := string(rune('A'+i/26)) + string(rune('A'+i%26)) // AA, AB, etc.
		holdings[i] = &models.NavexaHolding{
			ID: id, PortfolioID: "1", Ticker: id, Exchange: "AU",
			Name: "Stock " + id, Units: 100, CurrentPrice: 120.00,
			MarketValue: 12000, TotalCost: 10000, LastUpdated: today,
		}
		trades[id] = []*models.NavexaTrade{
			{ID: "b" + id, HoldingID: id, Symbol: id, Type: "buy", Units: 120, Price: 100.00, Fees: 0},
			{ID: "s" + id, HoldingID: id, Symbol: id, Type: "sell", Units: 20, Price: 100.00, Fees: 0},
		}
	}

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: holdings,
		trades:   trades,
	}

	cashSvc := &stubCashFlowSvc{
		ledgers: map[string]*models.CashFlowLedger{
			"SMSF": {
				PortfolioName: "SMSF",
				Transactions: []models.CashTransaction{
					{Account: "Trading", Category: models.CashCatContribution, Amount: 500000, Date: today},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// Each holding: TotalInvested = 120*100 = 12000, TotalProceeds = 20*100 = 2000
	// Net per holding = 10000. Total = 50 * 10000 = 500000
	if !approxEqual(portfolio.NetEquityCost, 500000, 1.0) {
		t.Errorf("TotalCost = %.2f, want 500000", portfolio.NetEquityCost)
	}

	// availableCash = 500000 - 500000 = 0
	if !approxEqual(portfolio.NetCashBalance, 0, 1.0) {
		t.Errorf("AvailableCash = %.2f, want ~0", portfolio.NetCashBalance)
	}

	// Equity = 50 * 12000 = 600000
	// TotalValue = 600000 + 0 = 600000
	if !approxEqual(portfolio.EquityValue, 600000, 1.0) {
		t.Errorf("TotalValue = %.2f, want ~600000", portfolio.PortfolioValue)
	}

	// All weights should sum to ~100
	var weightSum float64
	for _, h := range portfolio.Holdings {
		weightSum += h.PortfolioWeightPct
	}
	if !approxEqual(weightSum, 100.0, 0.1) {
		t.Errorf("Weight sum = %.2f, want ~100", weightSum)
	}
}

// =============================================================================
// 12. ReviewPortfolio uses AvailableCash (not TotalCash) in TotalValue
//
// With the fix, review.EquityValue = liveTotal + portfolio.NetCashBalance.
// When AvailableCash = 0 (not stored on old portfolios), it degrades to
// liveTotal + 0 = liveTotal (backward compatible).
// When AvailableCash is set, it properly reflects uninvested cash.
// =============================================================================

func TestReviewPortfolio_UsesAvailableCash_NotTotalCash(t *testing.T) {
	today := time.Now()
	holdingPrice := 50.00
	units := 100.0
	holdingMV := holdingPrice * units // 5000

	portfolio := &models.Portfolio{
		Name:               "SMSF",
		EquityValue: holdingMV,
		PortfolioValue:         5000 + 3000, // equity + availableCash
		GrossCashBalance:          10000,       // total ledger balance (larger)
		NetEquityCost:          7000,        // net capital in equities
		NetCashBalance:         3000,        // = GrossCashBalance - NetEquityCost
		LastSynced:         today,
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "AU", Name: "BHP Group", Units: units, CurrentPrice: holdingPrice, MarketValue: holdingMV, PortfolioWeightPct: 100},
		},
	}

	uds := newMemUserDataStore()
	storePortfolio(t, uds, portfolio)

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: []models.EODBar{
				{Date: today, Close: holdingPrice},
				{Date: today.AddDate(0, 0, -1), Close: 49.00},
			}},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU"},
		}},
	}

	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			return nil, errNotFound
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)
	review, err := svc.ReviewPortfolio(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("ReviewPortfolio: %v", err)
	}

	// review.PortfolioValue should be liveTotal(5000) + AvailableCash(3000) = 8000
	expectedTV := holdingMV + 3000.0
	if math.Abs(review.PortfolioValue-expectedTV) > 1.0 {
		t.Errorf("PortfolioValue = %.2f, want %.2f (liveTotal + AvailableCash, not + TotalCash)", review.PortfolioValue, expectedTV)
	}

	// Must NOT include full TotalCash ($10k) — that would give 15000
	if review.PortfolioValue > 14000 {
		t.Errorf("PortfolioValue = %.2f is inflated — using TotalCash instead of AvailableCash", review.PortfolioValue)
	}
}

// =============================================================================
// 13. Holding with TotalProceeds populated — verify it's in the JSON output
//
// TotalProceeds should appear in the Holding struct when populated from trades.
// For holdings without sells, TotalProceeds = 0.
// =============================================================================

func TestSyncPortfolio_HoldingTotalProceeds_Populated(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "1", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP Group", Units: 50, CurrentPrice: 60.00,
				MarketValue: 3000, TotalCost: 2500, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"1": {
				{ID: "t1", HoldingID: "1", Symbol: "BHP", Type: "buy", Units: 100, Price: 50.00, Fees: 10},
				{ID: "t2", HoldingID: "1", Symbol: "BHP", Type: "sell", Units: 50, Price: 55.00, Fees: 5},
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

	var bhp *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "BHP" {
			bhp = &portfolio.Holdings[i]
			break
		}
	}
	if bhp == nil {
		t.Fatal("BHP holding not found")
	}

	// TotalInvested = 100*50 + 10 = 5010
	if !approxEqual(bhp.GrossInvested, 5010, 0.01) {
		t.Errorf("TotalInvested = %.2f, want 5010", bhp.GrossInvested)
	}

	// TotalProceeds = 50*55 - 5 = 2745
	if !approxEqual(bhp.GrossProceeds, 2745, 0.01) {
		t.Errorf("TotalProceeds = %.2f, want 2745", bhp.GrossProceeds)
	}

	// Portfolio totalCost = TotalInvested - TotalProceeds = 5010 - 2745 = 2265
	if !approxEqual(portfolio.NetEquityCost, 2265, 0.01) {
		t.Errorf("TotalCost = %.2f, want 2265", portfolio.NetEquityCost)
	}
}

// =============================================================================
// 14. Historical values use AvailableCash not TotalCash
//
// yesterday_total = yesterdayEquityTotal + availableCash
// last_week_total = lastWeekEquityTotal + availableCash
// =============================================================================

func TestPopulateHistoricalValues_Stress_AvailableCashNotTotalCash(t *testing.T) {
	today := time.Now()

	portfolio := &models.Portfolio{
		Name:               "SMSF",
		EquityValue: 10000,
		PortfolioValue:         13000, // equity(10000) + availableCash(3000)
		GrossCashBalance:          8000,  // total ledger balance
		NetEquityCost:          5000,  // net capital in equities
		NetCashBalance:         3000,  // = GrossCashBalance - NetEquityCost
		Holdings: []models.Holding{
			{
				Ticker: "BHP", Exchange: "AU", Name: "BHP",
				Units: 100, CurrentPrice: 100.00, MarketValue: 10000,
			},
		},
	}

	// Yesterday BHP was at $98, last week at $95
	marketData := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU",
			EOD: []models.EODBar{
				{Date: today, Close: 100, AdjClose: 100},                 // today
				{Date: today.AddDate(0, 0, -1), Close: 98, AdjClose: 98}, // yesterday
				{Date: today.AddDate(0, 0, -2), Close: 97, AdjClose: 97}, // 2 days ago
				{Date: today.AddDate(0, 0, -3), Close: 96, AdjClose: 96}, // 3 days ago
				{Date: today.AddDate(0, 0, -4), Close: 96, AdjClose: 96}, // 4 days ago
				{Date: today.AddDate(0, 0, -5), Close: 95, AdjClose: 95}, // 5 days ago (last week)
				{Date: today.AddDate(0, 0, -6), Close: 94, AdjClose: 94}, // 6 days ago
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: marketData},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)
	svc.populateHistoricalValues(context.Background(), portfolio)

	// yesterday_total = 98*100 + availableCash(3000) = 12800
	expectedYesterday := 98.0*100 + 3000.0
	if !approxEqual(portfolio.PortfolioYesterdayValue, expectedYesterday, 1.0) {
		t.Errorf("YesterdayTotal = %.2f, want %.2f (yesterday equity + availableCash)", portfolio.PortfolioYesterdayValue, expectedYesterday)
	}

	// Must NOT use TotalCash (8000) — that would give 17800
	if portfolio.PortfolioYesterdayValue > 15000 {
		t.Errorf("YesterdayTotal = %.2f is inflated — using TotalCash instead of AvailableCash", portfolio.PortfolioYesterdayValue)
	}

	// last_week_total = 95*100 + availableCash(3000) = 12500
	expectedLastWeek := 95.0*100 + 3000.0
	if !approxEqual(portfolio.PortfolioLastWeekValue, expectedLastWeek, 1.0) {
		t.Errorf("LastWeekTotal = %.2f, want %.2f (lastweek equity + availableCash)", portfolio.PortfolioLastWeekValue, expectedLastWeek)
	}
}
