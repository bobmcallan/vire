package cashflow

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// --- Cash Flow Validation: hostile inputs ---

func TestValidation_ZeroAmount(t *testing.T) {
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Amount:      0,
		Description: "test",
	}
	if err := validateCashTransaction(tx); err == nil {
		t.Error("zero amount should be rejected")
	}
}

func TestValidation_AmountEdgeCases(t *testing.T) {
	base := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Description: "test",
	}

	tests := []struct {
		name   string
		amount float64
		valid  bool
	}{
		{"positive infinity", math.Inf(1), false},
		{"negative infinity", math.Inf(-1), false},
		{"NaN", math.NaN(), false},
		{"zero", 0, false},
		{"negative", -1, true},
		{"smallest positive subnormal", math.SmallestNonzeroFloat64, true},
		{"just below max", 1e15 - 1, true},
		{"exactly at max", 1e15, false},
		{"above max", 1e16, false},
		{"max float64", math.MaxFloat64, false},
		{"one cent", 0.01, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := base
			tx.Amount = tt.amount
			err := validateCashTransaction(tx)
			if tt.valid && err != nil {
				t.Errorf("amount %v should be valid, got error: %v", tt.amount, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("amount %v should be invalid", tt.amount)
			}
		})
	}
}

func TestValidation_DateEdgeCases(t *testing.T) {
	base := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Amount:      100,
		Description: "test",
	}

	tests := []struct {
		name  string
		date  time.Time
		valid bool
	}{
		{"zero time", time.Time{}, false},
		{"far past (year 1900)", time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC), true},
		{"year 1 (same as zero time)", time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), false}, // Go's zero time IS year 1
		{"yesterday", time.Now().Add(-24 * time.Hour), true},
		{"today", time.Now(), true},
		{"23 hours from now (within 24h grace)", time.Now().Add(23 * time.Hour), true},
		{"48 hours from now", time.Now().Add(48 * time.Hour), false},
		{"far future (year 3000)", time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := base
			tx.Date = tt.date
			err := validateCashTransaction(tx)
			if tt.valid && err != nil {
				t.Errorf("date %v should be valid, got error: %v", tt.date, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("date %v should be invalid", tt.date)
			}
		})
	}
}

func TestValidation_DescriptionEdgeCases(t *testing.T) {
	base := models.CashTransaction{
		Account:  "Trading",
		Category: models.CashCatContribution,
		Date:     time.Now().Add(-time.Hour),
		Amount:   100,
	}

	tests := []struct {
		name        string
		description string
		valid       bool
	}{
		{"empty", "", false},
		{"whitespace only", "   \t\n  ", false},
		{"normal", "Initial deposit", true},
		{"exactly 500 chars", strings.Repeat("x", 500), true},
		{"501 chars", strings.Repeat("x", 501), false},
		{"unicode", "Initial deposit \u00e9\u00e8\u00ea", true},
		{"html tags", "<script>alert(1)</script>", true}, // stored as data, not rendered
		{"null bytes", "test\x00hidden", true},           // JSON will handle encoding
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := base
			tx.Description = tt.description
			err := validateCashTransaction(tx)
			if tt.valid && err != nil {
				t.Errorf("description %q should be valid, got error: %v", tt.description, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("description %q should be invalid", tt.description)
			}
		})
	}
}

// --- Transaction ID security ---

func TestTransactionID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateCashTransactionID()
		if ids[id] {
			t.Fatalf("duplicate ID generated after %d iterations: %q", i, id)
		}
		ids[id] = true
	}
}

func TestTransactionID_Format(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := generateCashTransactionID()
		if !strings.HasPrefix(id, "ct_") {
			t.Errorf("ID should start with ct_, got %q", id)
		}
		if len(id) != 11 { // "ct_" + 8 hex chars
			t.Errorf("ID should be 11 chars, got %d: %q", len(id), id)
		}
	}
}

// --- XIRR edge cases ---

func TestXIRR_AllSameDate(t *testing.T) {
	// All transactions on the same date should not panic or hang
	// For XIRR: deposits = negative outflow
	d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	transactions := []models.CashTransaction{
		{Date: d, Amount: -50000},
		{Date: d, Amount: -50000},
		{Date: d, Amount: -50000},
	}
	// Should return 0 or a reasonable number, not NaN/Inf/panic
	rate := computeXIRR(transactions, 160000)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with same-date transactions returned %v, should be 0 or finite", rate)
	}
}

