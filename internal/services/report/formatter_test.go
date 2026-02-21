package report

import (
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// TestFormatStockReport_EODHDSection verifies that the stock report wraps
// Fundamentals and Technical Signals under an "## EODHD Market Analysis" section.
func TestFormatStockReport_EODHDSection(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{
			Ticker:       "BHP",
			Name:         "BHP Group",
			Exchange:     "ASX",
			Units:        100,
			CurrentPrice: 42.0,
			MarketValue:  4200.0,
			AvgCost:      40.0,
			TotalCost:    4000.0,
			GainLoss:     200.0,
		},
		Fundamentals: &models.Fundamentals{
			MarketCap:     100000000000,
			PE:            15.0,
			EPS:           2.80,
			DividendYield: 0.045,
			Beta:          1.1,
			Sector:        "Materials",
			Industry:      "Mining",
		},
		Signals: &models.TickerSignals{
			Ticker: "BHP",
			Trend:  "bullish",
			Price: models.PriceSignals{
				Current: 42.0,
				SMA20:   41.0,
				SMA50:   40.0,
				SMA200:  38.0,
			},
			Technical: models.TechnicalSignals{
				RSI:         55.0,
				RSISignal:   "neutral",
				MACD:        0.5,
				VolumeRatio: 1.2,
			},
			PBAS:   models.PBASSignal{Score: 0.8, Interpretation: "normal"},
			VLI:    models.VLISignal{Score: 0.6, Interpretation: "normal"},
			Regime: models.RegimeSignal{Current: "bullish", Description: "uptrend"},
		},
		ActionRequired: "HOLD",
		ActionReason:   "stable",
	}

	review := &models.PortfolioReview{
		PortfolioName:  "SMSF",
		ReviewDate:     time.Now(),
		HoldingReviews: []models.HoldingReview{*hr},
	}

	result := formatStockReport(hr, review)

	// Should contain the EODHD parent section
	if !strings.Contains(result, "## EODHD Market Analysis") {
		t.Error("expected '## EODHD Market Analysis' section header")
	}

	// Fundamentals should be at ### level (not ##)
	if strings.Contains(result, "\n## Fundamentals\n") {
		t.Error("Fundamentals should be ### level, not ## level")
	}
	if !strings.Contains(result, "### Fundamentals") {
		t.Error("expected '### Fundamentals' sub-section")
	}

	// Sub-sections should be at #### level
	if strings.Contains(result, "\n### Valuation\n") {
		t.Error("Valuation should be #### level, not ### level")
	}
	if !strings.Contains(result, "#### Valuation") {
		t.Error("expected '#### Valuation' sub-sub-section")
	}

	// Technical Signals should be at ### level (not ##)
	if strings.Contains(result, "\n## Technical Signals\n") {
		t.Error("Technical Signals should be ### level, not ## level")
	}
	if !strings.Contains(result, "### Technical Signals") {
		t.Error("expected '### Technical Signals' sub-section")
	}

	// Sections after EODHD should still be at ## level
	if !strings.Contains(result, "## Risk Flags") {
		t.Error("Risk Flags should still be at ## level")
	}

	// Verify ordering: EODHD section should come before Risk Flags
	eodhIdx := strings.Index(result, "## EODHD Market Analysis")
	riskIdx := strings.Index(result, "## Risk Flags")
	if eodhIdx >= riskIdx {
		t.Error("EODHD Market Analysis section should appear before Risk Flags")
	}
}

// TestFormatStockReport_NoFundamentals verifies behavior when fundamentals are nil.
func TestFormatStockReport_NoFundamentals(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{
			Ticker:       "XYZ",
			Name:         "XYZ Corp",
			Units:        50,
			CurrentPrice: 10.0,
			MarketValue:  500.0,
		},
		Signals: &models.TickerSignals{
			Ticker: "XYZ",
			Trend:  "neutral",
			Price:  models.PriceSignals{Current: 10.0},
			Technical: models.TechnicalSignals{
				RSI:       50.0,
				RSISignal: "neutral",
			},
			PBAS:   models.PBASSignal{Score: 0.5, Interpretation: "normal"},
			VLI:    models.VLISignal{Score: 0.5, Interpretation: "normal"},
			Regime: models.RegimeSignal{Current: "neutral", Description: "flat"},
		},
		ActionRequired: "WATCH",
		ActionReason:   "new stock",
	}

	review := &models.PortfolioReview{
		PortfolioName:  "Test",
		HoldingReviews: []models.HoldingReview{*hr},
	}

	result := formatStockReport(hr, review)

	// EODHD section should still be present (for Technical Signals at minimum)
	if !strings.Contains(result, "## EODHD Market Analysis") {
		t.Error("expected EODHD Market Analysis section even without fundamentals")
	}

	// Should NOT have fundamentals sub-section
	if strings.Contains(result, "### Fundamentals") {
		t.Error("should not have Fundamentals section when fundamentals are nil")
	}

	// Should have technical signals
	if !strings.Contains(result, "### Technical Signals") {
		t.Error("expected Technical Signals section")
	}
}

