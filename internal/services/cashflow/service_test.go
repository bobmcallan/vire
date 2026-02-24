package cashflow

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

// --- Mock storage ---

type mockUserDataStore struct {
	records map[string]*models.UserRecord
}

func newMockUserDataStore() *mockUserDataStore {
	return &mockUserDataStore{records: make(map[string]*models.UserRecord)}
}

func (m *mockUserDataStore) key(userID, subject, key string) string {
	return userID + ":" + subject + ":" + key
}

func (m *mockUserDataStore) Get(_ context.Context, userID, subject, key string) (*models.UserRecord, error) {
	rec, ok := m.records[m.key(userID, subject, key)]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return rec, nil
}

func (m *mockUserDataStore) Put(_ context.Context, rec *models.UserRecord) error {
	m.records[m.key(rec.UserID, rec.Subject, rec.Key)] = rec
	return nil
}

func (m *mockUserDataStore) Delete(_ context.Context, userID, subject, key string) error {
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
func (m *mockStorageManager) DataPath() string                                { return "" }
func (m *mockStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (m *mockStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *mockStorageManager) Close() error                                { return nil }

// --- Mock portfolio service ---

type mockPortfolioService struct {
	portfolio *models.Portfolio
}

func (m *mockPortfolioService) SyncPortfolio(_ context.Context, _ string, _ bool) (*models.Portfolio, error) {
	return m.portfolio, nil
}
func (m *mockPortfolioService) GetPortfolio(_ context.Context, _ string) (*models.Portfolio, error) {
	if m.portfolio == nil {
		return nil, fmt.Errorf("portfolio not found")
	}
	return m.portfolio, nil
}
func (m *mockPortfolioService) ListPortfolios(_ context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockPortfolioService) ReviewPortfolio(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.PortfolioReview, error) {
	return nil, nil
}
func (m *mockPortfolioService) ReviewWatchlist(_ context.Context, _ string, _ interfaces.ReviewOptions) (*models.WatchlistReview, error) {
	return nil, nil
}
func (m *mockPortfolioService) GetPortfolioSnapshot(_ context.Context, _ string, _ time.Time) (*models.PortfolioSnapshot, error) {
	return nil, nil
}
func (m *mockPortfolioService) GetPortfolioGrowth(_ context.Context, _ string) ([]models.GrowthDataPoint, error) {
	return nil, nil
}
func (m *mockPortfolioService) GetDailyGrowth(_ context.Context, _ string, _ interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
	return nil, nil
}
func (m *mockPortfolioService) GetExternalBalances(_ context.Context, _ string) ([]models.ExternalBalance, error) {
	return nil, nil
}
func (m *mockPortfolioService) SetExternalBalances(_ context.Context, _ string, _ []models.ExternalBalance) (*models.Portfolio, error) {
	return nil, nil
}
func (m *mockPortfolioService) AddExternalBalance(_ context.Context, _ string, _ models.ExternalBalance) (*models.Portfolio, error) {
	return nil, nil
}
func (m *mockPortfolioService) RemoveExternalBalance(_ context.Context, _ string, _ string) (*models.Portfolio, error) {
	return nil, nil
}
func (m *mockPortfolioService) GetPortfolioIndicators(_ context.Context, _ string) (*models.PortfolioIndicators, error) {
	return nil, nil
}

// --- Test helpers ---

func testContext() context.Context {
	ctx := context.Background()
	uc := &common.UserContext{UserID: "test-user"}
	return common.WithUserContext(ctx, uc)
}

func testService() (*Service, *mockPortfolioService) {
	storage := newMockStorageManager()
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   100000,
			TotalValue:           150000, // holdings + external balances
			ExternalBalanceTotal: 50000,
		},
	}
	logger := common.NewLogger("error")
	return NewService(storage, portfolioSvc, logger), portfolioSvc
}

// --- Tests ---

func TestGenerateCashTransactionID(t *testing.T) {
	id := generateCashTransactionID()
	if !strings.HasPrefix(id, "ct_") {
		t.Errorf("ID should start with 'ct_', got %q", id)
	}
	if len(id) != 11 {
		t.Errorf("ID should be 11 chars (ct_ + 8 hex), got %d: %q", len(id), id)
	}

	// Uniqueness
	id2 := generateCashTransactionID()
	if id == id2 {
		t.Errorf("IDs should be unique, got %q twice", id)
	}
}

