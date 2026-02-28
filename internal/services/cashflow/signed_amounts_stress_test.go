package cashflow

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for signed amounts redesign.
// Covers: zero rejection, negative amounts, signed semantics, NonTransactionalBalance,
// UpdateAccount validation, transfers with signed amounts, XIRR edge cases,
// old stored data with Direction field, and UpdateTransaction merge semantics.

// =============================================================================
// 1. Amount = 0 rejection
// =============================================================================

func TestSignedAmounts_ZeroAmountRejected(t *testing.T) {
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Amount:      0,
		Description: "test",
	}
	err := validateCashTransaction(tx)
	if err == nil {
		t.Fatal("amount=0 should be rejected")
	}
	if !strings.Contains(err.Error(), "zero") {
		t.Errorf("error should mention zero, got: %v", err)
	}
}

func TestSignedAmounts_NegativeAmountAccepted(t *testing.T) {
	// Negative amounts are now valid (debits)
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Amount:      -500,
		Description: "withdrawal",
	}
	err := validateCashTransaction(tx)
	if err != nil {
		t.Errorf("negative amount should be valid for signed amounts, got: %v", err)
	}
}

func TestSignedAmounts_PositiveAmountAccepted(t *testing.T) {
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Amount:      500,
		Description: "deposit",
	}
	err := validateCashTransaction(tx)
	if err != nil {
		t.Errorf("positive amount should be valid, got: %v", err)
	}
}

// =============================================================================
// 2. Very large/small amounts — overflow, precision
// =============================================================================

func TestSignedAmounts_NegativeInfinity(t *testing.T) {
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Amount:      math.Inf(-1),
		Description: "test",
	}
	err := validateCashTransaction(tx)
	if err == nil {
		t.Fatal("negative infinity should be rejected")
	}
}

func TestSignedAmounts_NegativeNearMax(t *testing.T) {
	// Just under the negative max boundary
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Amount:      -(1e15 - 1),
		Description: "large withdrawal",
	}
	err := validateCashTransaction(tx)
	if err != nil {
		t.Errorf("amount -%v should be valid, got: %v", 1e15-1, err)
	}
}

func TestSignedAmounts_NegativeAtMax(t *testing.T) {
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Amount:      -1e15,
		Description: "test",
	}
	err := validateCashTransaction(tx)
	if err == nil {
		t.Fatal("amount=-1e15 should be rejected (at max)")
	}
}

func TestSignedAmounts_SmallestNegative(t *testing.T) {
	tx := models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Amount:      -math.SmallestNonzeroFloat64,
		Description: "tiny withdrawal",
	}
	err := validateCashTransaction(tx)
	if err != nil {
		t.Errorf("smallest negative amount should be valid, got: %v", err)
	}
}

// =============================================================================
// 3. Mixed sign amounts in same account — balance correctness
// =============================================================================

func TestSignedAmounts_MixedSignBalance(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Deposit +1000
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      1000,
		Description: "Deposit",
	})
	if err != nil {
		t.Fatalf("AddTransaction +1000: %v", err)
	}

	// Withdrawal -300
	_, err = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatOther,
		Date:        time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -300,
		Description: "Withdrawal",
	})
	if err != nil {
		t.Fatalf("AddTransaction -300: %v", err)
	}

	// Another deposit +500
	_, err = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		Amount:      500,
		Description: "Second deposit",
	})
	if err != nil {
		t.Fatalf("AddTransaction +500: %v", err)
	}

	ledger, err := svc.GetLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}

	// Balance should be 1000 + (-300) + 500 = 1200
	balance := ledger.AccountBalance("Trading")
	if balance != 1200 {
		t.Errorf("AccountBalance = %v, want 1200", balance)
	}

	// TotalDeposited = sum of positive amounts = 1000 + 500 = 1500
	if ledger.TotalDeposited() != 1500 {
		t.Errorf("TotalDeposited = %v, want 1500", ledger.TotalDeposited())
	}

	// TotalWithdrawn = |sum of negative amounts| = |-300| = 300
	if ledger.TotalWithdrawn() != 300 {
		t.Errorf("TotalWithdrawn = %v, want 300", ledger.TotalWithdrawn())
	}
}

