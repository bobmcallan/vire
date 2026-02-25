package plan

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- mock implementations ---

// memUserDataStore is a simple in-memory UserDataStore for tests.
type memUserDataStore struct {
	mu      sync.Mutex
	records map[string]*models.UserRecord
}

func newMemUserDataStore() *memUserDataStore {
	return &memUserDataStore{records: make(map[string]*models.UserRecord)}
}

func (m *memUserDataStore) Get(_ context.Context, userID, subject, key string) (*models.UserRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ck := userID + ":" + subject + ":" + key
	if r, ok := m.records[ck]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("%s '%s' not found", subject, key)
}

func (m *memUserDataStore) Put(_ context.Context, record *models.UserRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.mu.Lock()
	defer m.mu.Unlock()
	ck := userID + ":" + subject + ":" + key
	delete(m.records, ck)
	return nil
}

func (m *memUserDataStore) List(_ context.Context, userID, subject string) ([]*models.UserRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.mu.Lock()
	defer m.mu.Unlock()
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

type mockSignalStorage struct {
	signals map[string]*models.TickerSignals
}

func (m *mockSignalStorage) GetSignals(_ context.Context, ticker string) (*models.TickerSignals, error) {
	s, ok := m.signals[ticker]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return s, nil
}
func (m *mockSignalStorage) SaveSignals(_ context.Context, _ *models.TickerSignals) error { return nil }
func (m *mockSignalStorage) GetSignalsBatch(_ context.Context, _ []string) ([]*models.TickerSignals, error) {
	return nil, nil
}

type mockMarketDataStorage struct {
	data map[string]*models.MarketData
}

func (m *mockMarketDataStorage) GetMarketData(_ context.Context, ticker string) (*models.MarketData, error) {
	d, ok := m.data[ticker]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return d, nil
}
func (m *mockMarketDataStorage) SaveMarketData(_ context.Context, _ *models.MarketData) error {
	return nil
}
func (m *mockMarketDataStorage) GetMarketDataBatch(_ context.Context, _ []string) ([]*models.MarketData, error) {
	return nil, nil
}
func (m *mockMarketDataStorage) GetStaleTickers(_ context.Context, _ string, _ int64) ([]string, error) {
	return nil, nil
}

type mockStorageManager struct {
	userDataStore *memUserDataStore
	signals       *mockSignalStorage
	marketData    *mockMarketDataStorage
}

func newMockStorageManager() *mockStorageManager {
	return &mockStorageManager{
		userDataStore: newMemUserDataStore(),
		signals:       &mockSignalStorage{signals: make(map[string]*models.TickerSignals)},
		marketData:    &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
	}
}

func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage         { return m.signals }
func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return m.marketData }
func (m *mockStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *mockStorageManager) UserDataStore() interfaces.UserDataStore         { return m.userDataStore }
func (m *mockStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) PurgeReports(_ context.Context) (int, error)    { return 0, nil }
func (m *mockStorageManager) StockIndexStore() interfaces.StockIndexStore    { return nil }
func (m *mockStorageManager) JobQueueStore() interfaces.JobQueueStore        { return nil }
func (m *mockStorageManager) FileStore() interfaces.FileStore                { return nil }
func (m *mockStorageManager) FeedbackStore() interfaces.FeedbackStore        { return nil }
func (m *mockStorageManager) OAuthStore() interfaces.OAuthStore              { return nil }
func (m *mockStorageManager) DataPath() string                               { return "" }
func (m *mockStorageManager) WriteRaw(subdir, key string, data []byte) error { return nil }
func (m *mockStorageManager) Close() error                                   { return nil }

type mockStrategyService struct{}

func (m *mockStrategyService) GetStrategy(_ context.Context, _ string) (*models.PortfolioStrategy, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockStrategyService) SaveStrategy(_ context.Context, _ *models.PortfolioStrategy) ([]models.StrategyWarning, error) {
	return nil, nil
}
func (m *mockStrategyService) DeleteStrategy(_ context.Context, _ string) error { return nil }
func (m *mockStrategyService) ValidateStrategy(_ context.Context, _ *models.PortfolioStrategy) []models.StrategyWarning {
	return nil
}

// --- helpers ---