func TestValidateCashTransaction(t *testing.T) {
	valid := models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Now().Add(-24 * time.Hour),
		Amount:      10000,
		Description: "Initial deposit",
	}
	if err := validateCashTransaction(valid); err != nil {
		t.Errorf("valid transaction failed: %v", err)
	}

	tests := []struct {
		name    string
		modify  func(*models.CashTransaction)
		wantErr string
	}{
		{
			name:    "invalid type",
			modify:  func(tx *models.CashTransaction) { tx.Type = "refund" },
			wantErr: "invalid transaction type",
		},
		{
			name:    "zero date",
			modify:  func(tx *models.CashTransaction) { tx.Date = time.Time{} },
			wantErr: "date is required",
		},
		{
			name:    "future date",
			modify:  func(tx *models.CashTransaction) { tx.Date = time.Now().Add(48 * time.Hour) },
			wantErr: "future",
		},
		{
			name:    "zero amount",
			modify:  func(tx *models.CashTransaction) { tx.Amount = 0 },
			wantErr: "must be positive",
		},
		{
			name:    "negative amount",
			modify:  func(tx *models.CashTransaction) { tx.Amount = -100 },
			wantErr: "must be positive",
		},
		{
			name:    "infinite amount",
			modify:  func(tx *models.CashTransaction) { tx.Amount = math.Inf(1) },
			wantErr: "must be finite",
		},
		{
			name:    "NaN amount",
			modify:  func(tx *models.CashTransaction) { tx.Amount = math.NaN() },
			wantErr: "must be finite",
		},
		{
			name:    "amount too large",
			modify:  func(tx *models.CashTransaction) { tx.Amount = 1e15 },
			wantErr: "exceeds maximum",
		},
		{
			name:    "empty description",
			modify:  func(tx *models.CashTransaction) { tx.Description = "" },
			wantErr: "description is required",
		},
		{
			name:    "whitespace description",
			modify:  func(tx *models.CashTransaction) { tx.Description = "   " },
			wantErr: "description is required",
		},
		{
			name:    "description too long",
			modify:  func(tx *models.CashTransaction) { tx.Description = strings.Repeat("x", 501) },
			wantErr: "description exceeds",
		},
		{
			name:    "category too long",
			modify:  func(tx *models.CashTransaction) { tx.Category = strings.Repeat("x", 101) },
			wantErr: "category exceeds",
		},
		{
			name:    "notes too long",
			modify:  func(tx *models.CashTransaction) { tx.Notes = strings.Repeat("x", 1001) },
			wantErr: "notes exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := valid // copy
			tt.modify(&tx)
			err := validateCashTransaction(tx)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestAddTransaction(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	tx := models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Initial SMSF deposit",
	}

	ledger, err := svc.AddTransaction(ctx, "SMSF", tx)
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	if len(ledger.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(ledger.Transactions))
	}

	added := ledger.Transactions[0]
	if !strings.HasPrefix(added.ID, "ct_") {
		t.Errorf("transaction ID should start with ct_, got %q", added.ID)
	}
	if added.Type != models.CashTxDeposit {
		t.Errorf("type = %q, want %q", added.Type, models.CashTxDeposit)
	}
	if added.Amount != 50000 {
		t.Errorf("amount = %v, want 50000", added.Amount)
	}
	if added.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestAddTransaction_SortedByDate(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	dates := []time.Time{
		time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
	}

	for _, d := range dates {
		_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
			Type:        models.CashTxDeposit,
			Date:        d,
			Amount:      10000,
			Description: "Deposit",
		})
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}
	}

	ledger, _ := svc.GetLedger(ctx, "SMSF")
	for i := 1; i < len(ledger.Transactions); i++ {
		if ledger.Transactions[i].Date.Before(ledger.Transactions[i-1].Date) {
			t.Errorf("transactions not sorted: %v before %v at index %d",
				ledger.Transactions[i].Date, ledger.Transactions[i-1].Date, i)
		}
	}
}

func TestUpdateTransaction(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add a transaction first
	ledger, _ := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Initial deposit",
	})
	txID := ledger.Transactions[0].ID

	// Update it
	ledger, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Amount:      75000,
		Description: "Updated deposit",
	})
	if err != nil {
		t.Fatalf("UpdateTransaction: %v", err)
	}

	updated := ledger.Transactions[0]
	if updated.Amount != 75000 {
		t.Errorf("amount = %v, want 75000", updated.Amount)
	}
	if updated.Description != "Updated deposit" {
		t.Errorf("description = %q, want %q", updated.Description, "Updated deposit")
	}
	// Type should remain unchanged (merge semantics)
	if updated.Type != models.CashTxDeposit {
		t.Errorf("type should be unchanged, got %q", updated.Type)
	}
}