// =============================================================================
// 4. NonTransactionalBalance — edge cases
// =============================================================================

func TestNonTransactionalBalance_Empty(t *testing.T) {
	ledger := &models.CashFlowLedger{
		Accounts:     []models.CashAccount{{Name: "Trading", Type: "trading", IsTransactional: true}},
		Transactions: []models.CashTransaction{},
	}
	if ledger.NonTransactionalBalance() != 0 {
		t.Errorf("NonTransactionalBalance with empty txns = %v, want 0", ledger.NonTransactionalBalance())
	}
}

func TestNonTransactionalBalance_AllTransactional(t *testing.T) {
	ledger := &models.CashFlowLedger{
		Accounts: []models.CashAccount{
			{Name: "Trading", Type: "trading", IsTransactional: true},
		},
		Transactions: []models.CashTransaction{
			{Account: "Trading", Amount: 50000},
			{Account: "Trading", Amount: -10000},
		},
	}
	// No non-transactional accounts -> balance = 0
	if ledger.NonTransactionalBalance() != 0 {
		t.Errorf("NonTransactionalBalance = %v, want 0 (all accounts are transactional)", ledger.NonTransactionalBalance())
	}
}

func TestNonTransactionalBalance_MixedAccounts(t *testing.T) {
	ledger := &models.CashFlowLedger{
		Accounts: []models.CashAccount{
			{Name: "Trading", Type: "trading", IsTransactional: true},
			{Name: "Accumulate", Type: "accumulate", IsTransactional: false},
			{Name: "Term Deposit", Type: "term_deposit", IsTransactional: false},
		},
		Transactions: []models.CashTransaction{
			{Account: "Trading", Amount: 100000},      // transactional - excluded
			{Account: "Accumulate", Amount: 30000},    // non-transactional
			{Account: "Accumulate", Amount: -5000},    // non-transactional
			{Account: "Term Deposit", Amount: 50000},  // non-transactional
			{Account: "Term Deposit", Amount: -10000}, // non-transactional
		},
	}
	// Non-transactional balance: 30000 + (-5000) + 50000 + (-10000) = 65000
	expected := 65000.0
	got := ledger.NonTransactionalBalance()
	if got != expected {
		t.Errorf("NonTransactionalBalance = %v, want %v", got, expected)
	}
}

func TestNonTransactionalBalance_NegativeResult(t *testing.T) {
	// If more debits than credits in non-transactional accounts
	ledger := &models.CashFlowLedger{
		Accounts: []models.CashAccount{
			{Name: "Savings", Type: "accumulate", IsTransactional: false},
		},
		Transactions: []models.CashTransaction{
			{Account: "Savings", Amount: 1000},
			{Account: "Savings", Amount: -3000},
		},
	}
	// Balance = 1000 + (-3000) = -2000
	got := ledger.NonTransactionalBalance()
	if got != -2000 {
		t.Errorf("NonTransactionalBalance = %v, want -2000 (negative balance is valid)", got)
	}
}

func TestNonTransactionalBalance_OrphanTransactions(t *testing.T) {
	// Transactions for an account that doesn't appear in Accounts slice
	// This should NOT count — the account lookup uses the Accounts slice
	ledger := &models.CashFlowLedger{
		Accounts: []models.CashAccount{
			{Name: "Trading", Type: "trading", IsTransactional: true},
		},
		Transactions: []models.CashTransaction{
			{Account: "Ghost Account", Amount: 50000}, // orphan
		},
	}
	// "Ghost Account" is not in Accounts, so it won't be in the nonTx map
	got := ledger.NonTransactionalBalance()
	if got != 0 {
		t.Errorf("NonTransactionalBalance with orphan txn = %v, want 0", got)
	}
}

// =============================================================================
// 5. UpdateAccount — validation edge cases
// =============================================================================

