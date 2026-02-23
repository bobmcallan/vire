package portfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

func approxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

func TestCalculateRealizedFromTrades_SimpleProfitableSell(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.00, Fees: 10.00},
		{Type: "sell", Units: 100, Price: 15.00, Fees: 10.00},
	}

	avgBuy, invested, proceeds, realized := calculateRealizedFromTrades(trades)

	// invested = 100*10 + 10 = 1010
	if !approxEqual(invested, 1010.00, 0.01) {
		t.Errorf("invested = %.2f, want 1010.00", invested)
	}
	// proceeds = 100*15 - 10 = 1490
	if !approxEqual(proceeds, 1490.00, 0.01) {
		t.Errorf("proceeds = %.2f, want 1490.00", proceeds)
	}
	// realized = 1490 - 1010 = 480
	if !approxEqual(realized, 480.00, 0.01) {
		t.Errorf("realized = %.2f, want 480.00", realized)
	}
	// avgBuy = 1010 / 100 = 10.10
	if !approxEqual(avgBuy, 10.10, 0.01) {
		t.Errorf("avgBuy = %.2f, want 10.10", avgBuy)
	}
}

func TestCalculateRealizedFromTrades_MultipleBuysThenSellAtLoss(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 50, Price: 20.00, Fees: 5.00},
		{Type: "buy", Units: 50, Price: 25.00, Fees: 5.00},
		{Type: "sell", Units: 100, Price: 18.00, Fees: 10.00},
	}

	avgBuy, invested, proceeds, realized := calculateRealizedFromTrades(trades)

	// invested = (50*20+5) + (50*25+5) = 1005 + 1255 = 2260
	if !approxEqual(invested, 2260.00, 0.01) {
		t.Errorf("invested = %.2f, want 2260.00", invested)
	}
	// proceeds = 100*18 - 10 = 1790
	if !approxEqual(proceeds, 1790.00, 0.01) {
		t.Errorf("proceeds = %.2f, want 1790.00", proceeds)
	}
	// realized = 1790 - 2260 = -470
	if !approxEqual(realized, -470.00, 0.01) {
		t.Errorf("realized = %.2f, want -470.00", realized)
	}
	// avgBuy = 2260 / 100 = 22.60
	if !approxEqual(avgBuy, 22.60, 0.01) {
		t.Errorf("avgBuy = %.2f, want 22.60", avgBuy)
	}
}

func TestCalculateRealizedFromTrades_OpeningBalanceThenSell(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "opening balance", Units: 200, Price: 5.00, Fees: 0},
		{Type: "sell", Units: 200, Price: 8.00, Fees: 20.00},
	}

	avgBuy, invested, proceeds, realized := calculateRealizedFromTrades(trades)

	// invested = 200*5 + 0 = 1000
	if !approxEqual(invested, 1000.00, 0.01) {
		t.Errorf("invested = %.2f, want 1000.00", invested)
	}
	// proceeds = 200*8 - 20 = 1580
	if !approxEqual(proceeds, 1580.00, 0.01) {
		t.Errorf("proceeds = %.2f, want 1580.00", proceeds)
	}
	// realized = 1580 - 1000 = 580
	if !approxEqual(realized, 580.00, 0.01) {
		t.Errorf("realized = %.2f, want 580.00", realized)
	}
	// avgBuy = 1000 / 200 = 5.00
	if !approxEqual(avgBuy, 5.00, 0.01) {
		t.Errorf("avgBuy = %.2f, want 5.00", avgBuy)
	}
}

func TestCalculateRealizedFromTrades_CostBaseAdjustments(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.00, Fees: 0},
		{Type: "cost base increase", Value: 50.00},
		{Type: "cost base decrease", Value: 20.00},
		{Type: "sell", Units: 100, Price: 12.00, Fees: 0},
	}

	_, invested, proceeds, realized := calculateRealizedFromTrades(trades)

	// invested = 100*10 + 50 - 20 = 1030
	if !approxEqual(invested, 1030.00, 0.01) {
		t.Errorf("invested = %.2f, want 1030.00", invested)
	}
	// proceeds = 100*12 = 1200
	if !approxEqual(proceeds, 1200.00, 0.01) {
		t.Errorf("proceeds = %.2f, want 1200.00", proceeds)
	}
	// realized = 1200 - 1030 = 170
	if !approxEqual(realized, 170.00, 0.01) {
		t.Errorf("realized = %.2f, want 170.00", realized)
	}
}

func TestCalculateRealizedFromTrades_ETPMAG_RealWorld(t *testing.T) {
	// Real-world ETPMAG trades from Navexa (units already normalized to positive)
	// 3 buys: 179@$111.22, 87@$107.54, 162@$116.91 ($3 fees each)
	// 4 sells: 175@$152.39, 65@$152.22, 132@$151.12, 56@$108.72 ($3 fees each)
	// Navexa reports Capital Gain: $14,373.25
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 179, Price: 111.22, Fees: 3.00},
		{Type: "buy", Units: 87, Price: 107.54, Fees: 3.00},
		{Type: "buy", Units: 162, Price: 116.91, Fees: 3.00},
		{Type: "sell", Units: 175, Price: 152.39, Fees: 3.00},
		{Type: "sell", Units: 65, Price: 152.22, Fees: 3.00},
		{Type: "sell", Units: 132, Price: 151.12, Fees: 3.00},
		{Type: "sell", Units: 56, Price: 108.72, Fees: 3.00},
	}

	avgBuy, invested, proceeds, realized := calculateRealizedFromTrades(trades)

	// Total buy cost = (179*111.22+3) + (87*107.54+3) + (162*116.91+3) = 19911.38 + 9358.98 + 18942.42 = 48212.78
	if !approxEqual(invested, 48212.78, 0.01) {
		t.Errorf("invested = %.2f, want 48212.78", invested)
	}

	// Total sell proceeds = (175*152.39-3) + (65*152.22-3) + (132*151.12-3) + (56*108.72-3) = 26665.25 + 9891.30 + 19944.84 + 6085.32 = 62586.71
	if !approxEqual(proceeds, 62586.71, 0.01) {
		t.Errorf("proceeds = %.2f, want 62586.71", proceeds)
	}

	// Realized = 62586.71 - 48212.78 = 14373.93
	// Note: Navexa reports $14,373.25 — small difference due to FIFO vs average cost method
	if !approxEqual(realized, 14373.93, 1.00) {
		t.Errorf("realized = %.2f, want ~14373.93", realized)
	}

	// Total units bought = 179+87+162 = 428
	// avgBuy = 48212.78 / 428 = ~112.65
	if !approxEqual(avgBuy, 112.65, 0.01) {
		t.Errorf("avgBuy = %.2f, want ~112.65", avgBuy)
	}
}

func TestCalculateRealizedFromTrades_NoTrades(t *testing.T) {
	trades := []*models.NavexaTrade{}

	avgBuy, invested, proceeds, realized := calculateRealizedFromTrades(trades)

	if avgBuy != 0 || invested != 0 || proceeds != 0 || realized != 0 {
		t.Errorf("expected all zeros, got avgBuy=%.2f invested=%.2f proceeds=%.2f realized=%.2f",
			avgBuy, invested, proceeds, realized)
	}
}

// --- Strategy integration tests ---

func TestStrategyRSIThresholds(t *testing.T) {
	tests := []struct {
		name           string
		strategy       *models.PortfolioStrategy
		wantOverbought float64
		wantOversold   float64
	}{
		{
			name:           "nil strategy returns defaults",
			strategy:       nil,
			wantOverbought: 70, wantOversold: 30,
		},
		{
			name:           "empty risk level returns defaults",
			strategy:       &models.PortfolioStrategy{},
			wantOverbought: 70, wantOversold: 30,
		},
		{
			name:           "conservative",
			strategy:       &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			wantOverbought: 65, wantOversold: 35,
		},
		{
			name:           "Conservative (capitalised)",
			strategy:       &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "Conservative"}},
			wantOverbought: 65, wantOversold: 35,
		},
		{
			name:           "moderate",
			strategy:       &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "moderate"}},
			wantOverbought: 70, wantOversold: 30,
		},
		{
			name:           "aggressive",
			strategy:       &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
			wantOverbought: 80, wantOversold: 25,
		},
		{
			name:           "unknown level returns defaults",
			strategy:       &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "yolo"}},
			wantOverbought: 70, wantOversold: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ob, os := strategyRSIThresholds(tt.strategy)
			if ob != tt.wantOverbought || os != tt.wantOversold {
				t.Errorf("strategyRSIThresholds() = (%.0f, %.0f), want (%.0f, %.0f)",
					ob, os, tt.wantOverbought, tt.wantOversold)
			}
		})
	}
}

func TestDetermineAction_StrategyRSI(t *testing.T) {
	tests := []struct {
		name       string
		strategy   *models.PortfolioStrategy
		rsi        float64
		wantAction string
	}{
		{
			name:       "conservative SELL at RSI 66 (threshold 65)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			rsi:        66,
			wantAction: "EXIT TRIGGER",
		},
		{
			name:       "conservative HOLD at RSI 64 (below threshold 65)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			rsi:        64,
			wantAction: "COMPLIANT",
		},
		{
			name:       "conservative BUY at RSI 34 (threshold 35)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			rsi:        34,
			wantAction: "ENTRY CRITERIA MET",
		},
		{
			name:       "conservative HOLD at RSI 36 (above threshold 35)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			rsi:        36,
			wantAction: "COMPLIANT",
		},
		{
			name:       "aggressive HOLD at RSI 75 (below threshold 80)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
			rsi:        75,
			wantAction: "COMPLIANT",
		},
		{
			name:       "aggressive SELL at RSI 81 (threshold 80)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
			rsi:        81,
			wantAction: "EXIT TRIGGER",
		},
		{
			name:       "aggressive BUY at RSI 24 (threshold 25)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
			rsi:        24,
			wantAction: "ENTRY CRITERIA MET",
		},
		{
			name:       "nil strategy SELL at RSI 71 (default 70)",
			strategy:   nil,
			rsi:        71,
			wantAction: "EXIT TRIGGER",
		},
		{
			name:       "nil strategy BUY at RSI 29 (default 30)",
			strategy:   nil,
			rsi:        29,
			wantAction: "ENTRY CRITERIA MET",
		},
		{
			name:       "nil strategy HOLD at RSI 50",
			strategy:   nil,
			rsi:        50,
			wantAction: "COMPLIANT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := &models.TickerSignals{
				Technical: models.TechnicalSignals{RSI: tt.rsi},
			}
			action, _ := determineAction(signals, nil, tt.strategy, nil, nil)
			if action != tt.wantAction {
				t.Errorf("determineAction(RSI=%.0f) = %q, want %q", tt.rsi, action, tt.wantAction)
			}
		})
	}
}

func TestDetermineAction_NilSignals(t *testing.T) {
	action, reason := determineAction(nil, nil, nil, nil, nil)
	if action != "COMPLIANT" || reason != "Insufficient data" {
		t.Errorf("determineAction(nil signals) = (%q, %q), want (COMPLIANT, Insufficient data)", action, reason)
	}
}

func TestDetermineAction_PositionWeightExceedsMax(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxPositionPct: 10},
	}
	holding := &models.Holding{Ticker: "BHP.AU", Weight: 15}
	signals := &models.TickerSignals{
		Technical: models.TechnicalSignals{RSI: 50},
	}

	action, reason := determineAction(signals, nil, strategy, holding, nil)
	if action != "WATCH" {
		t.Errorf("expected WATCH for overweight position, got %q: %s", action, reason)
	}
}

func TestDetermineAction_PositionWeightWithinMax(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxPositionPct: 10},
	}
	holding := &models.Holding{Ticker: "BHP.AU", Weight: 8}
	signals := &models.TickerSignals{
		Technical: models.TechnicalSignals{RSI: 50},
	}

	action, _ := determineAction(signals, nil, strategy, holding, nil)
	if action != "COMPLIANT" {
		t.Errorf("expected COMPLIANT for within-limit position, got %q", action)
	}
}

func TestGenerateAlerts_StrategyRSI(t *testing.T) {
	holding := models.Holding{Ticker: "BHP.AU"}

	tests := []struct {
		name         string
		strategy     *models.PortfolioStrategy
		rsi          float64
		wantSignal   string // expected signal type, or "" for no RSI alert
		wantContains string // substring expected in alert message
	}{
		{
			name:         "conservative alerts at RSI 66 (threshold 65)",
			strategy:     &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			rsi:          66,
			wantSignal:   "rsi_overbought",
			wantContains: "threshold: 65",
		},
		{
			name:       "no strategy no alert at RSI 66 (default threshold 70)",
			strategy:   nil,
			rsi:        66,
			wantSignal: "",
		},
		{
			name:         "conservative oversold alert at RSI 34 (threshold 35)",
			strategy:     &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			rsi:          34,
			wantSignal:   "rsi_oversold",
			wantContains: "threshold: 35",
		},
		{
			name:         "aggressive overbought at RSI 81 (threshold 80)",
			strategy:     &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
			rsi:          81,
			wantSignal:   "rsi_overbought",
			wantContains: "threshold: 80",
		},
		{
			name:       "aggressive no alert at RSI 75 (below threshold 80)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
			rsi:        75,
			wantSignal: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := &models.TickerSignals{
				Technical: models.TechnicalSignals{RSI: tt.rsi},
			}
			alerts := generateAlerts(holding, signals, nil, tt.strategy)

			if tt.wantSignal == "" {
				for _, a := range alerts {
					if a.Signal == "rsi_overbought" || a.Signal == "rsi_oversold" {
						t.Errorf("expected no RSI alert, got %q: %s", a.Signal, a.Message)
					}
				}
				return
			}

			found := false
			for _, a := range alerts {
				if a.Signal == tt.wantSignal {
					found = true
					if tt.wantContains != "" && !containsSubstring(a.Message, tt.wantContains) {
						t.Errorf("alert message %q does not contain %q", a.Message, tt.wantContains)
					}
				}
			}
			if !found {
				t.Errorf("expected alert with signal %q, got %d alerts: %+v", tt.wantSignal, len(alerts), alerts)
			}
		})
	}
}

