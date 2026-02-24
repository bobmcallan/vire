package portfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// storeWatchlist is a test helper that saves a watchlist into a memUserDataStore as JSON.
func storeWatchlist(t *testing.T, store *memUserDataStore, wl *models.PortfolioWatchlist) {
	t.Helper()
	data, err := json.Marshal(wl)
	if err != nil {
		t.Fatalf("failed to marshal watchlist: %v", err)
	}
	store.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "watchlist",
		Key:     wl.PortfolioName,
		Value:   string(data),
	})
}

// storeStrategy is a test helper that saves a strategy into a memUserDataStore as JSON.
func storeStrategy(t *testing.T, store *memUserDataStore, strategy *models.PortfolioStrategy) {
	t.Helper()
	data, err := json.Marshal(strategy)
	if err != nil {
		t.Fatalf("failed to marshal strategy: %v", err)
	}
	store.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "strategy",
		Key:     strategy.PortfolioName,
		Value:   string(data),
	})
}

func TestReviewWatchlist_EmptyWatchlistReturnsError(t *testing.T) {
	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items:         []models.WatchlistItem{},
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	_, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err == nil {
		t.Fatal("expected error for empty watchlist")
	}
	if got := err.Error(); got != "watchlist 'SMSF' is empty" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestReviewWatchlist_NotFoundReturnsError(t *testing.T) {
	uds := newMemUserDataStore()

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	_, err := svc.ReviewWatchlist(context.Background(), "NonExistent", interfaces.ReviewOptions{})
	if err == nil {
		t.Fatal("expected error for missing watchlist")
	}
}

func TestReviewWatchlist_NoMarketDataReturnsWatchAction(t *testing.T) {
	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{Ticker: "BHP.AU", Name: "BHP Group", Verdict: models.WatchlistVerdictPass},
			{Ticker: "CBA.AU", Name: "Commonwealth Bank", Verdict: models.WatchlistVerdictWatch},
		},
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	review, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(review.ItemReviews) != 2 {
		t.Fatalf("expected 2 item reviews, got %d", len(review.ItemReviews))
	}

	for _, ir := range review.ItemReviews {
		if ir.ActionRequired != "WATCH" {
			t.Errorf("ticker %s: expected ActionRequired=WATCH, got %s", ir.Item.Ticker, ir.ActionRequired)
		}
		if ir.ActionReason != "Market data unavailable — signals pending data collection" {
			t.Errorf("ticker %s: unexpected ActionReason: %s", ir.Item.Ticker, ir.ActionReason)
		}
		if ir.Signals != nil {
			t.Errorf("ticker %s: expected nil signals when no market data", ir.Item.Ticker)
		}
	}

	if review.PortfolioName != "SMSF" {
		t.Errorf("expected portfolio_name=SMSF, got %s", review.PortfolioName)
	}
	if review.ReviewDate.IsZero() {
		t.Error("expected non-zero review date")
	}
}

func TestReviewWatchlist_WithMarketDataReturnsSignalsAndAction(t *testing.T) {
	today := time.Now()

	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{Ticker: "BHP.AU", Name: "BHP Group", Verdict: models.WatchlistVerdictPass},
		},
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					EOD: []models.EODBar{
						{Date: today, Close: 45.00},
						{Date: today.AddDate(0, 0, -1), Close: 44.00},
					},
					Fundamentals: &models.Fundamentals{
						Ticker:    "BHP.AU",
						PE:        15.5,
						MarketCap: 100e9,
						Sector:    "Materials",
					},
				},
			},
		},
		signalStore: &reviewSignalStorage{
			signals: map[string]*models.TickerSignals{
				"BHP.AU": {
					Ticker:    "BHP.AU",
					Technical: models.TechnicalSignals{RSI: 55},
				},
			},
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	review, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(review.ItemReviews) != 1 {
		t.Fatalf("expected 1 item review, got %d", len(review.ItemReviews))
	}

	ir := review.ItemReviews[0]
	if ir.Item.Ticker != "BHP.AU" {
		t.Errorf("expected ticker BHP.AU, got %s", ir.Item.Ticker)
	}
	if ir.Signals == nil {
		t.Fatal("expected non-nil signals")
	}
	if ir.Fundamentals == nil {
		t.Fatal("expected non-nil fundamentals")
	}
	if ir.Fundamentals.Sector != "Materials" {
		t.Errorf("expected sector Materials, got %s", ir.Fundamentals.Sector)
	}

	// Overnight move: 45.00 - 44.00 = 1.00
	if !approxEqual(ir.OvernightMove, 1.00, 0.01) {
		t.Errorf("OvernightMove = %.2f, want 1.00", ir.OvernightMove)
	}
	// Overnight pct: (1.00 / 44.00) * 100 = ~2.27%
	if !approxEqual(ir.OvernightPct, (1.0/44.0)*100, 0.1) {
		t.Errorf("OvernightPct = %.2f, want ~2.27", ir.OvernightPct)
	}

	// With RSI at 55, no strategy, expect COMPLIANT
	if ir.ActionRequired != "COMPLIANT" {
		t.Errorf("expected ActionRequired=COMPLIANT, got %s", ir.ActionRequired)
	}
}

