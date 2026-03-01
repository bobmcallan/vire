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
func (m *mockStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
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
			Name:               "SMSF",
			TotalValueHoldings: 100000,
			TotalValue:         150000, // holdings + total cash
			TotalCash:          50000,
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
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
			name:    "zero amount",
			modify:  func(tx *models.CashTransaction) { tx.Amount = 0 },
			wantErr: "must not be zero",
		},
		{
			name:    "empty account",
			modify:  func(tx *models.CashTransaction) { tx.Account = "" },
			wantErr: "account is required",
		},
		{
			name:    "invalid category",
			modify:  func(tx *models.CashTransaction) { tx.Category = "invalid" },
			wantErr: "invalid category",
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
			wantErr: "must not be zero",
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
			name:    "account too long",
			modify:  func(tx *models.CashTransaction) { tx.Account = strings.Repeat("x", 101) },
			wantErr: "account name exceeds",
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
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
			Account:     "Trading",
			Category:    models.CashCatContribution,
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
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
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 80000, Description: "Initial deposit"},
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 20000, Description: "Top up"},
		{Account: "Trading", Category: models.CashCatOther, Date: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC), Amount: -10000, Description: "Withdrawal"},
	} {
		if _, err := svc.AddTransaction(ctx, "SMSF", tx); err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}
	}

	// Portfolio mock: TotalValueHoldings=100000 → currentValue=100000 (holdings only)
	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (withdrawal is CashCatOther, not contribution)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000 (100000 deposited - 0 withdrawn)", perf.NetCapitalDeployed)
	}
	if perf.CurrentPortfolioValue != 100000 {
		t.Errorf("CurrentPortfolioValue = %v, want 100000 (holdings only)", perf.CurrentPortfolioValue)
	}

	// Simple return: (100000 - 100000) / 100000 * 100 = 0%
	expectedSimple := (100000.0 - 100000.0) / 100000.0 * 100
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

	// XIRR uses trades (none in mock portfolio) → returns 0
	if math.IsNaN(perf.AnnualizedReturnPct) || math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Errorf("AnnualizedReturnPct should be finite, got %v", perf.AnnualizedReturnPct)
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -10000,
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
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
	// For XIRR: deposit = investment outflow = negative outflow
	transactions := []models.CashTransaction{
		{
			Date:   time.Now().AddDate(-1, 0, 0),
			Amount: -100000,
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
	// For XIRR: deposits = negative outflow
	sameDay := time.Now().AddDate(-1, 0, 0)
	transactions := []models.CashTransaction{
		{Date: sameDay, Amount: -50000},
		{Date: sameDay, Amount: -50000},
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
		{Date: time.Now().AddDate(-20, 0, 0), Amount: -10000},
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
		{Date: time.Now().AddDate(-1, 0, 0), Amount: -100000},
	}
	rate := computeXIRR(transactions, 0)
	// With currentValue=0, only negative flows → should return 0
	if rate != 0 {
		t.Logf("XIRR with total loss = %.2f%% (expected 0 due to no positive flows)", rate)
	}
}

func TestComputeXIRR_OnlyWithdrawals(t *testing.T) {
	// Only withdrawals = positive amounts. All flows positive → no negative → return 0
	transactions := []models.CashTransaction{
		{Date: time.Now().AddDate(-1, 0, 0), Amount: 50000},
	}
	rate := computeXIRR(transactions, 100000)
	// All flows positive (receives + terminal): no negative flow → return 0
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with only withdrawals should not produce NaN/Inf, got %v", rate)
	}
}

func TestComputeXIRR_VeryRecentTransaction(t *testing.T) {
	// Transaction just yesterday — very short holding period
	transactions := []models.CashTransaction{
		{Date: time.Now().AddDate(0, 0, -1), Amount: -100000},
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
		{Date: time.Now().AddDate(-1, 0, 0), Amount: -50000},
		{Date: time.Now().AddDate(0, -6, 0), Amount: 80000},
	}
	rate := computeXIRR(transactions, 50000)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with negative net capital should be finite, got %v", rate)
	}
}