func TestXIRR_OnlyOutflows(t *testing.T) {
	// Only positive flows (withdrawals) — no negative flows → return 0
	transactions := []models.CashTransaction{
		{Date: time.Now().AddDate(-1, 0, 0), Amount: 50000},
	}
	// With no negative flows (deposits), should return 0
	rate := computeXIRR(transactions, 0)
	if rate != 0 {
		t.Errorf("XIRR with only outflows = %v, want 0", rate)
	}
}

func TestXIRR_ZeroPortfolioValue(t *testing.T) {
	transactions := []models.CashTransaction{
		{Date: time.Now().AddDate(-1, 0, 0), Amount: -100000},
	}
	// Portfolio lost everything
	rate := computeXIRR(transactions, 0)
	// Should be a large negative return (close to -100%)
	if rate > 0 {
		t.Errorf("XIRR with zero portfolio value = %v, should be negative", rate)
	}
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with zero portfolio value should be finite, got %v", rate)
	}
}

func TestXIRR_VeryLargeAmounts(t *testing.T) {
	transactions := []models.CashTransaction{
		{Date: time.Now().AddDate(-1, 0, 0), Amount: -1e14},
	}
	rate := computeXIRR(transactions, 1.1e14)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with large amounts should be finite, got %v", rate)
	}
}

func TestXIRR_SingleDayHolding(t *testing.T) {
	// Deposit yesterday, check performance today
	transactions := []models.CashTransaction{
		{Date: time.Now().AddDate(0, 0, -1), Amount: -100000},
	}
	rate := computeXIRR(transactions, 100100) // tiny gain
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR for single-day holding should be finite, got %v", rate)
	}
}

// --- Performance calculation edge cases ---

func TestPerformance_OnlyWithdrawals(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add only a withdrawal (edge case: no deposits)
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatOther,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -10000,
		Description: "Withdrawal",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.NetCapitalDeployed >= 0 {
		t.Logf("NetCapitalDeployed = %v (negative as expected for outflow-only)", perf.NetCapitalDeployed)
	}
	// SimpleReturnPct should be 0 when NetCapitalDeployed <= 0
	// Actually: netCapital = 0 - 10000 = -10000. The code checks if netCapital > 0.
	// So simpleReturnPct stays 0. Correct.
	if perf.SimpleCapitalReturnPct != 0 {
		t.Errorf("SimpleReturnPct with negative net capital = %v, want 0", perf.SimpleCapitalReturnPct)
	}
}

func TestPerformance_PortfolioNotFound(t *testing.T) {
	svc, portfolioSvc := testService()
	ctx := testContext()

	// Add a transaction so ledger is non-empty
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -50000,
		Description: "Deposit",
	})

	// Now make portfolio unavailable
	portfolioSvc.portfolio = nil

	_, err := svc.CalculatePerformance(ctx, "SMSF")
	if err == nil {
		t.Error("expected error when portfolio is not found")
	}
	if !strings.Contains(err.Error(), "portfolio") {
		t.Errorf("error should mention portfolio, got: %v", err)
	}
}

func TestPerformance_ZeroPortfolioValue(t *testing.T) {
	svc, portfolioSvc := testService()
	ctx := testContext()

	portfolioSvc.portfolio = &models.Portfolio{
		Name:       "SMSF",
		PortfolioValue: 0,
		GrossCashBalance:  0,
	}

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

	// Lost everything: simple return should be -100%
	if perf.SimpleCapitalReturnPct != -100 {
		t.Errorf("SimpleReturnPct = %v, want -100", perf.SimpleCapitalReturnPct)
	}
}

// --- Update merge semantics ---

func TestUpdate_CannotClearCategoryOrNotes(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add with category and notes
	ledger, _ := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatDividend,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Deposit",
		Notes:       "Q1 contribution",
	})
	txID := ledger.Transactions[0].ID

	// Update with empty category and notes — should NOT clear them
	ledger, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Category: "",
		Notes:    "",
	})
	if err != nil {
		t.Fatalf("UpdateTransaction: %v", err)
	}

	tx := ledger.Transactions[0]
	if tx.Category != models.CashCatDividend {
		t.Errorf("Category should be preserved, got %q", tx.Category)
	}
	if tx.Notes != "Q1 contribution" {
		t.Errorf("Notes should be preserved, got %q", tx.Notes)
	}
}

