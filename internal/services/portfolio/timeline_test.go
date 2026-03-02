package portfolio

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func TestSnapshotsToGrowthPoints(t *testing.T) {
	d1 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)

	snapshots := []models.TimelineSnapshot{
		{
			Date:               d1,
			EquityValue:        100000,
			NetEquityCost:      80000,
			NetEquityReturn:    20000,
			NetEquityReturnPct: 25.0,
			HoldingCount:       5,
			GrossCashBalance:   10000,
			NetCashBalance:     2000,
			PortfolioValue:     102000,
			NetCapitalDeployed: 80000,
		},
		{
			Date:               d2,
			EquityValue:        105000,
			NetEquityCost:      80000,
			NetEquityReturn:    25000,
			NetEquityReturnPct: 31.25,
			HoldingCount:       5,
			GrossCashBalance:   10000,
			NetCashBalance:     2000,
			PortfolioValue:     107000,
			NetCapitalDeployed: 80000,
		},
	}

	points := snapshotsToGrowthPoints(snapshots)
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}

	if points[0].EquityValue != 100000 {
		t.Errorf("expected equity_value 100000, got %f", points[0].EquityValue)
	}
	if points[0].HoldingCount != 5 {
		t.Errorf("expected holding_count 5, got %d", points[0].HoldingCount)
	}
	if points[1].PortfolioValue != 107000 {
		t.Errorf("expected portfolio_value 107000, got %f", points[1].PortfolioValue)
	}
	if !points[0].Date.Equal(d1) {
		t.Errorf("expected date %v, got %v", d1, points[0].Date)
	}
}

func TestSnapshotsToGrowthPoints_Empty(t *testing.T) {
	points := snapshotsToGrowthPoints(nil)
	if len(points) != 0 {
		t.Errorf("expected 0 points for nil input, got %d", len(points))
	}
}

func TestComputeTradeHash_Deterministic(t *testing.T) {
	holdings := []models.Holding{
		{
			Ticker:   "BHP",
			Exchange: "ASX",
			Trades: []*models.NavexaTrade{
				{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 45.50, Fees: 9.95},
				{Date: "2024-06-15", Type: "Sell", Units: 50, Price: 52.00, Fees: 9.95},
			},
		},
		{
			Ticker:   "CBA",
			Exchange: "ASX",
			Trades: []*models.NavexaTrade{
				{Date: "2023-11-05", Type: "Buy", Units: 200, Price: 95.00, Fees: 19.95},
			},
		},
	}

	hash1 := computeTradeHash(holdings)
	hash2 := computeTradeHash(holdings)

	if hash1 != hash2 {
		t.Errorf("expected deterministic hash, got %s and %s", hash1, hash2)
	}
	if len(hash1) != 16 {
		t.Errorf("expected 16-char hash, got %d chars: %s", len(hash1), hash1)
	}
}

func TestComputeTradeHash_ChangesOnModification(t *testing.T) {
	holdings1 := []models.Holding{
		{
			Ticker:   "BHP",
			Exchange: "ASX",
			Trades: []*models.NavexaTrade{
				{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 45.50, Fees: 9.95},
			},
		},
	}

	holdings2 := []models.Holding{
		{
			Ticker:   "BHP",
			Exchange: "ASX",
			Trades: []*models.NavexaTrade{
				{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 45.50, Fees: 9.95},
				{Date: "2024-02-20", Type: "Buy", Units: 50, Price: 48.00, Fees: 9.95}, // new trade
			},
		},
	}

	hash1 := computeTradeHash(holdings1)
	hash2 := computeTradeHash(holdings2)

	if hash1 == hash2 {
		t.Error("expected different hashes when trades change")
	}
}

func TestComputeTradeHash_OrderIndependent(t *testing.T) {
	// Holdings in different order should produce the same hash
	// (because we sort by ticker internally)
	holdings1 := []models.Holding{
		{Ticker: "BHP", Exchange: "ASX", Trades: []*models.NavexaTrade{{Date: "2024-01-01", Type: "Buy", Units: 100, Price: 45}}},
		{Ticker: "CBA", Exchange: "ASX", Trades: []*models.NavexaTrade{{Date: "2024-01-01", Type: "Buy", Units: 50, Price: 95}}},
	}
	holdings2 := []models.Holding{
		{Ticker: "CBA", Exchange: "ASX", Trades: []*models.NavexaTrade{{Date: "2024-01-01", Type: "Buy", Units: 50, Price: 95}}},
		{Ticker: "BHP", Exchange: "ASX", Trades: []*models.NavexaTrade{{Date: "2024-01-01", Type: "Buy", Units: 100, Price: 45}}},
	}

	hash1 := computeTradeHash(holdings1)
	hash2 := computeTradeHash(holdings2)

	if hash1 != hash2 {
		t.Errorf("expected same hash regardless of holding order, got %s and %s", hash1, hash2)
	}
}