func TestReviewWatchlist_WithStrategyReturnsCompliance(t *testing.T) {
	today := time.Now()

	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{Ticker: "TINY.AU", Name: "Tiny Corp", Verdict: models.WatchlistVerdictWatch},
		},
	})
	storeStrategy(t, uds, &models.PortfolioStrategy{
		PortfolioName: "SMSF",
		CompanyFilter: models.CompanyFilter{
			MinMarketCap: 1e9, // Minimum $1B market cap
		},
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"TINY.AU": {
					Ticker: "TINY.AU",
					EOD: []models.EODBar{
						{Date: today, Close: 5.00},
						{Date: today.AddDate(0, 0, -1), Close: 5.00},
					},
					Fundamentals: &models.Fundamentals{
						Ticker:    "TINY.AU",
						MarketCap: 500e6, // $500M — below $1B minimum
						Sector:    "Technology",
					},
				},
			},
		},
		signalStore: &reviewSignalStorage{
			signals: map[string]*models.TickerSignals{
				"TINY.AU": {Ticker: "TINY.AU", Technical: models.TechnicalSignals{RSI: 50}},
			},
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	review, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ir := review.ItemReviews[0]
	if ir.Compliance == nil {
		t.Fatal("expected non-nil compliance when strategy exists")
	}
	if ir.Compliance.Status != models.ComplianceStatusNonCompliant {
		t.Errorf("expected non-compliant status, got %s", ir.Compliance.Status)
	}
	if len(ir.Compliance.Reasons) == 0 {
		t.Error("expected compliance reasons for small market cap")
	}
}

func TestReviewWatchlist_NoStrategyNoCompliance(t *testing.T) {
	today := time.Now()

	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{Ticker: "BHP.AU", Name: "BHP Group", Verdict: models.WatchlistVerdictPass},
		},
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					EOD: []models.EODBar{
						{Date: today, Close: 45.00},
						{Date: today.AddDate(0, 0, -1), Close: 44.00},
					},
				},
			},
		},
		signalStore: &reviewSignalStorage{
			signals: map[string]*models.TickerSignals{
				"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
			},
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	review, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ir := review.ItemReviews[0]
	if ir.Compliance != nil {
		t.Errorf("expected nil compliance when no strategy, got %+v", ir.Compliance)
	}
}

func TestReviewWatchlist_LiveQuotesOverlay(t *testing.T) {
	today := time.Now()

	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{Ticker: "BHP.AU", Name: "BHP Group", Verdict: models.WatchlistVerdictPass},
		},
	})

	prevClose := 44.00
	livePrice := 46.50

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					EOD: []models.EODBar{
						{Date: today, Close: 45.00},
						{Date: today.AddDate(0, 0, -1), Close: prevClose},
					},
				},
			},
		},
		signalStore: &reviewSignalStorage{
			signals: map[string]*models.TickerSignals{
				"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
			},
		},
	}

	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			if ticker == "BHP.AU" {
				return &models.RealTimeQuote{Code: ticker, Close: livePrice, Timestamp: today}, nil
			}
			return nil, fmt.Errorf("not found")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	review, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ir := review.ItemReviews[0]

	// Overnight move with live quote: 46.50 - 44.00 = 2.50
	if !approxEqual(ir.OvernightMove, 2.50, 0.01) {
		t.Errorf("OvernightMove = %.2f, want 2.50", ir.OvernightMove)
	}
	expectedPct := (2.50 / prevClose) * 100
	if !approxEqual(ir.OvernightPct, expectedPct, 0.1) {
		t.Errorf("OvernightPct = %.2f, want %.2f", ir.OvernightPct, expectedPct)
	}
}

