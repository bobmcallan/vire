package cashflow

import (
	"math"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Devils-advocate stress tests for feedback batch fixes.
// Targets: Fix 1 (PortfolioValue instead of EquityValue), Fix 2 (NetCapitalDeployed != 0).

// --- Fix 1: CalculatePerformance uses PortfolioValue ---

func TestCalculatePerformance_UsesPortfolioValue_NotEquityValue(t *testing.T) {
	// Fix 1 core assertion: SimpleCapitalReturnPct must use PortfolioValue
	// (equity + cash), not EquityValue (equity only).
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:             "SMSF",
			EquityValue:      265000, // equity only
			GrossCashBalance: 206000, // cash balance
			PortfolioValue:   471000, // equity + cash
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      477000,
		Description: "Capital deployed",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)
	require.NotNil(t, perf)

	// Post-fix: CurrentValue should be PortfolioValue (471000), not EquityValue (265000)
	assert.Equal(t, 471000.0, perf.CurrentValue, "CurrentValue must use PortfolioValue")

	// SimpleCapitalReturnPct = (471000 - 477000) / 477000 * 100 = -1.26%
	// NOT (265000 - 477000) / 477000 * 100 = -44.4%
	assert.InDelta(t, -1.26, perf.SimpleCapitalReturnPct, 0.1,
		"SimpleCapitalReturnPct should be ~-1.26%%, not ~-44.4%%")
}

func TestCalculatePerformance_CurrentValueFieldRenamed(t *testing.T) {
	// Verify the CapitalPerformance struct uses CurrentValue, not EquityValue.
	// This is a compile-time check — if the field is wrong, this won't compile.
	perf := models.CapitalPerformance{
		CurrentValue: 42.0,
	}
	assert.Equal(t, 42.0, perf.CurrentValue)
}

func TestCalculatePerformance_PortfolioValueZero_EquityNonZero(t *testing.T) {
	// Edge case: PortfolioValue=0 (cash not loaded yet?), EquityValue != 0.
	// After fix, SimpleCapitalReturnPct should use PortfolioValue (0), giving -100%.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:             "SMSF",
			EquityValue:      100000,
			GrossCashBalance: 0,
			PortfolioValue:   0, // cash not loaded — stale/unset
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)

	// PortfolioValue=0, so CurrentValue=0, return = -100%
	assert.Equal(t, 0.0, perf.CurrentValue)
	assert.Equal(t, -100.0, perf.SimpleCapitalReturnPct)
}

// --- Fix 2: NetCapitalDeployed != 0 guard ---

func TestSimpleReturn_NegativeNetCapital_StillComputed(t *testing.T) {
	// Fix 2: After changing guard from > 0 to != 0, negative net capital
	// should produce a non-zero SimpleCapitalReturnPct.
	// HOWEVER: CalculatePerformance code itself has `if netCapital > 0`
	// which is different from the handler guard. Let's test the handler-level
	// behavior through the cashflow service too.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:           "SMSF",
			EquityValue:    50000,
			PortfolioValue: 50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Deposit 100k, then withdraw 150k → net capital = -50k
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -150000,
		Description: "Withdrawal exceeding deposits",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)

	// NetCapitalDeployed = 100000 - 150000 = -50000
	assert.Equal(t, -50000.0, perf.NetCapitalDeployed)

	// FINDING: The CalculatePerformance code has `if netCapital > 0` guard,
	// meaning negative netCapital gives SimpleCapitalReturnPct=0.
	// Fix 2 only changes the HANDLER guard (not the service). Verify this behavior.
	// The service guard stays > 0, so negative net capital → 0% return.
	assert.Equal(t, 0.0, perf.SimpleCapitalReturnPct,
		"Service-level guard is > 0; negative net capital gives 0%% return in service")
}

func TestDeriveFromTrades_UsesPortfolioValue(t *testing.T) {
	// FINDING: deriveFromTrades at line 582 may still use portfolio.EquityValue
	// instead of portfolio.PortfolioValue. This is the same bug as Fix 1 but
	// in the trade-derived fallback path.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:             "SMSF",
			EquityValue:      265000, // equity only
			PortfolioValue:   471000, // equity + cash
			GrossCashBalance: 206000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP.AU",
					Trades: []*models.NavexaTrade{
						{
							Type:  "buy",
							Date:  "2024-01-15",
							Units: 100,
							Price: 47.70,
							Fees:  10,
						},
					},
				},
			},
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Empty ledger → triggers deriveFromTrades fallback
	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)
	require.NotNil(t, perf)

	// After Fix 1: deriveFromTrades should also use PortfolioValue (471000),
	// not EquityValue (265000)
	assert.Equal(t, 471000.0, perf.CurrentValue,
		"deriveFromTrades must use PortfolioValue, not EquityValue")
}