func TestUpdate_InvalidDirection(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	ledger, _ := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Deposit",
	})
	txID := ledger.Transactions[0].ID

	_, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Category: "evil_category",
	})
	if err == nil {
		t.Error("expected error for invalid category in update")
	}
}

func TestUpdate_FutureDate(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	ledger, _ := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Deposit",
	})
	txID := ledger.Transactions[0].ID

	_, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Date: time.Now().Add(48 * time.Hour),
	})
	if err == nil {
		t.Error("expected error for future date in update")
	}
}

func TestUpdate_InfiniteAmount(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	ledger, _ := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Deposit",
	})
	txID := ledger.Transactions[0].ID

	_, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Amount: math.Inf(1),
	})
	if err == nil {
		t.Error("expected error for infinite amount in update")
	}
}

func TestUpdate_NaNAmount(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	ledger, _ := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Deposit",
	})
	txID := ledger.Transactions[0].ID

	// NaN amount — the update check is `if update.Amount != 0`. NaN != 0 is true,
	// so NaN enters the validation path and is rejected as "must be finite".
	_, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Amount: math.NaN(),
	})
	if err == nil {
		t.Fatal("NaN amount should be rejected")
	}
	if !strings.Contains(err.Error(), "must be finite") {
		t.Errorf("expected 'must be finite' error, got: %v", err)
	}
}

// --- Ledger growth / DoS potential ---

func TestLedger_ManyTransactions(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add 100 transactions — verifies no crash, reasonable performance
	for i := 0; i < 100; i++ {
		_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * 24 * time.Hour),
			Amount:      1000,
			Description: "Monthly contribution",
		})
		if err != nil {
			t.Fatalf("AddTransaction at i=%d: %v", i, err)
		}
	}

	ledger, err := svc.GetLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}
	if len(ledger.Transactions) != 100 {
		t.Errorf("expected 100 transactions, got %d", len(ledger.Transactions))
	}

	// Performance calculation should still work
	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance with 100 transactions: %v", err)
	}
	if perf.TransactionCount != 100 {
		t.Errorf("TransactionCount = %d, want 100", perf.TransactionCount)
	}
}

// --- Cross-portfolio isolation ---

func TestLedger_PortfolioIsolation(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add to portfolio A
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "SMSF deposit",
	})

	// Add to portfolio B
	_, _ = svc.AddTransaction(ctx, "Personal", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Personal deposit",
	})

	// Read back — should be isolated
	smsfLedger, _ := svc.GetLedger(ctx, "SMSF")
	personalLedger, _ := svc.GetLedger(ctx, "Personal")

	if len(smsfLedger.Transactions) != 1 {
		t.Errorf("SMSF should have 1 transaction, got %d", len(smsfLedger.Transactions))
	}
	if len(personalLedger.Transactions) != 1 {
		t.Errorf("Personal should have 1 transaction, got %d", len(personalLedger.Transactions))
	}
	if smsfLedger.Transactions[0].Amount != 50000 {
		t.Errorf("SMSF transaction amount = %v, want 50000", smsfLedger.Transactions[0].Amount)
	}
	if personalLedger.Transactions[0].Amount != 10000 {
		t.Errorf("Personal transaction amount = %v, want 10000", personalLedger.Transactions[0].Amount)
	}
}

// --- Remove nonexistent ---

func TestRemove_EmptyLedger(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	_, err := svc.RemoveTransaction(ctx, "SMSF", "ct_nonexist")
	if err == nil {
		t.Error("expected error removing from empty ledger")
	}
}

// --- ClearLedger adversarial stress tests ---

func TestClearLedger_NonExistentPortfolioCreatesRecord(t *testing.T) {
	// ClearLedger on a portfolio with no prior ledger should succeed
	// and create a new empty ledger in storage (get-or-create pattern).
	svc, _ := testService()
	ctx := testContext()

	ledger, err := svc.ClearLedger(ctx, "NeverExisted")
	if err != nil {
		t.Fatalf("ClearLedger on non-existent portfolio: %v", err)
	}
	if ledger.PortfolioName != "NeverExisted" {
		t.Errorf("PortfolioName = %q, want %q", ledger.PortfolioName, "NeverExisted")
	}
	if len(ledger.Accounts) != 1 || ledger.Accounts[0].Name != models.DefaultTradingAccount {
		t.Errorf("expected only default Trading account, got %v", ledger.Accounts)
	}

	// Verify the ledger was persisted by re-fetching.
	refetch, err := svc.GetLedger(ctx, "NeverExisted")
	if err != nil {
		t.Fatalf("GetLedger after ClearLedger: %v", err)
	}
	if refetch.Version != 1 {
		t.Errorf("Version = %d, want 1 (first save increments from 0)", refetch.Version)
	}
}

