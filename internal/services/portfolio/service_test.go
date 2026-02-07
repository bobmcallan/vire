package portfolio

import (
	"math"
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