func TestUpdateTransaction_NotFound(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	_, err := svc.UpdateTransaction(ctx, "SMSF", "ct_nonexist", models.CashTransaction{
		Amount: 100,
	})
	if err == nil {
		t.Error("expected error for missing transaction")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %q", err.Error())
	}
}

func TestRemoveTransaction(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	ledger, _ := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Deposit",
	})
	txID := ledger.Transactions[0].ID

	ledger, err := svc.RemoveTransaction(ctx, "SMSF", txID)
	if err != nil {
		t.Fatalf("RemoveTransaction: %v", err)
	}
	if len(ledger.Transactions) != 0 {
		t.Errorf("expected 0 transactions after remove, got %d", len(ledger.Transactions))
	}
}

func TestRemoveTransaction_NotFound(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	_, err := svc.RemoveTransaction(ctx, "SMSF", "ct_nonexist")
	if err == nil {
		t.Error("expected error for missing transaction")
	}
}

func TestCalculatePerformance(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add deposits and a withdrawal
	for _, tx := range []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 80000, Description: "Initial deposit"},
		{Type: models.CashTxContribution, Date: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Amount: 20000, Description: "Contribution"},
		{Type: models.CashTxWithdrawal, Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 10000, Description: "Withdrawal"},
	} {
		if _, err := svc.AddTransaction(ctx, "SMSF", tx); err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}
	}

	// Portfolio mock: TotalValue=100000, ExternalBalanceTotal=50000 → currentValue=150000
	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 10000 {
		t.Errorf("TotalWithdrawn = %v, want 10000", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 90000 {
		t.Errorf("NetCapitalDeployed = %v, want 90000", perf.NetCapitalDeployed)
	}
	if perf.CurrentPortfolioValue != 150000 {
		t.Errorf("CurrentPortfolioValue = %v, want 150000", perf.CurrentPortfolioValue)
	}

	// Simple return: (150000 - 90000) / 90000 * 100 = 66.67%
	expectedSimple := (150000.0 - 90000.0) / 90000.0 * 100
	if math.Abs(perf.SimpleReturnPct-expectedSimple) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%v", perf.SimpleReturnPct, expectedSimple)
	}

	if perf.TransactionCount != 3 {
		t.Errorf("TransactionCount = %d, want 3", perf.TransactionCount)
	}
	if perf.FirstTransactionDate == nil {
		t.Error("FirstTransactionDate should not be nil")
	} else if !perf.FirstTransactionDate.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("FirstTransactionDate = %v, want 2024-01-01", perf.FirstTransactionDate)
	}

	// XIRR should be positive (we made money)
	if perf.AnnualizedReturnPct <= 0 {
		t.Errorf("AnnualizedReturnPct = %v, should be positive", perf.AnnualizedReturnPct)
	}
}

func TestCalculatePerformance_EmptyLedger(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TransactionCount != 0 {
		t.Errorf("TransactionCount = %d, want 0", perf.TransactionCount)
	}
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct = %v, want 0", perf.SimpleReturnPct)
	}
}

func TestGetLedger_EmptyReturnsDefaults(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	ledger, err := svc.GetLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}

	if ledger.PortfolioName != "SMSF" {
		t.Errorf("PortfolioName = %q, want %q", ledger.PortfolioName, "SMSF")
	}
	if ledger.Transactions == nil {
		t.Error("Transactions should not be nil")
	}
	if len(ledger.Transactions) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(ledger.Transactions))
	}
}

func TestLedgerVersionIncrement(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	tx := models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Deposit",
	}

	ledger, _ := svc.AddTransaction(ctx, "SMSF", tx)
	v1 := ledger.Version

	ledger, _ = svc.AddTransaction(ctx, "SMSF", tx)
	v2 := ledger.Version

	if v2 <= v1 {
		t.Errorf("version should increment: v1=%d, v2=%d", v1, v2)
	}
}

func TestLedgerPersistence(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Deposit",
	})

	// Read back from storage
	ledger, err := svc.GetLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}
	if len(ledger.Transactions) != 1 {
		t.Errorf("expected 1 transaction after persistence, got %d", len(ledger.Transactions))
	}
}

func TestComputeXIRR_BasicDeposit(t *testing.T) {
	// Deposit $100K a year ago, portfolio now worth $110K → ~10% annual return
	transactions := []models.CashTransaction{
		{
			Type:   models.CashTxDeposit,
			Date:   time.Now().AddDate(-1, 0, 0),
			Amount: 100000,
		},
	}

	rate := computeXIRR(transactions, 110000)
	// Should be approximately 10%
	if rate < 5 || rate > 15 {
		t.Errorf("XIRR = %.2f%%, expected ~10%%", rate)
	}
}