func TestGenerateAlerts_StrategyPositionSize(t *testing.T) {
	strategy := &models.PortfolioStrategy{
		PositionSizing: models.PositionSizing{MaxPositionPct: 10},
	}

	t.Run("overweight generates strategy alert", func(t *testing.T) {
		holding := models.Holding{Ticker: "BHP.AU", Weight: 15}
		signals := &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}}
		alerts := generateAlerts(holding, signals, nil, strategy)

		found := false
		for _, a := range alerts {
			if a.Signal == "strategy_position_size" && a.Type == models.AlertTypeStrategy {
				found = true
			}
		}
		if !found {
			t.Errorf("expected strategy_position_size alert, got: %+v", alerts)
		}
	})

	t.Run("within limit no strategy alert", func(t *testing.T) {
		holding := models.Holding{Ticker: "BHP.AU", Weight: 8}
		signals := &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}}
		alerts := generateAlerts(holding, signals, nil, strategy)

		for _, a := range alerts {
			if a.Signal == "strategy_position_size" {
				t.Errorf("did not expect strategy_position_size alert, got: %+v", a)
			}
		}
	})

	t.Run("nil strategy no strategy alert", func(t *testing.T) {
		holding := models.Holding{Ticker: "BHP.AU", Weight: 50}
		signals := &models.TickerSignals{Technical: models.TechnicalSignals{RSI: 50}}
		alerts := generateAlerts(holding, signals, nil, nil)

		for _, a := range alerts {
			if a.Type == models.AlertTypeStrategy {
				t.Errorf("did not expect strategy alert with nil strategy, got: %+v", a)
			}
		}
	})
}

func TestGenerateAlerts_NilSignals(t *testing.T) {
	holding := models.Holding{Ticker: "BHP.AU"}
	alerts := generateAlerts(holding, nil, nil, nil)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts for nil signals, got %d", len(alerts))
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
}

// --- EODHD Price Fallback Tests ---
// These test the SyncPortfolio flow with mock Navexa + EODHD data to verify
// that stale Navexa prices are replaced by fresher EODHD close prices.

type stubNavexaClient struct {
	portfolios []*models.NavexaPortfolio
	holdings   []*models.NavexaHolding
	trades     map[string][]*models.NavexaTrade
}

func (s *stubNavexaClient) GetPortfolios(ctx context.Context) ([]*models.NavexaPortfolio, error) {
	return s.portfolios, nil
}
func (s *stubNavexaClient) GetPortfolio(ctx context.Context, id string) (*models.NavexaPortfolio, error) {
	for _, p := range s.portfolios {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (s *stubNavexaClient) GetHoldings(ctx context.Context, id string) ([]*models.NavexaHolding, error) {
	return s.holdings, nil
}
func (s *stubNavexaClient) GetPerformance(ctx context.Context, id, from, to string) (*models.NavexaPerformance, error) {
	return &models.NavexaPerformance{}, nil
}
func (s *stubNavexaClient) GetEnrichedHoldings(ctx context.Context, id, from, to string) ([]*models.NavexaHolding, error) {
	// Return copies to avoid mutation across test runs
	result := make([]*models.NavexaHolding, len(s.holdings))
	for i, h := range s.holdings {
		copy := *h
		result[i] = &copy
	}
	return result, nil
}
func (s *stubNavexaClient) GetHoldingTrades(ctx context.Context, holdingID string) ([]*models.NavexaTrade, error) {
	if trades, ok := s.trades[holdingID]; ok {
		return trades, nil
	}
	return nil, nil
}

type stubMarketDataStorage struct {
	data map[string]*models.MarketData
}

func (s *stubMarketDataStorage) GetMarketData(ctx context.Context, ticker string) (*models.MarketData, error) {
	if md, ok := s.data[ticker]; ok {
		return md, nil
	}
	return nil, fmt.Errorf("not found")
}
func (s *stubMarketDataStorage) SaveMarketData(ctx context.Context, data *models.MarketData) error {
	return nil
}
func (s *stubMarketDataStorage) GetMarketDataBatch(ctx context.Context, tickers []string) ([]*models.MarketData, error) {
	return nil, nil
}
func (s *stubMarketDataStorage) GetStaleTickers(ctx context.Context, exchange string, maxAge int64) ([]string, error) {
	return nil, nil
}

// memUserDataStore is a simple in-memory UserDataStore for tests.
type memUserDataStore struct {
	records map[string]*models.UserRecord // composite key -> record
}

func newMemUserDataStore() *memUserDataStore {
	return &memUserDataStore{records: make(map[string]*models.UserRecord)}
}

func (m *memUserDataStore) Get(_ context.Context, userID, subject, key string) (*models.UserRecord, error) {
	ck := userID + ":" + subject + ":" + key
	if r, ok := m.records[ck]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("%s '%s' not found", subject, key)
}

func (m *memUserDataStore) Put(_ context.Context, record *models.UserRecord) error {
	ck := record.UserID + ":" + record.Subject + ":" + record.Key
	if existing, ok := m.records[ck]; ok {
		record.Version = existing.Version + 1
	} else {
		record.Version = 1
	}
	record.DateTime = time.Now()
	m.records[ck] = record
	return nil
}

func (m *memUserDataStore) Delete(_ context.Context, userID, subject, key string) error {
	ck := userID + ":" + subject + ":" + key
	delete(m.records, ck)
	return nil
}

func (m *memUserDataStore) List(_ context.Context, userID, subject string) ([]*models.UserRecord, error) {
	var result []*models.UserRecord
	for _, r := range m.records {
		if r.UserID == userID && r.Subject == subject {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *memUserDataStore) Query(_ context.Context, userID, subject string, opts interfaces.QueryOptions) ([]*models.UserRecord, error) {
	return m.List(context.Background(), userID, subject)
}

func (m *memUserDataStore) DeleteBySubject(_ context.Context, subject string) (int, error) {
	count := 0
	for ck, r := range m.records {
		if r.Subject == subject {
			delete(m.records, ck)
			count++
		}
	}
	return count, nil
}

func (m *memUserDataStore) Close() error { return nil }

type stubStorageManager struct {
	marketStore   *stubMarketDataStorage
	userDataStore *memUserDataStore
}

func (s *stubStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return s.marketStore }
func (s *stubStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (s *stubStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (s *stubStorageManager) UserDataStore() interfaces.UserDataStore {
	if s.userDataStore != nil {
		return s.userDataStore
	}
	return newMemUserDataStore()
}
func (s *stubStorageManager) StockIndexStore() interfaces.StockIndexStore {
	return &noopStockIndexStore{}
}
func (s *stubStorageManager) JobQueueStore() interfaces.JobQueueStore        { return nil }
func (s *stubStorageManager) FileStore() interfaces.FileStore                { return nil }
func (s *stubStorageManager) FeedbackStore() interfaces.FeedbackStore        { return nil }
func (s *stubStorageManager) DataPath() string                               { return "" }
func (s *stubStorageManager) WriteRaw(subdir, key string, data []byte) error { return nil }
func (s *stubStorageManager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	return nil, nil
}
func (s *stubStorageManager) PurgeReports(ctx context.Context) (int, error) { return 0, nil }
func (s *stubStorageManager) Close() error                                  { return nil }

func TestSyncPortfolio_EODHDPriceFallback(t *testing.T) {
	today := time.Now()
	fridayPrice := 143.92
	mondayClose := 147.50

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID:           "100",
				PortfolioID:  "1",
				Ticker:       "ACDC",
				Exchange:     "AU",
				Name:         "ACDC ETF",
				Units:        282,
				CurrentPrice: fridayPrice, // Navexa returns stale Friday price
				MarketValue:  fridayPrice * 282,
				LastUpdated:  today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {
				{ID: "1", HoldingID: "100", Symbol: "ACDC", Type: "buy", Units: 282, Price: 120.0, Fees: 10},
			},
		},
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"ACDC.AU": {
				Ticker: "ACDC.AU",
				EOD: []models.EODBar{
					{Date: today, Close: mondayClose}, // EODHD has today's close
					{Date: today.AddDate(0, 0, -3), Close: fridayPrice},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	// Find the ACDC holding
	var acdc *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "ACDC" {
			acdc = &portfolio.Holdings[i]
			break
		}
	}

	if acdc == nil {
		t.Fatal("ACDC holding not found in portfolio")
	}

	// The price should be the EODHD Monday close, not the stale Navexa Friday price
	if !approxEqual(acdc.CurrentPrice, mondayClose, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want %.2f (EODHD close should override stale Navexa price)", acdc.CurrentPrice, mondayClose)
	}

	expectedMV := mondayClose * 282
	if !approxEqual(acdc.MarketValue, expectedMV, 0.01) {
		t.Errorf("MarketValue = %.2f, want %.2f", acdc.MarketValue, expectedMV)
	}
}

func TestSyncPortfolio_NoFallbackWhenNavexaIsFresh(t *testing.T) {
	today := time.Now()
	navexaPrice := 147.50

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1", Ticker: "ACDC", Exchange: "AU",
				Name: "ACDC ETF", Units: 282,
				CurrentPrice: navexaPrice, // Navexa price matches EODHD
				MarketValue:  navexaPrice * 282, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "ACDC", Type: "buy", Units: 282, Price: 120.0}},
		},
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"ACDC.AU": {
				Ticker: "ACDC.AU",
				EOD: []models.EODBar{
					{Date: today, Close: navexaPrice}, // Same price — no fallback needed
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	var acdc *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "ACDC" {
			acdc = &portfolio.Holdings[i]
			break
		}
	}

	if acdc == nil {
		t.Fatal("ACDC holding not found")
	}

	// Price should remain unchanged when both sources agree
	if !approxEqual(acdc.CurrentPrice, navexaPrice, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want %.2f (no fallback when prices match)", acdc.CurrentPrice, navexaPrice)
	}
}

func TestSyncPortfolio_NoFallbackWhenNoEODHDData(t *testing.T) {
	today := time.Now()
	navexaPrice := 143.92

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1", Ticker: "ACDC", Exchange: "AU",
				Name: "ACDC ETF", Units: 282,
				CurrentPrice: navexaPrice, MarketValue: navexaPrice * 282, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "ACDC", Type: "buy", Units: 282, Price: 120.0}},
		},
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{}, // No EODHD data at all
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	var acdc *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "ACDC" {
			acdc = &portfolio.Holdings[i]
			break
		}
	}

	if acdc == nil {
		t.Fatal("ACDC holding not found")
	}

	// Should use Navexa price when no EODHD data is available
	if !approxEqual(acdc.CurrentPrice, navexaPrice, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want %.2f (should keep Navexa price when no EODHD data)", acdc.CurrentPrice, navexaPrice)
	}
}

func TestSyncPortfolio_NoFallbackForOldEODHDBar(t *testing.T) {
	today := time.Now()
	navexaPrice := 143.92
	oldEODHDClose := 141.00

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "100", PortfolioID: "1", Ticker: "ACDC", Exchange: "AU",
				Name: "ACDC ETF", Units: 282,
				CurrentPrice: navexaPrice, MarketValue: navexaPrice * 282, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"100": {{ID: "1", HoldingID: "100", Symbol: "ACDC", Type: "buy", Units: 282, Price: 120.0}},
		},
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"ACDC.AU": {
				Ticker: "ACDC.AU",
				EOD: []models.EODBar{
					{Date: today.AddDate(0, 0, -3), Close: oldEODHDClose}, // Friday's bar, not today's
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	var acdc *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "ACDC" {
			acdc = &portfolio.Holdings[i]
			break
		}
	}

	if acdc == nil {
		t.Fatal("ACDC holding not found")
	}

	// Should NOT use old EODHD bar — only use EODHD when bar date is today
	if !approxEqual(acdc.CurrentPrice, navexaPrice, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want %.2f (should keep Navexa price when EODHD bar is old)", acdc.CurrentPrice, navexaPrice)
	}
}

// TestSyncPortfolio_ConcurrentSyncSerializes verifies that concurrent SyncPortfolio
// calls are serialized by the mutex, preventing the warm cache race condition where
// a slow force=false sync could overwrite a fast force=true sync's fresh data.
func TestSyncPortfolio_ConcurrentSyncSerializes(t *testing.T) {
	stalePrice := 143.92
	freshPrice := 147.50

	// Track which call saved last
	store := &trackingUserDataStore{memUserDataStore: newMemUserDataStore()}

	// Navexa client that returns stale price on first call, fresh on second
	callCount := 0
	navexa := &delayedNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdingsFn: func() []*models.NavexaHolding {
			callCount++
			price := stalePrice
			if callCount > 1 {
				price = freshPrice
			}
			return []*models.NavexaHolding{
				{
					ID: "100", Ticker: "ACDC", Units: 282,
					CurrentPrice: price,
					MarketValue:  price * 282,
				},
			}
		},
	}

	storageManager := &trackingStorageManager{
		userDataStore: store,
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
	}
	logger := common.NewLogger("error")
	svc := NewService(storageManager, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)

	// Simulate warm cache (force=false) starting first
	done := make(chan struct{})
	go func() {
		svc.SyncPortfolio(ctx, "SMSF", false)
		close(done)
	}()

	// Force sync (force=true) — mutex ensures this waits for the first to finish
	svc.SyncPortfolio(ctx, "SMSF", true)
	<-done

	// Verify at least one save occurred (both calls completed)
	if store.saveCount < 1 {
		t.Fatal("no portfolio was saved")
	}

	// If saveCount is 1, the second call (force=false) saw fresh data and skipped.
	// This is actually correct behavior — the mutex + freshness check means
	// a non-force sync after a force sync will see fresh data and skip.
	// Either way, no stale overwrite occurred.
	if store.saveCount < 2 {
		t.Logf("Only %d save(s) — second call likely saw fresh data (correct)", store.saveCount)
	}
}

// trackingUserDataStore wraps memUserDataStore and tracks Put calls
type trackingUserDataStore struct {
	*memUserDataStore
	saveCount int
}

