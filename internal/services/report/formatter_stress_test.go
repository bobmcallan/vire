package report

import (
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// hasExactHeading checks if markdown contains a heading at exactly the given level.
// For example, hasExactHeading(md, 2, "Technical Signals") matches "## Technical Signals"
// but NOT "### Technical Signals".
func hasExactHeading(md string, level int, title string) bool {
	prefix := strings.Repeat("#", level) + " " + title
	deeper := "#" + prefix // one level deeper
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == prefix && !strings.HasPrefix(trimmed, deeper) {
			return true
		}
	}
	return false
}

// ============================================================================
// Devils-advocate stress tests for EODHD report sectioning changes.
// Validates heading structure, edge cases, and backward compatibility
// of formatStockReport, formatETFReport, and formatSignalsTable.
// ============================================================================

// ============================================================================
// 1. Stock report — EODHD Market Analysis section structure
// ============================================================================

func TestFormatter_StockReport_EODHDSection_HasParentHeading(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{
			Ticker:   "BHP",
			Exchange: "ASX",
			Name:     "BHP Group",
		},
		Fundamentals: &models.Fundamentals{
			Sector: "Materials",
			PE:     15.0,
			EPS:    3.50,
			Beta:   1.2,
		},
		Signals: &models.TickerSignals{
			Trend:            "bullish",
			TrendDescription: "Strong uptrend",
			Price:            models.PriceSignals{Current: 42.0, SMA20: 40.0, SMA50: 38.0, SMA200: 35.0},
			Technical:        models.TechnicalSignals{RSI: 55.0, MACD: 0.5, VolumeRatio: 1.2},
			PBAS:             models.PBASSignal{Score: 0.8},
			VLI:              models.VLISignal{Score: 0.6},
			Regime:           models.RegimeSignal{Current: "trending"},
		},
		ActionRequired: "HOLD",
		ActionReason:   "No signals",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatStockReport(hr, review)

	// Must have ## EODHD Market Analysis
	if !strings.Contains(md, "## EODHD Market Analysis") {
		t.Error("stock report missing '## EODHD Market Analysis' parent heading")
	}

	// Fundamentals must be ### (not ##)
	if strings.Contains(md, "## Fundamentals") && !strings.Contains(md, "### Fundamentals") {
		t.Error("stock report has '## Fundamentals' instead of '### Fundamentals'")
	}
	if !strings.Contains(md, "### Fundamentals") {
		t.Error("stock report missing '### Fundamentals' sub-heading")
	}

	// Technical Signals must be ### (not ##)
	if hasExactHeading(md, 2, "Technical Signals") {
		t.Error("stock report has '## Technical Signals' — should be '### Technical Signals'")
	}
	if !hasExactHeading(md, 3, "Technical Signals") {
		t.Error("stock report missing '### Technical Signals' sub-heading")
	}
}