func newTestService() (*Service, *mockStorageManager) {
	logger := common.NewLogger("error")
	sm := newMockStorageManager()
	svc := NewService(sm, &mockStrategyService{}, logger)
	return svc, sm
}

// --- tests ---

func TestAddPlanItem_CreatesNewPlan(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	item := &models.PlanItem{
		Type:        models.PlanItemTypeTime,
		Description: "Rebalance portfolio",
	}

	plan, err := svc.AddPlanItem(ctx, "SMSF", item)
	if err != nil {
		t.Fatalf("AddPlanItem failed: %v", err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(plan.Items))
	}
	if plan.Items[0].ID != "plan-1" {
		t.Errorf("expected auto-generated ID 'plan-1', got '%s'", plan.Items[0].ID)
	}
	if plan.Items[0].Status != models.PlanItemStatusPending {
		t.Errorf("expected status 'pending', got '%s'", plan.Items[0].Status)
	}
}

func TestAddPlanItem_AppendsToExisting(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		Type:        models.PlanItemTypeTime,
		Description: "First item",
	})

	plan, err := svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		Type:        models.PlanItemTypeEvent,
		Description: "Second item",
	})
	if err != nil {
		t.Fatalf("AddPlanItem failed: %v", err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(plan.Items))
	}
	if plan.Items[1].ID != "plan-2" {
		t.Errorf("expected ID 'plan-2', got '%s'", plan.Items[1].ID)
	}
}

func TestAddPlanItem_CustomID(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	item := &models.PlanItem{
		ID:          "my-custom-id",
		Type:        models.PlanItemTypeTime,
		Description: "Custom ID item",
	}

	plan, err := svc.AddPlanItem(ctx, "SMSF", item)
	if err != nil {
		t.Fatalf("AddPlanItem failed: %v", err)
	}
	if plan.Items[0].ID != "my-custom-id" {
		t.Errorf("expected custom ID, got '%s'", plan.Items[0].ID)
	}
}

func TestAddPlanItem_DuplicateID(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "dup-1",
		Type:        models.PlanItemTypeTime,
		Description: "First",
	})

	_, err := svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "dup-1",
		Type:        models.PlanItemTypeTime,
		Description: "Duplicate",
	})
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestUpdatePlanItem(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		Type:        models.PlanItemTypeTime,
		Description: "Original description",
	})

	plan, err := svc.UpdatePlanItem(ctx, "SMSF", "plan-1", &models.PlanItem{
		Description: "Updated description",
		Status:      models.PlanItemStatusCompleted,
	})
	if err != nil {
		t.Fatalf("UpdatePlanItem failed: %v", err)
	}
	if plan.Items[0].Description != "Updated description" {
		t.Errorf("description not updated")
	}
	if plan.Items[0].Status != models.PlanItemStatusCompleted {
		t.Errorf("status not updated")
	}
	if plan.Items[0].CompletedAt == nil {
		t.Error("CompletedAt should be set when status is completed")
	}
}

func TestUpdatePlanItem_NotFound(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		Type:        models.PlanItemTypeTime,
		Description: "Item",
	})

	_, err := svc.UpdatePlanItem(ctx, "SMSF", "nonexistent", &models.PlanItem{
		Description: "Nope",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}

func TestUpdatePlanItem_MergePreservesUnchanged(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	deadline := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		Type:        models.PlanItemTypeTime,
		Description: "Original",
		Deadline:    &deadline,
		Notes:       "Important note",
		Ticker:      "BHP.AU",
	})

	// Update only the description
	plan, err := svc.UpdatePlanItem(ctx, "SMSF", "plan-1", &models.PlanItem{
		Description: "Changed",
	})
	if err != nil {
		t.Fatalf("UpdatePlanItem failed: %v", err)
	}

	item := plan.Items[0]
	if item.Description != "Changed" {
		t.Errorf("description should be updated")
	}
	if item.Deadline == nil || !item.Deadline.Equal(deadline) {
		t.Errorf("deadline should be preserved")
	}
	if item.Notes != "Important note" {
		t.Errorf("notes should be preserved")
	}
	if item.Ticker != "BHP.AU" {
		t.Errorf("ticker should be preserved")
	}
}