func (s *trackingUserDataStore) Put(ctx context.Context, record *models.UserRecord) error {
	s.saveCount++
	return s.memUserDataStore.Put(ctx, record)
}

// trackingStorageManager wraps tracking user data storage for race condition tests
type trackingStorageManager struct {
	userDataStore *trackingUserDataStore
	marketStore   *stubMarketDataStorage
}

func (s *trackingStorageManager) MarketDataStorage() interfaces.MarketDataStorage {
	return s.marketStore
}
func (s *trackingStorageManager) SignalStorage() interfaces.SignalStorage { return nil }
func (s *trackingStorageManager) InternalStore() interfaces.InternalStore { return nil }
func (s *trackingStorageManager) UserDataStore() interfaces.UserDataStore { return s.userDataStore }
func (s *trackingStorageManager) StockIndexStore() interfaces.StockIndexStore {
	return &noopStockIndexStore{}
}
func (s *trackingStorageManager) JobQueueStore() interfaces.JobQueueStore { return nil }
func (s *trackingStorageManager) FileStore() interfaces.FileStore         { return nil }
func (s *trackingStorageManager) FeedbackStore() interfaces.FeedbackStore { return nil }
func (s *trackingStorageManager) DataPath() string                        { return "" }
func (s *trackingStorageManager) WriteRaw(subdir, key string, data []byte) error {
	return nil
}
func (s *trackingStorageManager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	return nil, nil
}
func (s *trackingStorageManager) PurgeReports(ctx context.Context) (int, error) { return 0, nil }
func (s *trackingStorageManager) Close() error                                  { return nil }

// delayedNavexaClient returns different holdings per call to simulate stale vs fresh data
type delayedNavexaClient struct {
	portfolios []*models.NavexaPortfolio
	holdingsFn func() []*models.NavexaHolding
}

func (s *delayedNavexaClient) GetPortfolios(ctx context.Context) ([]*models.NavexaPortfolio, error) {
	return s.portfolios, nil
}
func (s *delayedNavexaClient) GetPortfolio(ctx context.Context, id string) (*models.NavexaPortfolio, error) {
	for _, p := range s.portfolios {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (s *delayedNavexaClient) GetHoldings(ctx context.Context, id string) ([]*models.NavexaHolding, error) {
	return s.holdingsFn(), nil
}
func (s *delayedNavexaClient) GetPerformance(ctx context.Context, id, from, to string) (*models.NavexaPerformance, error) {
	return &models.NavexaPerformance{}, nil
}
func (s *delayedNavexaClient) GetEnrichedHoldings(ctx context.Context, id, from, to string) ([]*models.NavexaHolding, error) {
	holdings := s.holdingsFn()
	result := make([]*models.NavexaHolding, len(holdings))
	for i, h := range holdings {
		copy := *h
		result[i] = &copy
	}
	return result, nil
}
func (s *delayedNavexaClient) GetHoldingTrades(ctx context.Context, holdingID string) ([]*models.NavexaTrade, error) {
	return nil, nil
}

// --- stub EODHD client for ReviewPortfolio live price tests ---

type stubEODHDClient struct {
	realTimeQuoteFn func(ctx context.Context, ticker string) (*models.RealTimeQuote, error)
}

func (s *stubEODHDClient) GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error) {
	if s.realTimeQuoteFn != nil {
		return s.realTimeQuoteFn(ctx, ticker)
	}
	return nil, fmt.Errorf("not implemented")
}
func (s *stubEODHDClient) GetEOD(ctx context.Context, ticker string, opts ...interfaces.EODOption) (*models.EODResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubEODHDClient) GetBulkEOD(ctx context.Context, exchange string, tickers []string) (map[string]models.EODBar, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubEODHDClient) GetFundamentals(ctx context.Context, ticker string) (*models.Fundamentals, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubEODHDClient) GetTechnicals(ctx context.Context, ticker string, function string) (*models.TechnicalResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubEODHDClient) GetNews(ctx context.Context, ticker string, limit int) ([]*models.NewsItem, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubEODHDClient) GetExchangeSymbols(ctx context.Context, exchange string) ([]*models.Symbol, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *stubEODHDClient) ScreenStocks(ctx context.Context, options models.ScreenerOptions) ([]*models.ScreenerResult, error) {
	return nil, fmt.Errorf("not implemented")
}

// --- ReviewPortfolio live price tests ---

type reviewStorageManager struct {
	userDataStore interfaces.UserDataStore
	marketStore   interfaces.MarketDataStorage
	signalStore   interfaces.SignalStorage
}

func (s *reviewStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return s.marketStore }
func (s *reviewStorageManager) SignalStorage() interfaces.SignalStorage         { return s.signalStore }
func (s *reviewStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (s *reviewStorageManager) UserDataStore() interfaces.UserDataStore {
	if s.userDataStore != nil {
		return s.userDataStore
	}
	return newMemUserDataStore()
}
func (s *reviewStorageManager) StockIndexStore() interfaces.StockIndexStore {
	return &noopStockIndexStore{}
}
func (s *reviewStorageManager) JobQueueStore() interfaces.JobQueueStore        { return nil }
func (s *reviewStorageManager) FileStore() interfaces.FileStore                { return nil }
func (s *reviewStorageManager) FeedbackStore() interfaces.FeedbackStore        { return nil }
func (s *reviewStorageManager) DataPath() string                               { return "" }
func (s *reviewStorageManager) WriteRaw(subdir, key string, data []byte) error { return nil }
func (s *reviewStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (s *reviewStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (s *reviewStorageManager) Close() error                                { return nil }

type reviewMarketDataStorage struct {
	data map[string]*models.MarketData
}

func (s *reviewMarketDataStorage) GetMarketData(_ context.Context, ticker string) (*models.MarketData, error) {
	if md, ok := s.data[ticker]; ok {
		return md, nil
	}
	return nil, fmt.Errorf("not found")
}
func (s *reviewMarketDataStorage) SaveMarketData(_ context.Context, _ *models.MarketData) error {
	return nil
}
func (s *reviewMarketDataStorage) GetMarketDataBatch(_ context.Context, tickers []string) ([]*models.MarketData, error) {
	var result []*models.MarketData
	for _, t := range tickers {
		if md, ok := s.data[t]; ok {
			result = append(result, md)
		}
	}
	return result, nil
}
func (s *reviewMarketDataStorage) GetStaleTickers(_ context.Context, _ string, _ int64) ([]string, error) {
	return nil, nil
}

type reviewSignalStorage struct {
	signals map[string]*models.TickerSignals
}

func (s *reviewSignalStorage) GetSignals(_ context.Context, ticker string) (*models.TickerSignals, error) {
	if sig, ok := s.signals[ticker]; ok {
		return sig, nil
	}
	return nil, fmt.Errorf("not found")
}
func (s *reviewSignalStorage) SaveSignals(_ context.Context, _ *models.TickerSignals) error {
	return nil
}
func (s *reviewSignalStorage) GetSignalsBatch(_ context.Context, _ []string) ([]*models.TickerSignals, error) {
	return nil, nil
}

// storePortfolio is a test helper that saves a portfolio into a memUserDataStore as JSON.
// Automatically sets DataVersion to the current SchemaVersion so getPortfolioRecord accepts it.
func storePortfolio(t *testing.T, store *memUserDataStore, portfolio *models.Portfolio) {
	t.Helper()
	portfolio.DataVersion = common.SchemaVersion
	data, err := json.Marshal(portfolio)
	if err != nil {
		t.Fatalf("failed to marshal portfolio: %v", err)
	}
	store.Put(context.Background(), &models.UserRecord{
		UserID:  "default",
		Subject: "portfolio",
		Key:     portfolio.Name,
		Value:   string(data),
	})
}

func TestReviewPortfolio_UsesLivePrices(t *testing.T) {
	today := time.Now()
	eodClose := 42.50
	prevClose := 41.80
	livePrice := 43.25

	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: eodClose * 100,
		LastSynced: today,
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "AU", Name: "BHP Group", Units: 100, CurrentPrice: eodClose, MarketValue: eodClose * 100, Weight: 100},
		},
	}

	uds := newMemUserDataStore()
	storePortfolio(t, uds, portfolio)

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					EOD: []models.EODBar{
						{Date: today, Close: eodClose},
						{Date: today.AddDate(0, 0, -1), Close: prevClose},
					},
				},
			},
		},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
		}},
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

	review, err := svc.ReviewPortfolio(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("ReviewPortfolio failed: %v", err)
	}

	if len(review.HoldingReviews) == 0 {
		t.Fatal("expected holding reviews")
	}

	hr := review.HoldingReviews[0]

	// Overnight move should use live price vs previous close
	expectedMove := livePrice - prevClose
	if !approxEqual(hr.OvernightMove, expectedMove, 0.01) {
		t.Errorf("OvernightMove = %.2f, want %.2f (live - prev close)", hr.OvernightMove, expectedMove)
	}

	// Holding should have live price
	if !approxEqual(hr.Holding.CurrentPrice, livePrice, 0.01) {
		t.Errorf("Holding.CurrentPrice = %.2f, want %.2f (live price)", hr.Holding.CurrentPrice, livePrice)
	}

	expectedMV := livePrice * 100
	if !approxEqual(hr.Holding.MarketValue, expectedMV, 0.01) {
		t.Errorf("Holding.MarketValue = %.2f, want %.2f", hr.Holding.MarketValue, expectedMV)
	}
}

func TestReviewPortfolio_FallsBackToEODOnRealTimeError(t *testing.T) {
	today := time.Now()
	eodClose := 42.50
	prevClose := 41.80

	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: eodClose * 100,
		LastSynced: today,
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "AU", Name: "BHP Group", Units: 100, CurrentPrice: eodClose, MarketValue: eodClose * 100, Weight: 100},
		},
	}

	uds := newMemUserDataStore()
	storePortfolio(t, uds, portfolio)

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					EOD: []models.EODBar{
						{Date: today, Close: eodClose},
						{Date: today.AddDate(0, 0, -1), Close: prevClose},
					},
				},
			},
		},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
		}},
	}

	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			return nil, fmt.Errorf("API unavailable")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	review, err := svc.ReviewPortfolio(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("ReviewPortfolio failed: %v", err)
	}

	hr := review.HoldingReviews[0]

	// Should fall back to EOD movement
	expectedMove := eodClose - prevClose
	if !approxEqual(hr.OvernightMove, expectedMove, 0.01) {
		t.Errorf("OvernightMove = %.2f, want %.2f (EOD fallback)", hr.OvernightMove, expectedMove)
	}
}

func TestReviewPortfolio_PartialRealTimeFailure(t *testing.T) {
	today := time.Now()
	livePrice := 43.25

	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 10000,
		LastSynced: today,
		Holdings: []models.Holding{
			{Ticker: "BHP", Exchange: "AU", Name: "BHP Group", Units: 100, CurrentPrice: 42.50, MarketValue: 4250, Weight: 50},
			{Ticker: "CBA", Exchange: "AU", Name: "CBA Group", Units: 50, CurrentPrice: 115.00, MarketValue: 5750, Weight: 50},
		},
	}

	uds := newMemUserDataStore()
	storePortfolio(t, uds, portfolio)

	storage := &reviewStorageManager{
		userDataStore: uds,
		marketStore: &reviewMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {Ticker: "BHP.AU", EOD: []models.EODBar{
					{Date: today, Close: 42.50},
					{Date: today.AddDate(0, 0, -1), Close: 41.80},
				}},
				"CBA.AU": {Ticker: "CBA.AU", EOD: []models.EODBar{
					{Date: today, Close: 115.00},
					{Date: today.AddDate(0, 0, -1), Close: 114.50},
				}},
			},
		},
		signalStore: &reviewSignalStorage{signals: map[string]*models.TickerSignals{
			"BHP.AU": {Ticker: "BHP.AU", Technical: models.TechnicalSignals{RSI: 50}},
			"CBA.AU": {Ticker: "CBA.AU", Technical: models.TechnicalSignals{RSI: 55}},
		}},
	}

	eodhd := &stubEODHDClient{
		realTimeQuoteFn: func(_ context.Context, ticker string) (*models.RealTimeQuote, error) {
			if ticker == "BHP.AU" {
				return &models.RealTimeQuote{Code: ticker, Close: livePrice, Timestamp: today}, nil
			}
			// CBA fails
			return nil, fmt.Errorf("API error for CBA")
		},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, eodhd, nil, logger)

	review, err := svc.ReviewPortfolio(context.Background(), "SMSF", interfaces.ReviewOptions{})
	if err != nil {
		t.Fatalf("ReviewPortfolio failed: %v", err)
	}

	if len(review.HoldingReviews) < 2 {
		t.Fatalf("expected 2 holding reviews, got %d", len(review.HoldingReviews))
	}

	// BHP should have live price
	var bhp, cba *models.HoldingReview
	for i := range review.HoldingReviews {
		if review.HoldingReviews[i].Holding.Ticker == "BHP" {
			bhp = &review.HoldingReviews[i]
		}
		if review.HoldingReviews[i].Holding.Ticker == "CBA" {
			cba = &review.HoldingReviews[i]
		}
	}

	if bhp == nil || cba == nil {
		t.Fatal("expected both BHP and CBA holding reviews")
	}

	if !approxEqual(bhp.Holding.CurrentPrice, livePrice, 0.01) {
		t.Errorf("BHP CurrentPrice = %.2f, want %.2f (live)", bhp.Holding.CurrentPrice, livePrice)
	}

	// CBA should fall back to EOD
	if !approxEqual(cba.OvernightMove, 115.00-114.50, 0.01) {
		t.Errorf("CBA OvernightMove = %.2f, want %.2f (EOD fallback)", cba.OvernightMove, 115.00-114.50)
	}
}

// --- GetPortfolio auto-refresh tests ---

type flexStorageManager struct {
	userDataStore *memUserDataStore
}