func TestUpdateAccount_NotFound(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	_, err := svc.UpdateAccount(ctx, "SMSF", "NonExistent", models.CashAccountUpdate{Type: "accumulate"})
	if err == nil {
		t.Fatal("UpdateAccount on non-existent account should fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestUpdateAccount_EmptyType(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Default Trading account exists. Update with empty type -> type unchanged
	ledger, err := svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{Type: ""})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}

	acct := ledger.GetAccount("Trading")
	if acct == nil {
		t.Fatal("Trading account should exist")
	}
	// Empty type should NOT overwrite existing type (merge semantics)
	// The current implementation only sets if Type != ""
}

func TestUpdateAccount_IsTransactionalPreservedWhenOmitted(t *testing.T) {
	// Verify that updating only Type preserves the existing IsTransactional value.
	// Uses *bool so nil means "not provided" (no change), distinguishing from false.
	svc, _ := testService()
	ctx := testContext()

	// Trading starts as IsTransactional=true
	ledger, err := svc.GetLedger(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetLedger: %v", err)
	}
	acct := ledger.GetAccount("Trading")
	if !acct.IsTransactional {
		t.Fatal("Trading should start as transactional")
	}

	// Update only the Type — IsTransactional is nil (not provided)
	ledger, err = svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{
		Type: "trading",
		// IsTransactional not set -> nil -> no change
	})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}

	acct = ledger.GetAccount("Trading")
	if !acct.IsTransactional {
		t.Error("IsTransactional should remain true when not provided in update")
	}
}

func TestUpdateAccount_SetNonTransactional(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Create a non-transactional account
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Savings",
		Category:    models.CashCatContribution,
		Date:        time.Now().Add(-time.Hour),
		Amount:      1000,
		Description: "deposit",
	})

	// Update type and set non-transactional
	boolFalse := false
	ledger, err := svc.UpdateAccount(ctx, "SMSF", "Savings", models.CashAccountUpdate{
		Type:            "accumulate",
		IsTransactional: &boolFalse,
	})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}

	acct := ledger.GetAccount("Savings")
	if acct.Type != "accumulate" {
		t.Errorf("Type = %q, want accumulate", acct.Type)
	}
	if acct.IsTransactional {
		t.Error("IsTransactional should be false")
	}
}

func TestUpdateAccount_InvalidType(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	_, err := svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{Type: "invalid_type"})
	if err == nil {
		t.Fatal("UpdateAccount with invalid type should fail")
	}
	if !strings.Contains(err.Error(), "invalid account type") {
		t.Errorf("error should mention invalid account type, got: %v", err)
	}
}

func TestUpdateAccount_ValidTypes(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	for _, typ := range []string{"trading", "accumulate", "term_deposit", "offset", "other"} {
		_, err := svc.UpdateAccount(ctx, "SMSF", "Trading", models.CashAccountUpdate{Type: typ})
		if err != nil {
			t.Errorf("UpdateAccount with type %q should succeed, got: %v", typ, err)
		}
	}
}

// =============================================================================
// 6. Transfer with signed amounts
// =============================================================================

func TestTransfer_CreatesSignedEntries(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	ledger, err := svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", 5000,
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), "Move to accumulate")
	if err != nil {
		t.Fatalf("AddTransfer: %v", err)
	}

	if len(ledger.Transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(ledger.Transactions))
	}

	// Find from/to entries
	var fromTx, toTx *models.CashTransaction
	for i := range ledger.Transactions {
		if ledger.Transactions[i].Account == "Trading" {
			fromTx = &ledger.Transactions[i]
		} else if ledger.Transactions[i].Account == "Accumulate" {
			toTx = &ledger.Transactions[i]
		}
	}

	if fromTx == nil || toTx == nil {
		t.Fatal("should have both from and to transactions")
	}

	// from_account gets negative amount
	if fromTx.Amount != -5000 {
		t.Errorf("from_account Amount = %v, want -5000", fromTx.Amount)
	}

	// to_account gets positive amount
	if toTx.Amount != 5000 {
		t.Errorf("to_account Amount = %v, want 5000", toTx.Amount)
	}

	// Both should be linked
	if fromTx.LinkedID == "" || toTx.LinkedID == "" {
		t.Error("both transactions should have LinkedID set")
	}
	if fromTx.LinkedID != toTx.ID || toTx.LinkedID != fromTx.ID {
		t.Error("LinkedID should reference each other")
	}

	// Net balance should be zero across both accounts
	balance := ledger.TotalCashBalance()
	if balance != 0 {
		t.Errorf("TotalCashBalance after transfer = %v, want 0", balance)
	}
}

