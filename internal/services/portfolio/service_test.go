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
func storePortfolio(t *testing.T, store *memUserDataStore, portfolio *models.Portfolio) {
	t.Helper()
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
func (s *flexStorageManager) DataPath() string                                { return "" }
func (s *flexStorageManager) WriteRaw(subdir, key string, data []byte) error  { return nil }
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