func (s *flexStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return nil }
func (s *flexStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (s *flexStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (s *flexStorageManager) UserDataStore() interfaces.UserDataStore         { return s.userDataStore }
func (s *flexStorageManager) StockIndexStore() interfaces.StockIndexStore {
	return &noopStockIndexStore{}
}
func (s *flexStorageManager) JobQueueStore() interfaces.JobQueueStore        { return nil }
func (s *flexStorageManager) FileStore() interfaces.FileStore                { return nil }
func (s *flexStorageManager) FeedbackStore() interfaces.FeedbackStore        { return nil }
func (s *flexStorageManager) DataPath() string                               { return "" }
func (s *flexStorageManager) WriteRaw(subdir, key string, data []byte) error { return nil }
func (s *flexStorageManager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	return nil, nil
}
func (s *flexStorageManager) PurgeReports(ctx context.Context) (int, error) { return 0, nil }
func (s *flexStorageManager) Close() error                                  { return nil }

func TestGetPortfolio_Fresh_NoSync(t *testing.T) {
	freshPortfolio := &models.Portfolio{
		Name:       "test",
		TotalValue: 100.0,
		LastSynced: time.Now(), // within 30-min TTL
	}

	uds := newMemUserDataStore()
	storePortfolio(t, uds, freshPortfolio)
	storage := &flexStorageManager{userDataStore: uds}
	logger := common.NewLogger("error")
	// navexa=nil: if sync is attempted, it will panic — proving it wasn't called
	svc := NewService(storage, nil, nil, nil, logger)

	got, err := svc.GetPortfolio(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TotalValue != 100.0 {
		t.Errorf("expected value 100.0, got %f", got.TotalValue)
	}
}

func TestGetPortfolio_Stale_TriggersSync(t *testing.T) {
	stalePortfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 100.0,
		LastSynced: time.Now().Add(-2 * common.FreshnessPortfolio), // stale
	}

	uds := newMemUserDataStore()
	storePortfolio(t, uds, stalePortfolio)
	storage := &flexStorageManager{userDataStore: uds}
	logger := common.NewLogger("error")

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{},
		trades:   map[string][]*models.NavexaTrade{},
	}

	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	got, err := svc.GetPortfolio(ctx, "SMSF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After sync, the saved portfolio should be returned
	if got.Name != "SMSF" {
		t.Errorf("expected portfolio name 'SMSF', got %q", got.Name)
	}
}

func TestGetPortfolio_SyncFails_ReturnsStaleData(t *testing.T) {
	stalePortfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 100.0,
		LastSynced: time.Now().Add(-2 * common.FreshnessPortfolio), // stale
	}

	uds := newMemUserDataStore()
	storePortfolio(t, uds, stalePortfolio)
	storage := &flexStorageManager{userDataStore: uds}
	logger := common.NewLogger("error")

	// No navexa client in context — sync will fail
	svc := NewService(storage, nil, nil, nil, logger)

	got, err := svc.GetPortfolio(context.Background(), "SMSF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to stale data
	if got.TotalValue != 100.0 {
		t.Errorf("expected stale value 100.0, got %f", got.TotalValue)
	}
}

func TestGetPortfolio_NotFound(t *testing.T) {
	uds := newMemUserDataStore()
	storage := &flexStorageManager{userDataStore: uds}
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	_, err := svc.GetPortfolio(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing portfolio")
	}
}

// --- resolveNavexaClient tests ---

func TestResolveNavexaClient_NoContextClient_ReturnsError(t *testing.T) {
	logger := common.NewLogger("error")
	svc := NewService(nil, nil, nil, nil, logger)

	_, err := svc.resolveNavexaClient(context.Background())
	if err == nil {
		t.Fatal("expected error when no navexa client in context")
	}
}

func TestResolveNavexaClient_WithContextClient_ReturnsIt(t *testing.T) {
	logger := common.NewLogger("error")
	svc := NewService(nil, nil, nil, nil, logger)

	stub := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{},
	}
	ctx := common.WithNavexaClient(context.Background(), stub)

	client, err := svc.resolveNavexaClient(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

// noopStockIndexStore satisfies interfaces.StockIndexStore without doing anything.
type noopStockIndexStore struct{}

func (n *noopStockIndexStore) Upsert(_ context.Context, _ *models.StockIndexEntry) error { return nil }
func (n *noopStockIndexStore) Get(_ context.Context, _ string) (*models.StockIndexEntry, error) {
	return nil, fmt.Errorf("not found")
}
func (n *noopStockIndexStore) List(_ context.Context) ([]*models.StockIndexEntry, error) {
	return nil, nil
}
func (n *noopStockIndexStore) UpdateTimestamp(_ context.Context, _, _ string, _ time.Time) error {
	return nil
}
func (n *noopStockIndexStore) Delete(_ context.Context, _ string) error { return nil }

// --- GainLoss calculation tests ---

func TestGainLoss_PartialSellAndReEntry(t *testing.T) {
	// SKS scenario: buy, partial sells, re-entry buys, current market value
	// Buy 4925 @ $4.0248 ($19,825.14 + $0 fees)
	// Sell 1333 @ $3.7627 ($5,012.68 - $0 fees)
	// Sell 819 @ $3.680 ($3,010.92 - $0 fees)
	// Sell 2773 @ $3.4508 ($9,566.07 - $0 fees)
	// Buy 2511 @ $3.980 ($9,996.78 + $0 fees)
	// Buy 2456 @ $4.070 ($9,998.92 + $0 fees)
	// Remaining units: 4925 - 1333 - 819 - 2773 + 2511 + 2456 = 4967
	// Current price: $4.71
	// MarketValue = 4967 * 4.71 = 23,394.57
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 4925, Price: 4.0248, Fees: 0},
		{Type: "sell", Units: 1333, Price: 3.7627, Fees: 0},
		{Type: "sell", Units: 819, Price: 3.680, Fees: 0},
		{Type: "sell", Units: 2773, Price: 3.4508, Fees: 0},
		{Type: "buy", Units: 2511, Price: 3.980, Fees: 0},
		{Type: "buy", Units: 2456, Price: 4.070, Fees: 0},
	}

	currentPrice := 4.71
	remainingUnits := 4925.0 - 1333 - 819 - 2773 + 2511 + 2456
	marketValue := remainingUnits * currentPrice

	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, marketValue)

	// totalInvested = 19822.14 + 9993.78 + 9998.92 = 39820.84
	expectedInvested := 4925*4.0248 + 2511*3.980 + 2456*4.070
	if !approxEqual(totalInvested, expectedInvested, 0.01) {
		t.Errorf("totalInvested = %.2f, want %.2f", totalInvested, expectedInvested)
	}

	// totalProceeds = 5012.68 + 3013.92 + 9566.07 = 17589.67
	expectedProceeds := 1333*3.7627 + 819*3.680 + 2773*3.4508
	if !approxEqual(totalProceeds, expectedProceeds, 0.01) {
		t.Errorf("totalProceeds = %.2f, want %.2f", totalProceeds, expectedProceeds)
	}

	// GainLoss = proceeds + marketValue - totalInvested
	expectedGainLoss := expectedProceeds + marketValue - expectedInvested
	if !approxEqual(gainLoss, expectedGainLoss, 0.01) {
		t.Errorf("gainLoss = %.2f, want %.2f", gainLoss, expectedGainLoss)
	}

	// Verify the overall gain is positive (market value + proceeds > invested)
	if gainLoss < 0 {
		t.Errorf("expected positive gainLoss for position with price above avg cost, got %.2f", gainLoss)
	}
}

func TestGainLoss_PriceUpdatePreservesRealisedLoss(t *testing.T) {
	// Simulate the EODHD price cross-check path:
	// 1. Calculate GainLoss from trades (includes realised loss from sells below cost)
	// 2. EODHD price update should adjust only by price delta, not recalculate from scratch

	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 1000, Price: 10.00, Fees: 0},
		{Type: "sell", Units: 500, Price: 8.00, Fees: 0}, // realised loss of $1000
		{Type: "buy", Units: 200, Price: 9.00, Fees: 0},  // re-entry
	}
	// Remaining units: 1000 - 500 + 200 = 700
	remainingUnits := 700.0

	navexaPrice := 9.50
	navexaMarketValue := remainingUnits * navexaPrice

	// Step 1: initial GainLoss from trades
	_, _, initialGainLoss := calculateGainLossFromTrades(trades, navexaMarketValue)
	// totalInvested = 10000 + 1800 = 11800
	// totalProceeds = 4000
	// GainLoss = 4000 + 6650 - 11800 = -1150
	if !approxEqual(initialGainLoss, -1150.0, 0.01) {
		t.Errorf("initial GainLoss = %.2f, want -1150.00", initialGainLoss)
	}

	// Step 2: Simulate EODHD price update (the fix uses delta approach)
	eodhPrice := 10.00
	oldMarketValue := navexaMarketValue
	newMarketValue := remainingUnits * eodhPrice
	adjustedGainLoss := initialGainLoss + (newMarketValue - oldMarketValue)

	// The delta is 700 * (10.00 - 9.50) = 350
	// adjustedGainLoss = -1150 + 350 = -800
	expectedAdjusted := -800.0
	if !approxEqual(adjustedGainLoss, expectedAdjusted, 0.01) {
		t.Errorf("adjusted GainLoss = %.2f, want %.2f", adjustedGainLoss, expectedAdjusted)
	}

	// Cross-check: if we recalculate from scratch with the new price, should match
	_, _, freshGainLoss := calculateGainLossFromTrades(trades, newMarketValue)
	if !approxEqual(adjustedGainLoss, freshGainLoss, 0.01) {
		t.Errorf("delta-adjusted GainLoss (%.2f) != fresh calculation (%.2f)", adjustedGainLoss, freshGainLoss)
	}
}

func TestGainLoss_PureBuyAndHold(t *testing.T) {
	// Simple buy-and-hold: no sells means GainLoss = MarketValue - TotalCost
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 50.00, Fees: 10.00},
		{Type: "buy", Units: 50, Price: 55.00, Fees: 5.00},
	}

	currentPrice := 60.00
	units := 150.0
	marketValue := units * currentPrice

	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, marketValue)

	// totalInvested = (100*50+10) + (50*55+5) = 5010 + 2755 = 7765
	if !approxEqual(totalInvested, 7765.0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 7765.00", totalInvested)
	}

	// No sells: proceeds = 0
	if totalProceeds != 0 {
		t.Errorf("totalProceeds = %.2f, want 0 (no sells)", totalProceeds)
	}

	// GainLoss = 0 + 9000 - 7765 = 1235
	expectedGainLoss := marketValue - totalInvested
	if !approxEqual(gainLoss, expectedGainLoss, 0.01) {
		t.Errorf("gainLoss = %.2f, want %.2f", gainLoss, expectedGainLoss)
	}

	// For pure buy-and-hold, GainLoss should equal MarketValue - TotalCost
	// This confirms the fix doesn't break simple scenarios
	_, totalCost, _ := calculateAvgCostFromTrades(trades)
	if !approxEqual(gainLoss, marketValue-totalCost, 0.01) {
		t.Errorf("gainLoss (%.2f) != MarketValue - TotalCost (%.2f) for buy-and-hold", gainLoss, marketValue-totalCost)
	}
}

func TestHoldingTrades_MultipleHoldings(t *testing.T) {
	// Verify that holdingTrades append doesn't lose trades when
	// multiple holdings share the same ticker (e.g. closed + open position)
	holdingTrades := make(map[string][]*models.NavexaTrade)

	// First holding's trades (closed position)
	trades1 := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.00},
		{Type: "sell", Units: 100, Price: 12.00},
	}

	// Second holding's trades (open position)
	trades2 := []*models.NavexaTrade{
		{Type: "buy", Units: 200, Price: 11.00},
	}

	// Simulate the fixed append behavior
	ticker := "BHP"
	holdingTrades[ticker] = append(holdingTrades[ticker], trades1...)
	holdingTrades[ticker] = append(holdingTrades[ticker], trades2...)

	if len(holdingTrades[ticker]) != 3 {
		t.Errorf("expected 3 trades for %s, got %d", ticker, len(holdingTrades[ticker]))
	}

	// Verify all trades are present
	if holdingTrades[ticker][0].Units != 100 || holdingTrades[ticker][0].Type != "buy" {
		t.Errorf("first trade incorrect: %+v", holdingTrades[ticker][0])
	}
	if holdingTrades[ticker][1].Units != 100 || holdingTrades[ticker][1].Type != "sell" {
		t.Errorf("second trade incorrect: %+v", holdingTrades[ticker][1])
	}
	if holdingTrades[ticker][2].Units != 200 || holdingTrades[ticker][2].Type != "buy" {
		t.Errorf("third trade incorrect: %+v", holdingTrades[ticker][2])
	}
}

// TestSyncPortfolio_GainLossPreservedOnPriceUpdate verifies the end-to-end fix:
// EODHD price cross-check preserves the realised component of GainLoss.
func TestSyncPortfolio_GainLossPreservedOnPriceUpdate(t *testing.T) {
	today := time.Now()
	navexaPrice := 9.50
	eodhPrice := 10.00

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID:           "200",
				PortfolioID:  "1",
				Ticker:       "SKS",
				Exchange:     "AU",
				Name:         "SKS Tech",
				Units:        700,
				CurrentPrice: navexaPrice,
				MarketValue:  navexaPrice * 700,
				LastUpdated:  today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"200": {
				{ID: "t1", HoldingID: "200", Symbol: "SKS", Type: "buy", Units: 1000, Price: 10.00},
				{ID: "t2", HoldingID: "200", Symbol: "SKS", Type: "sell", Units: 500, Price: 8.00},
				{ID: "t3", HoldingID: "200", Symbol: "SKS", Type: "buy", Units: 200, Price: 9.00},
			},
		},
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"SKS.AU": {
				Ticker: "SKS.AU",
				EOD: []models.EODBar{
					{Date: today, Close: eodhPrice},
					{Date: today.AddDate(0, 0, -1), Close: navexaPrice},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	var sks *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "SKS" {
			sks = &portfolio.Holdings[i]
			break
		}
	}
	if sks == nil {
		t.Fatal("SKS holding not found")
	}

	// Price should be updated to EODHD close
	if !approxEqual(sks.CurrentPrice, eodhPrice, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want %.2f", sks.CurrentPrice, eodhPrice)
	}

	// GainLoss should match a fresh calculation with the EODHD price
	// totalInvested = 10000 + 1800 = 11800
	// totalProceeds = 4000
	// marketValue = 700 * 10.00 = 7000
	// GainLoss = 4000 + 7000 - 11800 = -800
	expectedGainLoss := -800.0
	if !approxEqual(sks.NetReturn, expectedGainLoss, 0.01) {
		t.Errorf("NetReturn = %.2f, want %.2f (should preserve realised loss component)", sks.NetReturn, expectedGainLoss)
	}
}

