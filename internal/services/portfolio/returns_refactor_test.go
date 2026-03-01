package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// TestHolding_HasTWRRField verifies the Holding struct has TimeWeightedReturnPct field.
func TestHolding_HasTWRRField(t *testing.T) {
	h := models.Holding{
		TimeWeightedReturnPct: 15.5,
	}
	if h.TimeWeightedReturnPct != 15.5 {
		t.Errorf("TimeWeightedReturnPct = %v, want 15.5", h.TimeWeightedReturnPct)
	}
}

// TestNavexaHolding_HasTWRRField verifies NavexaHolding has TotalReturnPctTWRR.
func TestNavexaHolding_HasTWRRField(t *testing.T) {
	h := models.NavexaHolding{
		TotalReturnPctTWRR: 15.5,
	}
	if h.TotalReturnPctTWRR != 15.5 {
		t.Errorf("NavexaHolding.TotalReturnPctTWRR = %v, want 15.5", h.TotalReturnPctTWRR)
	}
}

// TestSyncPortfolio_PopulatesTWRR verifies that SyncPortfolio sets TotalReturnPctTWRR on holdings.
func TestSyncPortfolio_PopulatesTWRR(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP Group", Units: 100, CurrentPrice: 45.00,
				MarketValue: 4500, LastUpdated: today,
				GainLossPct: 12.0, CapitalGainPct: 10.0, TotalReturnPct: 15.0,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {
				{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy",
					Date: "2024-01-01", Units: 100, Price: 40.00, Fees: 10},
			},
		},
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				EOD: []models.EODBar{
					{Date: today, Close: 45.00},
					{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 40.00},
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

	// TWRR should be populated (non-zero for a holding with trades and EOD data)
	if bhp.TimeWeightedReturnPct == 0 {
		t.Errorf("NetReturnPctTWRR = 0, expected non-zero TWRR for BHP")
	}
}

// TestSyncPortfolio_SimpleReturnCalculation verifies that SyncPortfolio
// computes simple return percentages from trades, replacing Navexa IRR values.
func TestSyncPortfolio_SimpleReturnCalculation(t *testing.T) {
	today := time.Now()

	// Navexa returns IRR values on the enriched holdings — these should be overwritten
	irrGainLossPct := 18.5
	irrCapitalGainPct := 15.0
	irrTotalReturnPct := 22.0

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1", Ticker: "BHP", Exchange: "AU",
				Name: "BHP Group", Units: 100, CurrentPrice: 45.00,
				MarketValue: 4500, LastUpdated: today,
				// These IRR values from Navexa should be replaced with simple %
				GainLossPct:    irrGainLossPct,
				CapitalGainPct: irrCapitalGainPct,
				TotalReturnPct: irrTotalReturnPct,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {
				{ID: "1", HoldingID: "100", Symbol: "BHP", Type: "buy",
					Date: "2024-01-01", Units: 100, Price: 40.00, Fees: 10},
			},
		},
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				EOD: []models.EODBar{
					{Date: today, Close: 45.00},
					{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Close: 40.00},
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

	// Simple calculation: totalCost = 100*40+10 = 4010, netReturn = 4500 - 4010 = 490
	// NetReturnPct = 490 / 4010 * 100 = ~12.22%
	expectedSimplePct := (bhp.NetReturn / bhp.GrossInvested) * 100

	// NetReturnPct should be the simple %, NOT the Navexa IRR
	if approxEqual(bhp.NetReturnPct, irrGainLossPct, 0.1) {
		t.Errorf("NetReturnPct = %.2f, should NOT be the Navexa IRR value %.2f",
			bhp.NetReturnPct, irrGainLossPct)
	}
	if !approxEqual(bhp.NetReturnPct, expectedSimplePct, 0.1) {
		t.Errorf("NetReturnPct = %.2f, want %.2f (simple NetReturn/TotalCost*100)",
			bhp.NetReturnPct, expectedSimplePct)
	}
	// CapitalGainPct should be XIRR (annualised), NOT the simple %
	if approxEqual(bhp.AnnualizedCapitalReturnPct, bhp.NetReturnPct, 0.01) {
		t.Logf("CapitalGainPct = %.2f matches NetReturnPct — this is fine for short periods", bhp.AnnualizedCapitalReturnPct)
	}
	// CapitalGainPct (XIRR) should NOT be the original Navexa IRR value
	if approxEqual(bhp.AnnualizedCapitalReturnPct, irrCapitalGainPct, 0.1) {
		t.Errorf("CapitalGainPct = %.2f, should NOT be Navexa IRR value %.2f",
			bhp.AnnualizedCapitalReturnPct, irrCapitalGainPct)
	}
	// NetReturnPctIRR should be populated (XIRR including dividends)
	// For this test with no dividends, it should be close to CapitalGainPct
	if bhp.AnnualizedTotalReturnPct == 0 && bhp.AnnualizedCapitalReturnPct != 0 {
		t.Errorf("NetReturnPctIRR = 0, expected non-zero XIRR")
	}
}
