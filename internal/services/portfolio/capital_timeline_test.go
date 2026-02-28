package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Feature 1: Capital Allocation Timeline in GetDailyGrowth ---

func TestGetDailyGrowth_CashFlowTimeline(t *testing.T) {
	// Setup: one holding with trades, plus cash transactions
	logger := common.NewLogger("error")

	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -10)
	day5 := now.AddDate(0, 0, -6)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				EOD:    generateEODBars(day1, 11, 50.0), // 11 days of data at $50
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	// Save a portfolio with one holding
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{
				Ticker:   "BHP",
				Exchange: "AU",
				Units:    100,
				Trades: []*models.NavexaTrade{
					{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0},
				},
			},
		},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	svc := NewService(storage, nil, nil, nil, logger)

	// Cash transactions: deposit on day1, withdrawal on day5
	transactions := []models.CashTransaction{
		{
			ID:       "tx1",
			Account:  "Trading",
			Category: models.CashCatContribution,
			Date:     day1,
			Amount:   10000,
		},
		{
			ID:       "tx2",
			Account:  "Trading",
			Category: models.CashCatOther,
			Date:     day5,
			Amount:   -2000,
		},
	}

	opts := interfaces.GrowthOptions{
		From:         day1,
		To:           now.AddDate(0, 0, -1),
		Transactions: transactions,
	}

	points, err := svc.GetDailyGrowth(context.Background(), "test", opts)
	if err != nil {
		t.Fatalf("GetDailyGrowth error: %v", err)
	}

	if len(points) == 0 {
		t.Fatal("expected growth points, got none")
	}

	// Before withdrawal (first point): deposit +10000, buy -5000 → cash_balance = 5000
	first := points[0]
	if first.CashBalance != 5000 { // 10000 deposit - 5000 buy (100*50)
		t.Errorf("first point CashBalance = %.2f, want 5000 (10000 deposit - 5000 buy)", first.CashBalance)
	}
	if first.NetDeployed != 10000 {
		t.Errorf("first point NetDeployed = %.2f, want 10000", first.NetDeployed)
	}

	// After withdrawal: find a point after day5
	var afterWithdrawal *models.GrowthDataPoint
	for i := range points {
		if !points[i].Date.Before(day5) {
			afterWithdrawal = &points[i]
			break
		}
	}
	if afterWithdrawal == nil {
		t.Fatal("expected point after withdrawal date")
	}

	if afterWithdrawal.CashBalance != 3000 { // 5000 - 2000 withdrawal
		t.Errorf("after withdrawal CashBalance = %.2f, want 3000", afterWithdrawal.CashBalance)
	}
	if afterWithdrawal.NetDeployed != 8000 { // 10000 - 2000
		t.Errorf("after withdrawal NetDeployed = %.2f, want 8000", afterWithdrawal.NetDeployed)
	}
}

func TestGetDailyGrowth_NoTransactions(t *testing.T) {
	// Verify that when no transactions are provided, cash fields are zero
	logger := common.NewLogger("error")

	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -5)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				EOD:    generateEODBars(day1, 6, 50.0),
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{
				Ticker:   "BHP",
				Exchange: "AU",
				Units:    100,
				Trades: []*models.NavexaTrade{
					{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0},
				},
			},
		},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	svc := NewService(storage, nil, nil, nil, logger)

	opts := interfaces.GrowthOptions{
		From: day1,
		To:   now.AddDate(0, 0, -1),
	}

	points, err := svc.GetDailyGrowth(context.Background(), "test", opts)
	if err != nil {
		t.Fatalf("GetDailyGrowth error: %v", err)
	}

	// No cash transactions, but buy trade on day1 costs $5000 (100*50)
	// → cash_balance = -5000 (all days after the buy), net_deployed = 0
	for i, p := range points {
		if p.CashBalance != -5000 {
			t.Errorf("points[%d].CashBalance = %.2f, want -5000 (buy trade consumes cash)", i, p.CashBalance)
		}
		if p.NetDeployed != 0 {
			t.Errorf("points[%d].NetDeployed = %.2f, want 0", i, p.NetDeployed)
		}
	}
}