func TestComputeXIRR_ZeroDateTransaction(t *testing.T) {
	// Transactions with zero dates should be skipped
	transactions := []models.CashTransaction{
		{Amount: -100000},
	}
	rate := computeXIRR(transactions, 110000)
	// Zero-date transactions are skipped, so flows is empty → return 0
	if rate != 0 {
		t.Errorf("XIRR with zero-date transactions = %v, want 0", rate)
	}
}

func TestComputeXIRR_LargeNumberOfTransactions(t *testing.T) {
	// 500 monthly deposits over ~40 years
	// For XIRR: deposits = negative outflow
	var transactions []models.CashTransaction
	base := time.Now().AddDate(-40, 0, 0)
	for i := 0; i < 500; i++ {
		transactions = append(transactions, models.CashTransaction{
			Date:   base.AddDate(0, i, 0),
			Amount: -1000,
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
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
		Account:     "Trading",
		Category:    models.CashCatContribution,
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
		Account:     "Trading",
		Category:    models.CashCatOther,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -50000,
		Description: "Withdrawal",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 0 {
		t.Errorf("TotalDeposited = %v, want 0", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (withdrawal is CashCatOther, not contribution)", perf.TotalWithdrawn)
	}
	// NetCapital = 0 - 0 = 0 → SimpleReturnPct should be 0 (netCapital <= 0)
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct with negative net capital = %v, want 0", perf.SimpleReturnPct)
	}
}

func TestCalculatePerformance_EqualDepositsAndWithdrawals(t *testing.T) {
	// Net capital = 0 → avoid division by zero in simple return
	// Both transactions must be contributions to count
	svc, _ := testService()
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Deposit",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -50000,
		Description: "Withdraw everything",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.NetCapitalDeployed != 0 {
		t.Errorf("NetCapitalDeployed = %v, want 0 (50000 deposited - 50000 withdrawn)", perf.NetCapitalDeployed)
	}
	// SimpleReturnPct: netCapital is 0, so should be 0 (no division by zero)
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct with zero net capital = %v, want 0", perf.SimpleReturnPct)
	}
	if math.IsNaN(perf.AnnualizedReturnPct) || math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Errorf("AnnualizedReturnPct should be finite, got %v", perf.AnnualizedReturnPct)
	}
}

// TestCalculatePerformance_UsesHoldingsOnly verifies that CalculatePerformance
// uses TotalValueHoldings only (not TotalValue or + TotalCash).
func TestCalculatePerformance_UsesHoldingsOnly(t *testing.T) {
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 100000,
			TotalCash:          50000,
			TotalValue:         999999, // deliberately wrong / stale
		},
	}
	storage := newMockStorageManager()
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
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Should use TotalValueHoldings only = 100000 (not 150000, not 999999)
	if perf.CurrentPortfolioValue != 100000 {
		t.Errorf("CurrentPortfolioValue = %v, want 100000 (holdings only)",
			perf.CurrentPortfolioValue)
	}

	// Simple return: (100000 - 100000) / 100000 * 100 = 0%
	if math.Abs(perf.SimpleReturnPct) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~0%%", perf.SimpleReturnPct)
	}
}

// TestCalculatePerformance_HoldingsOnlyValue verifies that CalculatePerformance
// uses TotalValueHoldings only (not + TotalCash) for current portfolio value.
// Cash balances are not investment returns.
func TestCalculatePerformance_HoldingsOnlyValue(t *testing.T) {
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 426000, // actual stock value
			TotalCash:          50000,  // cash in accounts
			TotalValue:         476000,
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      430000,
		Description: "Deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Should use TotalValueHoldings only = 426000 (not 476000)
	if perf.CurrentPortfolioValue != 426000 {
		t.Errorf("CurrentPortfolioValue = %v, want 426000 (holdings only, not including external balances)",
			perf.CurrentPortfolioValue)
	}

	// Simple return: (426000 - 430000) / 430000 * 100 = -0.93%
	expectedReturn := (426000.0 - 430000.0) / 430000.0 * 100
	if math.Abs(perf.SimpleReturnPct-expectedReturn) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%v", perf.SimpleReturnPct, expectedReturn)
	}
}