func TestRemovePlanItem(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		Type:        models.PlanItemTypeTime,
		Description: "Keep",
	})
	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		Type:        models.PlanItemTypeTime,
		Description: "Remove",
	})

	plan, err := svc.RemovePlanItem(ctx, "SMSF", "plan-2")
	if err != nil {
		t.Fatalf("RemovePlanItem failed: %v", err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(plan.Items))
	}
	if plan.Items[0].ID != "plan-1" {
		t.Errorf("wrong item remaining")
	}
}

func TestRemovePlanItem_NotFound(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		Type:        models.PlanItemTypeTime,
		Description: "Item",
	})

	_, err := svc.RemovePlanItem(ctx, "SMSF", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}

func TestCheckPlanDeadlines(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	pastDeadline := time.Now().Add(-24 * time.Hour)
	futureDeadline := time.Now().Add(24 * time.Hour)

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "overdue",
		Type:        models.PlanItemTypeTime,
		Description: "Overdue item",
		Deadline:    &pastDeadline,
	})
	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "future",
		Type:        models.PlanItemTypeTime,
		Description: "Future item",
		Deadline:    &futureDeadline,
	})

	expired, err := svc.CheckPlanDeadlines(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CheckPlanDeadlines failed: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired, got %d", len(expired))
	}
	if expired[0].ID != "overdue" {
		t.Errorf("expected 'overdue' item, got '%s'", expired[0].ID)
	}
	if expired[0].Status != models.PlanItemStatusExpired {
		t.Errorf("expected status 'expired', got '%s'", expired[0].Status)
	}

	// Verify the plan was persisted with the status change
	plan, _ := svc.GetPlan(ctx, "SMSF")
	for _, item := range plan.Items {
		if item.ID == "overdue" && item.Status != models.PlanItemStatusExpired {
			t.Error("overdue item should be persisted as expired")
		}
		if item.ID == "future" && item.Status != models.PlanItemStatusPending {
			t.Error("future item should still be pending")
		}
	}
}

func TestCheckPlanDeadlines_SkipsNonPending(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	pastDeadline := time.Now().Add(-24 * time.Hour)

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "already-done",
		Type:        models.PlanItemTypeTime,
		Description: "Already completed",
		Deadline:    &pastDeadline,
		Status:      models.PlanItemStatusCompleted,
	})

	expired, err := svc.CheckPlanDeadlines(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CheckPlanDeadlines failed: %v", err)
	}
	if len(expired) != 0 {
		t.Errorf("expected 0 expired, got %d", len(expired))
	}
}

func TestCheckPlanEvents(t *testing.T) {
	svc, sm := newTestService()
	ctx := context.Background()

	// Set up signals for BHP
	sm.signals.signals["BHP.AU"] = &models.TickerSignals{
		Ticker: "BHP.AU",
		Technical: models.TechnicalSignals{
			RSI: 25, // Oversold
		},
	}

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "buy-bhp",
		Type:        models.PlanItemTypeEvent,
		Description: "Buy BHP when RSI < 30",
		Ticker:      "BHP.AU",
		Action:      models.RuleActionBuy,
		Conditions: []models.RuleCondition{
			{Field: "signals.rsi", Operator: models.RuleOpLT, Value: float64(30)},
		},
	})
	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "sell-bhp",
		Type:        models.PlanItemTypeEvent,
		Description: "Sell BHP when RSI > 70",
		Ticker:      "BHP.AU",
		Action:      models.RuleActionSell,
		Conditions: []models.RuleCondition{
			{Field: "signals.rsi", Operator: models.RuleOpGT, Value: float64(70)},
		},
	})

	triggered, err := svc.CheckPlanEvents(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CheckPlanEvents failed: %v", err)
	}
	if len(triggered) != 1 {
		t.Fatalf("expected 1 triggered, got %d", len(triggered))
	}
	if triggered[0].ID != "buy-bhp" {
		t.Errorf("expected 'buy-bhp', got '%s'", triggered[0].ID)
	}
	if triggered[0].Status != models.PlanItemStatusTriggered {
		t.Errorf("expected status 'triggered', got '%s'", triggered[0].Status)
	}
}