func TestGetDailyGrowth_DividendInflowIncreasesCashBalance(t *testing.T) {
	logger := common.NewLogger("error")

	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -5)
	day3 := now.AddDate(0, 0, -3)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				EOD:    generateEODBars(day1, 6, 50.0),
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{
				Ticker:   "BHP",
				Exchange: "AU",
				Units:    100,
				Trades: []*models.NavexaTrade{
					{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0},
				},
			},
		},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	svc := NewService(storage, nil, nil, nil, logger)

	// Dividend is an inflow, but should NOT affect net_deployed
	transactions := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 5000},
		{Account: "Trading", Category: models.CashCatDividend, Date: day3, Amount: 200},
	}

	opts := interfaces.GrowthOptions{
		From:         day1,
		To:           now.AddDate(0, 0, -1),
		Transactions: transactions,
	}

	points, err := svc.GetDailyGrowth(context.Background(), "test", opts)
	if err != nil {
		t.Fatalf("GetDailyGrowth error: %v", err)
	}

	// Find point on/after day3
	var afterDiv *models.GrowthDataPoint
	for i := range points {
		if !points[i].Date.Before(day3) {
			afterDiv = &points[i]
			break
		}
	}
	if afterDiv == nil {
		t.Fatal("expected point after dividend date")
	}

	// CashBalance: deposit +5000, buy -5000, dividend +200 = 200
	if afterDiv.CashBalance != 200 {
		t.Errorf("after dividend CashBalance = %.2f, want 200 (5000 - 5000 buy + 200 div)", afterDiv.CashBalance)
	}
	// NetDeployed should NOT include dividend (only deposit: 5000)
	if afterDiv.NetDeployed != 5000 {
		t.Errorf("after dividend NetDeployed = %.2f, want 5000", afterDiv.NetDeployed)
	}
}

func TestGetDailyGrowth_InternalTransfersAffectCash(t *testing.T) {
	// Transfers affect the running cash balance and net deployed.
	logger := common.NewLogger("error")

	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -10)
	day3 := now.AddDate(0, 0, -8)
	day5 := now.AddDate(0, 0, -6)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				EOD:    generateEODBars(day1, 11, 50.0),
			},
		},
	}

	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}

	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{
				Ticker:   "BHP",
				Exchange: "AU",
				Units:    100,
				Trades: []*models.NavexaTrade{
					{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0},
				},
			},
		},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	svc := NewService(storage, nil, nil, nil, logger)

	transactions := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 50000},
		{Account: "Trading", Category: models.CashCatTransfer, Date: day3, Amount: -20000},
		{Account: "Trading", Category: models.CashCatOther, Date: day5, Amount: -5000},
	}

	opts := interfaces.GrowthOptions{
		From:         day1,
		To:           now.AddDate(0, 0, -1),
		Transactions: transactions,
	}

	points, err := svc.GetDailyGrowth(context.Background(), "test", opts)
	if err != nil {
		t.Fatalf("GetDailyGrowth error: %v", err)
	}

	if len(points) == 0 {
		t.Fatal("expected growth points, got none")
	}

	// First point (day1): deposit +50000, buy -5000 (100*50) = 45000
	first := points[0]
	if first.CashBalance != 45000 {
		t.Errorf("first point CashBalance = %.2f, want 45000 (50000 deposit - 5000 buy)", first.CashBalance)
	}
	if first.NetDeployed != 50000 {
		t.Errorf("first point NetDeployed = %.2f, want 50000", first.NetDeployed)
	}

	// After day3 (transfer debit): cash = 45000 - 20000 = 25000
	var afterTransfer *models.GrowthDataPoint
	for i := range points {
		if !points[i].Date.Before(day3) {
			afterTransfer = &points[i]
			break
		}
	}
	if afterTransfer == nil {
		t.Fatal("expected point after internal transfer date")
	}
	if afterTransfer.CashBalance != 25000 {
		t.Errorf("after internal transfer CashBalance = %.2f, want 25000 (45000 - 20000 transfer debit)", afterTransfer.CashBalance)
	}
	if afterTransfer.NetDeployed != 30000 {
		t.Errorf("after internal transfer NetDeployed = %.2f, want 30000 (50000 - 20000 transfer debit)", afterTransfer.NetDeployed)
	}

	// After day5 (real withdrawal): cash = 25000 - 5000 = 20000
	var afterWithdrawal *models.GrowthDataPoint
	for i := range points {
		if !points[i].Date.Before(day5) {
			afterWithdrawal = &points[i]
			break
		}
	}
	if afterWithdrawal == nil {
		t.Fatal("expected point after withdrawal date")
	}
	if afterWithdrawal.CashBalance != 20000 {
		t.Errorf("after withdrawal CashBalance = %.2f, want 20000 (25000 - 5000)", afterWithdrawal.CashBalance)
	}
	if afterWithdrawal.NetDeployed != 25000 {
		t.Errorf("after withdrawal NetDeployed = %.2f, want 25000 (30000 - 5000)", afterWithdrawal.NetDeployed)
	}
}

