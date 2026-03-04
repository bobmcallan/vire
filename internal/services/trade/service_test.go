package trade

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
)

// --- Mock storage ---

type mockUserDataStore struct {
	mu      sync.RWMutex
	records map[string]*models.UserRecord
}

func newMockUserDataStore() *mockUserDataStore {
	return &mockUserDataStore{records: make(map[string]*models.UserRecord)}
}

func (m *mockUserDataStore) key(userID, subject, key string) string {
	return userID + ":" + subject + ":" + key
}

func (m *mockUserDataStore) Get(_ context.Context, userID, subject, key string) (*models.UserRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.records[m.key(userID, subject, key)]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return rec, nil
}

func (m *mockUserDataStore) Put(_ context.Context, rec *models.UserRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[m.key(rec.UserID, rec.Subject, rec.Key)] = rec
	return nil
}

func (m *mockUserDataStore) Delete(_ context.Context, userID, subject, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, m.key(userID, subject, key))
	return nil
}

func (m *mockUserDataStore) List(_ context.Context, _, _ string) ([]*models.UserRecord, error) {
	return nil, nil
}

func (m *mockUserDataStore) Query(_ context.Context, _, _ string, _ interfaces.QueryOptions) ([]*models.UserRecord, error) {
	return nil, nil
}

func (m *mockUserDataStore) DeleteBySubject(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockUserDataStore) Close() error { return nil }

// --- Mock storage manager ---

type mockStorageManager struct {
	userDataStore *mockUserDataStore
}

func newMockStorageManager() *mockStorageManager {
	return &mockStorageManager{userDataStore: newMockUserDataStore()}
}