func TestDeriveFromTrades_NegativeNetCapital_Guard(t *testing.T) {
	// In deriveFromTrades, the guard `if netCapital > 0` prevents computation
	// of SimpleCapitalReturnPct when net capital is negative (more sells than buys).
	// Verify this behavior explicitly.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:           "SMSF",
			EquityValue:    50000,
			PortfolioValue: 50000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP.AU",
					Trades: []*models.NavexaTrade{
						{Type: "sell", Date: "2024-01-15", Units: 200, Price: 50, Fees: 10},
					},
				},
			},
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)
	require.NotNil(t, perf)

	// All sells, no buys: totalDeposited=0, totalWithdrawn=9990, net=-9990
	assert.Less(t, perf.NetCapitalDeployed, 0.0, "all sells should give negative net capital")
	assert.Equal(t, 0.0, perf.SimpleCapitalReturnPct,
		"negative net capital in deriveFromTrades should give 0%% return")
}

// --- Division by zero edge cases ---

func TestCalculatePerformance_ExactlyZeroNetCapital(t *testing.T) {
	// Deposits exactly equal withdrawals → netCapital = 0
	// Guard prevents division by zero.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:           "SMSF",
			EquityValue:    50000,
			PortfolioValue: 50000,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount: 100000, Description: "Deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount: -100000, Description: "Full withdrawal",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)

	assert.Equal(t, 0.0, perf.NetCapitalDeployed)
	assert.Equal(t, 0.0, perf.SimpleCapitalReturnPct, "zero net capital must not divide by zero")
	assert.False(t, math.IsNaN(perf.SimpleCapitalReturnPct), "must not be NaN")
	assert.False(t, math.IsInf(perf.SimpleCapitalReturnPct, 0), "must not be Inf")
}

func TestDeriveFromTrades_SplitTradeType_Ignored(t *testing.T) {
	// NavexaTrade.Type can be "split", "dividend", "corporate action" — not just buy/sell.
	// deriveFromTrades should silently skip unknown types.
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:           "SMSF",
			EquityValue:    50000,
			PortfolioValue: 50000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP.AU",
					Trades: []*models.NavexaTrade{
						{Type: "split", Date: "2024-03-15", Units: 100, Price: 0, Fees: 0},
						{Type: "dividend", Date: "2024-06-15", Units: 0, Price: 5.00, Fees: 0},
						{Type: "corporate action", Date: "2024-09-15", Units: 50, Price: 0, Fees: 0},
						{Type: "BUY", Date: "2024-01-15", Units: 100, Price: 47.70, Fees: 10}, // case sensitive?
					},
				},
			},
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)
	require.NotNil(t, perf)

	// "split", "dividend", "corporate action" should be ignored.
	// "BUY" (uppercase) should match "buy" via strings.ToLower.
	assert.Greater(t, perf.TransactionCount, 0, "BUY trade should be counted")
}

func TestDeriveFromTrades_EmptyHoldings(t *testing.T) {
	// No holdings at all — deriveFromTrades should return nil, nil
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:     "SMSF",
			Holdings: nil,
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)
	require.NotNil(t, perf) // returns empty CapitalPerformance, not nil
	assert.Equal(t, 0, perf.TransactionCount)
}

func TestDeriveFromTrades_NilTradesArray(t *testing.T) {
	// Holding exists but Trades is nil
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name: "SMSF",
			Holdings: []models.Holding{
				{Ticker: "BHP.AU", Trades: nil},
			},
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)
	require.NotNil(t, perf)
	assert.Equal(t, 0, perf.TransactionCount)
}

func TestDeriveFromTrades_MalformedDates(t *testing.T) {
	// Bad date strings in NavexaTrade should be skipped, not crash
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:           "SMSF",
			EquityValue:    50000,
			PortfolioValue: 50000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP.AU",
					Trades: []*models.NavexaTrade{
						{Type: "buy", Date: "", Units: 100, Price: 50, Fees: 0},
						{Type: "buy", Date: "not-a-date", Units: 100, Price: 50, Fees: 0},
						{Type: "buy", Date: "2024/01/15", Units: 100, Price: 50, Fees: 0}, // wrong format
						{Type: "buy", Date: "2024-01-15", Units: 100, Price: 50, Fees: 0}, // valid
					},
				},
			},
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)
	require.NotNil(t, perf)

	// Only the last trade (valid date) should be counted
	assert.Equal(t, 1, perf.TransactionCount,
		"only trades with valid dates should count")
}

func TestDeriveFromTrades_SellProceeds_NegativeFees(t *testing.T) {
	// Edge case: fees > units*price → proceeds would go negative.
	// The code clamps to 0: `if proceeds < 0 { proceeds = 0 }`
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:           "SMSF",
			EquityValue:    50000,
			PortfolioValue: 50000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP.AU",
					Trades: []*models.NavexaTrade{
						{Type: "sell", Date: "2024-01-15", Units: 1, Price: 5, Fees: 100}, // fees > value
					},
				},
			},
		},
	}
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	require.NoError(t, err)
	require.NotNil(t, perf)

	// Proceeds clamped to 0, so NetCapitalDeployed = 0 - 0 = 0
	assert.Equal(t, 0.0, perf.NetCapitalDeployed,
		"sell with fees > value should have 0 proceeds")
	assert.False(t, math.IsNaN(perf.SimpleCapitalReturnPct))
}