// --- Feature 1: growthPointsToTimeSeries capital fields ---

func TestGrowthPointsToTimeSeries_CapitalTimelineFields(t *testing.T) {
	points := []models.GrowthDataPoint{
		{
			Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			TotalValue:  100000,
			TotalCost:   90000,
			CashBalance: 15000,
			NetDeployed: 50000,
		},
	}
	externalBalance := 25000.0

	ts := growthPointsToTimeSeries(points, externalBalance)

	if len(ts) != 1 {
		t.Fatalf("expected 1 point, got %d", len(ts))
	}

	pt := ts[0]

	// Value = TotalValue + externalBalance = 125000
	if pt.Value != 125000 {
		t.Errorf("Value = %.0f, want 125000", pt.Value)
	}

	// CashBalance passed through
	if pt.CashBalance != 15000 {
		t.Errorf("CashBalance = %.0f, want 15000", pt.CashBalance)
	}

	// ExternalBalance = externalBalanceTotal
	if pt.ExternalBalance != 25000 {
		t.Errorf("ExternalBalance = %.0f, want 25000", pt.ExternalBalance)
	}

	// TotalCapital = Value + CashBalance = 125000 + 15000 = 140000
	if pt.TotalCapital != 140000 {
		t.Errorf("TotalCapital = %.0f, want 140000", pt.TotalCapital)
	}

	// NetDeployed passed through
	if pt.NetDeployed != 50000 {
		t.Errorf("NetDeployed = %.0f, want 50000", pt.NetDeployed)
	}
}

func TestGrowthPointsToTimeSeries_ZeroCashTimelineFields(t *testing.T) {
	// When no cash data, fields should be zero (omitempty on JSON)
	points := []models.GrowthDataPoint{
		{
			Date:       time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			TotalValue: 100000,
			TotalCost:  90000,
		},
	}

	ts := growthPointsToTimeSeries(points, 0)

	pt := ts[0]
	if pt.CashBalance != 0 {
		t.Errorf("CashBalance = %.0f, want 0", pt.CashBalance)
	}
	if pt.ExternalBalance != 0 {
		t.Errorf("ExternalBalance = %.0f, want 0", pt.ExternalBalance)
	}
	// TotalCapital = Value (100000) + CashBalance (0) = 100000
	if pt.TotalCapital != 100000 {
		t.Errorf("TotalCapital = %.0f, want 100000", pt.TotalCapital)
	}
	if pt.NetDeployed != 0 {
		t.Errorf("NetDeployed = %.0f, want 0", pt.NetDeployed)
	}
}

// --- Feature 2: Net Flow on Daily/Weekly Change ---

func TestPopulateNetFlows_WithTransactions(t *testing.T) {
	logger := common.NewLogger("error")

	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	threeDaysAgo := now.AddDate(0, 0, -3)
	tenDaysAgo := now.AddDate(0, 0, -10)

	cashSvc := &stubCashFlowService{
		ledger: &models.CashFlowLedger{
			PortfolioName: "test",
			Transactions: []models.CashTransaction{
				{Account: "Trading", Category: models.CashCatOther, Date: tenDaysAgo, Amount: -5000},         // outside last week
				{Account: "Trading", Category: models.CashCatContribution, Date: threeDaysAgo, Amount: 2000}, // in last week
				{Account: "Trading", Category: models.CashCatOther, Date: yesterday, Amount: -500},           // yesterday withdrawal
				{Account: "Trading", Category: models.CashCatContribution, Date: yesterday, Amount: 1000},    // yesterday deposit
			},
		},
	}

	svc := &Service{cashflowSvc: cashSvc, logger: logger}

	p := &models.Portfolio{Name: "test"}
	svc.populateNetFlows(context.Background(), p)

	// Yesterday: -500 withdrawal + 1000 contribution = +500
	if p.YesterdayNetFlow != 500 {
		t.Errorf("YesterdayNetFlow = %.2f, want 500", p.YesterdayNetFlow)
	}

	// Last week (last 7 days, exclusive of today):
	// threeDaysAgo: +2000 deposit
	// yesterday: -500 withdrawal + 1000 contribution = +500
	// total: +2500
	if p.LastWeekNetFlow != 2500 {
		t.Errorf("LastWeekNetFlow = %.2f, want 2500", p.LastWeekNetFlow)
	}
}