func TestTransfer_ZeroAmountRejected(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	_, err := svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", 0,
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), "Zero transfer")
	if err == nil {
		t.Fatal("transfer with amount=0 should be rejected")
	}
}

func TestTransfer_NegativeAmountRejected(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Transfer amount must be positive — the service uses abs(amount) internally
	_, err := svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", -5000,
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), "Negative transfer")
	if err == nil {
		t.Fatal("transfer with negative amount should be rejected")
	}
}

func TestTransfer_SameAccountRejected(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	_, err := svc.AddTransfer(ctx, "SMSF", "Trading", "Trading", 5000,
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), "Self transfer")
	if err == nil {
		t.Fatal("self-transfer should be rejected")
	}
}

// =============================================================================
// 7. XIRR with edge case cash flows
// =============================================================================

func TestXIRR_AllNegativeFlows(t *testing.T) {
	// All negative amounts (outflows) — no positive inflows or terminal value
	transactions := []models.CashTransaction{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: -100000},
		{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: -50000},
	}
	// currentValue = 0 → no positive flow → returns 0
	rate := computeXIRR(transactions, 0)
	if rate != 0 {
		t.Errorf("XIRR with only negative flows and zero value = %v, want 0", rate)
	}
	if math.IsNaN(rate) {
		t.Error("XIRR should not be NaN")
	}
}

func TestXIRR_AllPositiveFlows(t *testing.T) {
	// All positive amounts (inflows) — no outflows
	transactions := []models.CashTransaction{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 50000},
		{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Amount: 25000},
	}
	// No negative flows → returns 0
	rate := computeXIRR(transactions, 100000)
	if rate != 0 {
		t.Errorf("XIRR with only positive flows = %v, want 0", rate)
	}
}

func TestXIRR_SignedAmountsCorrectSign(t *testing.T) {
	// Buy (negative) followed by sell (positive) with gain
	transactions := []models.CashTransaction{
		{Date: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Amount: -100000}, // buy
	}
	// Portfolio now worth 120000
	rate := computeXIRR(transactions, 120000)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR should be finite, got %v", rate)
	}
	if rate <= 0 {
		t.Errorf("XIRR should be positive (20%% gain), got %v", rate)
	}
}

func TestXIRR_SingleDayHolding_Signed(t *testing.T) {
	// Bought yesterday, check today
	transactions := []models.CashTransaction{
		{Date: time.Now().Add(-24 * time.Hour), Amount: -100000},
	}
	rate := computeXIRR(transactions, 100100) // tiny gain
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR for single-day holding should be finite, got %v", rate)
	}
}

func TestXIRR_ZeroPortfolioValue_Signed(t *testing.T) {
	// Total loss
	transactions := []models.CashTransaction{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Amount: -100000},
	}
	rate := computeXIRR(transactions, 0)
	// Should be a large negative return (close to -100%)
	if rate > 0 {
		t.Errorf("XIRR with zero portfolio value = %v, should be negative", rate)
	}
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		t.Errorf("XIRR with zero portfolio value should be finite, got %v", rate)
	}
}

// =============================================================================
// 8. Old stored data with Direction field — JSON deserialization
// =============================================================================