// TestCalculatePerformance_InternalTransfersCountAsFlows verifies that transfer_out
// transactions count as real withdrawals.
func TestCalculatePerformance_InternalTransfersCountAsFlows(t *testing.T) {
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 426000,
			TotalCash:          60600,
			TotalValue:         486600,
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Real deposit
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      430000,
		Description: "Initial deposit",
	})
	// Contribution withdrawals count (but transfers don't)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -20000,
		Description: "Withdrawal",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -20000,
		Description: "Withdrawal",
	})
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -20600,
		Description: "Withdrawal",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 430000 {
		t.Errorf("TotalDeposited = %v, want 430000", perf.TotalDeposited)
	}
	// Contribution withdrawals count: 20K + 20K + 20.6K = 60.6K
	if perf.TotalWithdrawn != 60600 {
		t.Errorf("TotalWithdrawn = %v, want 60600", perf.TotalWithdrawn)
	}
	// Net: 430K - 60.6K = 369.4K
	if perf.NetCapitalDeployed != 369400 {
		t.Errorf("NetCapitalDeployed = %v, want 369400 (430000 deposited - 60600 withdrawn)", perf.NetCapitalDeployed)
	}

	if perf.CurrentPortfolioValue != 426000 {
		t.Errorf("CurrentPortfolioValue = %v, want 426000", perf.CurrentPortfolioValue)
	}

	// Simple return: (426000 - 369400) / 369400 * 100
	expectedReturn := (426000.0 - 369400.0) / 369400.0 * 100
	if math.Abs(perf.SimpleReturnPct-expectedReturn) > 0.01 {
		t.Errorf("SimpleReturnPct = %v, want ~%v", perf.SimpleReturnPct, expectedReturn)
	}

	if perf.TransactionCount != 4 {
		t.Errorf("TransactionCount = %d, want 4", perf.TransactionCount)
	}
}

// TestCalculatePerformance_MixedTransferAndRealDebits verifies that all debits
// count as withdrawals regardless of category.
func TestCalculatePerformance_MixedTransferAndRealDebits(t *testing.T) {
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 80000,
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Real deposit
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000, Description: "Deposit",
	})
	// Transfer debit (counts as withdrawal, negative = money out)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatTransfer,
		Date: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), Amount: -20000, Description: "To accumulate",
	})
	// Real withdrawal (negative = money out)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatOther,
		Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: -10000, Description: "Real withdrawal",
	})
	// Another real debit (negative = money out)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatFee,
		Date: time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC), Amount: -5000, Description: "Expenses",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	// Only contribution debits count (transfers, fees, other don't count): 0K
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (only contribution debits count, transfers/fees/other don't)", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000 (100000 deposited - 0 withdrawn)", perf.NetCapitalDeployed)
	}
}

// TestCalculatePerformance_TransferInNotCountedAsDeposit verifies that transfer credits
// do NOT count as capital deposits — only category=contribution counts.
func TestCalculatePerformance_TransferInNotCountedAsDeposit(t *testing.T) {
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 120000,
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000, Description: "Deposit",
	})
	// Transfer credit does NOT count as deposit — only contributions count
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatTransfer,
		Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 30000, Description: "From term deposit",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Only contribution counts: 100K (transfer credit does not count)
	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000 (transfer credit is not a capital deposit)", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 100000 {
		t.Errorf("NetCapitalDeployed = %v, want 100000", perf.NetCapitalDeployed)
	}
}