func TestFormatter_StockReport_SubSections_AreFourthLevel(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group"},
		Fundamentals: &models.Fundamentals{
			PE:                 15.0,
			EPS:                3.50,
			Beta:               1.2,
			ProfitMargin:       0.15,
			ReturnOnEquityTTM:  0.20,
			RevenueTTM:         50000000000,
			EPSEstimateCurrent: 4.0,
			AnalystRatings:     &models.AnalystRatings{Rating: "Buy", TargetPrice: 50.0},
		},
		Signals:        &models.TickerSignals{Trend: "neutral", Price: models.PriceSignals{Current: 42.0}, Technical: models.TechnicalSignals{RSI: 50}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatStockReport(hr, review)

	// Sub-sections under Fundamentals should be #### (not ###)
	fourthLevelTitles := []string{
		"Valuation",
		"Profitability",
		"Growth & Scale",
		"Estimates",
		"Analyst Consensus",
	}
	for _, title := range fourthLevelTitles {
		if !hasExactHeading(md, 4, title) {
			t.Errorf("stock report missing '#### %s' sub-section", title)
		}
	}

	// These should NOT appear as ### level (that would be wrong nesting)
	for _, title := range fourthLevelTitles {
		if hasExactHeading(md, 3, title) {
			t.Errorf("stock report has '### %s' — should be #### level, not ###", title)
		}
	}
}

func TestFormatter_StockReport_SectionsOutsideEODHD_StillH2(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group"},
		Fundamentals: &models.Fundamentals{
			PE:   15.0,
			EPS:  3.50,
			Beta: 1.2,
		},
		Signals:        &models.TickerSignals{Trend: "neutral", Price: models.PriceSignals{Current: 42.0}, Technical: models.TechnicalSignals{RSI: 50}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
		NewsIntelligence: &models.NewsIntelligence{
			OverallSentiment: "neutral",
			Summary:          "Nothing notable",
		},
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatStockReport(hr, review)

	// Non-EODHD sections must remain at ## level
	h2Sections := []string{
		"## Position",
		"## News Intelligence",
		"## Risk Flags",
	}
	for _, section := range h2Sections {
		if !strings.Contains(md, section) {
			t.Errorf("stock report missing %q — non-EODHD sections should remain at ## level", section)
		}
	}
}

// ============================================================================
// 2. ETF report — EODHD Market Analysis section structure
// ============================================================================

func TestFormatter_ETFReport_EODHDSection_HasParentHeading(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{
			Ticker:   "VAS",
			Exchange: "ASX",
			Name:     "Vanguard Australian Shares Index ETF",
		},
		Fundamentals: &models.Fundamentals{
			Beta:         0.95,
			ExpenseRatio: 0.001,
			TopHoldings: []models.ETFHolding{
				{Name: "BHP Group", Ticker: "BHP", Weight: 10.5},
			},
			SectorWeights: []models.SectorWeight{
				{Sector: "Financials", Weight: 30.0},
			},
			CountryWeights: []models.CountryWeight{
				{Country: "Australia", Weight: 100.0},
			},
		},
		Signals: &models.TickerSignals{
			Trend:     "neutral",
			Price:     models.PriceSignals{Current: 90.0, SMA20: 89.0, SMA50: 88.0, SMA200: 85.0},
			Technical: models.TechnicalSignals{RSI: 50.0, VolumeRatio: 1.0},
			PBAS:      models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{},
		},
		ActionRequired: "HOLD",
		ActionReason:   "Index tracker",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatETFReport(hr, review)

	// Must have ## EODHD Market Analysis
	if !strings.Contains(md, "## EODHD Market Analysis") {
		t.Error("ETF report missing '## EODHD Market Analysis' parent heading")
	}

	// Sub-sections must be ### level
	etfSubSections := []string{
		"### Fund Metrics",
		"### Top Holdings",
		"### Sector Breakdown",
		"### Country Exposure",
		"### Technical Signals",
	}
	for _, section := range etfSubSections {
		if !strings.Contains(md, section) {
			t.Errorf("ETF report missing %q sub-heading", section)
		}
	}

	// Must NOT have ## level for fund-specific sections
	wrongLevelTitles := []string{
		"Fund Metrics",
		"Top Holdings",
		"Sector Breakdown",
		"Country Exposure",
		"Technical Signals",
	}
	for _, title := range wrongLevelTitles {
		if hasExactHeading(md, 2, title) {
			t.Errorf("ETF report has '## %s' — should be ### level under EODHD Market Analysis", title)
		}
	}
}

func TestFormatter_ETFReport_NonEODHDSections_StillH2(t *testing.T) {
	hr := &models.HoldingReview{
		Holding:        models.Holding{Ticker: "VAS", Exchange: "ASX", Name: "VAS ETF"},
		Fundamentals:   &models.Fundamentals{Beta: 0.95},
		Signals:        &models.TickerSignals{Trend: "neutral", Price: models.PriceSignals{Current: 90.0}, Technical: models.TechnicalSignals{RSI: 50}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
		NewsIntelligence: &models.NewsIntelligence{
			OverallSentiment: "neutral",
			Summary:          "ETF tracking well",
		},
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatETFReport(hr, review)

	h2Sections := []string{
		"## Position",
		"## News Intelligence",
		"## Risk Flags",
	}
	for _, section := range h2Sections {
		if !strings.Contains(md, section) {
			t.Errorf("ETF report missing %q — non-EODHD sections should remain at ## level", section)
		}
	}
}

// ============================================================================
// 3. formatSignalsTable — always ### level
// ============================================================================

func TestFormatter_SignalsTable_IsH3(t *testing.T) {
	signals := &models.TickerSignals{
		Trend: "bullish",
		Price: models.PriceSignals{Current: 42.0, SMA20: 40.0, SMA50: 38.0, SMA200: 35.0},
		Technical: models.TechnicalSignals{
			RSI: 55.0, MACD: 0.5, VolumeRatio: 1.2,
		},
		PBAS:   models.PBASSignal{Score: 0.8},
		VLI:    models.VLISignal{Score: 0.6},
		Regime: models.RegimeSignal{Current: "trending"},
	}

	md := formatSignalsTable(signals)

	if !hasExactHeading(md, 3, "Technical Signals") {
		t.Error("formatSignalsTable should produce '### Technical Signals' heading")
	}
	if hasExactHeading(md, 2, "Technical Signals") {
		t.Error("formatSignalsTable should use ### (not ##) for heading")
	}
}

func TestFormatter_SignalsTable_NilSignals(t *testing.T) {
	md := formatSignalsTable(nil)

	if !strings.Contains(md, "### Technical Signals") {
		t.Error("nil signals should still produce '### Technical Signals' heading")
	}
	if !strings.Contains(md, "*Signal data not available*") {
		t.Error("nil signals should show 'Signal data not available' message")
	}
}

// ============================================================================
// 4. Edge cases — no fundamentals, no signals, empty everything
// ============================================================================

func TestFormatter_StockReport_NilFundamentals(t *testing.T) {
	hr := &models.HoldingReview{
		Holding:        models.Holding{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group"},
		Fundamentals:   nil,
		Signals:        &models.TickerSignals{Trend: "neutral", Price: models.PriceSignals{Current: 42.0}, Technical: models.TechnicalSignals{RSI: 50}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatStockReport(hr, review)

	// Should still have EODHD section with Technical Signals (even without Fundamentals)
	if !strings.Contains(md, "## EODHD Market Analysis") {
		t.Error("stock report without fundamentals should still have EODHD Market Analysis section")
	}
	if !strings.Contains(md, "### Technical Signals") {
		t.Error("stock report without fundamentals should still have Technical Signals")
	}
	// Should NOT have ### Fundamentals when fundamentals is nil
	if strings.Contains(md, "### Fundamentals") {
		t.Error("stock report should not have Fundamentals heading when fundamentals is nil")
	}
}

func TestFormatter_StockReport_NilSignals(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group"},
		Fundamentals: &models.Fundamentals{
			PE:   15.0,
			EPS:  3.50,
			Beta: 1.2,
		},
		Signals:        nil,
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatStockReport(hr, review)

	// Should have EODHD section with Fundamentals and "Signal data not available"
	if !strings.Contains(md, "## EODHD Market Analysis") {
		t.Error("stock report without signals should still have EODHD Market Analysis section")
	}
	if !strings.Contains(md, "### Fundamentals") {
		t.Error("stock report should still have Fundamentals heading when signals is nil")
	}
	if !strings.Contains(md, "*Signal data not available*") {
		t.Error("stock report should show 'Signal data not available' when signals is nil")
	}
}

func TestFormatter_StockReport_BothNil(t *testing.T) {
	hr := &models.HoldingReview{
		Holding:        models.Holding{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group"},
		Fundamentals:   nil,
		Signals:        nil,
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatStockReport(hr, review)

	// Should not panic, should produce valid markdown
	if md == "" {
		t.Error("stock report with nil fundamentals and nil signals should not be empty")
	}
	// The EODHD section should still exist (with "Signal data not available")
	if !strings.Contains(md, "## EODHD Market Analysis") {
		t.Error("stock report with all nil data should still have EODHD Market Analysis section")
	}
}

func TestFormatter_ETFReport_NilFundamentals(t *testing.T) {
	hr := &models.HoldingReview{
		Holding:        models.Holding{Ticker: "VAS", Exchange: "ASX", Name: "VAS ETF"},
		Fundamentals:   nil,
		Signals:        &models.TickerSignals{Trend: "neutral", Price: models.PriceSignals{Current: 90.0}, Technical: models.TechnicalSignals{RSI: 50}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatETFReport(hr, review)

	// Should still have EODHD section
	if !strings.Contains(md, "## EODHD Market Analysis") {
		t.Error("ETF report without fundamentals should still have EODHD Market Analysis section")
	}
	// Fund-specific sections should be absent when fundamentals is nil
	if strings.Contains(md, "### Fund Metrics") {
		t.Error("ETF report should not have Fund Metrics when fundamentals is nil")
	}
	if strings.Contains(md, "### Top Holdings") {
		t.Error("ETF report should not have Top Holdings when fundamentals is nil")
	}
}

func TestFormatter_ETFReport_NilSignals(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{Ticker: "VAS", Exchange: "ASX", Name: "VAS ETF"},
		Fundamentals: &models.Fundamentals{
			Beta:         0.95,
			ExpenseRatio: 0.001,
		},
		Signals:        nil,
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatETFReport(hr, review)

	if !strings.Contains(md, "*Signal data not available*") {
		t.Error("ETF report should show 'Signal data not available' when signals is nil")
	}
}

// ============================================================================
// 5. EODHD section ordering — Fundamentals before Technical Signals
// ============================================================================

func TestFormatter_StockReport_EODHDSectionOrdering(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group"},
		Fundamentals: &models.Fundamentals{
			PE: 15.0, EPS: 3.50, Beta: 1.2,
		},
		Signals:        &models.TickerSignals{Trend: "bullish", Price: models.PriceSignals{Current: 42.0, SMA20: 40.0}, Technical: models.TechnicalSignals{RSI: 55}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatStockReport(hr, review)

	// EODHD section must come before News Intelligence
	eodhIdx := strings.Index(md, "## EODHD Market Analysis")
	fundamentalsIdx := strings.Index(md, "### Fundamentals")
	signalsIdx := strings.Index(md, "### Technical Signals")

	if eodhIdx < 0 || fundamentalsIdx < 0 || signalsIdx < 0 {
		t.Fatal("missing expected sections in stock report")
	}

	if eodhIdx > fundamentalsIdx {
		t.Error("EODHD Market Analysis must come before Fundamentals")
	}
	if fundamentalsIdx > signalsIdx {
		t.Error("Fundamentals must come before Technical Signals")
	}

	// News Intelligence (if present) should come after EODHD section
	newsIdx := strings.Index(md, "## News Intelligence")
	if newsIdx >= 0 && newsIdx < signalsIdx {
		t.Error("News Intelligence should come after the EODHD Market Analysis section")
	}
}

func TestFormatter_ETFReport_EODHDSectionOrdering(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{Ticker: "VAS", Exchange: "ASX", Name: "VAS ETF"},
		Fundamentals: &models.Fundamentals{
			Beta:           0.95,
			ExpenseRatio:   0.001,
			TopHoldings:    []models.ETFHolding{{Name: "BHP", Weight: 10.0}},
			SectorWeights:  []models.SectorWeight{{Sector: "Financials", Weight: 30.0}},
			CountryWeights: []models.CountryWeight{{Country: "Australia", Weight: 100.0}},
		},
		Signals:        &models.TickerSignals{Trend: "neutral", Price: models.PriceSignals{Current: 90.0}, Technical: models.TechnicalSignals{RSI: 50}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatETFReport(hr, review)

	eodhIdx := strings.Index(md, "## EODHD Market Analysis")
	fundMetricsIdx := strings.Index(md, "### Fund Metrics")
	topHoldingsIdx := strings.Index(md, "### Top Holdings")
	sectorIdx := strings.Index(md, "### Sector Breakdown")
	countryIdx := strings.Index(md, "### Country Exposure")
	signalsIdx := strings.Index(md, "### Technical Signals")

	positions := []struct {
		name string
		idx  int
	}{
		{"## EODHD Market Analysis", eodhIdx},
		{"### Fund Metrics", fundMetricsIdx},
		{"### Top Holdings", topHoldingsIdx},
		{"### Sector Breakdown", sectorIdx},
		{"### Country Exposure", countryIdx},
		{"### Technical Signals", signalsIdx},
	}

	for i := 1; i < len(positions); i++ {
		if positions[i].idx < 0 {
			continue // section absent (e.g., conditionally rendered)
		}
		if positions[i-1].idx < 0 {
			continue
		}
		if positions[i].idx < positions[i-1].idx {
			t.Errorf("section order violation: %q (idx %d) should come after %q (idx %d)",
				positions[i].name, positions[i].idx,
				positions[i-1].name, positions[i-1].idx)
		}
	}
}

// ============================================================================
// 6. Backward compatibility — report pipeline uses full HoldingReview
// ============================================================================

func TestFormatter_StockReport_StillUsesFullData(t *testing.T) {
	// Verify the formatter still accesses all fields from HoldingReview
	// (it should — the slim response is only for the HTTP handler, not the report)
	hr := &models.HoldingReview{
		Holding: models.Holding{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group"},
		Fundamentals: &models.Fundamentals{
			PE: 15.0, EPS: 3.50, Beta: 1.2, Sector: "Materials",
		},
		Signals: &models.TickerSignals{
			Trend: "bullish", Price: models.PriceSignals{Current: 42.0, SMA20: 40.0, SMA50: 38.0, SMA200: 35.0},
			Technical: models.TechnicalSignals{RSI: 55, MACD: 0.5, VolumeRatio: 1.2},
			PBAS:      models.PBASSignal{Score: 0.8}, VLI: models.VLISignal{Score: 0.6},
			Regime: models.RegimeSignal{Current: "trending"},
		},
		NewsIntelligence: &models.NewsIntelligence{
			OverallSentiment: "positive",
			Summary:          "Strong earnings",
		},
		FilingSummaries: []models.FilingSummary{
			{
				Headline: "H1 Results",
				Type:     "financial_results",
				Date:     time.Now(),
				Revenue:  "$25B",
			},
		},
		Timeline: &models.CompanyTimeline{
			BusinessModel: "Mining",
			GeneratedAt:   time.Now(),
		},
		ActionRequired: "HOLD",
		ActionReason:   "All signals positive",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatStockReport(hr, review)

	// All these sections should be rendered from the full HoldingReview
	expectedSections := []string{
		"## EODHD Market Analysis",
		"### Fundamentals",
		"### Technical Signals",
		"## News Intelligence",
		"## Company Releases",
		"## Company Timeline",
		"## Risk Flags",
	}
	for _, section := range expectedSections {
		if !strings.Contains(md, section) {
			t.Errorf("report pipeline should still render %q from full HoldingReview", section)
		}
	}
}

// ============================================================================
// 7. Heading uniqueness — no duplicate EODHD headings
// ============================================================================

func TestFormatter_StockReport_SingleEODHDHeading(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{Ticker: "BHP", Exchange: "ASX", Name: "BHP Group"},
		Fundamentals: &models.Fundamentals{
			PE: 15.0, EPS: 3.50, Beta: 1.2,
		},
		Signals:        &models.TickerSignals{Trend: "neutral", Price: models.PriceSignals{Current: 42.0}, Technical: models.TechnicalSignals{RSI: 50}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatStockReport(hr, review)

	count := strings.Count(md, "## EODHD Market Analysis")
	if count != 1 {
		t.Errorf("expected exactly 1 '## EODHD Market Analysis' heading, got %d", count)
	}
}

func TestFormatter_ETFReport_SingleEODHDHeading(t *testing.T) {
	hr := &models.HoldingReview{
		Holding: models.Holding{Ticker: "VAS", Exchange: "ASX", Name: "VAS ETF"},
		Fundamentals: &models.Fundamentals{
			Beta:           0.95,
			TopHoldings:    []models.ETFHolding{{Name: "BHP", Weight: 10.0}},
			SectorWeights:  []models.SectorWeight{{Sector: "Financials", Weight: 30.0}},
			CountryWeights: []models.CountryWeight{{Country: "AU", Weight: 100.0}},
		},
		Signals:        &models.TickerSignals{Trend: "neutral", Price: models.PriceSignals{Current: 90.0}, Technical: models.TechnicalSignals{RSI: 50}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "SMSF"}

	md := formatETFReport(hr, review)

	count := strings.Count(md, "## EODHD Market Analysis")
	if count != 1 {
		t.Errorf("expected exactly 1 '## EODHD Market Analysis' heading, got %d", count)
	}
}

// ============================================================================
// 8. Edge case — empty fundamentals (all zeroes)
// ============================================================================

func TestFormatter_StockReport_EmptyFundamentals(t *testing.T) {
	// Fundamentals struct exists but all fields are zero-valued
	hr := &models.HoldingReview{
		Holding:        models.Holding{Ticker: "XYZ", Exchange: "ASX", Name: "XYZ Corp"},
		Fundamentals:   &models.Fundamentals{},
		Signals:        &models.TickerSignals{Trend: "neutral", Price: models.PriceSignals{Current: 10.0}, Technical: models.TechnicalSignals{RSI: 50}, PBAS: models.PBASSignal{}, VLI: models.VLISignal{}, Regime: models.RegimeSignal{}},
		ActionRequired: "HOLD",
	}
	review := &models.PortfolioReview{PortfolioName: "TEST"}

	md := formatStockReport(hr, review)

	// Should still have EODHD section and Fundamentals heading
	if !strings.Contains(md, "## EODHD Market Analysis") {
		t.Error("empty fundamentals should still produce EODHD section")
	}
	if !strings.Contains(md, "### Fundamentals") {
		t.Error("empty fundamentals should still produce Fundamentals heading")
	}
	// Valuation should always appear (it's unconditional)
	if !strings.Contains(md, "#### Valuation") {
		t.Error("Valuation sub-section should always appear when fundamentals is non-nil")
	}
}