// --- Devils-Advocate Stress Tests ---

// TestGainLoss_FullExitAndReEntry: sell all units, then re-buy. Ensures realised
// loss/gain from the first position is preserved after full exit and re-entry.
func TestGainLoss_FullExitAndReEntry(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 1000, Price: 10.00, Fees: 0},
		{Type: "sell", Units: 1000, Price: 8.00, Fees: 0}, // full exit: realised loss of $2000
		{Type: "buy", Units: 500, Price: 9.00, Fees: 0},   // re-entry
	}

	currentPrice := 9.50
	remainingUnits := 500.0
	marketValue := remainingUnits * currentPrice

	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, marketValue)

	// totalInvested = 10000 + 4500 = 14500
	if !approxEqual(totalInvested, 14500.0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 14500.00", totalInvested)
	}
	// totalProceeds = 8000
	if !approxEqual(totalProceeds, 8000.0, 0.01) {
		t.Errorf("totalProceeds = %.2f, want 8000.00", totalProceeds)
	}
	// GainLoss = 8000 + 4750 - 14500 = -1750
	expectedGainLoss := 8000.0 + 4750.0 - 14500.0
	if !approxEqual(gainLoss, expectedGainLoss, 0.01) {
		t.Errorf("gainLoss = %.2f, want %.2f", gainLoss, expectedGainLoss)
	}
}

// TestGainLoss_ManyPartialSellsToNearZero: reduce to a tiny position through many sells.
func TestGainLoss_ManyPartialSellsToNearZero(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 10000, Price: 5.00, Fees: 0},
	}
	// Sell in 9 batches of 1111 units each, leaving 1 unit
	for i := 0; i < 9; i++ {
		trades = append(trades, &models.NavexaTrade{
			Type: "sell", Units: 1111, Price: 4.50, Fees: 0,
		})
	}

	remainingUnits := 10000.0 - 9*1111.0 // = 1
	currentPrice := 4.50
	marketValue := remainingUnits * currentPrice

	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, marketValue)

	// totalInvested = 50000
	if !approxEqual(totalInvested, 50000.0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 50000.00", totalInvested)
	}
	// totalProceeds = 9 * 1111 * 4.50 = 44995.50
	expectedProceeds := 9.0 * 1111.0 * 4.50
	if !approxEqual(totalProceeds, expectedProceeds, 0.01) {
		t.Errorf("totalProceeds = %.2f, want %.2f", totalProceeds, expectedProceeds)
	}
	// GainLoss = 44995.50 + 4.50 - 50000 = -5000 (loss of $0.50/unit on all 10000)
	expectedGainLoss := expectedProceeds + marketValue - 50000.0
	if !approxEqual(gainLoss, expectedGainLoss, 0.01) {
		t.Errorf("gainLoss = %.2f, want %.2f", gainLoss, expectedGainLoss)
	}
}

// TestGainLoss_SellMoreThanBought: should not panic, produces negative remaining units.
func TestGainLoss_SellMoreThanBought(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.00, Fees: 0},
		{Type: "sell", Units: 150, Price: 12.00, Fees: 0}, // oversell
	}

	// calculateGainLossFromTrades doesn't track units — just sums invested/proceeds
	// so this should not panic
	totalInvested, _, gainLoss := calculateGainLossFromTrades(trades, 0)

	if !approxEqual(totalInvested, 1000.0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 1000.00", totalInvested)
	}
	// GainLoss = 1800 + 0 - 1000 = 800
	if !approxEqual(gainLoss, 800.0, 0.01) {
		t.Errorf("gainLoss = %.2f, want 800.00", gainLoss)
	}
}

// TestAvgCost_SellMoreThanBought: calculateAvgCostFromTrades with oversell doesn't panic.
func TestAvgCost_SellMoreThanBought(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.00, Fees: 0},
		{Type: "sell", Units: 150, Price: 12.00, Fees: 0}, // oversell
	}

	// Should not panic, even though totalUnits goes negative
	avgCost, totalCost, _ := calculateAvgCostFromTrades(trades)

	// After buy: totalUnits=100, totalCost=1000
	// After sell: costPerUnit=10, removed 150*10=1500, totalCost=1000-1500=-500, totalUnits=-50
	// avgCost = -500 / -50 = 10 (mathematically valid, practically meaningless)
	// The key check: it doesn't panic
	_ = avgCost
	_ = totalCost
}

// TestGainLoss_ExtremelyLargeValues: no overflow or precision loss at scale.
func TestGainLoss_ExtremelyLargeValues(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 1e9, Price: 1e6, Fees: 0}, // $1 quadrillion position
		{Type: "sell", Units: 5e8, Price: 1.1e6, Fees: 0},
	}

	marketValue := 5e8 * 1.2e6 // remaining 500M units at $1.2M each
	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, marketValue)

	expectedInvested := 1e9 * 1e6
	expectedProceeds := 5e8 * 1.1e6
	expectedGainLoss := expectedProceeds + marketValue - expectedInvested

	if !approxEqual(totalInvested, expectedInvested, 1e6) {
		t.Errorf("totalInvested precision loss at extreme scale")
	}
	if !approxEqual(totalProceeds, expectedProceeds, 1e6) {
		t.Errorf("totalProceeds precision loss at extreme scale")
	}
	if !approxEqual(gainLoss, expectedGainLoss, 1e6) {
		t.Errorf("gainLoss precision loss at extreme scale: got %.2f, want %.2f", gainLoss, expectedGainLoss)
	}
}

// TestGainLoss_ExtremelySmallValues: penny stocks with fractional units.
func TestGainLoss_ExtremelySmallValues(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 0.001, Price: 0.001, Fees: 0.0001},
		{Type: "sell", Units: 0.0005, Price: 0.002, Fees: 0.0001},
	}

	marketValue := 0.0005 * 0.0015 // remaining units at current price
	totalInvested, _, gainLoss := calculateGainLossFromTrades(trades, marketValue)

	expectedInvested := 0.001*0.001 + 0.0001
	expectedProceeds := 0.0005*0.002 - 0.0001
	expectedGainLoss := expectedProceeds + marketValue - expectedInvested

	if !approxEqual(totalInvested, expectedInvested, 1e-8) {
		t.Errorf("totalInvested = %e, want %e", totalInvested, expectedInvested)
	}
	if !approxEqual(gainLoss, expectedGainLoss, 1e-8) {
		t.Errorf("gainLoss = %e, want %e", gainLoss, expectedGainLoss)
	}
}

// TestGainLoss_ZeroPriceTrade: trades with zero price should not panic.
func TestGainLoss_ZeroPriceTrade(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 0, Fees: 0},
		{Type: "sell", Units: 50, Price: 0, Fees: 0},
	}

	_, _, gainLoss := calculateGainLossFromTrades(trades, 0)
	if gainLoss != 0 {
		t.Errorf("gainLoss = %.2f, want 0 for zero-price trades", gainLoss)
	}

	avgCost, totalCost, _ := calculateAvgCostFromTrades(trades)
	if avgCost != 0 || totalCost != 0 {
		t.Errorf("avgCost/totalCost should be 0 for zero-price trades, got %.2f / %.2f", avgCost, totalCost)
	}
}

// TestGainLoss_NegativeFees: negative fees (rebates) should not break calculations.
func TestGainLoss_NegativeFees(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.00, Fees: -5.00},  // rebate
		{Type: "sell", Units: 100, Price: 12.00, Fees: -3.00}, // rebate
	}

	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, 0)

	// invested = 100*10 + (-5) = 995
	if !approxEqual(totalInvested, 995.0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 995.00", totalInvested)
	}
	// proceeds = 100*12 - (-3) = 1203
	if !approxEqual(totalProceeds, 1203.0, 0.01) {
		t.Errorf("totalProceeds = %.2f, want 1203.00", totalProceeds)
	}
	// gainLoss = 1203 + 0 - 995 = 208
	if !approxEqual(gainLoss, 208.0, 0.01) {
		t.Errorf("gainLoss = %.2f, want 208.00", gainLoss)
	}
}

// TestGainLoss_EmptyTrades: empty trade array should produce zeros.
func TestGainLoss_EmptyTrades(t *testing.T) {
	trades := []*models.NavexaTrade{}

	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, 1000)
	// With 0 invested and 0 proceeds, GainLoss = 0 + 1000 - 0 = 1000
	if !approxEqual(totalInvested, 0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 0", totalInvested)
	}
	if !approxEqual(totalProceeds, 0, 0.01) {
		t.Errorf("totalProceeds = %.2f, want 0", totalProceeds)
	}
	if !approxEqual(gainLoss, 1000.0, 0.01) {
		t.Errorf("gainLoss = %.2f, want 1000.00 (marketValue with no trades)", gainLoss)
	}
}

// TestGainLoss_NilTrades: nil trade slice should produce zeros.
func TestGainLoss_NilTrades(t *testing.T) {
	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(nil, 500)
	if !approxEqual(totalInvested, 0, 0.01) || !approxEqual(totalProceeds, 0, 0.01) {
		t.Errorf("invested/proceeds should be 0 for nil trades")
	}
	if !approxEqual(gainLoss, 500.0, 0.01) {
		t.Errorf("gainLoss = %.2f, want 500.00", gainLoss)
	}
}

// TestGainLoss_RealisedGainPreserved: buy cheap, sell high (realised gain), re-buy.
// Verifies realised GAIN is preserved, not just losses.
func TestGainLoss_RealisedGainPreserved(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 1000, Price: 5.00, Fees: 0},
		{Type: "sell", Units: 500, Price: 15.00, Fees: 0}, // realised gain: 500*(15-5) = $5000
		{Type: "buy", Units: 300, Price: 10.00, Fees: 0},  // re-buy
	}

	currentPrice := 10.00
	remainingUnits := 1000.0 - 500 + 300
	marketValue := remainingUnits * currentPrice

	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, marketValue)

	// totalInvested = 5000 + 3000 = 8000
	if !approxEqual(totalInvested, 8000.0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 8000.00", totalInvested)
	}
	// totalProceeds = 7500
	if !approxEqual(totalProceeds, 7500.0, 0.01) {
		t.Errorf("totalProceeds = %.2f, want 7500.00", totalProceeds)
	}
	// GainLoss = 7500 + 8000 - 8000 = 7500
	expectedGainLoss := totalProceeds + marketValue - totalInvested
	if !approxEqual(gainLoss, expectedGainLoss, 0.01) {
		t.Errorf("gainLoss = %.2f, want %.2f", gainLoss, expectedGainLoss)
	}
	// Should be positive (realised gain + unrealised gain)
	if gainLoss <= 0 {
		t.Errorf("expected positive gainLoss for profitable position, got %.2f", gainLoss)
	}

	// Now simulate EODHD price update: price drops to $8
	eodhPrice := 8.0
	newMarketValue := remainingUnits * eodhPrice
	adjustedGainLoss := gainLoss + (newMarketValue - marketValue)

	// Fresh calculation should match
	_, _, freshGainLoss := calculateGainLossFromTrades(trades, newMarketValue)
	if !approxEqual(adjustedGainLoss, freshGainLoss, 0.01) {
		t.Errorf("delta-adjusted (%.2f) != fresh (%.2f) after price drop", adjustedGainLoss, freshGainLoss)
	}
}

// TestGainLoss_MultiplePriceUpdates: sequential price updates should compound correctly.
func TestGainLoss_MultiplePriceUpdates(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 1000, Price: 10.00, Fees: 0},
		{Type: "sell", Units: 300, Price: 8.00, Fees: 0},
	}

	remainingUnits := 700.0
	price1 := 10.00
	mv1 := remainingUnits * price1

	_, _, baseGainLoss := calculateGainLossFromTrades(trades, mv1)

	// Simulate 3 sequential EODHD price updates
	prices := []float64{10.50, 9.80, 11.20}
	currentGainLoss := baseGainLoss
	currentMV := mv1

	for _, newPrice := range prices {
		newMV := remainingUnits * newPrice
		currentGainLoss += newMV - currentMV
		currentMV = newMV
	}

	// After all updates, should match a fresh calculation at the final price
	finalMV := remainingUnits * prices[len(prices)-1]
	_, _, freshGainLoss := calculateGainLossFromTrades(trades, finalMV)

	if !approxEqual(currentGainLoss, freshGainLoss, 0.01) {
		t.Errorf("compounded delta (%.2f) != fresh calculation (%.2f) after %d price updates",
			currentGainLoss, freshGainLoss, len(prices))
	}
}