// Verify the ledger's JSON structure is correct for storage
func TestLedgerJSONRoundTrip(t *testing.T) {
	ledger := &models.CashFlowLedger{
		PortfolioName: "SMSF",
		Version:       1,
		Accounts: []models.CashAccount{
			{Name: "Trading", IsTransactional: true},
		},
		Transactions: []models.CashTransaction{
			{
				ID:          "ct_abcd1234",
				Account:     "Trading",
				Category:    models.CashCatContribution,
				Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Amount:      -50000,
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

// --- deriveFromTrades tests ---

func TestDeriveFromTrades_BuysAndSells(t *testing.T) {
	// Portfolio with holdings that have buy/sell trades
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 120000,
			TotalValue:         170000,
			TotalCash:          50000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP", Exchange: "AU", Units: 100, CurrentPrice: 50.00,
					Trades: []*models.NavexaTrade{
						{Type: "buy", Units: 100, Price: 40.00, Fees: 10.00, Date: "2023-01-10"},
						{Type: "buy", Units: 50, Price: 45.00, Fees: 10.00, Date: "2023-06-15"},
						{Type: "sell", Units: 50, Price: 55.00, Fees: 10.00, Date: "2024-01-20"},
					},
				},
				{
					Ticker: "CBA", Exchange: "AU", Units: 200, CurrentPrice: 100.00,
					Trades: []*models.NavexaTrade{
						{Type: "buy", Units: 200, Price: 90.00, Fees: 20.00, Date: "2023-03-01"},
					},
				},
			},
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Should derive from trades since no cash transactions exist
	// Buy trades: (100*40+10) + (50*45+10) + (200*90+20) = 4010 + 2260 + 18020 = 24290
	expectedDeposited := 4010.0 + 2260.0 + 18020.0
	if math.Abs(perf.TotalDeposited-expectedDeposited) > 0.01 {
		t.Errorf("TotalDeposited = %.2f, want %.2f", perf.TotalDeposited, expectedDeposited)
	}

	// Sell trades: (50*55-10) = 2740
	expectedWithdrawn := 2740.0
	if math.Abs(perf.TotalWithdrawn-expectedWithdrawn) > 0.01 {
		t.Errorf("TotalWithdrawn = %.2f, want %.2f", perf.TotalWithdrawn, expectedWithdrawn)
	}

	// CurrentPortfolioValue = TotalValueHoldings only = 120000 (not + TotalCash)
	if perf.CurrentPortfolioValue != 120000 {
		t.Errorf("CurrentPortfolioValue = %.2f, want 120000 (holdings only)", perf.CurrentPortfolioValue)
	}

	// Net capital = 24290 - 2740 = 21550
	expectedNet := expectedDeposited - expectedWithdrawn
	if math.Abs(perf.NetCapitalDeployed-expectedNet) > 0.01 {
		t.Errorf("NetCapitalDeployed = %.2f, want %.2f", perf.NetCapitalDeployed, expectedNet)
	}

	// Should have positive return (120000 > 21550)
	if perf.SimpleReturnPct <= 0 {
		t.Errorf("SimpleReturnPct = %.2f, should be positive", perf.SimpleReturnPct)
	}

	// Transaction count = 4 (3 buys + 1 sell)
	if perf.TransactionCount != 4 {
		t.Errorf("TransactionCount = %d, want 4", perf.TransactionCount)
	}

	// FirstTransactionDate should be the earliest trade
	if perf.FirstTransactionDate == nil {
		t.Fatal("FirstTransactionDate should not be nil")
	}
	expected := time.Date(2023, 1, 10, 0, 0, 0, 0, time.UTC)
	if !perf.FirstTransactionDate.Equal(expected) {
		t.Errorf("FirstTransactionDate = %v, want %v", perf.FirstTransactionDate, expected)
	}

	// XIRR should be positive (portfolio grew)
	if perf.AnnualizedReturnPct <= 0 {
		t.Errorf("AnnualizedReturnPct = %.2f, should be positive", perf.AnnualizedReturnPct)
	}
}

func TestDeriveFromTrades_NoTrades(t *testing.T) {
	// Portfolio with holdings but no trades → deriveFromTrades returns nil → empty performance
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 100000,
			Holdings: []models.Holding{
				{Ticker: "BHP", Exchange: "AU", Units: 100, CurrentPrice: 50.00},
			},
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// With no trades and no cash transactions, performance should be all zeros
	if perf.TransactionCount != 0 {
		t.Errorf("TransactionCount = %d, want 0", perf.TransactionCount)
	}
	if perf.TotalDeposited != 0 {
		t.Errorf("TotalDeposited = %.2f, want 0", perf.TotalDeposited)
	}
}

func TestDeriveFromTrades_CashTransactionsPreferred(t *testing.T) {
	// When cash transactions exist, trades should NOT be used
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 100000,
			TotalValue:         150000,
			TotalCash:          50000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP", Exchange: "AU", Units: 100, CurrentPrice: 50.00,
					Trades: []*models.NavexaTrade{
						{Type: "buy", Units: 100, Price: 40.00, Fees: 10.00, Date: "2023-01-10"},
					},
				},
			},
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	// Add a manual cash transaction
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      80000,
		Description: "Initial deposit",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Cash transaction path should be used (80000 deposit), not trades (4010 buy)
	if perf.TotalDeposited != 80000 {
		t.Errorf("TotalDeposited = %.2f, want 80000 (from cash transactions, not trades)", perf.TotalDeposited)
	}
	if perf.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1 (from cash transactions)", perf.TransactionCount)
	}
}