func TestPopulateNetFlows_NilCashFlowService(t *testing.T) {
	logger := common.NewLogger("error")

	svc := &Service{cashflowSvc: nil, logger: logger}
	p := &models.Portfolio{Name: "test"}
	svc.populateNetFlows(context.Background(), p)

	if p.YesterdayNetFlow != 0 {
		t.Errorf("YesterdayNetFlow = %.2f, want 0 (nil cashflow svc)", p.YesterdayNetFlow)
	}
	if p.LastWeekNetFlow != 0 {
		t.Errorf("LastWeekNetFlow = %.2f, want 0 (nil cashflow svc)", p.LastWeekNetFlow)
	}
}

func TestPopulateNetFlows_EmptyLedger(t *testing.T) {
	logger := common.NewLogger("error")

	cashSvc := &stubCashFlowService{
		ledger: &models.CashFlowLedger{
			PortfolioName: "test",
			Transactions:  nil,
		},
	}

	svc := &Service{cashflowSvc: cashSvc, logger: logger}
	p := &models.Portfolio{Name: "test"}
	svc.populateNetFlows(context.Background(), p)

	if p.YesterdayNetFlow != 0 {
		t.Errorf("YesterdayNetFlow = %.2f, want 0 (empty ledger)", p.YesterdayNetFlow)
	}
	if p.LastWeekNetFlow != 0 {
		t.Errorf("LastWeekNetFlow = %.2f, want 0 (empty ledger)", p.LastWeekNetFlow)
	}
}

func TestPopulateNetFlows_LedgerError(t *testing.T) {
	logger := common.NewLogger("error")

	cashSvc := &stubCashFlowService{err: errStubNotFound}

	svc := &Service{cashflowSvc: cashSvc, logger: logger}
	p := &models.Portfolio{Name: "test"}
	svc.populateNetFlows(context.Background(), p)

	// Should gracefully handle error
	if p.YesterdayNetFlow != 0 {
		t.Errorf("YesterdayNetFlow = %.2f, want 0 (ledger error)", p.YesterdayNetFlow)
	}
}

func TestSetCashFlowService(t *testing.T) {
	logger := common.NewLogger("error")
	svc := NewService(nil, nil, nil, nil, logger)

	if svc.cashflowSvc != nil {
		t.Error("expected nil cashflowSvc before set")
	}

	cashSvc := &stubCashFlowService{}
	svc.SetCashFlowService(cashSvc)

	if svc.cashflowSvc == nil {
		t.Error("expected non-nil cashflowSvc after set")
	}
}

// --- Helpers ---

var errStubNotFound = &stubError{"not found"}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }

// stubCashFlowService implements interfaces.CashFlowService for tests.
type stubCashFlowService struct {
	ledger *models.CashFlowLedger
	err    error
}

func (s *stubCashFlowService) GetLedger(_ context.Context, _ string) (*models.CashFlowLedger, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ledger, nil
}

func (s *stubCashFlowService) AddTransaction(_ context.Context, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return s.ledger, nil
}

func (s *stubCashFlowService) AddTransfer(_ context.Context, _ string, _, _ string, _ float64, _ time.Time, _ string) (*models.CashFlowLedger, error) {
	return s.ledger, nil
}

func (s *stubCashFlowService) UpdateTransaction(_ context.Context, _, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return s.ledger, nil
}

func (s *stubCashFlowService) RemoveTransaction(_ context.Context, _, _ string) (*models.CashFlowLedger, error) {
	return s.ledger, nil
}

func (s *stubCashFlowService) SetTransactions(_ context.Context, _ string, _ []models.CashTransaction, _ string) (*models.CashFlowLedger, error) {
	return s.ledger, nil
}

func (s *stubCashFlowService) UpdateAccount(_ context.Context, _ string, _ string, _ models.CashAccountUpdate) (*models.CashFlowLedger, error) {
	return s.ledger, nil
}

func (s *stubCashFlowService) CalculatePerformance(_ context.Context, _ string) (*models.CapitalPerformance, error) {
	return nil, nil
}

// generateEODBars creates N daily EOD bars starting from startDate, newest first.
func generateEODBars(startDate time.Time, count int, price float64) []models.EODBar {
	bars := make([]models.EODBar, count)
	for i := 0; i < count; i++ {
		bars[i] = models.EODBar{
			Date:     startDate.AddDate(0, 0, count-1-i),
			Close:    price,
			AdjClose: price,
			Open:     price,
			High:     price,
			Low:      price,
		}
	}
	return bars
}