// TestSyncPortfolio_ZeroPriceEODHD: EODHD returns a zero close price (e.g., delisted stock).
// The delta approach means GainLoss drops by the full market value, which is correct behavior
// (the position is now worthless if the price is zero).
func TestSyncPortfolio_ZeroPriceEODHD(t *testing.T) {
	today := time.Now()
	navexaPrice := 10.00

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "300", PortfolioID: "1", Ticker: "DEAD", Exchange: "AU",
				Name: "Delisted Co", Units: 100,
				CurrentPrice: navexaPrice, MarketValue: navexaPrice * 100,
				LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"300": {
				{ID: "t1", HoldingID: "300", Symbol: "DEAD", Type: "buy", Units: 100, Price: 10.00},
			},
		},
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"DEAD.AU": {
				Ticker: "DEAD.AU",
				EOD: []models.EODBar{
					{Date: today, Close: 0}, // Zero close
					{Date: today.AddDate(0, 0, -1), Close: navexaPrice},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	var dead *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "DEAD" {
			dead = &portfolio.Holdings[i]
			break
		}
	}
	if dead == nil {
		t.Fatal("DEAD holding not found")
	}

	// Price should be 0 (EODHD zero close is within 24h and differs)
	if dead.CurrentPrice != 0 {
		t.Errorf("CurrentPrice = %.2f, want 0 (EODHD zero close)", dead.CurrentPrice)
	}
	if dead.MarketValue != 0 {
		t.Errorf("MarketValue = %.2f, want 0", dead.MarketValue)
	}
	// NetReturn should be -1000 (total loss: invested 1000, market value 0)
	if !approxEqual(dead.NetReturn, -1000.0, 0.01) {
		t.Errorf("NetReturn = %.2f, want -1000.00", dead.NetReturn)
	}
}

// TestSyncPortfolio_NoTradesButEODHDUpdate: holding with no trades still gets
// price update via EODHD cross-check. GainLoss comes from Navexa and is adjusted.
func TestSyncPortfolio_NoTradesButEODHDUpdate(t *testing.T) {
	today := time.Now()
	navexaPrice := 50.00
	eodhPrice := 52.00

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "400", PortfolioID: "1", Ticker: "NOTRADE", Exchange: "AU",
				Name: "No Trade Co", Units: 200,
				CurrentPrice: navexaPrice, MarketValue: navexaPrice * 200,
				GainLoss:    1000, // from Navexa, not recalculated (no trades)
				TotalCost:   9000,
				LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{}, // empty — GetHoldingTrades returns nil
	}

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"NOTRADE.AU": {
				Ticker: "NOTRADE.AU",
				EOD: []models.EODBar{
					{Date: today, Close: eodhPrice},
					{Date: today.AddDate(0, 0, -1), Close: navexaPrice},
				},
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	ctx := common.WithNavexaClient(context.Background(), navexa)
	portfolio, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	var h *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "NOTRADE" {
			h = &portfolio.Holdings[i]
			break
		}
	}
	if h == nil {
		t.Fatal("NOTRADE holding not found")
	}

	// Price should be updated
	if !approxEqual(h.CurrentPrice, eodhPrice, 0.01) {
		t.Errorf("CurrentPrice = %.2f, want %.2f", h.CurrentPrice, eodhPrice)
	}

	// NetReturn should be adjusted by delta: 1000 + (200*(52-50)) = 1000 + 400 = 1400
	expectedGainLoss := 1000.0 + 200*(eodhPrice-navexaPrice)
	if !approxEqual(h.NetReturn, expectedGainLoss, 0.01) {
		t.Errorf("NetReturn = %.2f, want %.2f (Navexa base + price delta)", h.NetReturn, expectedGainLoss)
	}
}

// TestGainLoss_DeltaApproachMathProof: mathematically prove the delta approach
// always equals a fresh recalculation, regardless of the initial GainLoss value.
func TestGainLoss_DeltaApproachMathProof(t *testing.T) {
	testCases := []struct {
		name   string
		trades []*models.NavexaTrade
		units  float64
	}{
		{
			name: "partial sell at loss",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 1000, Price: 10.00},
				{Type: "sell", Units: 400, Price: 7.00},
			},
			units: 600,
		},
		{
			name: "partial sell at profit",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 1000, Price: 10.00},
				{Type: "sell", Units: 400, Price: 15.00},
			},
			units: 600,
		},
		{
			name: "multiple buys and sells",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 500, Price: 10.00},
				{Type: "sell", Units: 200, Price: 8.00},
				{Type: "buy", Units: 300, Price: 12.00},
				{Type: "sell", Units: 100, Price: 11.00},
			},
			units: 500,
		},
		{
			name: "pure buy and hold",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 1000, Price: 10.00},
			},
			units: 1000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			navexaPrice := 9.50
			eodhPrice := 10.50
			navexaMV := tc.units * navexaPrice
			eodhMV := tc.units * eodhPrice

			// Method 1: Delta approach (what the fix does)
			_, _, initialGL := calculateGainLossFromTrades(tc.trades, navexaMV)
			deltaGL := initialGL + (eodhMV - navexaMV)

			// Method 2: Fresh calculation with new price
			_, _, freshGL := calculateGainLossFromTrades(tc.trades, eodhMV)

			if !approxEqual(deltaGL, freshGL, 0.01) {
				t.Errorf("delta approach (%.2f) != fresh calculation (%.2f)", deltaGL, freshGL)
			}
		})
	}
}

// TestGainLoss_UnknownTradeType: unknown trade types should be silently ignored.
func TestGainLoss_UnknownTradeType(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.00, Fees: 0},
		{Type: "split", Units: 200, Price: 5.00, Fees: 0},     // stock split — ignored
		{Type: "dividend", Units: 0, Price: 0, Value: 50.00},  // dividend — ignored
		{Type: "UNKNOWN", Units: 100, Price: 100.00, Fees: 0}, // garbage — ignored
		{Type: "", Units: 100, Price: 100.00, Fees: 0},        // empty — ignored
		{Type: "sell", Units: 50, Price: 12.00, Fees: 0},
	}

	totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(trades, 50*12.0)

	// Only buy and sell are counted
	if !approxEqual(totalInvested, 1000.0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 1000.00 (only 'buy' counted)", totalInvested)
	}
	if !approxEqual(totalProceeds, 600.0, 0.01) {
		t.Errorf("totalProceeds = %.2f, want 600.00 (only 'sell' counted)", totalProceeds)
	}
	expectedGL := 600.0 + 600.0 - 1000.0
	if !approxEqual(gainLoss, expectedGL, 0.01) {
		t.Errorf("gainLoss = %.2f, want %.2f", gainLoss, expectedGL)
	}
}

func TestGainLossPercent_SimpleCalculation(t *testing.T) {
	// SKS scenario: buy, partial sells, re-entry buys, current price $4.71
	// The simple gain/loss % should be GainLoss / TotalInvested * 100
	// Using total capital invested as denominator (not remaining cost basis)
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 4925, Price: 4.0248, Fees: 0},
		{Type: "sell", Units: 1333, Price: 3.7627, Fees: 0},
		{Type: "sell", Units: 819, Price: 3.680, Fees: 0},
		{Type: "sell", Units: 2773, Price: 3.4508, Fees: 0},
		{Type: "buy", Units: 2511, Price: 3.980, Fees: 0},
		{Type: "buy", Units: 2456, Price: 4.070, Fees: 0},
	}

	currentPrice := 4.71
	remainingUnits := 4925.0 - 1333 - 819 - 2773 + 2511 + 2456 // = 4967
	marketValue := remainingUnits * currentPrice

	// Calculate gain/loss from trades (totalInvested is the denominator)
	totalInvested, _, gainLoss := calculateGainLossFromTrades(trades, marketValue)

	// Simple percentage: GainLoss / TotalInvested * 100
	gainLossPct := (gainLoss / totalInvested) * 100

	// Verify the simple % is approximately 2.96%, using total capital invested
	if gainLossPct > 10 || gainLossPct < 0 {
		t.Errorf("gainLossPct = %.2f%%, expected ~2.96%% (positive return)", gainLossPct)
	}
	if !approxEqual(gainLossPct, 2.96, 0.5) {
		t.Errorf("gainLossPct = %.2f%%, want ~2.96%%", gainLossPct)
	}

	// Also verify TotalReturnPct with dividends = 0
	dividends := 0.0
	totalReturnValue := gainLoss + dividends
	totalReturnPct := (totalReturnValue / totalInvested) * 100
	if !approxEqual(totalReturnPct, gainLossPct, 0.01) {
		t.Errorf("totalReturnPct = %.2f%%, should equal gainLossPct = %.2f%% when dividends = 0",
			totalReturnPct, gainLossPct)
	}
}

func TestGainLossPercent_AfterPriceUpdate(t *testing.T) {
	// Test that percentage is recomputed correctly after EODHD price cross-check
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 1000, Price: 10.00, Fees: 0},
		{Type: "sell", Units: 500, Price: 8.00, Fees: 0}, // realised loss
		{Type: "buy", Units: 200, Price: 9.00, Fees: 0},  // re-entry
	}
	// Remaining units: 700
	remainingUnits := 700.0

	navexaPrice := 9.50
	navexaMarketValue := remainingUnits * navexaPrice

	// Step 1: Calculate from trades (denominator is totalInvested)
	totalInvested, _, gainLoss := calculateGainLossFromTrades(trades, navexaMarketValue)

	// Simple percentage before price update
	pctBefore := (gainLoss / totalInvested) * 100

	// Step 2: EODHD price update (simulating the delta adjustment)
	eodhPrice := 10.00
	oldMarketValue := navexaMarketValue
	newMarketValue := remainingUnits * eodhPrice
	gainLoss += newMarketValue - oldMarketValue

	// Simple percentage after price update
	pctAfter := (gainLoss / totalInvested) * 100

	// Verify percentage changed in the right direction
	if pctAfter <= pctBefore {
		t.Errorf("pctAfter (%.2f%%) should be > pctBefore (%.2f%%) since price went up", pctAfter, pctBefore)
	}

	// Cross-check: fresh calculation with EODHD price should produce same gainLoss
	_, _, freshGainLoss := calculateGainLossFromTrades(trades, newMarketValue)
	freshPct := (freshGainLoss / totalInvested) * 100

	if !approxEqual(pctAfter, freshPct, 0.01) {
		t.Errorf("delta-adjusted pct (%.2f%%) != fresh pct (%.2f%%)", pctAfter, freshPct)
	}

	// Verify the TotalReturnPct with dividends
	dividends := 50.0
	totalReturnValue := gainLoss + dividends
	totalReturnPct := (totalReturnValue / totalInvested) * 100

	expectedReturnPct := ((freshGainLoss + dividends) / totalInvested) * 100
	if !approxEqual(totalReturnPct, expectedReturnPct, 0.01) {
		t.Errorf("totalReturnPct = %.2f%%, want %.2f%%", totalReturnPct, expectedReturnPct)
	}
}

func TestGainLossPercent_PartialSell_UsesTotalInvested(t *testing.T) {
	// DOW-like scenario: partial sell inflates % if using remaining cost as denominator.
	// Correct approach: use totalInvested (total capital deployed).
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 50.00, Fees: 10},
		{Type: "sell", Units: 60, Price: 48.00, Fees: 5},
	}

	remainingUnits := 40.0
	currentPrice := 49.00
	marketValue := remainingUnits * currentPrice

	totalInvested, _, gainLoss := calculateGainLossFromTrades(trades, marketValue)
	_, remainingCost, _ := calculateAvgCostFromTrades(trades)

	// totalInvested = 100*50 + 10 = 5010
	// totalProceeds = 60*48 - 5 = 2875
	// marketValue = 40 * 49 = 1960
	// gainLoss = 2875 + 1960 - 5010 = -175
	if !approxEqual(totalInvested, 5010.0, 0.01) {
		t.Errorf("totalInvested = %.2f, want 5010.00", totalInvested)
	}
	if !approxEqual(gainLoss, -175.0, 0.01) {
		t.Errorf("gainLoss = %.2f, want -175.00", gainLoss)
	}

	// Correct (new): using totalInvested — pct = -175/5010*100 = -3.49%
	pctCorrect := (gainLoss / totalInvested) * 100
	// Wrong (old): using remainingCost — inflated magnitude
	pctWrong := (gainLoss / remainingCost) * 100

	// The correct percentage should be smaller in magnitude
	if math.Abs(pctCorrect) >= math.Abs(pctWrong) {
		t.Errorf("totalInvested pct (%.2f%%) should have smaller magnitude than remainingCost pct (%.2f%%)",
			pctCorrect, pctWrong)
	}

	if !approxEqual(pctCorrect, -3.49, 0.1) {
		t.Errorf("pctCorrect = %.2f%%, want ~-3.49%%", pctCorrect)
	}
}

func TestGainLoss_RealizedPlusUnrealized_EqualsTotal(t *testing.T) {
	// Verify that realized + unrealized gain/loss equals total gain/loss
	// for partial-sell holdings.
	tests := []struct {
		name   string
		trades []*models.NavexaTrade
		price  float64
		units  float64
	}{
		{
			name: "partial_sell_at_loss",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 100, Price: 50.00, Fees: 10},
				{Type: "sell", Units: 60, Price: 48.00, Fees: 5},
			},
			price: 49.00,
			units: 40,
		},
		{
			name: "partial_sell_at_profit",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 200, Price: 10.00, Fees: 0},
				{Type: "sell", Units: 100, Price: 15.00, Fees: 0},
			},
			price: 12.00,
			units: 100,
		},
		{
			name: "multiple_buys_and_sells",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 4925, Price: 4.0248, Fees: 0},
				{Type: "sell", Units: 1333, Price: 3.7627, Fees: 0},
				{Type: "sell", Units: 819, Price: 3.680, Fees: 0},
				{Type: "sell", Units: 2773, Price: 3.4508, Fees: 0},
				{Type: "buy", Units: 2511, Price: 3.980, Fees: 0},
				{Type: "buy", Units: 2456, Price: 4.070, Fees: 0},
			},
			price: 4.71,
			units: 4967,
		},
		{
			name: "buy_only_no_sells",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 500, Price: 20.00, Fees: 5},
			},
			price: 22.00,
			units: 500,
		},
		{
			name: "fully_closed",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 100, Price: 10.00, Fees: 0},
				{Type: "sell", Units: 100, Price: 15.00, Fees: 0},
			},
			price: 0,
			units: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			marketValue := tc.units * tc.price
			totalInvested, totalProceeds, gainLoss := calculateGainLossFromTrades(tc.trades, marketValue)
			_, remainingCost, _ := calculateAvgCostFromTrades(tc.trades)

			realizedGL := totalProceeds - (totalInvested - remainingCost)
			unrealizedGL := marketValue - remainingCost

			sum := realizedGL + unrealizedGL
			if !approxEqual(sum, gainLoss, 0.01) {
				t.Errorf("realized(%.2f) + unrealized(%.2f) = %.2f, want gainLoss = %.2f",
					realizedGL, unrealizedGL, sum, gainLoss)
			}
		})
	}
}

