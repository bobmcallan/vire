package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/models"
)

// TestHolding_HasTWRRField verifies the Holding struct has TotalReturnPctTWRR field.
// This test fails until the refactor adds the field.
func TestHolding_HasTWRRField(t *testing.T) {
	h := models.Holding{
		TotalReturnPctTWRR: 15.5,
	}
	if h.TotalReturnPctTWRR != 15.5 {
		t.Errorf("TotalReturnPctTWRR = %v, want 15.5", h.TotalReturnPctTWRR)
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

	portfolioStore := &stubPortfolioStorage{}
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
		portfolioStore: portfolioStore,
		marketStore:    marketStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, navexa, nil, nil, logger)

	portfolio, err := svc.SyncPortfolio(context.Background(), "SMSF", true)
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
	if bhp.TotalReturnPctTWRR == 0 {
		t.Errorf("TotalReturnPctTWRR = 0, expected non-zero TWRR for BHP")
	}
}

// TestSyncPortfolio_NoSimpleReturnCalculation verifies that SyncPortfolio
// no longer calculates simple return percentages from trades.
// After the refactor, GainLossPct should come from Navexa IRR, not from local trade calculation.
func TestSyncPortfolio_NoSimpleReturnCalculation(t *testing.T) {
	today := time.Now()

	// Navexa returns IRR values on the enriched holdings
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
				// These are the IRR values from Navexa that should be preserved
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

	portfolioStore := &stubPortfolioStorage{}
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
		portfolioStore: portfolioStore,
		marketStore:    marketStore,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, navexa, nil, nil, logger)

	portfolio, err := svc.SyncPortfolio(context.Background(), "SMSF", true)
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

	// After refactor: GainLossPct should be the IRR value from Navexa, NOT locally computed simple %
	// The simple calculation would give: (4500 - 4010) / 4010 * 100 = ~12.22%
	// The IRR from Navexa is 18.5%
	// If the simple calculation is still running, it will overwrite the Navexa IRR value
	if !approxEqual(bhp.GainLossPct, irrGainLossPct, 0.1) {
		t.Errorf("GainLossPct = %.2f, want %.2f (IRR from Navexa, not simple calculation)",
			bhp.GainLossPct, irrGainLossPct)
	}
	if !approxEqual(bhp.CapitalGainPct, irrCapitalGainPct, 0.1) {
		t.Errorf("CapitalGainPct = %.2f, want %.2f (IRR from Navexa)",
			bhp.CapitalGainPct, irrCapitalGainPct)
	}
	if !approxEqual(bhp.TotalReturnPct, irrTotalReturnPct, 0.1) {
		t.Errorf("TotalReturnPct = %.2f, want %.2f (IRR from Navexa)",
			bhp.TotalReturnPct, irrTotalReturnPct)
	}
}