func TestClearLedger_VersionIncrements(t *testing.T) {
	// Each ClearLedger call should increment the version.
	svc, _ := testService()
	ctx := testContext()

	// Create some data first.
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Now().Add(-time.Hour), Amount: 1000, Description: "seed",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	beforeLedger, _ := svc.GetLedger(ctx, "SMSF")
	versionBefore := beforeLedger.Version

	cleared, err := svc.ClearLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("ClearLedger: %v", err)
	}
	if cleared.Version != versionBefore+1 {
		t.Errorf("Version = %d, want %d (should increment)", cleared.Version, versionBefore+1)
	}
}

func TestClearLedger_NotesPreserved(t *testing.T) {
	// ClearLedger clears transactions and accounts but does NOT clear Notes.
	// Notes are portfolio-level metadata, not transaction data.
	svc, _ := testService()
	ctx := testContext()

	// Set notes via SetTransactions (which supports notes param).
	_, err := svc.SetTransactions(ctx, "SMSF", []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution,
			Date: time.Now().Add(-time.Hour), Amount: 1000, Description: "seed"},
	}, "Important portfolio context")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	cleared, err := svc.ClearLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("ClearLedger: %v", err)
	}

	// Notes should survive the clear.
	if cleared.Notes != "Important portfolio context" {
		t.Errorf("Notes = %q, want %q (notes should be preserved)", cleared.Notes, "Important portfolio context")
	}
}

func TestClearLedger_DefaultAccountIsTransactional(t *testing.T) {
	// After ClearLedger, the default Trading account must be IsTransactional: true.
	// Even if the user had previously changed it to false.
	svc, _ := testService()
	ctx := testContext()

	// First, add a transaction so the ledger exists.
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Now().Add(-time.Hour), Amount: 1000, Description: "seed",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	// Update Trading to non-transactional.
	_, err = svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{
		IsTransactional: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}

	// Clear and verify factory reset.
	cleared, err := svc.ClearLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("ClearLedger: %v", err)
	}
	if !cleared.Accounts[0].IsTransactional {
		t.Error("Default Trading account should be IsTransactional: true after ClearLedger (factory reset)")
	}
	if cleared.Accounts[0].Type != "trading" {
		t.Errorf("Default account Type = %q, want %q", cleared.Accounts[0].Type, "trading")
	}
}

func TestClearLedger_DoubleClearIdempotent(t *testing.T) {
	// Calling ClearLedger twice should be safe and produce consistent results.
	svc, _ := testService()
	ctx := testContext()

	// Populate then clear twice.
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Now().Add(-time.Hour), Amount: 5000, Description: "seed",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	first, err := svc.ClearLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("first ClearLedger: %v", err)
	}
	second, err := svc.ClearLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("second ClearLedger: %v", err)
	}

	// Both should have empty transactions and default account.
	if len(first.Transactions) != 0 || len(second.Transactions) != 0 {
		t.Error("expected empty transactions after both clears")
	}
	if len(first.Accounts) != 1 || len(second.Accounts) != 1 {
		t.Error("expected 1 account after both clears")
	}
	// Version should increment each time.
	if second.Version != first.Version+1 {
		t.Errorf("second Version = %d, want %d (should increment)", second.Version, first.Version+1)
	}
}

func TestClearLedger_SummaryAfterClear(t *testing.T) {
	// After clearing, the summary should show all zeros.
	svc, _ := testService()
	ctx := testContext()

	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account: "Trading", Category: models.CashCatContribution,
		Date: time.Now().Add(-time.Hour), Amount: 50000, Description: "big deposit",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	cleared, err := svc.ClearLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("ClearLedger: %v", err)
	}

	summary := cleared.Summary()
	if summary.GrossCashBalance != 0 {
		t.Errorf("TotalCash = %v, want 0 after clear", summary.GrossCashBalance)
	}
	if summary.TransactionCount != 0 {
		t.Errorf("TransactionCount = %d, want 0 after clear", summary.TransactionCount)
	}
	for cat, val := range summary.NetCashByCategory {
		if val != 0 {
			t.Errorf("ByCategory[%q] = %v, want 0 after clear", cat, val)
		}
	}
}