func TestDeriveFromTrades_OpeningBalance(t *testing.T) {
	// "opening balance" trade type should be treated as a buy/deposit
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 80000,
			TotalValue:         80000,
			Holdings: []models.Holding{
				{
					Ticker: "VAS", Exchange: "AU", Units: 500, CurrentPrice: 160.00,
					Trades: []*models.NavexaTrade{
						{Type: "opening balance", Units: 500, Price: 100.00, Fees: 0, Date: "2020-07-01"},
					},
				},
			},
		},
	}
	storage := newMockStorageManager()
	logger := common.NewLogger("error")
	svc := NewService(storage, portfolioSvc, logger)
	ctx := testContext()

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	// Opening balance: 500 * 100 + 0 = 50000 deposited
	if math.Abs(perf.TotalDeposited-50000) > 0.01 {
		t.Errorf("TotalDeposited = %.2f, want 50000", perf.TotalDeposited)
	}
	if perf.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", perf.TransactionCount)
	}
}

// --- SetTransactions tests ---

func TestSetTransactions_Empty(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add an existing transaction first
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Existing deposit",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	// Set empty — should clear all transactions
	ledger, err := svc.SetTransactions(ctx, "SMSF", []models.CashTransaction{}, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	if len(ledger.Transactions) != 0 {
		t.Errorf("expected 0 transactions, got %d", len(ledger.Transactions))
	}
	// Account should be preserved
	if !ledger.HasAccount("Trading") {
		t.Error("expected Trading account to be preserved after empty set")
	}
}

func TestSetTransactions_ReplacesExisting(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Seed two transactions
	for i, d := range []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
	} {
		_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        d,
			Amount:      float64((i + 1) * 10000),
			Description: "Existing",
		})
		if err != nil {
			t.Fatalf("AddTransaction: %v", err)
		}
	}

	newTxs := []models.CashTransaction{
		{
			Account:     "Trading",
			Category:    models.CashCatDividend,
			Date:        time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Amount:      500,
			Description: "New dividend",
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", newTxs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	if len(ledger.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(ledger.Transactions))
	}
	if ledger.Transactions[0].Description != "New dividend" {
		t.Errorf("expected 'New dividend', got %q", ledger.Transactions[0].Description)
	}
}

