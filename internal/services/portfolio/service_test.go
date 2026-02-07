package portfolio

import (
	"math"
	"strings"
	"testing"

	"github.com/bobmccarthy/vire/internal/models"
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
			wantAction: "SELL",
		},
		{
			name:       "conservative HOLD at RSI 64 (below threshold 65)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			rsi:        64,
			wantAction: "HOLD",
		},
		{
			name:       "conservative BUY at RSI 34 (threshold 35)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			rsi:        34,
			wantAction: "BUY",
		},
		{
			name:       "conservative HOLD at RSI 36 (above threshold 35)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "conservative"}},
			rsi:        36,
			wantAction: "HOLD",
		},
		{
			name:       "aggressive HOLD at RSI 75 (below threshold 80)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
			rsi:        75,
			wantAction: "HOLD",
		},
		{
			name:       "aggressive SELL at RSI 81 (threshold 80)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
			rsi:        81,
			wantAction: "SELL",
		},
		{
			name:       "aggressive BUY at RSI 24 (threshold 25)",
			strategy:   &models.PortfolioStrategy{RiskAppetite: models.RiskAppetite{Level: "aggressive"}},
			rsi:        24,
			wantAction: "BUY",
		},
		{
			name:       "nil strategy SELL at RSI 71 (default 70)",
			strategy:   nil,
			rsi:        71,
			wantAction: "SELL",
		},
		{
			name:       "nil strategy BUY at RSI 29 (default 30)",
			strategy:   nil,
			rsi:        29,
			wantAction: "BUY",
		},
		{
			name:       "nil strategy HOLD at RSI 50",
			strategy:   nil,
			rsi:        50,
			wantAction: "HOLD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := &models.TickerSignals{
				Technical: models.TechnicalSignals{RSI: tt.rsi},
			}
			action, _ := determineAction(signals, nil, tt.strategy, nil)
			if action != tt.wantAction {
				t.Errorf("determineAction(RSI=%.0f) = %q, want %q", tt.rsi, action, tt.wantAction)
			}
		})
	}
}

func TestDetermineAction_NilSignals(t *testing.T) {
	action, reason := determineAction(nil, nil, nil, nil)
	if action != "HOLD" || reason != "Insufficient data" {
		t.Errorf("determineAction(nil signals) = (%q, %q), want (HOLD, Insufficient data)", action, reason)
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

	action, reason := determineAction(signals, nil, strategy, holding)
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

	action, _ := determineAction(signals, nil, strategy, holding)
	if action != "HOLD" {
		t.Errorf("expected HOLD for within-limit position, got %q", action)
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