func (m *mockStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *mockStorageManager) UserDataStore() interfaces.UserDataStore         { return m.userDataStore }
func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return nil }
func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (m *mockStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (m *mockStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *mockStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (m *mockStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (m *mockStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
func (m *mockStorageManager) TimelineStore() interfaces.TimelineStore         { return nil }
func (m *mockStorageManager) DataPath() string                                { return "" }
func (m *mockStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (m *mockStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *mockStorageManager) Close() error                                { return nil }

// --- Test helpers ---

func testContext() context.Context {
	ctx := context.Background()
	uc := &common.UserContext{UserID: "test-user"}
	return common.WithUserContext(ctx, uc)
}

func testService() *Service {
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	return NewService(storage, logger)
}

func buyTrade(ticker string, units, price, fees float64, date time.Time) models.Trade {
	return models.Trade{
		Ticker: ticker,
		Action: models.TradeActionBuy,
		Units:  units,
		Price:  price,
		Fees:   fees,
		Date:   date,
	}
}

func sellTrade(ticker string, units, price, fees float64, date time.Time) models.Trade {
	return models.Trade{
		Ticker: ticker,
		Action: models.TradeActionSell,
		Units:  units,
		Price:  price,
		Fees:   fees,
		Date:   date,
	}
}

// --- Tests ---

func TestGenerateTradeID(t *testing.T) {
	id := generateTradeID()
	if !strings.HasPrefix(id, "tr_") {
		t.Errorf("ID should start with 'tr_', got %q", id)
	}
	if len(id) != 11 {
		t.Errorf("ID should be 11 chars (tr_ + 8 hex), got %d: %q", len(id), id)
	}

	// Uniqueness
	id2 := generateTradeID()
	if id == id2 {
		t.Errorf("IDs should be unique, got %q twice", id)
	}
}

func TestAddTrade_Buy(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := buyTrade("BHP.AU", 100, 45.00, 9.95, time.Now().Add(-24*time.Hour))
	created, holding, err := svc.AddTrade(ctx, "SMSF", trade)
	if err != nil {
		t.Fatalf("AddTrade failed: %v", err)
	}
	if created == nil {
		t.Fatal("expected created trade, got nil")
	}
	if !strings.HasPrefix(created.ID, "tr_") {
		t.Errorf("expected tr_ prefix, got %q", created.ID)
	}
	if created.PortfolioName != "SMSF" {
		t.Errorf("expected PortfolioName=SMSF, got %q", created.PortfolioName)
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected CreatedAt set")
	}
	if holding == nil {
		t.Fatal("expected derived holding, got nil")
	}
	if holding.Ticker != "BHP.AU" {
		t.Errorf("expected ticker=BHP.AU, got %q", holding.Ticker)
	}
	if holding.Units != 100 {
		t.Errorf("expected units=100, got %f", holding.Units)
	}
	expectedCost := 100*45.00 + 9.95 // 4509.95
	if holding.CostBasis != expectedCost {
		t.Errorf("expected cost_basis=%f, got %f", expectedCost, holding.CostBasis)
	}
}

func TestAddTrade_Sell(t *testing.T) {
	svc := testService()
	ctx := testContext()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Buy 100 @ $40 + $10 fee → cost = $4010
	_, _, err := svc.AddTrade(ctx, "SMSF", buyTrade("BHP.AU", 100, 40.00, 10.00, base))
	if err != nil {
		t.Fatalf("buy failed: %v", err)
	}
	// Sell 50 @ $50 - $10 fee → proceeds = $2490
	// avg cost at sell = 4010/100 = 40.10
	// cost of sold = 40.10 * 50 = 2005
	// realized = 2490 - 2005 = 485
	_, holding, err := svc.AddTrade(ctx, "SMSF", sellTrade("BHP.AU", 50, 50.00, 10.00, base.Add(24*time.Hour)))
	if err != nil {
		t.Fatalf("sell failed: %v", err)
	}
	if holding.Units != 50 {
		t.Errorf("expected 50 units remaining, got %f", holding.Units)
	}
	if holding.TradeCount != 2 {
		t.Errorf("expected 2 trades, got %d", holding.TradeCount)
	}
	// Realized return should be positive
	if holding.RealizedReturn <= 0 {
		t.Errorf("expected positive realized return, got %f", holding.RealizedReturn)
	}
}

func TestAddTrade_SellValidation(t *testing.T) {
	svc := testService()
	ctx := testContext()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, _, err := svc.AddTrade(ctx, "SMSF", buyTrade("BHP.AU", 100, 40.00, 10.00, base))
	if err != nil {
		t.Fatalf("buy failed: %v", err)
	}

	// Attempt to sell more than held
	_, _, err = svc.AddTrade(ctx, "SMSF", sellTrade("BHP.AU", 200, 50.00, 10.00, base.Add(24*time.Hour)))
	if err == nil {
		t.Fatal("expected error selling more than held")
	}
	if !strings.Contains(err.Error(), "insufficient") {
		t.Errorf("expected 'insufficient' in error, got: %v", err)
	}
}

func TestAddTrade_ValidationErrors(t *testing.T) {
	svc := testService()
	ctx := testContext()
	date := time.Now().Add(-time.Hour)

	tests := []struct {
		name    string
		trade   models.Trade
		wantErr string
	}{
		{
			name:    "empty ticker",
			trade:   models.Trade{Action: models.TradeActionBuy, Units: 1, Price: 10, Date: date},
			wantErr: "ticker",
		},
		{
			name:    "zero units",
			trade:   models.Trade{Ticker: "BHP.AU", Action: models.TradeActionBuy, Units: 0, Price: 10, Date: date},
			wantErr: "units",
		},
		{
			name:    "negative units",
			trade:   models.Trade{Ticker: "BHP.AU", Action: models.TradeActionBuy, Units: -5, Price: 10, Date: date},
			wantErr: "units",
		},
		{
			name:    "invalid action",
			trade:   models.Trade{Ticker: "BHP.AU", Action: "hold", Units: 1, Price: 10, Date: date},
			wantErr: "action",
		},
		{
			name:    "zero date",
			trade:   models.Trade{Ticker: "BHP.AU", Action: models.TradeActionBuy, Units: 1, Price: 10},
			wantErr: "date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := svc.AddTrade(ctx, "SMSF", tt.trade)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
				t.Errorf("expected %q in error, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestRemoveTrade(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := buyTrade("BHP.AU", 100, 45.00, 9.95, time.Now().Add(-time.Hour))
	created, _, err := svc.AddTrade(ctx, "SMSF", trade)
	if err != nil {
		t.Fatalf("AddTrade failed: %v", err)
	}

	tb, err := svc.RemoveTrade(ctx, "SMSF", created.ID)
	if err != nil {
		t.Fatalf("RemoveTrade failed: %v", err)
	}
	if len(tb.Trades) != 0 {
		t.Errorf("expected 0 trades, got %d", len(tb.Trades))
	}
}

func TestRemoveTrade_NotFound(t *testing.T) {
	svc := testService()
	ctx := testContext()

	_, err := svc.RemoveTrade(ctx, "SMSF", "tr_nonexist")
	if err == nil {
		t.Fatal("expected error for nonexistent trade ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestUpdateTrade(t *testing.T) {
	svc := testService()
	ctx := testContext()

	date := time.Now().Add(-time.Hour)
	trade := buyTrade("BHP.AU", 100, 45.00, 9.95, date)
	created, _, err := svc.AddTrade(ctx, "SMSF", trade)
	if err != nil {
		t.Fatalf("AddTrade failed: %v", err)
	}

	// Update price
	update := models.Trade{Price: 50.00}
	updated, err := svc.UpdateTrade(ctx, "SMSF", created.ID, update)
	if err != nil {
		t.Fatalf("UpdateTrade failed: %v", err)
	}
	if updated.Price != 50.00 {
		t.Errorf("expected price=50, got %f", updated.Price)
	}
	// Other fields unchanged
	if updated.Units != 100 {
		t.Errorf("expected units=100 unchanged, got %f", updated.Units)
	}
	if updated.Ticker != "BHP.AU" {
		t.Errorf("expected ticker=BHP.AU unchanged, got %q", updated.Ticker)
	}
}

func TestUpdateTrade_NotFound(t *testing.T) {
	svc := testService()
	ctx := testContext()

	update := models.Trade{Price: 50.00}
	_, err := svc.UpdateTrade(ctx, "SMSF", "tr_nonexist", update)
	if err == nil {
		t.Fatal("expected error for nonexistent trade ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestListTrades(t *testing.T) {
	svc := testService()
	ctx := testContext()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	trades := []models.Trade{
		buyTrade("BHP.AU", 100, 40, 10, base),
		buyTrade("CBA.AU", 50, 100, 10, base.Add(24*time.Hour)),
		sellTrade("BHP.AU", 50, 50, 10, base.Add(48*time.Hour)),
	}
	for _, tr := range trades {
		if _, _, err := svc.AddTrade(ctx, "SMSF", tr); err != nil {
			t.Fatalf("AddTrade failed: %v", err)
		}
	}

	// List all
	all, total, err := svc.ListTrades(ctx, "SMSF", TradeFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListTrades failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total=3, got %d", total)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 trades, got %d", len(all))
	}

	// Filter by ticker
	bhp, total, err := svc.ListTrades(ctx, "SMSF", TradeFilter{Ticker: "BHP.AU", Limit: 50})
	if err != nil {
		t.Fatalf("ListTrades with ticker filter failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 BHP trades, got %d", total)
	}
	if len(bhp) != 2 {
		t.Errorf("expected 2 BHP trades in result, got %d", len(bhp))
	}

	// Filter by action
	sells, total, err := svc.ListTrades(ctx, "SMSF", TradeFilter{Action: models.TradeActionSell, Limit: 50})
	if err != nil {
		t.Fatalf("ListTrades with action filter failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 sell trade, got %d", total)
	}
	_ = sells
}

func TestListTrades_Pagination(t *testing.T) {
	svc := testService()
	ctx := testContext()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		tr := buyTrade("BHP.AU", 10, 40, 10, base.Add(time.Duration(i)*24*time.Hour))
		if _, _, err := svc.AddTrade(ctx, "SMSF", tr); err != nil {
			t.Fatalf("AddTrade failed: %v", err)
		}
	}

	// Page 1: limit=2, offset=0
	page1, total, err := svc.ListTrades(ctx, "SMSF", TradeFilter{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("ListTrades page 1 failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
	if len(page1) != 2 {
		t.Errorf("expected 2 trades on page 1, got %d", len(page1))
	}

	// Page 2: limit=2, offset=2
	page2, _, err := svc.ListTrades(ctx, "SMSF", TradeFilter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListTrades page 2 failed: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("expected 2 trades on page 2, got %d", len(page2))
	}
}

func TestDeriveHolding_SingleBuy(t *testing.T) {
	trades := []models.Trade{
		{Action: models.TradeActionBuy, Units: 100, Price: 40.00, Fees: 10.00, Date: time.Now()},
	}
	h := DeriveHolding(trades, 0)
	if h.Units != 100 {
		t.Errorf("expected 100 units, got %f", h.Units)
	}
	expectedCost := 100*40.0 + 10.0 // 4010
	if h.CostBasis != expectedCost {
		t.Errorf("expected cost_basis=%f, got %f", expectedCost, h.CostBasis)
	}
	expectedAvg := expectedCost / 100.0 // 40.10
	if h.AvgCost != expectedAvg {
		t.Errorf("expected avg_cost=%f, got %f", expectedAvg, h.AvgCost)
	}
	if h.GrossInvested != expectedCost {
		t.Errorf("expected gross_invested=%f, got %f", expectedCost, h.GrossInvested)
	}
	if h.RealizedReturn != 0 {
		t.Errorf("expected 0 realized, got %f", h.RealizedReturn)
	}
	if h.TradeCount != 1 {
		t.Errorf("expected trade_count=1, got %d", h.TradeCount)
	}
}

func TestDeriveHolding_MultipleBuys(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	trades := []models.Trade{
		{Action: models.TradeActionBuy, Units: 100, Price: 40.00, Fees: 10.00, Date: base},
		{Action: models.TradeActionBuy, Units: 50, Price: 60.00, Fees: 10.00, Date: base.Add(24 * time.Hour)},
	}
	h := DeriveHolding(trades, 0)
	if h.Units != 150 {
		t.Errorf("expected 150 units, got %f", h.Units)
	}
	// cost1 = 100*40 + 10 = 4010
	// cost2 = 50*60 + 10 = 3010
	// total = 7020
	expectedCost := 4010.0 + 3010.0
	if h.CostBasis != expectedCost {
		t.Errorf("expected cost_basis=%f, got %f", expectedCost, h.CostBasis)
	}
	expectedAvg := expectedCost / 150
	if h.AvgCost != expectedAvg {
		t.Errorf("expected avg_cost=%f, got %f", expectedAvg, h.AvgCost)
	}
}

func TestDeriveHolding_BuyThenSell(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	trades := []models.Trade{
		{Action: models.TradeActionBuy, Units: 100, Price: 40.00, Fees: 10.00, Date: base},
		{Action: models.TradeActionSell, Units: 50, Price: 60.00, Fees: 10.00, Date: base.Add(24 * time.Hour)},
	}
	// Buy: cost = 4010, avg = 40.10
	// Sell 50: avg_cost_at_sell = 4010/100 = 40.10
	//           cost_of_sold = 40.10 * 50 = 2005
	//           proceeds = 50*60 - 10 = 2990
	//           realized = 2990 - 2005 = 985
	// Remaining: 50 units, cost = 4010 - 2005 = 2005
	h := DeriveHolding(trades, 0)
	if h.Units != 50 {
		t.Errorf("expected 50 units, got %f", h.Units)
	}
	expectedRealized := 2990.0 - 2005.0 // 985
	if h.RealizedReturn != expectedRealized {
		t.Errorf("expected realized=%f, got %f", expectedRealized, h.RealizedReturn)
	}
	expectedCostBasis := 4010.0 - 2005.0 // 2005
	if h.CostBasis != expectedCostBasis {
		t.Errorf("expected cost_basis=%f, got %f", expectedCostBasis, h.CostBasis)
	}
}

func TestDeriveHolding_FullSell(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	trades := []models.Trade{
		{Action: models.TradeActionBuy, Units: 100, Price: 40.00, Fees: 10.00, Date: base},
		{Action: models.TradeActionSell, Units: 100, Price: 50.00, Fees: 10.00, Date: base.Add(24 * time.Hour)},
	}
	// Sell all: avg_cost = 4010/100 = 40.10
	//           cost_of_sold = 40.10 * 100 = 4010
	//           proceeds = 100*50 - 10 = 4990
	//           realized = 4990 - 4010 = 980
	h := DeriveHolding(trades, 0)
	if h.Units != 0 {
		t.Errorf("expected 0 units after full sell, got %f", h.Units)
	}
	if h.CostBasis != 0 {
		t.Errorf("expected 0 cost_basis after full sell, got %f", h.CostBasis)
	}
	if h.AvgCost != 0 {
		t.Errorf("expected 0 avg_cost after full sell, got %f", h.AvgCost)
	}
	expectedRealized := 4990.0 - 4010.0 // 980
	if h.RealizedReturn != expectedRealized {
		t.Errorf("expected realized=%f, got %f", expectedRealized, h.RealizedReturn)
	}
}

func TestDeriveHolding_MultipleBuySell(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	trades := []models.Trade{
		// Buy 100 @ $40 + $10 → cost = 4010
		{Action: models.TradeActionBuy, Units: 100, Price: 40.00, Fees: 10.00, Date: base},
		// Buy 50 @ $60 + $10 → cost = 3010; running: 150 units, cost = 7020, avg = 46.80
		{Action: models.TradeActionBuy, Units: 50, Price: 60.00, Fees: 10.00, Date: base.Add(24 * time.Hour)},
		// Sell 75 @ $70 - $10 → proceeds = 5240; avg_cost = 7020/150 = 46.80; cost_of_sold = 3510; realized = 1730
		{Action: models.TradeActionSell, Units: 75, Price: 70.00, Fees: 10.00, Date: base.Add(48 * time.Hour)},
	}
	h := DeriveHolding(trades, 0)
	if h.Units != 75 {
		t.Errorf("expected 75 units, got %f", h.Units)
	}
	if h.TradeCount != 3 {
		t.Errorf("expected 3 trades, got %d", h.TradeCount)
	}
	if h.RealizedReturn <= 0 {
		t.Errorf("expected positive realized return, got %f", h.RealizedReturn)
	}
	if h.GrossInvested != 7020 {
		t.Errorf("expected gross_invested=7020, got %f", h.GrossInvested)
	}
}

func TestSnapshotPositions_Replace(t *testing.T) {
	svc := testService()
	ctx := testContext()

	positions := []models.SnapshotPosition{
		{Ticker: "BHP.AU", Units: 100, AvgCost: 40.00},
		{Ticker: "CBA.AU", Units: 50, AvgCost: 100.00},
	}
	tb, err := svc.SnapshotPositions(ctx, "SMSF", positions, "replace", "commsec", "2026-01-01")
	if err != nil {
		t.Fatalf("SnapshotPositions failed: %v", err)
	}
	if len(tb.SnapshotPositions) != 2 {
		t.Errorf("expected 2 positions, got %d", len(tb.SnapshotPositions))
	}

	// Replace with a different set
	newPositions := []models.SnapshotPosition{
		{Ticker: "NAB.AU", Units: 200, AvgCost: 30.00},
	}
	tb, err = svc.SnapshotPositions(ctx, "SMSF", newPositions, "replace", "", "2026-01-02")
	if err != nil {
		t.Fatalf("SnapshotPositions replace failed: %v", err)
	}
	if len(tb.SnapshotPositions) != 1 {
		t.Errorf("expected 1 position after replace, got %d", len(tb.SnapshotPositions))
	}
	if tb.SnapshotPositions[0].Ticker != "NAB.AU" {
		t.Errorf("expected NAB.AU, got %q", tb.SnapshotPositions[0].Ticker)
	}
}

func TestSnapshotPositions_Merge(t *testing.T) {
	svc := testService()
	ctx := testContext()

	// Initial set
	positions := []models.SnapshotPosition{
		{Ticker: "BHP.AU", Units: 100, AvgCost: 40.00},
		{Ticker: "CBA.AU", Units: 50, AvgCost: 100.00},
	}
	_, err := svc.SnapshotPositions(ctx, "SMSF", positions, "replace", "", "2026-01-01")
	if err != nil {
		t.Fatalf("initial snapshot failed: %v", err)
	}

	// Merge: update BHP, add NAB, CBA unchanged
	merge := []models.SnapshotPosition{
		{Ticker: "BHP.AU", Units: 120, AvgCost: 42.00}, // updated
		{Ticker: "NAB.AU", Units: 80, AvgCost: 35.00},  // new
	}
	tb, err := svc.SnapshotPositions(ctx, "SMSF", merge, "merge", "", "2026-01-02")
	if err != nil {
		t.Fatalf("SnapshotPositions merge failed: %v", err)
	}
	// Should have BHP (updated), CBA (unchanged), NAB (new) = 3 positions
	if len(tb.SnapshotPositions) != 3 {
		t.Errorf("expected 3 positions after merge, got %d", len(tb.SnapshotPositions))
	}

	// Verify BHP was updated
	var bhp *models.SnapshotPosition
	for i := range tb.SnapshotPositions {
		if tb.SnapshotPositions[i].Ticker == "BHP.AU" {
			bhp = &tb.SnapshotPositions[i]
			break
		}
	}
	if bhp == nil {
		t.Fatal("BHP.AU not found after merge")
	}
	if bhp.Units != 120 {
		t.Errorf("expected BHP units=120 after merge, got %f", bhp.Units)
	}
}

// TestGetTradeBook verifies the empty book is returned when no data stored.
func TestGetTradeBook_Empty(t *testing.T) {
	svc := testService()
	ctx := testContext()

	tb, err := svc.GetTradeBook(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetTradeBook failed: %v", err)
	}
	if tb == nil {
		t.Fatal("expected non-nil TradeBook")
	}
	if tb.Trades == nil {
		t.Error("expected Trades to be initialized (not nil)")
	}
}

// TestDeriveAllHoldings verifies multi-ticker aggregation.
func TestDeriveAllHoldings(t *testing.T) {
	svc := testService()
	ctx := testContext()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, _, err := svc.AddTrade(ctx, "SMSF", buyTrade("BHP.AU", 100, 40, 10, base))
	if err != nil {
		t.Fatalf("AddTrade BHP failed: %v", err)
	}
	_, _, err = svc.AddTrade(ctx, "SMSF", buyTrade("CBA.AU", 50, 100, 10, base))
	if err != nil {
		t.Fatalf("AddTrade CBA failed: %v", err)
	}

	holdings, err := svc.DeriveHoldings(ctx, "SMSF")
	if err != nil {
		t.Fatalf("DeriveHoldings failed: %v", err)
	}
	if len(holdings) != 2 {
		t.Errorf("expected 2 holdings, got %d", len(holdings))
	}
}

// TestTradeBookPersistence verifies that trades are persisted and loaded correctly.
func TestTradeBookPersistence(t *testing.T) {
	svc := testService()
	ctx := testContext()

	trade := buyTrade("BHP.AU", 100, 45.00, 9.95, time.Now().Add(-time.Hour))
	created, _, err := svc.AddTrade(ctx, "SMSF", trade)
	if err != nil {
		t.Fatalf("AddTrade failed: %v", err)
	}

	// Load the trade book fresh
	tb, err := svc.GetTradeBook(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetTradeBook failed: %v", err)
	}
	if len(tb.Trades) != 1 {
		t.Errorf("expected 1 trade, got %d", len(tb.Trades))
	}
	if tb.Trades[0].ID != created.ID {
		t.Errorf("expected trade ID %q, got %q", created.ID, tb.Trades[0].ID)
	}

	// Verify JSON round-trip
	data, err := json.Marshal(tb)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var tb2 models.TradeBook
	if err := json.Unmarshal(data, &tb2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if tb2.Trades[0].ID != created.ID {
		t.Errorf("ID mismatch after round-trip: %q vs %q", tb2.Trades[0].ID, created.ID)
	}
}