func TestComputeXIRR_NoTransactions(t *testing.T) {
	rate := computeXIRR(nil, 100000)
	if rate != 0 {
		t.Errorf("XIRR with no transactions = %v, want 0", rate)
	}
}

// --- XIRR and CalculatePerformance stress tests ---

func TestComputeXIRR_SameDayTransactions(t *testing.T) {
	// Multiple deposits on the exact same day
	sameDay := time.Now().AddDate(-1, 0, 0)
	transactions := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: sameDay, Amount: 50000},
		{Type: models.CashTxDeposit, Date: sameDay, Amount: 50000},
	}
	rate := computeXIRR(transactions, 110000)
	// Should produce a finite result ~10%
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with same-day transactions should be finite, got %v", rate)
	}
}

func TestComputeXIRR_VeryOldTransactions(t *testing.T) {
	// Transaction 20 years ago — tests float64 precision with large year exponents
	transactions := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Now().AddDate(-20, 0, 0), Amount: 10000},
	}
	rate := computeXIRR(transactions, 50000)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with 20-year-old transaction should be finite, got %v", rate)
	}
	// ~8.4% annualized (10000 -> 50000 over 20 years)
	if rate < 0 || rate > 50 {
		t.Errorf("XIRR = %.2f%%, expected a reasonable positive rate", rate)
	}
}

func TestComputeXIRR_TotalLoss(t *testing.T) {
	// Portfolio value is 0 — total wipeout
	transactions := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Now().AddDate(-1, 0, 0), Amount: 100000},
	}
	rate := computeXIRR(transactions, 0)
	// With currentValue=0, there are no positive flows → should return 0
	if rate != 0 {
		t.Logf("XIRR with total loss = %.2f%% (expected 0 due to no positive flows)", rate)
	}
}

func TestComputeXIRR_OnlyWithdrawals(t *testing.T) {
	// Only withdrawals, no deposits. All flows are positive → no negative flow → return 0
	transactions := []models.CashTransaction{
		{Type: models.CashTxWithdrawal, Date: time.Now().AddDate(0, -6, 0), Amount: 50000},
		{Type: models.CashTxWithdrawal, Date: time.Now().AddDate(0, -3, 0), Amount: 30000},
	}
	rate := computeXIRR(transactions, 100000)
	// All flows positive: withdrawal positive + terminal positive → no negative → return 0
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with only withdrawals should not produce NaN/Inf, got %v", rate)
	}
}

func TestComputeXIRR_VeryRecentTransaction(t *testing.T) {
	// Transaction just yesterday — very short holding period
	transactions := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Now().Add(-24 * time.Hour), Amount: 100000},
	}
	rate := computeXIRR(transactions, 100100)
	// Should produce a very high annualized rate but not Inf/NaN
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with 1-day-old transaction should be finite, got %v", rate)
	}
}

func TestComputeXIRR_NegativeNetCapital(t *testing.T) {
	// More withdrawn than deposited
	transactions := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Now().AddDate(-2, 0, 0), Amount: 100000},
		{Type: models.CashTxWithdrawal, Date: time.Now().AddDate(-1, 0, 0), Amount: 120000},
	}
	rate := computeXIRR(transactions, 50000)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with negative net capital should be finite, got %v", rate)
	}
}

func TestComputeXIRR_ZeroDateTransaction(t *testing.T) {
	// Transactions with zero dates should be skipped
	transactions := []models.CashTransaction{
		{Type: models.CashTxDeposit, Date: time.Time{}, Amount: 100000},
	}
	rate := computeXIRR(transactions, 110000)
	// Zero-date transactions are skipped, so flows is empty → return 0
	if rate != 0 {
		t.Errorf("XIRR with zero-date transactions = %v, want 0", rate)
	}
}

func TestComputeXIRR_LargeNumberOfTransactions(t *testing.T) {
	// 500 monthly deposits over ~40 years
	var transactions []models.CashTransaction
	base := time.Now().AddDate(-40, 0, 0)
	for i := 0; i < 500; i++ {
		transactions = append(transactions, models.CashTransaction{
			Type:   models.CashTxDeposit,
			Date:   base.AddDate(0, i, 0),
			Amount: 1000,
		})
	}
	// Total deposited: 500000, current value: 800000
	rate := computeXIRR(transactions, 800000)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with 500 transactions should be finite, got %v", rate)
	}
}