func TestReviewWatchlist_NilEODHDClientDoesNotBreak(t *testing.T) {
	today := time.Now()

	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{Ticker: "BHP.AU", Name: "BHP Group", Verdict: models.WatchlistVerdictPass},
		},
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					EOD: []models.EODBar{
						{Date: today, Close: 45.00},
						{Date: today.AddDate(0, 0, -1), Close: 44.00},
					},
				},
			},
		},
		signalStore: &reviewSignalStorage{
			signals: map[string]*models.TickerSignals{
				"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
			},
		},
	}

	logger := common.NewLogger("error")
	// Explicitly nil eodhd client
	svc := NewService(storage, nil, nil, nil, logger)

	review, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error with nil EODHD client: %v", err)
	}
	if len(review.ItemReviews) != 1 {
		t.Fatalf("expected 1 item review, got %d", len(review.ItemReviews))
	}

	// Falls back to EOD-based overnight move: 45.00 - 44.00 = 1.00
	ir := review.ItemReviews[0]
	if !approxEqual(ir.OvernightMove, 1.00, 0.01) {
		t.Errorf("OvernightMove = %.2f, want 1.00 (EOD fallback)", ir.OvernightMove)
	}
}

func TestReviewWatchlist_GeneratesAlerts(t *testing.T) {
	today := time.Now()

	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{Ticker: "HOT.AU", Name: "Hot Stock", Verdict: models.WatchlistVerdictWatch},
		},
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"HOT.AU": {
					Ticker: "HOT.AU",
					EOD: []models.EODBar{
						{Date: today, Close: 10.00},
						{Date: today.AddDate(0, 0, -1), Close: 10.00},
					},
				},
			},
		},
		signalStore: &reviewSignalStorage{
			signals: map[string]*models.TickerSignals{
				"HOT.AU": {
					Ticker: "HOT.AU",
					Technical: models.TechnicalSignals{
						RSI: 80, // overbought — should trigger alert
					},
				},
			},
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	review, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(review.Alerts) == 0 {
		t.Fatal("expected at least one alert for overbought RSI")
	}

	foundRSI := false
	for _, alert := range review.Alerts {
		if alert.Signal == "rsi_overbought" {
			foundRSI = true
			if alert.Ticker != "HOT.AU" {
				t.Errorf("alert ticker = %s, want HOT.AU", alert.Ticker)
			}
		}
	}
	if !foundRSI {
		t.Error("expected rsi_overbought alert")
	}

	// Action should be EXIT TRIGGER for overbought RSI
	ir := review.ItemReviews[0]
	if ir.ActionRequired != "EXIT TRIGGER" {
		t.Errorf("ActionRequired = %s, want EXIT TRIGGER", ir.ActionRequired)
	}
}

func TestReviewWatchlist_MixedMarketDataAvailability(t *testing.T) {
	today := time.Now()

	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{Ticker: "BHP.AU", Name: "BHP Group", Verdict: models.WatchlistVerdictPass},
			{Ticker: "UNKNOWN.AU", Name: "Unknown Corp", Verdict: models.WatchlistVerdictWatch},
		},
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					EOD: []models.EODBar{
						{Date: today, Close: 45.00},
						{Date: today.AddDate(0, 0, -1), Close: 44.00},
					},
				},
				// UNKNOWN.AU has no market data
			},
		},
		signalStore: &reviewSignalStorage{
			signals: map[string]*models.TickerSignals{
				"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
			},
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	review, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(review.ItemReviews) != 2 {
		t.Fatalf("expected 2 item reviews, got %d", len(review.ItemReviews))
	}

	// BHP should have signals
	bhp := review.ItemReviews[0]
	if bhp.Signals == nil {
		t.Error("BHP.AU should have signals")
	}
	if bhp.ActionRequired == "WATCH" && bhp.ActionReason == "Market data unavailable — signals pending data collection" {
		t.Error("BHP.AU should not have market-data-unavailable status")
	}

	// UNKNOWN should fall back to WATCH
	unknown := review.ItemReviews[1]
	if unknown.Signals != nil {
		t.Error("UNKNOWN.AU should have nil signals")
	}
	if unknown.ActionRequired != "WATCH" {
		t.Errorf("UNKNOWN.AU ActionRequired = %s, want WATCH", unknown.ActionRequired)
	}
}

func TestReviewWatchlist_SignalsFallbackToCompute(t *testing.T) {
	today := time.Now()

	uds := newMemUserDataStore()
	storeWatchlist(t, uds, &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{Ticker: "NEW.AU", Name: "New Stock", Verdict: models.WatchlistVerdictWatch},
		},
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"NEW.AU": {
					Ticker: "NEW.AU",
					EOD: []models.EODBar{
						{Date: today, Close: 10.00},
						{Date: today.AddDate(0, 0, -1), Close: 9.50},
					},
				},
			},
		},
		// Empty signal storage — forces fallback to compute
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	review, err := svc.ReviewWatchlist(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ir := review.ItemReviews[0]
	// Signal should be computed (signalComputer.Compute produces minimal signals from EOD data)
	if ir.Signals == nil {
		t.Error("expected signals to be computed when not in signal storage")
	}
}
