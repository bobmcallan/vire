package cashflow

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for currency field on CashAccount — service layer.
// Covers UpdateAccount currency changes, auto-create defaults, and ledger consistency.

// =============================================================================
// 1. UpdateAccount: setting Currency to "USD" is persisted and readable
// =============================================================================

func TestUpdateAccount_Currency_Persists(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Seed a transaction to materialise the ledger with a default Trading account.
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Initial deposit",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	// Update Trading account currency to "USD".
	ledger, err := svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{
		Currency: "USD",
	})
	if err != nil {
		t.Fatalf("UpdateAccount(currency=USD): %v", err)
	}

	acct := ledger.GetAccount("Trading")
	if acct == nil {
		t.Fatal("Trading account not found after UpdateAccount")
	}
	if acct.Currency != "USD" {
		t.Errorf("Currency = %q, want %q", acct.Currency, "USD")
	}

	// Re-fetch to confirm persistence (not just in-memory).
	refetch, err := svc.GetLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}
	persisted := refetch.GetAccount("Trading")
	if persisted == nil {
		t.Fatal("Trading account not found after re-fetch")
	}
	if persisted.Currency != "USD" {
		t.Errorf("persisted Currency = %q, want %q", persisted.Currency, "USD")
	}
}

// =============================================================================
// 2. UpdateAccount: empty Currency in update must NOT overwrite existing currency
// (merge semantics — empty string = not provided)
// =============================================================================

func TestUpdateAccount_EmptyCurrencyPreservesExisting(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Seed and set currency to EUR.
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      1000,
		Description: "seed",
	})
	_, err := svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{Currency: "EUR"})
	if err != nil {
		t.Fatalf("set EUR: %v", err)
	}

	// Now update only the Type — empty Currency should not reset to "".
	ledger, err := svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{
		Type: "accumulate",
		// Currency intentionally empty
	})
	if err != nil {
		t.Fatalf("UpdateAccount(type=accumulate): %v", err)
	}

	acct := ledger.GetAccount("Trading")
	if acct == nil {
		t.Fatal("Trading account not found")
	}
	if acct.Currency != "EUR" {
		t.Errorf("Currency = %q, want %q (empty update must not overwrite existing)", acct.Currency, "EUR")
	}
	if acct.Type != "accumulate" {
		t.Errorf("Type = %q, want %q", acct.Type, "accumulate")
	}
}

// =============================================================================
// 3. UpdateAccount: non-existent account returns error — not a panic
// =============================================================================

func TestUpdateAccount_NonExistentAccount_Currency(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	_, err := svc.UpdateAccount(ctx, "SMSF", "DoesNotExist", models.CashAccountUpdate{
		Currency: "USD",
	})
	if err == nil {
		t.Error("expected error when updating non-existent account currency")
	}
}

// =============================================================================
// 4. Auto-create via AddTransaction defaults to "AUD" currency
// =============================================================================

func TestAddTransaction_AutoCreate_DefaultsCurrencyToAUD(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	ledger, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "NewAccount",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Auto-create test",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	acct := ledger.GetAccount("NewAccount")
	if acct == nil {
		t.Fatal("auto-created account not found")
	}
	if acct.Currency != "AUD" {
		t.Errorf("auto-created Currency = %q, want %q (default must be AUD)", acct.Currency, "AUD")
	}
}

// =============================================================================
// 5. GetLedger default Trading account has AUD currency
// =============================================================================

func TestGetLedger_DefaultTradingAccount_HasAUDCurrency(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Brand new portfolio — no prior data.
	ledger, err := svc.GetLedger(ctx, "BrandNew")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}

	if len(ledger.Accounts) != 1 {
		t.Fatalf("expected 1 default account, got %d", len(ledger.Accounts))
	}
	if ledger.Accounts[0].Currency != "AUD" {
		t.Errorf("default Trading account Currency = %q, want %q", ledger.Accounts[0].Currency, "AUD")
	}
}

// =============================================================================
// 6. SetTransactions auto-creates accounts with "AUD" default
// =============================================================================

func TestSetTransactions_AutoCreate_DefaultsCurrencyToAUD(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	txs := []models.CashTransaction{
		{
			Account:     "FreshAccount",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      5000,
			Description: "Test",
		},
	}
	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	acct := ledger.GetAccount("FreshAccount")
	if acct == nil {
		t.Fatal("auto-created account not found after SetTransactions")
	}
	if acct.Currency != "AUD" {
		t.Errorf("auto-created Currency = %q, want %q", acct.Currency, "AUD")
	}
}

// =============================================================================
// 7. ClearLedger default Trading account has AUD currency (factory reset)
// =============================================================================