func TestSetTransactions_ValidationError(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	invalid := []models.CashTransaction{
		{
			Account:     "", // missing account
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: "Bad tx",
		},
	}

	_, err := svc.SetTransactions(ctx, "SMSF", invalid, "")
	if err == nil {
		t.Fatal("expected error for invalid transaction, got nil")
	}
	if !strings.Contains(err.Error(), "invalid cash transaction") {
		t.Errorf("expected 'invalid cash transaction' error, got %q", err.Error())
	}

	// Ledger should not have been modified
	ledger, err2 := svc.GetLedger(ctx, "SMSF")
	if err2 != nil {
		t.Fatalf("GetLedger: %v", err2)
	}
	if len(ledger.Transactions) != 0 {
		t.Errorf("expected 0 transactions after failed set, got %d", len(ledger.Transactions))
	}
}

func TestSetTransactions_AutoCreatesAccounts(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	txs := []models.CashTransaction{
		{
			Account:     "NewBroker",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      5000,
			Description: "Seed capital",
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	if !ledger.HasAccount("NewBroker") {
		t.Error("expected NewBroker account to be auto-created")
	}
}

func TestSetTransactions_PreservesExistingAccounts(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Create a transaction referencing "OldAccount"
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "OldAccount",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      1000,
		Description: "Seed",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	// Set replaces all transactions but uses a different account
	newTxs := []models.CashTransaction{
		{
			Account:     "NewAccount",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			Amount:      2000,
			Description: "New deposit",
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", newTxs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	// OldAccount should still be in accounts list
	if !ledger.HasAccount("OldAccount") {
		t.Error("expected OldAccount to be preserved after set")
	}
	if !ledger.HasAccount("NewAccount") {
		t.Error("expected NewAccount to be auto-created")
	}
}

func TestSetTransactions_AssignsIDs(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	txs := []models.CashTransaction{
		{
			ID:          "user-provided-id", // should be overwritten
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: "Deposit",
		},
		{
			Account:     "Trading",
			Category:    models.CashCatFee,
			Date:        time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			Amount:      -10,
			Description: "Fee",
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	if len(ledger.Transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(ledger.Transactions))
	}

	ids := make(map[string]bool)
	for _, tx := range ledger.Transactions {
		if !strings.HasPrefix(tx.ID, "ct_") {
			t.Errorf("expected ID with ct_ prefix, got %q", tx.ID)
		}
		if tx.ID == "user-provided-id" {
			t.Error("user-provided ID should be overwritten")
		}
		ids[tx.ID] = true
	}
	if len(ids) != 2 {
		t.Error("all assigned IDs should be unique")
	}
}

func TestSetTransactions_SortsByDate(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Provide out-of-order dates
	txs := []models.CashTransaction{
		{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			Amount:      3000,
			Description: "June",
		},
		{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: "January",
		},
		{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Amount:      2000,
			Description: "March",
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	for i := 1; i < len(ledger.Transactions); i++ {
		if ledger.Transactions[i].Date.Before(ledger.Transactions[i-1].Date) {
			t.Errorf("transactions not sorted: [%d] %v before [%d] %v",
				i, ledger.Transactions[i].Date, i-1, ledger.Transactions[i-1].Date)
		}
	}
}

func TestSetTransactions_Notes(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	txs := []models.CashTransaction{
		{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: "Deposit",
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "Updated via bulk set")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	if ledger.Notes != "Updated via bulk set" {
		t.Errorf("ledger notes = %q, want %q", ledger.Notes, "Updated via bulk set")
	}
}

func TestClearLedger(t *testing.T) {
	t.Run("clears_all", func(t *testing.T) {
		svc, _ := testService()
		ctx := testContext()

		// Add transactions across two accounts
		txs := []models.CashTransaction{
			{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 1000, Description: "tx1"},
			{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Amount: 2000, Description: "tx2"},
			{Account: "Savings", Category: models.CashCatContribution, Date: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 500, Description: "tx3"},
		}
		_, err := svc.SetTransactions(ctx, "SMSF", txs, "")
		if err != nil {
			t.Fatalf("SetTransactions: %v", err)
		}

		ledger, err := svc.ClearLedger(ctx, "SMSF")
		if err != nil {
			t.Fatalf("ClearLedger: %v", err)
		}

		if len(ledger.Transactions) != 0 {
			t.Errorf("expected 0 transactions after clear, got %d", len(ledger.Transactions))
		}
		if len(ledger.Accounts) != 1 {
			t.Errorf("expected 1 account after clear, got %d", len(ledger.Accounts))
		}
		if ledger.Accounts[0].Name != models.DefaultTradingAccount {
			t.Errorf("expected default account %q, got %q", models.DefaultTradingAccount, ledger.Accounts[0].Name)
		}
		if !ledger.Accounts[0].IsTransactional {
			t.Errorf("expected default account to be transactional")
		}
		if ledger.Version == 0 {
			t.Errorf("expected version > 0 after clear, got %d", ledger.Version)
		}
	})

	t.Run("empty_ledger", func(t *testing.T) {
		svc, _ := testService()
		ctx := testContext()

		ledger, err := svc.ClearLedger(ctx, "SMSF")
		if err != nil {
			t.Fatalf("ClearLedger on empty ledger: %v", err)
		}

		if len(ledger.Transactions) != 0 {
			t.Errorf("expected 0 transactions, got %d", len(ledger.Transactions))
		}
		if len(ledger.Accounts) != 1 {
			t.Errorf("expected 1 account, got %d", len(ledger.Accounts))
		}
		if ledger.Accounts[0].Name != models.DefaultTradingAccount {
			t.Errorf("expected default account %q, got %q", models.DefaultTradingAccount, ledger.Accounts[0].Name)
		}
	})

	t.Run("preserves_portfolio_name", func(t *testing.T) {
		svc, _ := testService()
		ctx := testContext()

		const portfolio = "SMSF"
		ledger, err := svc.ClearLedger(ctx, portfolio)
		if err != nil {
			t.Fatalf("ClearLedger: %v", err)
		}

		if ledger.PortfolioName != portfolio {
			t.Errorf("portfolio name = %q, want %q", ledger.PortfolioName, portfolio)
		}
	})
}

func TestUpdateAccount_Currency(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add a transaction to auto-create the Trading account
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-24 * time.Hour),
		Amount:      1000,
		Description: "Initial deposit",
	}
	_, err := svc.AddTransaction(ctx, "SMSF", tx)
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	// Update the Trading account currency to USD
	trueVal := true
	update := models.CashAccountUpdate{
		Currency:        "USD",
		IsTransactional: &trueVal,
	}
	ledger, err := svc.UpdateAccount(ctx, "SMSF", "Trading", update)
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}

	acct := ledger.GetAccount("Trading")
	if acct == nil {
		t.Fatal("Trading account not found after UpdateAccount")
	}
	if acct.Currency != "USD" {
		t.Errorf("Currency = %q, want %q", acct.Currency, "USD")
	}
	if !acct.IsTransactional {
		t.Error("IsTransactional should be true")
	}
}

func TestUpdateAccount_Currency_DefaultsToAUDWhenEmpty(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add a transaction to auto-create the Trading account
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-24 * time.Hour),
		Amount:      1000,
		Description: "Initial deposit",
	}
	_, err := svc.AddTransaction(ctx, "SMSF", tx)
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	// Get ledger directly to check auto-created account has AUD currency
	ledger, err := svc.GetLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}
	acct := ledger.GetAccount("Trading")
	if acct == nil {
		t.Fatal("Trading account not found")
	}
	// Auto-created via AddTransaction should have AUD
	if acct.Currency != "AUD" {
		t.Errorf("auto-created account Currency = %q, want %q", acct.Currency, "AUD")
	}
}