func TestOldDataWithDirection_SilentlyDropped(t *testing.T) {
	// Simulate old stored data that has a "direction" field.
	// After Direction removal, json.Unmarshal should silently ignore it.
	oldJSON := `{
		"portfolio_name": "SMSF",
		"version": 5,
		"accounts": [{"name": "Trading", "is_transactional": true}],
		"transactions": [
			{
				"id": "ct_aabbccdd",
				"direction": "credit",
				"account": "Trading",
				"category": "contribution",
				"date": "2025-01-01T00:00:00Z",
				"amount": 50000,
				"description": "Old deposit"
			},
			{
				"id": "ct_eeff0011",
				"direction": "debit",
				"account": "Trading",
				"category": "other",
				"date": "2025-02-01T00:00:00Z",
				"amount": 10000,
				"description": "Old withdrawal"
			}
		]
	}`

	var ledger models.CashFlowLedger
	err := json.Unmarshal([]byte(oldJSON), &ledger)
	if err != nil {
		t.Fatalf("Unmarshal of old data should succeed: %v", err)
	}

	if len(ledger.Transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(ledger.Transactions))
	}

	// Both amounts stay positive (old data had "amount: always positive, direction determines sign")
	// With Direction removed, SignedAmount() = Amount = positive
	// Old debit with positive amount now LOOKS like a credit
	// This is the expected (and documented) behavior — old data needs re-entry
	tx0 := ledger.Transactions[0]
	if tx0.Amount != 50000 {
		t.Errorf("Old credit amount = %v, want 50000 (preserved)", tx0.Amount)
	}
	if tx0.SignedAmount() != 50000 {
		t.Errorf("Old credit SignedAmount = %v, want 50000", tx0.SignedAmount())
	}

	tx1 := ledger.Transactions[1]
	if tx1.Amount != 10000 {
		t.Errorf("Old debit amount = %v, want 10000 (preserved)", tx1.Amount)
	}
	// This is the key issue: old debit now appears as a credit (+10000)
	// because Direction is dropped and Amount stays positive
	if tx1.SignedAmount() != 10000 {
		t.Errorf("Old debit SignedAmount = %v, want 10000 (old data treated as credit)", tx1.SignedAmount())
	}

	// TotalDeposited = 50000 + 10000 = 60000 (both look like credits)
	if ledger.TotalDeposited() != 60000 {
		t.Errorf("TotalDeposited for old data = %v, want 60000 (both positive)", ledger.TotalDeposited())
	}

	// TotalWithdrawn = 0 (no negative amounts)
	if ledger.TotalWithdrawn() != 0 {
		t.Errorf("TotalWithdrawn for old data = %v, want 0", ledger.TotalWithdrawn())
	}
}

// =============================================================================
// 9. UpdateTransaction with signed amounts — merge semantics
// =============================================================================

func TestUpdateTransaction_NegativeAmount(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Add a positive transaction
	ledger, _ := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Deposit",
	})
	txID := ledger.Transactions[0].ID

	// Update to negative amount (change credit to debit)
	ledger, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Amount: -5000,
	})
	if err != nil {
		t.Fatalf("UpdateTransaction to negative amount: %v", err)
	}

	if ledger.Transactions[0].Amount != -5000 {
		t.Errorf("Amount after update = %v, want -5000", ledger.Transactions[0].Amount)
	}
}

func TestUpdateTransaction_ZeroAmountSkipped(t *testing.T) {
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

	// Amount=0 means "don't update" in merge semantics
	ledger, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Amount: 0,
	})
	if err != nil {
		t.Fatalf("UpdateTransaction with amount=0: %v", err)
	}

	// Amount should be unchanged
	if ledger.Transactions[0].Amount != 10000 {
		t.Errorf("Amount should be unchanged at 10000, got %v", ledger.Transactions[0].Amount)
	}
}

func TestUpdateTransaction_NaNAmountRejected(t *testing.T) {
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

	// NaN != 0 is true, so the amount update path is entered.
	// The IsInf/IsNaN check should catch it.
	_, err := svc.UpdateTransaction(ctx, "SMSF", txID, models.CashTransaction{
		Amount: math.NaN(),
	})
	if err == nil {
		t.Fatal("NaN amount in update should be rejected")
	}
}

func TestUpdateTransaction_InfAmountRejected(t *testing.T) {
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
		t.Fatal("Inf amount in update should be rejected")
	}
}

func TestUpdateTransaction_NegativeInfAmountRejected(t *testing.T) {
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
		Amount: math.Inf(-1),
	})
	if err == nil {
		t.Fatal("negative Inf amount in update should be rejected")
	}
}

// =============================================================================
// 10. CalculatePerformance with signed amounts
// =============================================================================