func TestClearLedger_DefaultTradingAccount_HasAUDCurrency(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// First set a USD currency on Trading.
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      1000,
		Description: "seed",
	})
	_, err := svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{Currency: "USD"})
	if err != nil {
		t.Fatalf("set USD: %v", err)
	}

	// Clear should reset to factory state (AUD).
	cleared, err := svc.ClearLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("ClearLedger: %v", err)
	}

	if len(cleared.Accounts) != 1 {
		t.Fatalf("expected 1 account after clear, got %d", len(cleared.Accounts))
	}
	if cleared.Accounts[0].Currency != "AUD" {
		t.Errorf("Currency after ClearLedger = %q, want %q (factory reset to AUD)", cleared.Accounts[0].Currency, "AUD")
	}
}

// =============================================================================
// 8. Summary per-currency after UpdateAccount currency change
// Changing an account's currency must retroactively affect the per-currency totals.
// =============================================================================

func TestSummary_ReflectsCurrencyAfterUpdate(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add a transaction to Trading (defaults to AUD).
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Initial deposit",
	})

	// Change Trading to USD.
	ledger, err := svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{Currency: "USD"})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}

	s := ledger.Summary()

	// After currency change, the 100000 should now be under "USD", not "AUD".
	if s.GrossCashBalanceByCurrency["AUD"] != 0 {
		t.Errorf("TotalCashByCurrency[AUD] = %v, want 0 (account changed to USD)", s.GrossCashBalanceByCurrency["AUD"])
	}
	if s.GrossCashBalanceByCurrency["USD"] != 100000 {
		t.Errorf("TotalCashByCurrency[USD] = %v, want 100000", s.GrossCashBalanceByCurrency["USD"])
	}
}

// =============================================================================
// 9. AddTransfer auto-creates both accounts with "AUD" default currency
// =============================================================================

func TestAddTransfer_AutoCreate_DefaultsCurrencyToAUD(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	ledger, err := svc.AddTransfer(ctx, "SMSF",
		"FromNew", "ToNew",
		5000,
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		"Transfer test",
	)
	if err != nil {
		t.Fatalf("AddTransfer: %v", err)
	}

	// Check both auto-created accounts have AUD default.
	for _, name := range []string{"FromNew", "ToNew"} {
		acct := ledger.GetAccount(name)
		if acct == nil {
			t.Fatalf("auto-created account %q not found", name)
		}
		if acct.Currency != "AUD" {
			t.Errorf("auto-created %q Currency = %q, want %q", name, acct.Currency, "AUD")
		}
	}
}

// =============================================================================
// 10. UpdateTransaction: moving a transaction to a new account auto-creates it
// with AUD default (not panics or leaves currency empty)
// =============================================================================

func TestUpdateTransaction_AutoCreate_DefaultsCurrencyToAUD(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add initial transaction.
	ledger, _ := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Initial",
	})
	txID := ledger.Transactions[0].ID

	// Update transaction to reference a new (non-existent) account.
	updated, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Account: "BrandNewAccount",
	})
	if err != nil {
		t.Fatalf("UpdateTransaction: %v", err)
	}

	acct := updated.GetAccount("BrandNewAccount")
	if acct == nil {
		t.Fatal("BrandNewAccount not auto-created")
	}
	if acct.Currency != "AUD" {
		t.Errorf("auto-created Currency = %q, want %q", acct.Currency, "AUD")
	}
}

// =============================================================================
// 11. Multi-currency portfolio — CalculatePerformance still works (no crash)
// CalculatePerformance doesn't use currency at all, but changing ledger structure
// must not break it.
// =============================================================================

func TestCalculatePerformance_MultiCurrencyLedger_NoCrash(t *testing.T) {
	svc, portfolioSvc := testService()
	ctx := testContext()

	portfolioSvc.portfolio = &models.Portfolio{
		Name:             "SMSF",
		PortfolioValue:   150000,
		GrossCashBalance: 0,
	}

	// AUD account
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "AUD deposit",
	})
	// Change to USD
	_, err := svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{Currency: "USD"})
	if err != nil {
		t.Fatalf("set USD: %v", err)
	}

	// Should not crash or return error.
	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance with multi-currency ledger: %v", err)
	}
	if perf == nil {
		t.Error("CalculatePerformance returned nil")
	}
}

// =============================================================================
// 12. GetLedger fallback account (empty accounts after unmarshal) gets AUD
// =============================================================================

func TestGetLedger_FallbackAccount_HasAUDCurrency(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add a transaction to create a ledger with accounts.
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      1000,
		Description: "seed",
	})

	// Verify the account has AUD currency.
	ledger, err := svc.GetLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}
	acct := ledger.GetAccount("Trading")
	if acct == nil {
		t.Fatal("Trading account not found")
	}
	if acct.Currency != "AUD" {
		t.Errorf("Currency = %q, want %q", acct.Currency, "AUD")
	}
}