// TestFormatETFReport_EODHDSection verifies that the ETF report wraps fund metrics,
// top holdings, sector breakdown, country exposure, and technical signals under EODHD.
func TestFormatETFReport_EODHDSection(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{
			Ticker:       "VAS",
			Name:         "Vanguard Australian Shares",
			Exchange:     "ASX",
			Units:        200,
			CurrentPrice: 95.0,
			MarketValue:  19000.0,
			AvgCost:      80.0,
			TotalCost:    16000.0,
			GainLoss:     3000.0,
		},
		Fundamentals: &models.Fundamentals{
			Beta:            0.95,
			ExpenseRatio:    0.001,
			ManagementStyle: "Passive",
			TopHoldings: []models.ETFHolding{
				{Ticker: "BHP", Name: "BHP Group", Weight: 10.5},
				{Ticker: "CBA", Name: "Commonwealth Bank", Weight: 8.2},
			},
			SectorWeights: []models.SectorWeight{
				{Sector: "Financials", Weight: 30.0},
				{Sector: "Materials", Weight: 20.0},
			},
			CountryWeights: []models.CountryWeight{
				{Country: "Australia", Weight: 98.0},
			},
		},
		Signals: &models.TickerSignals{
			Ticker: "VAS",
			Trend:  "bullish",
			Price: models.PriceSignals{
				Current: 95.0,
				SMA20:   93.0,
				SMA50:   91.0,
				SMA200:  88.0,
			},
			Technical: models.TechnicalSignals{
				RSI:         60.0,
				RSISignal:   "neutral",
				MACD:        0.3,
				VolumeRatio: 1.0,
			},
			PBAS:   models.PBASSignal{Score: 0.5, Interpretation: "normal"},
			VLI:    models.VLISignal{Score: 0.5, Interpretation: "normal"},
			Regime: models.RegimeSignal{Current: "bullish", Description: "uptrend"},
		},
		ActionRequired: "HOLD",
		ActionReason:   "stable ETF",
	}

	review := &models.PortfolioReview{
		PortfolioName:  "SMSF",
		HoldingReviews: []models.HoldingReview{*hr},
	}

	result := formatETFReport(hr, review)

	// Should contain the EODHD parent section
	if !strings.Contains(result, "## EODHD Market Analysis") {
		t.Error("expected '## EODHD Market Analysis' section header in ETF report")
	}

	// Fund Metrics should be ### level
	if strings.Contains(result, "\n## Fund Metrics\n") {
		t.Error("Fund Metrics should be ### level, not ## level")
	}
	if !strings.Contains(result, "### Fund Metrics") {
		t.Error("expected '### Fund Metrics'")
	}

	// Top Holdings should be ### level
	if strings.Contains(result, "\n## Top Holdings\n") {
		t.Error("Top Holdings should be ### level, not ## level")
	}
	if !strings.Contains(result, "### Top Holdings") {
		t.Error("expected '### Top Holdings'")
	}

	// Sector Breakdown should be ### level
	if strings.Contains(result, "\n## Sector Breakdown\n") {
		t.Error("Sector Breakdown should be ### level, not ## level")
	}
	if !strings.Contains(result, "### Sector Breakdown") {
		t.Error("expected '### Sector Breakdown'")
	}

	// Country Exposure should be ### level
	if strings.Contains(result, "\n## Country Exposure\n") {
		t.Error("Country Exposure should be ### level, not ## level")
	}
	if !strings.Contains(result, "### Country Exposure") {
		t.Error("expected '### Country Exposure'")
	}

	// Technical Signals should be ### level
	if strings.Contains(result, "\n## Technical Signals\n") {
		t.Error("Technical Signals should be ### level, not ## level")
	}
	if !strings.Contains(result, "### Technical Signals") {
		t.Error("expected '### Technical Signals'")
	}

	// Sections after EODHD should still be at ## level
	if !strings.Contains(result, "## Risk Flags") {
		t.Error("Risk Flags should remain at ## level")
	}
}

// TestFormatSignalsTable_HeadingLevel verifies the signals table uses ### heading.
func TestFormatSignalsTable_HeadingLevel(t *testing.T) {
	signals := &models.TickerSignals{
		Ticker: "BHP",
		Trend:  "bullish",
		Price: models.PriceSignals{
			Current: 42.0,
			SMA20:   41.0,
			SMA50:   40.0,
			SMA200:  38.0,
		},
		Technical: models.TechnicalSignals{
			RSI:         55.0,
			RSISignal:   "neutral",
			MACD:        0.5,
			VolumeRatio: 1.2,
		},
		PBAS:   models.PBASSignal{Score: 0.8, Interpretation: "normal"},
		VLI:    models.VLISignal{Score: 0.6, Interpretation: "normal"},
		Regime: models.RegimeSignal{Current: "bullish", Description: "uptrend"},
	}

	result := formatSignalsTable(signals)

	if !strings.Contains(result, "### Technical Signals") {
		t.Error("expected '### Technical Signals' heading")
	}
	// Make sure it's NOT ## (without ###)
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "## Technical Signals") && !strings.HasPrefix(line, "### Technical Signals") {
			t.Error("should use ### not ## for Technical Signals heading")
		}
	}
}

// TestFormatSignalsTable_Nil verifies the signals table handles nil signals.
func TestFormatSignalsTable_Nil(t *testing.T) {
	result := formatSignalsTable(nil)

	if !strings.Contains(result, "### Technical Signals") {
		t.Error("expected '### Technical Signals' heading even with nil signals")
	}
	if !strings.Contains(result, "Signal data not available") {
		t.Error("expected 'Signal data not available' message")
	}
}