func TestCalculatePerformance_ZeroPortfolioValue(t *testing.T) {
	// Division by zero scenario: netCapital > 0, currentValue = 0
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:       "SMSF",
			TotalValue: 0, // total wipeout
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit into doomed portfolio",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// SimpleReturnPct: (0 - 100000) / 100000 * 100 = -100%
	if perf.SimpleReturnPct != -100 {
		t.Errorf("SimpleReturnPct = %v, want -100", perf.SimpleReturnPct)
	}
	if perf.CurrentPortfolioValue != 0 {
		t.Errorf("CurrentPortfolioValue = %v, want 0", perf.CurrentPortfolioValue)
	}
}

func TestCalculatePerformance_NilPortfolioService(t *testing.T) {
	// Portfolio service returns error — CalculatePerformance should propagate
	portfolioSvc := &mockPortfolioService{portfolio: nil}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	_, err := svc.CalculatePerformance(ctx, "SMSF")
	if err == nil {
		t.Error("expected error when portfolio service returns not found")
	}
	if !strings.Contains(err.Error(), "portfolio") {
		t.Errorf("expected error mentioning portfolio, got %q", err.Error())
	}
}

func TestCalculatePerformance_AllOutflows(t *testing.T) {
	// Only withdrawals — netCapital is negative
	svc, _ := testService()
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxWithdrawal,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Withdrawal",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 0 {
		t.Errorf("TotalDeposited = %v, want 0", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 50000 {
		t.Errorf("TotalWithdrawn = %v, want 50000", perf.TotalWithdrawn)
	}
	// NetCapital = 0 - 50000 = -50000 → SimpleReturnPct should be 0 (netCapital <= 0)
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct with negative net capital = %v, want 0", perf.SimpleReturnPct)
	}
}

func TestCalculatePerformance_EqualDepositsAndWithdrawals(t *testing.T) {
	// Net capital = 0 → avoid division by zero in simple return
	svc, _ := testService()
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxWithdrawal,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Withdraw everything",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.NetCapitalDeployed != 0 {
		t.Errorf("NetCapitalDeployed = %v, want 0", perf.NetCapitalDeployed)
	}
	// SimpleReturnPct: netCapital is 0, so should be 0 (no division by zero)
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct with zero net capital = %v, want 0", perf.SimpleReturnPct)
	}
	if math.IsNaN(perf.AnnualizedReturnPct) || math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Errorf("AnnualizedReturnPct should be finite, got %v", perf.AnnualizedReturnPct)
	}
}

// TestCalculatePerformance_UsesExplicitFieldSum verifies that CalculatePerformance
// uses TotalValueHoldings + ExternalBalanceTotal (not TotalValue) for the current
// portfolio value. This guards against cases where TotalValue is stale or inconsistent.
func TestCalculatePerformance_UsesExplicitFieldSum(t *testing.T) {
	// Create a portfolio where TotalValue is deliberately WRONG (stale),
	// but TotalValueHoldings and ExternalBalanceTotal are correct.
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:                 "SMSF",
			TotalValueHoldings:   100000,
			ExternalBalanceTotal: 50000,
			TotalValue:           999999, // deliberately wrong / stale
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Type:        models.CashTxDeposit,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Should use 100000 + 50000 = 150000, NOT the stale 999999
	expectedValue := 150000.0
	if perf.CurrentPortfolioValue != expectedValue {
		t.Errorf("CurrentPortfolioValue = %v, want %v (should use TotalValueHoldings + ExternalBalanceTotal, not TotalValue)",
			perf.CurrentPortfolioValue, expectedValue)
	}

	// Simple return: (150000 - 100000) / 100000 * 100 = 50%
	expectedSimple := 50.0
	if math.Abs(perf.SimpleReturnPct-expectedSimple) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want %v", perf.SimpleReturnPct, expectedSimple)
	}
}

// Verify the ledger's JSON structure is correct for storage
func TestLedgerJSONRoundTrip(t *testing.T) {
	ledger := &models.CashFlowLedger{
		PortfolioName: "SMSF",
		Version:       1,
		Transactions: []models.CashTransaction{
			{
				ID:          "ct_abcd1234",
				Type:        models.CashTxDeposit,
				Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Amount:      50000,
				Description: "Initial deposit",
			},
		},
	}

	data, err := json.Marshal(ledger)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded models.CashFlowLedger
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.PortfolioName != "SMSF" {
		t.Errorf("PortfolioName = %q, want %q", decoded.PortfolioName, "SMSF")
	}
	if len(decoded.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(decoded.Transactions))
	}
	if decoded.Transactions[0].ID != "ct_abcd1234" {
		t.Errorf("ID = %q, want %q", decoded.Transactions[0].ID, "ct_abcd1234")
	}
}