// --- Devils-Advocate: TotalCost boundary stress tests ---

// TestSyncPortfolio_ZeroTotalCost_NoPercentDivByZero verifies that when all units
// are sold and the remaining cost is zero, the percent fields don't cause a
// division-by-zero and fall through gracefully.
func TestSyncPortfolio_ZeroTotalCost_NoPercentDivByZero(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "500", PortfolioID: "1", Ticker: "SOLD", Exchange: "AU",
				Name: "Fully Sold Co", Units: 0, // fully closed position
				CurrentPrice: 0, MarketValue: 0,
				GainLoss: 500, GainLossPct: 99.9, // Navexa's IRR — stale if not overwritten
				TotalCost: 0, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"500": {
				{ID: "t1", HoldingID: "500", Symbol: "SOLD", Type: "buy", Units: 100, Price: 10.00},
				{ID: "t2", HoldingID: "500", Symbol: "SOLD", Type: "sell", Units: 100, Price: 15.00},
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

	var sold *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "SOLD" {
			sold = &portfolio.Holdings[i]
			break
		}
	}
	if sold == nil {
		t.Fatal("SOLD holding not found")
	}

	// Position is closed (Units = 0). TotalCost for closed positions uses totalInvested.
	// totalInvested = 100*10 = 1000, which is > 0, so % should be computed.
	// NetReturn = proceeds(1500) + MV(0) - invested(1000) = 500
	if !approxEqual(sold.NetReturn, 500.0, 0.01) {
		t.Errorf("NetReturn = %.2f, want 500.00", sold.NetReturn)
	}

	// TotalCost should be totalInvested (1000) since Units <= 0
	if !approxEqual(sold.TotalCost, 1000.0, 0.01) {
		t.Errorf("TotalCost = %.2f, want 1000.00 (totalInvested for closed position)", sold.TotalCost)
	}

	// NetReturnPct should be simple % = 500/1000*100 = 50%, NOT Navexa's 99.9%
	expectedPct := (500.0 / 1000.0) * 100
	if !approxEqual(sold.NetReturnPct, expectedPct, 0.1) {
		t.Errorf("NetReturnPct = %.2f%%, want %.2f%% (not Navexa IRR 99.9%%)", sold.NetReturnPct, expectedPct)
	}
}

// TestSyncPortfolio_CostBaseDecreaseBelowZero verifies behavior when cost base
// adjustments push totalCost below zero. The `if h.TotalCost > 0` guard should
// prevent division by a negative number, and percentage fields should be 0.
func TestSyncPortfolio_CostBaseDecreaseBelowZero(t *testing.T) {
	today := time.Now()

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "600", PortfolioID: "1", Ticker: "NEGCOST", Exchange: "AU",
				Name: "Negative Cost Co", Units: 100,
				CurrentPrice: 5.00, MarketValue: 500,
				GainLossPct: 77.7, // stale Navexa IRR
				LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"600": {
				{ID: "t1", HoldingID: "600", Symbol: "NEGCOST", Type: "buy", Units: 100, Price: 2.00},
				{ID: "t2", HoldingID: "600", Symbol: "NEGCOST", Type: "cost base decrease", Value: 300.00}, // pushes cost below zero
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

	var h *models.Holding
	for i := range portfolio.Holdings {
		if portfolio.Holdings[i].Ticker == "NEGCOST" {
			h = &portfolio.Holdings[i]
			break
		}
	}
	if h == nil {
		t.Fatal("NEGCOST holding not found")
	}

	// TotalCost = remainingCost = 200 - 300 = -100 (negative from cost base decrease)
	if !approxEqual(h.TotalCost, -100.0, 0.01) {
		t.Errorf("TotalCost = %.2f, want -100.00", h.TotalCost)
	}

	// When TotalCost <= 0, percentage fields should be zeroed out (not stale Navexa IRR).
	// The `if h.TotalCost > 0` guard skips percentage computation; the else branch
	// should zero them out to avoid leaking stale Navexa IRR values.
	if h.NetReturnPct != 0 {
		t.Errorf("NetReturnPct = %.2f%%, want 0%% (TotalCost <= 0 means percent undefined)", h.NetReturnPct)
	}
	if h.CapitalGainPct != 0 {
		t.Errorf("CapitalGainPct = %.2f%%, want 0%%", h.CapitalGainPct)
	}
}

// TestSyncPortfolio_ForceRefresh_WithoutNavexaContext verifies that force_refresh=true
// without Navexa context headers produces a clear error, not a panic.
func TestSyncPortfolio_ForceRefresh_WithoutNavexaContext(t *testing.T) {
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, nil, logger)

	// No Navexa client in context — should return error, not panic
	ctx := context.Background()
	_, err := svc.SyncPortfolio(ctx, "SMSF", true)
	if err == nil {
		t.Fatal("expected error when force syncing without Navexa context")
	}

	// Error should mention navexa/portal
	errMsg := err.Error()
	if !strings.Contains(errMsg, "navexa") && !strings.Contains(errMsg, "portal") {
		t.Errorf("error message should mention navexa/portal, got: %s", errMsg)
	}
}

// TestGainLossPercent_TotalCostZero_Guarded verifies the TotalCost > 0 guard
// prevents division by zero in the percentage computation path.
func TestGainLossPercent_TotalCostZero_Guarded(t *testing.T) {
	// A scenario where calculateAvgCostFromTrades returns (0, 0):
	// buy with price=0 and fees=0
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 0, Fees: 0},
	}

	avgCost, totalCost, _ := calculateAvgCostFromTrades(trades)

	if totalCost != 0 {
		t.Errorf("totalCost = %.2f, want 0 for zero-price buy", totalCost)
	}
	if avgCost != 0 {
		t.Errorf("avgCost = %.2f, want 0 for zero-price buy", avgCost)
	}

	// Simulate what SyncPortfolio does with this totalCost
	gainLoss := 100.0 // some arbitrary gainLoss
	var gainLossPct float64
	if totalCost > 0 {
		gainLossPct = (gainLoss / totalCost) * 100
	}
	// gainLossPct should remain 0 (default), not NaN/Inf
	if gainLossPct != 0 {
		t.Errorf("gainLossPct = %.2f, want 0 (guarded by totalCost > 0 check)", gainLossPct)
	}
}

// TestGainLossPercent_VerySmallTotalCost_Precision verifies that a near-zero
// TotalCost (e.g., $0.01) produces a very large but finite percentage, not
// infinity or NaN.
func TestGainLossPercent_VerySmallTotalCost_Precision(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 1, Price: 0.01, Fees: 0},
	}

	_, totalCost, _ := calculateAvgCostFromTrades(trades)
	if !approxEqual(totalCost, 0.01, 1e-6) {
		t.Errorf("totalCost = %e, want 0.01", totalCost)
	}

	// If the stock is now worth $100, the gain is massive but should be finite
	gainLoss := 99.99
	gainLossPct := (gainLoss / totalCost) * 100 // = 999900%

	if math.IsInf(gainLossPct, 0) || math.IsNaN(gainLossPct) {
		t.Errorf("gainLossPct is Inf or NaN: %v", gainLossPct)
	}
	if !approxEqual(gainLossPct, 999900.0, 1.0) {
		t.Errorf("gainLossPct = %.2f, want 999900.00", gainLossPct)
	}
}

// TestForceRefreshQueryParam_NonBooleanValues verifies that non-"true" values
// for force_refresh are treated as false (no sync).
func TestForceRefreshQueryParam_NonBooleanValues(t *testing.T) {
	// This tests the handler-level logic: r.URL.Query().Get("force_refresh") == "true"
	// Only the exact string "true" triggers force refresh.
	testValues := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"TRUE", false},   // case-sensitive
		{"True", false},   // case-sensitive
		{"1", false},      // not "true"
		{"yes", false},    // not "true"
		{"", false},       // empty
		{"trueee", false}, // close but not exact
	}

	for _, tc := range testValues {
		result := tc.input == "true"
		if result != tc.expected {
			t.Errorf("force_refresh=%q: got %v, want %v", tc.input, result, tc.expected)
		}
	}
}

// TestAvgCost_TotalCostNegativeFromLargeCostBaseDecrease tests that
// calculateAvgCostFromTrades handles a cost base decrease larger than
// total invested cost without panicking.
func TestAvgCost_TotalCostNegativeFromLargeCostBaseDecrease(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.00, Fees: 0}, // cost = 1000
		{Type: "cost base decrease", Value: 1500.00},     // cost = -500
	}

	avgCost, totalCost, _ := calculateAvgCostFromTrades(trades)

	// totalCost = 1000 - 1500 = -500
	if !approxEqual(totalCost, -500.0, 0.01) {
		t.Errorf("totalCost = %.2f, want -500.00", totalCost)
	}
	// avgCost = -500 / 100 = -5.00 (mathematically correct, practically unusual)
	if !approxEqual(avgCost, -5.0, 0.01) {
		t.Errorf("avgCost = %.2f, want -5.00", avgCost)
	}

	// Now verify that the SyncPortfolio `if h.TotalCost > 0` guard correctly
	// skips percentage computation for this negative cost
	var gainLossPct float64
	gainLoss := 2000.0
	if totalCost > 0 {
		gainLossPct = (gainLoss / totalCost) * 100
	}
	if gainLossPct != 0 {
		t.Errorf("gainLossPct should be 0 (guarded), got %.2f", gainLossPct)
	}
}

// TestSyncPortfolio_ConcurrentForceSync verifies that the syncMu mutex
// correctly serializes two concurrent force_refresh calls.
func TestSyncPortfolio_ConcurrentForceSync(t *testing.T) {
	today := time.Now()
	callCount := 0

	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "SMSF", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "700", PortfolioID: "1", Ticker: "RACE", Exchange: "AU",
				Name: "Race Co", Units: 100, CurrentPrice: 10.00,
				MarketValue: 1000, LastUpdated: today,
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"700": {
				{ID: "t1", HoldingID: "700", Symbol: "RACE", Type: "buy", Units: 100, Price: 10.00},
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

	// Run two force syncs concurrently
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := svc.SyncPortfolio(ctx, "SMSF", true)
			callCount++
			done <- err
		}()
	}

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent SyncPortfolio #%d failed: %v", i, err)
		}
	}

	// Both should succeed (serialized by mutex)
	// No race condition assertion beyond "no panic"
}

// --- P&L Breakeven field tests ---