func TestCheckPlanEvents_SkipsNonEvent(t *testing.T) {
	svc, sm := newTestService()
	ctx := context.Background()

	sm.signals.signals["BHP.AU"] = &models.TickerSignals{
		Ticker:    "BHP.AU",
		Technical: models.TechnicalSignals{RSI: 25},
	}

	// Time-based item should not be evaluated as event
	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "time-item",
		Type:        models.PlanItemTypeTime,
		Description: "Time-based item with conditions",
		Ticker:      "BHP.AU",
		Conditions: []models.RuleCondition{
			{Field: "signals.rsi", Operator: models.RuleOpLT, Value: float64(30)},
		},
	})

	triggered, err := svc.CheckPlanEvents(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CheckPlanEvents failed: %v", err)
	}
	if len(triggered) != 0 {
		t.Errorf("expected 0 triggered, got %d", len(triggered))
	}
}

func TestCheckPlanEvents_NoConditions(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "no-cond",
		Type:        models.PlanItemTypeEvent,
		Description: "Event with no conditions",
	})

	triggered, err := svc.CheckPlanEvents(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CheckPlanEvents failed: %v", err)
	}
	if len(triggered) != 0 {
		t.Errorf("expected 0 triggered for no-condition event, got %d", len(triggered))
	}
}

func TestValidatePlanAgainstStrategy_InvestmentUniverse(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	plan := &models.PortfolioPlan{
		PortfolioName: "SMSF",
		Items: []models.PlanItem{
			{
				ID:     "buy-us",
				Type:   models.PlanItemTypeEvent,
				Action: models.RuleActionBuy,
				Ticker: "AAPL.US",
				Status: models.PlanItemStatusPending,
			},
		},
	}

	strategy := &models.PortfolioStrategy{
		InvestmentUniverse: []string{"AU"},
	}

	warnings := svc.ValidatePlanAgainstStrategy(ctx, plan, strategy)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Severity != "medium" {
		t.Errorf("expected medium severity, got '%s'", warnings[0].Severity)
	}
}

func TestValidatePlanAgainstStrategy_NilInputs(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	warnings := svc.ValidatePlanAgainstStrategy(ctx, nil, nil)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for nil inputs, got %d", len(warnings))
	}
}

func TestSavePlan_And_GetPlan(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	plan := &models.PortfolioPlan{
		PortfolioName: "SMSF",
		Items: []models.PlanItem{
			{
				ID:          "item-1",
				Type:        models.PlanItemTypeTime,
				Description: "Test item",
				Status:      models.PlanItemStatusPending,
			},
		},
	}

	if err := svc.SavePlan(ctx, plan); err != nil {
		t.Fatalf("SavePlan failed: %v", err)
	}

	got, err := svc.GetPlan(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetPlan failed: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got.Items))
	}
	if got.Items[0].ID != "item-1" {
		t.Errorf("expected ID 'item-1', got '%s'", got.Items[0].ID)
	}
}

func TestDeletePlan(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		Type:        models.PlanItemTypeTime,
		Description: "Item to delete",
	})

	if err := svc.DeletePlan(ctx, "SMSF"); err != nil {
		t.Fatalf("DeletePlan failed: %v", err)
	}

	_, err := svc.GetPlan(ctx, "SMSF")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestCheckPlanEvents_FundamentalsCondition(t *testing.T) {
	svc, sm := newTestService()
	ctx := context.Background()

	sm.marketData.data["CBA.AU"] = &models.MarketData{
		Ticker:       "CBA.AU",
		Fundamentals: &models.Fundamentals{PE: 12.5},
	}
	sm.signals.signals["CBA.AU"] = &models.TickerSignals{Ticker: "CBA.AU"}

	svc.AddPlanItem(ctx, "SMSF", &models.PlanItem{
		ID:          "buy-cba-pe",
		Type:        models.PlanItemTypeEvent,
		Description: "Buy CBA when P/E < 15",
		Ticker:      "CBA.AU",
		Action:      models.RuleActionBuy,
		Conditions: []models.RuleCondition{
			{Field: "fundamentals.pe", Operator: models.RuleOpLT, Value: float64(15)},
		},
	})

	triggered, err := svc.CheckPlanEvents(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CheckPlanEvents failed: %v", err)
	}
	if len(triggered) != 1 {
		t.Fatalf("expected 1 triggered, got %d", len(triggered))
	}
	if triggered[0].ID != "buy-cba-pe" {
		t.Errorf("expected 'buy-cba-pe', got '%s'", triggered[0].ID)
	}
}