func TestCalcPerf_SignedAmounts_DepositAndWithdrawal(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Deposit (positive)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Deposit",
	})

	// Withdrawal (negative)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatOther,
		Date:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -20000,
		Description: "Withdrawal",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 100000 {
		t.Errorf("TotalDeposited = %v, want 100000", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 20000 {
		t.Errorf("TotalWithdrawn = %v, want 20000", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != 80000 {
		t.Errorf("NetCapitalDeployed = %v, want 80000", perf.NetCapitalDeployed)
	}
}

func TestCalcPerf_SignedAmounts_OnlyNegative(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Only withdrawals (negative amounts)
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatOther,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      -10000,
		Description: "Withdrawal",
	})

	perf, err := svc.CalculatePerformance(ctx, "SMSF")
	if err != nil {
		t.Fatalf("CalculatePerformance: %v", err)
	}

	if perf.TotalDeposited != 0 {
		t.Errorf("TotalDeposited = %v, want 0", perf.TotalDeposited)
	}
	if perf.TotalWithdrawn != 10000 {
		t.Errorf("TotalWithdrawn = %v, want 10000", perf.TotalWithdrawn)
	}
	if perf.NetCapitalDeployed != -10000 {
		t.Errorf("NetCapitalDeployed = %v, want -10000", perf.NetCapitalDeployed)
	}
	// SimpleReturnPct = 0 when net capital <= 0
	if perf.SimpleReturnPct != 0 {
		t.Errorf("SimpleReturnPct = %v, want 0", perf.SimpleReturnPct)
	}
}

// =============================================================================
// 11. SignedAmount() is identity function
// =============================================================================

func TestSignedAmount_Identity(t *testing.T) {
	tests := []float64{
		0, 1, -1, 100000, -100000, 0.01, -0.01,
		math.MaxFloat64, -math.MaxFloat64,
		math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64,
	}
	for _, amount := range tests {
		tx := models.CashTransaction{Amount: amount}
		if tx.SignedAmount() != amount {
			t.Errorf("SignedAmount() for %v = %v, want %v", amount, tx.SignedAmount(), amount)
		}
	}
}

// =============================================================================
// 12. NetDeployedImpact with signed amounts
// =============================================================================

func TestNetDeployedImpact_PositiveContribution(t *testing.T) {
	tx := models.CashTransaction{Category: models.CashCatContribution, Amount: 50000}
	if tx.NetDeployedImpact() != 50000 {
		t.Errorf("NetDeployedImpact for +50000 contribution = %v, want 50000", tx.NetDeployedImpact())
	}
}

func TestNetDeployedImpact_NegativeContribution(t *testing.T) {
	// Negative contribution (refund?) — should NOT increase deployed capital
	tx := models.CashTransaction{Category: models.CashCatContribution, Amount: -10000}
	if tx.NetDeployedImpact() != 0 {
		t.Errorf("NetDeployedImpact for -10000 contribution = %v, want 0", tx.NetDeployedImpact())
	}
}

func TestNetDeployedImpact_NegativeFee(t *testing.T) {
	// Fee debit (negative) decreases deployed capital
	tx := models.CashTransaction{Category: models.CashCatFee, Amount: -500}
	if tx.NetDeployedImpact() != -500 {
		t.Errorf("NetDeployedImpact for -500 fee = %v, want -500", tx.NetDeployedImpact())
	}
}

func TestNetDeployedImpact_PositiveFee(t *testing.T) {
	// Positive fee (refund?) — should NOT affect deployed capital
	tx := models.CashTransaction{Category: models.CashCatFee, Amount: 500}
	if tx.NetDeployedImpact() != 0 {
		t.Errorf("NetDeployedImpact for +500 fee = %v, want 0", tx.NetDeployedImpact())
	}
}

func TestNetDeployedImpact_Dividend(t *testing.T) {
	// Dividends are returns on investment, not capital deployment
	tx := models.CashTransaction{Category: models.CashCatDividend, Amount: 5000}
	if tx.NetDeployedImpact() != 0 {
		t.Errorf("NetDeployedImpact for dividend = %v, want 0", tx.NetDeployedImpact())
	}
}

// =============================================================================
// 13. deriveFromTrades with signed amounts
// =============================================================================

func TestDeriveFromTrades_SignedAmounts(t *testing.T) {
	portfolioSvc := &mockPortfolioService{
		portfolio: &models.Portfolio{
			Name:               "SMSF",
			TotalValueHoldings: 55000,
			Holdings: []models.Holding{
				{
					Ticker: "BHP", Exchange: "AU", Units: 100, CurrentPrice: 50.00,
					Trades: []*models.NavexaTrade{
						{Type: "buy", Units: 100, Price: 40.00, Fees: 10.00, Date: "2023-01-10"},
						{Type: "sell", Units: 50, Price: 55.00, Fees: 10.00, Date: "2024-06-15"},
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

	// Buy: 100 * 40 + 10 = 4010 (deposited)
	// Sell: 50 * 55 - 10 = 2740 (withdrawn)
	if math.Abs(perf.TotalDeposited-4010) > 0.01 {
		t.Errorf("TotalDeposited = %v, want 4010", perf.TotalDeposited)
	}
	if math.Abs(perf.TotalWithdrawn-2740) > 0.01 {
		t.Errorf("TotalWithdrawn = %v, want 2740", perf.TotalWithdrawn)
	}

	// XIRR should be finite
	if math.IsNaN(perf.AnnualizedReturnPct) || math.IsInf(perf.AnnualizedReturnPct, 0) {
		t.Errorf("AnnualizedReturnPct should be finite, got %v", perf.AnnualizedReturnPct)
	}
}

// =============================================================================
// 14. Ledger balance consistency with transfers
// =============================================================================

func TestSignedAmounts_TransferNetZero(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Deposit 10000
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      10000,
		Description: "Initial deposit",
	})

	// Transfer 3000 from Trading to Savings
	_, _ = svc.AddTransfer(ctx, "SMSF", "Trading", "Savings", 3000,
		time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), "Move to savings")

	ledger, _ := svc.GetLedger(ctx, "SMSF")

	// Trading balance: 10000 + (-3000) = 7000
	tradingBal := ledger.AccountBalance("Trading")
	if tradingBal != 7000 {
		t.Errorf("Trading balance = %v, want 7000", tradingBal)
	}

	// Savings balance: 3000
	savingsBal := ledger.AccountBalance("Savings")
	if savingsBal != 3000 {
		t.Errorf("Savings balance = %v, want 3000", savingsBal)
	}

	// Total balance unchanged: 10000
	totalBal := ledger.TotalCashBalance()
	if totalBal != 10000 {
		t.Errorf("TotalCashBalance = %v, want 10000 (transfer is zero-sum)", totalBal)
	}
}

// =============================================================================
// 15. Multiple rapid transfers — accumulation correctness
// =============================================================================

func TestSignedAmounts_MultipleTransfers(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Initial deposit
	_, _ = svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      100000,
		Description: "Initial deposit",
	})

	// 10 transfers of 5000 each
	for i := 0; i < 10; i++ {
		_, err := svc.AddTransfer(ctx, "SMSF", "Trading", "Accumulate", 5000,
			time.Date(2024, 1, 1+i, 0, 0, 0, 0, time.UTC),
			"Transfer to accumulate")
		if err != nil {
			t.Fatalf("AddTransfer %d: %v", i, err)
		}
	}

	ledger, _ := svc.GetLedger(ctx, "SMSF")

	// Trading: 100000 - (10 * 5000) = 50000
	tradingBal := ledger.AccountBalance("Trading")
	if tradingBal != 50000 {
		t.Errorf("Trading balance = %v, want 50000", tradingBal)
	}

	// Accumulate: 10 * 5000 = 50000
	accBal := ledger.AccountBalance("Accumulate")
	if accBal != 50000 {
		t.Errorf("Accumulate balance = %v, want 50000", accBal)
	}

	// Total: 100000 (unchanged)
	totalBal := ledger.TotalCashBalance()
	if totalBal != 100000 {
		t.Errorf("TotalCashBalance = %v, want 100000", totalBal)
	}

	// 1 deposit + 20 transfer entries = 21
	if len(ledger.Transactions) != 21 {
		t.Errorf("Transaction count = %d, want 21", len(ledger.Transactions))
	}
}