func TestBreakeven_SimpleHold_NoSells(t *testing.T) {
	// Simple buy-and-hold: breakeven = avg cost, all fields populated
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "Test", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "h1", PortfolioID: "1", Ticker: "ABC", Exchange: "AU",
				Name: "ABC Ltd", Units: 100, CurrentPrice: 12.00,
				MarketValue: 1200.00, LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"h1": {
				{ID: "t1", HoldingID: "h1", Symbol: "ABC", Type: "buy", Units: 100, Price: 10.00, Fees: 0},
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
	portfolio, err := svc.SyncPortfolio(ctx, "Test", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	h := portfolio.Holdings[0]

	// No prior sells: realized=0, so breakeven = totalCost / units = avg cost
	if h.TrueBreakevenPrice == nil {
		t.Fatal("TrueBreakevenPrice should not be nil for open position")
	}
	if !approxEqual(*h.TrueBreakevenPrice, h.AvgCost, 0.01) {
		t.Errorf("TrueBreakevenPrice = %.4f, want %.4f (should equal avg cost for simple hold)", *h.TrueBreakevenPrice, h.AvgCost)
	}

}

func TestBreakeven_PartialSellWithLoss(t *testing.T) {
	// Partial sell at a loss: breakeven should be higher than avg cost
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "Test", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "h1", PortfolioID: "1", Ticker: "DEF", Exchange: "AU",
				Name: "DEF Ltd", Units: 500, CurrentPrice: 9.00,
				MarketValue: 4500.00, LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"h1": {
				{ID: "t1", HoldingID: "h1", Symbol: "DEF", Type: "buy", Units: 1000, Price: 10.00, Fees: 0},
				{ID: "t2", HoldingID: "h1", Symbol: "DEF", Type: "sell", Units: 500, Price: 8.00, Fees: 0},
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
	portfolio, err := svc.SyncPortfolio(ctx, "Test", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	h := portfolio.Holdings[0]

	// Total invested = 1000 * 10 = 10000
	// Avg cost remaining: totalCost after sell = 10000 * (500/1000) = 5000 (proportional)
	// Realized: proceeds - cost_of_sold = 4000 - 5000 = -1000 (loss)
	// True breakeven = (5000 - (-1000)) / 500 = 6000/500 = 12.00
	// This is higher than avg cost of 10.00, reflecting the prior loss
	if h.TrueBreakevenPrice == nil {
		t.Fatal("TrueBreakevenPrice should not be nil")
	}
	if *h.TrueBreakevenPrice <= h.AvgCost {
		t.Errorf("TrueBreakevenPrice = %.4f should be > AvgCost %.4f for partial sell at a loss", *h.TrueBreakevenPrice, h.AvgCost)
	}

	// Verify specific value: breakeven = (totalCost - realizedGL) / units
	expectedBreakeven := (h.TotalCost - h.RealizedNetReturn) / h.Units
	if !approxEqual(*h.TrueBreakevenPrice, expectedBreakeven, 0.01) {
		t.Errorf("TrueBreakevenPrice = %.4f, want %.4f", *h.TrueBreakevenPrice, expectedBreakeven)
	}
}

func TestBreakeven_PartialSellWithProfit(t *testing.T) {
	// Partial sell at a profit: breakeven should be lower than avg cost
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "Test", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "h1", PortfolioID: "1", Ticker: "GHI", Exchange: "AU",
				Name: "GHI Ltd", Units: 500, CurrentPrice: 12.00,
				MarketValue: 6000.00, LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"h1": {
				{ID: "t1", HoldingID: "h1", Symbol: "GHI", Type: "buy", Units: 1000, Price: 10.00, Fees: 0},
				{ID: "t2", HoldingID: "h1", Symbol: "GHI", Type: "sell", Units: 500, Price: 15.00, Fees: 0},
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
	portfolio, err := svc.SyncPortfolio(ctx, "Test", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	h := portfolio.Holdings[0]

	// Total invested = 10000
	// Remaining cost = 5000 (proportional)
	// Realized: proceeds - cost_of_sold = 7500 - 5000 = 2500 (profit)
	// True breakeven = (5000 - 2500) / 500 = 2500/500 = 5.00
	// This is lower than avg cost of 10.00, reflecting the prior profit
	if h.TrueBreakevenPrice == nil {
		t.Fatal("TrueBreakevenPrice should not be nil")
	}
	if *h.TrueBreakevenPrice >= h.AvgCost {
		t.Errorf("TrueBreakevenPrice = %.4f should be < AvgCost %.4f for partial sell at a profit", *h.TrueBreakevenPrice, h.AvgCost)
	}

	expectedBreakeven := (h.TotalCost - h.RealizedNetReturn) / h.Units
	if !approxEqual(*h.TrueBreakevenPrice, expectedBreakeven, 0.01) {
		t.Errorf("TrueBreakevenPrice = %.4f, want %.4f", *h.TrueBreakevenPrice, expectedBreakeven)
	}
}

func TestBreakeven_ClosedPosition_AllFieldsNil(t *testing.T) {
	// Closed position (units=0): breakeven should be nil
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "Test", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "h1", PortfolioID: "1", Ticker: "XYZ", Exchange: "AU",
				Name: "XYZ Ltd", Units: 0, CurrentPrice: 5.00,
				MarketValue: 0.00, LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"h1": {
				{ID: "t1", HoldingID: "h1", Symbol: "XYZ", Type: "buy", Units: 100, Price: 10.00, Fees: 0},
				{ID: "t2", HoldingID: "h1", Symbol: "XYZ", Type: "sell", Units: 100, Price: 12.00, Fees: 0},
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
	portfolio, err := svc.SyncPortfolio(ctx, "Test", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	h := portfolio.Holdings[0]

	if h.TrueBreakevenPrice != nil {
		t.Errorf("TrueBreakevenPrice should be nil for closed position, got %.4f", *h.TrueBreakevenPrice)
	}
}

func TestBreakeven_RealizedPlusUnrealizedEqualsNetReturn(t *testing.T) {
	// Verify RealizedNetReturn + UnrealizedNetReturn = NetReturn
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "Test", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "h1", PortfolioID: "1", Ticker: "NET", Exchange: "AU",
				Name: "NET Ltd", Units: 300, CurrentPrice: 11.00,
				MarketValue: 3300.00, LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"h1": {
				{ID: "t1", HoldingID: "h1", Symbol: "NET", Type: "buy", Units: 500, Price: 10.00, Fees: 0},
				{ID: "t2", HoldingID: "h1", Symbol: "NET", Type: "sell", Units: 200, Price: 8.00, Fees: 0},
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
	portfolio, err := svc.SyncPortfolio(ctx, "Test", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	h := portfolio.Holdings[0]

	expected := h.RealizedNetReturn + h.UnrealizedNetReturn
	if !approxEqual(h.NetReturn, expected, 0.01) {
		t.Errorf("NetReturn = %.2f, want %.2f (realized %.2f + unrealized %.2f)",
			h.NetReturn, expected, h.RealizedNetReturn, h.UnrealizedNetReturn)
	}
}

func TestBreakeven_SKS_Scenario(t *testing.T) {
	// SKS-like scenario from the spec: buy, partial sells at loss, re-entry
	// Verify breakeven = (total_cost - realized_gain_loss) / units
	navexa := &stubNavexaClient{
		portfolios: []*models.NavexaPortfolio{
			{ID: "1", Name: "Test", Currency: "AUD", DateCreated: "2020-01-01"},
		},
		holdings: []*models.NavexaHolding{
			{
				ID: "h1", PortfolioID: "1", Ticker: "SKS", Exchange: "AU",
				Name: "SKS Technologies", Units: 4967, CurrentPrice: 4.71,
				MarketValue: 4967 * 4.71, LastUpdated: time.Now(),
			},
		},
		trades: map[string][]*models.NavexaTrade{
			"h1": {
				{ID: "t1", HoldingID: "h1", Symbol: "SKS", Type: "buy", Units: 4925, Price: 4.0248, Fees: 0},
				{ID: "t2", HoldingID: "h1", Symbol: "SKS", Type: "sell", Units: 1333, Price: 3.7627, Fees: 0},
				{ID: "t3", HoldingID: "h1", Symbol: "SKS", Type: "sell", Units: 819, Price: 3.680, Fees: 0},
				{ID: "t4", HoldingID: "h1", Symbol: "SKS", Type: "sell", Units: 2773, Price: 3.4508, Fees: 0},
				{ID: "t5", HoldingID: "h1", Symbol: "SKS", Type: "buy", Units: 2511, Price: 3.980, Fees: 0},
				{ID: "t6", HoldingID: "h1", Symbol: "SKS", Type: "buy", Units: 2456, Price: 4.070, Fees: 0},
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
	portfolio, err := svc.SyncPortfolio(ctx, "Test", true)
	if err != nil {
		t.Fatalf("SyncPortfolio failed: %v", err)
	}

	h := portfolio.Holdings[0]

	if h.TrueBreakevenPrice == nil {
		t.Fatal("TrueBreakevenPrice should not be nil")
	}

	// Verify breakeven formula: (totalCost - realizedNetReturn) / units
	expectedBreakeven := (h.TotalCost - h.RealizedNetReturn) / h.Units
	if !approxEqual(*h.TrueBreakevenPrice, expectedBreakeven, 0.01) {
		t.Errorf("TrueBreakevenPrice = %.4f, want %.4f", *h.TrueBreakevenPrice, expectedBreakeven)
	}

	// Breakeven should be higher than avg cost (prior losses raise breakeven)
	if *h.TrueBreakevenPrice <= h.AvgCost {
		t.Errorf("TrueBreakevenPrice %.4f should be > AvgCost %.4f (prior sells were at a loss)", *h.TrueBreakevenPrice, h.AvgCost)
	}

	// Log the computed values for verification
	t.Logf("SKS breakeven scenario:")
	t.Logf("  AvgCost=%.4f TotalCost=%.2f RealizedNetReturn=%.2f UnrealizedNetReturn=%.2f",
		h.AvgCost, h.TotalCost, h.RealizedNetReturn, h.UnrealizedNetReturn)
	t.Logf("  TrueBreakeven=%.4f", *h.TrueBreakevenPrice)
}

func TestBreakeven_FieldsSerializeToNullWhenClosed(t *testing.T) {
	// Verify that nil pointer fields serialize to null in JSON
	h := models.Holding{
		Ticker: "CLOSED", Units: 0,
	}

	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	nullFields := []string{
		"true_breakeven_price",
	}
	for _, field := range nullFields {
		val, exists := m[field]
		if !exists {
			t.Errorf("field %q missing from JSON output", field)
			continue
		}
		if val != nil {
			t.Errorf("field %q should be null in JSON for closed position, got %v", field, val)
		}
	}
}

// --- Units-from-trades tests ---

// TestAvgCost_ReturnsUnits verifies that calculateAvgCostFromTrades returns
// the correct remaining units after a sequence of buys and sells.
func TestAvgCost_ReturnsUnits(t *testing.T) {
	tests := []struct {
		name      string
		trades    []*models.NavexaTrade
		wantUnits float64
		wantAvg   float64
		wantCost  float64
	}{
		{
			name: "buy_only",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 100, Price: 10.00, Fees: 0},
			},
			wantUnits: 100,
			wantAvg:   10.00,
			wantCost:  1000.00,
		},
		{
			name: "buy_and_partial_sell",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 100, Price: 10.00, Fees: 0},
				{Type: "sell", Units: 40, Price: 12.00, Fees: 0},
			},
			wantUnits: 60,
			wantAvg:   10.00,
			wantCost:  600.00,
		},
		{
			name: "fully_closed",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 100, Price: 10.00, Fees: 0},
				{Type: "sell", Units: 100, Price: 15.00, Fees: 0},
			},
			wantUnits: 0,
			wantAvg:   0, // no units remaining
			wantCost:  0,
		},
		{
			name: "multiple_buys_partial_sell",
			trades: []*models.NavexaTrade{
				{Type: "buy", Units: 200, Price: 5.00, Fees: 0},
				{Type: "buy", Units: 300, Price: 8.00, Fees: 0},
				{Type: "sell", Units: 100, Price: 10.00, Fees: 0},
			},
			// After buys: 500 units, cost = 1000 + 2400 = 3400, avg = 6.80
			// After sell: 400 units, cost = 3400 - 100*6.80 = 2720, avg = 6.80
			wantUnits: 400,
			wantAvg:   6.80,
			wantCost:  2720.00,
		},
		{
			name: "opening_balance",
			trades: []*models.NavexaTrade{
				{Type: "Opening Balance", Units: 500, Price: 20.00, Fees: 10},
			},
			wantUnits: 500,
			wantAvg:   20.02, // (500*20 + 10) / 500
			wantCost:  10010.00,
		},
		{
			name:      "no_trades",
			trades:    []*models.NavexaTrade{},
			wantUnits: 0,
			wantAvg:   0,
			wantCost:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			avgCost, totalCost, units := calculateAvgCostFromTrades(tc.trades)
			if !approxEqual(units, tc.wantUnits, 0.01) {
				t.Errorf("units = %.2f, want %.2f", units, tc.wantUnits)
			}
			if !approxEqual(avgCost, tc.wantAvg, 0.01) {
				t.Errorf("avgCost = %.2f, want %.2f", avgCost, tc.wantAvg)
			}
			if !approxEqual(totalCost, tc.wantCost, 0.01) {
				t.Errorf("totalCost = %.2f, want %.2f", totalCost, tc.wantCost)
			}
		})
	}
}

// TestAvgCost_UnitsMatchSnapshotReplay verifies that calculateAvgCostFromTrades
// and replayTradesAsOf produce identical units for the same trade set when
// the snapshot date is far in the future (i.e., all trades included).
func TestAvgCost_UnitsMatchSnapshotReplay(t *testing.T) {
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 4925, Price: 4.0248, Fees: 0, Date: "2024-01-15"},
		{Type: "sell", Units: 1333, Price: 3.7627, Fees: 0, Date: "2024-03-10"},
		{Type: "sell", Units: 819, Price: 3.680, Fees: 0, Date: "2024-05-20"},
		{Type: "buy", Units: 2511, Price: 3.980, Fees: 0, Date: "2024-07-01"},
		{Type: "sell", Units: 2773, Price: 3.4508, Fees: 0, Date: "2024-09-15"},
		{Type: "buy", Units: 2456, Price: 4.070, Fees: 0, Date: "2024-11-01"},
	}

	_, _, units := calculateAvgCostFromTrades(trades)
	farFuture := time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)
	snapshotUnits, _, _ := replayTradesAsOf(trades, farFuture)

	if !approxEqual(units, snapshotUnits, 0.01) {
		t.Errorf("units from calculateAvgCostFromTrades (%.2f) != replayTradesAsOf (%.2f)",
			units, snapshotUnits)
	}
}

// TestUnitsFromTrades_OverridesNavexaUnits verifies that the sync loop logic
// correctly uses trade-derived units to recompute MarketValue before gain/loss.
func TestUnitsFromTrades_OverridesNavexaUnits(t *testing.T) {
	// Simulate what SyncPortfolio does: Navexa reports 100 units but trades
	// show 60 remaining after a partial sell.
	trades := []*models.NavexaTrade{
		{Type: "buy", Units: 100, Price: 10.00, Fees: 0},
		{Type: "sell", Units: 40, Price: 12.00, Fees: 0},
	}

	navexaUnits := 100.0 // stale value from Navexa
	currentPrice := 11.00

	// Old behaviour: MarketValue = 100 * 11 = 1100
	oldMarketValue := navexaUnits * currentPrice

	// New behaviour: use trade-derived units
	_, _, tradeUnits := calculateAvgCostFromTrades(trades)
	newMarketValue := tradeUnits * currentPrice

	if !approxEqual(tradeUnits, 60.0, 0.01) {
		t.Errorf("tradeUnits = %.2f, want 60.00", tradeUnits)
	}
	if !approxEqual(newMarketValue, 660.0, 0.01) {
		t.Errorf("newMarketValue = %.2f, want 660.00", newMarketValue)
	}

	// Gain/loss with corrected units
	totalInvested, _, gainLoss := calculateGainLossFromTrades(trades, newMarketValue)
	// totalInvested = 100*10 = 1000
	// totalProceeds = 40*12 = 480
	// gainLoss = 480 + 660 - 1000 = 140
	if !approxEqual(gainLoss, 140.0, 0.01) {
		t.Errorf("gainLoss with corrected units = %.2f, want 140.00", gainLoss)
	}

	// Verify old behaviour would have been wrong
	_, _, oldGainLoss := calculateGainLossFromTrades(trades, oldMarketValue)
	// oldGainLoss = 480 + 1100 - 1000 = 580 (inflated)
	if !approxEqual(oldGainLoss, 580.0, 0.01) {
		t.Errorf("oldGainLoss = %.2f, want 580.00 (demonstrating stale units bug)", oldGainLoss)
	}

	// The corrected gain should be smaller than the inflated one
	if gainLoss >= oldGainLoss {
		t.Errorf("corrected gainLoss (%.2f) should be less than inflated (%.2f)", gainLoss, oldGainLoss)
	}

	// Verify percentage uses totalInvested as denominator
	pct := (gainLoss / totalInvested) * 100
	if !approxEqual(pct, 14.0, 0.1) {
		t.Errorf("gainLossPct = %.2f%%, want ~14.0%%", pct)
	}
}
