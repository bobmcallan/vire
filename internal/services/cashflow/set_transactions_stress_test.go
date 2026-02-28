package cashflow

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests for SetTransactions (bulk replace).
// Covers: nil input, partial validation failure atomicity, LinkedID injection,
// whitespace account dedup, notes preservation, large batch, boundary amounts,
// hostile descriptions, idempotency, account type preservation after replace.

// =============================================================================
// 1. Nil transactions slice — should not panic
// =============================================================================

func TestSetTransactions_NilSlice(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Seed a transaction first
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

	// Pass nil — should clear transactions like empty slice
	ledger, err := svc.SetTransactions(ctx, "SMSF", nil, "")
	if err != nil {
		t.Fatalf("SetTransactions(nil): %v", err)
	}

	if len(ledger.Transactions) != 0 {
		t.Errorf("expected 0 transactions after nil set, got %d", len(ledger.Transactions))
	}
}

// =============================================================================
// 2. Partial validation failure — atomicity
// =============================================================================

func TestSetTransactions_PartialFailureAtomicity(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Seed existing data
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      50000,
		Description: "Original deposit",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// First item valid, second invalid (zero amount)
	txs := []models.CashTransaction{
		{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: "Valid tx",
		},
		{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			Amount:      0, // invalid
			Description: "Invalid tx",
		},
	}

	_, err = svc.SetTransactions(ctx, "SMSF", txs, "")
	if err == nil {
		t.Fatal("expected error for invalid transaction")
	}

	// Original data should be untouched
	ledger, _ := svc.GetLedger(ctx, "SMSF")
	if len(ledger.Transactions) != 1 {
		t.Errorf("expected 1 original transaction preserved, got %d", len(ledger.Transactions))
	}
	if ledger.Transactions[0].Amount != 50000 {
		t.Errorf("original transaction amount should be 50000, got %v", ledger.Transactions[0].Amount)
	}
}

// =============================================================================
// 3. Invalid transaction at last position — error message includes index
// =============================================================================

func TestSetTransactions_ErrorIndexReported(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	txs := make([]models.CashTransaction, 5)
	for i := range txs {
		txs[i] = models.CashTransaction{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: fmt.Sprintf("tx %d", i),
		}
	}
	// Corrupt the last one
	txs[4].Amount = math.NaN()

	_, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "index 4") {
		t.Errorf("error should mention 'index 4', got: %v", err)
	}
}

// =============================================================================
// 4. LinkedID injection — user-provided LinkedIDs are preserved
// =============================================================================

func TestSetTransactions_LinkedIDPreserved(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// User provides transactions with fake LinkedIDs
	txs := []models.CashTransaction{
		{
			Account:     "Trading",
			Category:    models.CashCatTransfer,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      -5000,
			Description: "Fake transfer out",
			LinkedID:    "fake_linked_id_1",
		},
		{
			Account:     "Savings",
			Category:    models.CashCatTransfer,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      5000,
			Description: "Fake transfer in",
			LinkedID:    "fake_linked_id_2",
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	// LinkedIDs are NOT cleared by SetTransactions — they pass through.
	// This is acceptable for bulk import but worth documenting.
	for _, tx := range ledger.Transactions {
		if tx.LinkedID == "" {
			t.Error("user-provided LinkedID should be preserved in bulk set")
		}
	}
}

// =============================================================================
// 5. Whitespace account name deduplication
// =============================================================================

func TestSetTransactions_WhitespaceAccountDedup(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Two transactions with whitespace variations of same account name
	txs := []models.CashTransaction{
		{
			Account:     "  Savings  ",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: "Deposit 1",
		},
		{
			Account:     "Savings",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			Amount:      2000,
			Description: "Deposit 2",
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	// Both should be trimmed to "Savings" and result in one account
	savingsCount := 0
	for _, a := range ledger.Accounts {
		if a.Name == "Savings" {
			savingsCount++
		}
	}
	if savingsCount != 1 {
		t.Errorf("expected exactly 1 Savings account, got %d (dedup failure)", savingsCount)
	}

	// Both transactions should reference the trimmed account
	for _, tx := range ledger.Transactions {
		if tx.Account != "Savings" {
			t.Errorf("transaction account should be trimmed to 'Savings', got %q", tx.Account)
		}
	}
}

// =============================================================================
// 6. Notes preservation — empty notes does NOT clear existing
// =============================================================================

func TestSetTransactions_EmptyNotesPreservesExisting(t *testing.T) {
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

	// First set with notes
	_, err := svc.SetTransactions(ctx, "SMSF", txs, "Important notes")
	if err != nil {
		t.Fatalf("first SetTransactions: %v", err)
	}

	// Second set with empty notes — should preserve existing
	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("second SetTransactions: %v", err)
	}

	if ledger.Notes != "Important notes" {
		t.Errorf("notes should be preserved when empty string passed, got %q", ledger.Notes)
	}
}

func TestSetTransactions_NotesOverwritten(t *testing.T) {
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

	// Set with notes
	_, _ = svc.SetTransactions(ctx, "SMSF", txs, "Original notes")

	// Overwrite notes
	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "Updated notes")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	if ledger.Notes != "Updated notes" {
		t.Errorf("notes should be updated to 'Updated notes', got %q", ledger.Notes)
	}
}

// =============================================================================
// 7. Large batch — 1000 transactions
// =============================================================================

func TestSetTransactions_LargeBatch(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	const batchSize = 1000
	txs := make([]models.CashTransaction, batchSize)
	for i := range txs {
		txs[i] = models.CashTransaction{
			Account:     fmt.Sprintf("Account-%d", i%10), // 10 distinct accounts
			Category:    models.CashCatContribution,
			Date:        time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			Amount:      float64(i + 1),
			Description: fmt.Sprintf("Transaction %d", i),
		}
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "Large batch test")
	if err != nil {
		t.Fatalf("SetTransactions with %d items: %v", batchSize, err)
	}

	if len(ledger.Transactions) != batchSize {
		t.Errorf("expected %d transactions, got %d", batchSize, len(ledger.Transactions))
	}

	// Should be sorted by date
	for i := 1; i < len(ledger.Transactions); i++ {
		if ledger.Transactions[i].Date.Before(ledger.Transactions[i-1].Date) {
			t.Errorf("transactions not sorted at index %d", i)
			break
		}
	}

	// 10 auto-created accounts + default Trading
	expectedAccounts := 11 // 10 new + 1 default
	if len(ledger.Accounts) != expectedAccounts {
		t.Errorf("expected %d accounts, got %d", expectedAccounts, len(ledger.Accounts))
	}
}

// =============================================================================
// 8. Boundary amounts — just under and at limit
// =============================================================================

func TestSetTransactions_BoundaryAmounts(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	tests := []struct {
		name    string
		amount  float64
		wantErr bool
	}{
		{"just under positive max", 1e15 - 1, false},
		{"just under negative max", -(1e15 - 1), false},
		{"at positive max", 1e15, true},
		{"at negative max", -1e15, true},
		{"one cent", 0.01, false},
		{"negative one cent", -0.01, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txs := []models.CashTransaction{
				{
					Account:     "Trading",
					Category:    models.CashCatContribution,
					Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Amount:      tt.amount,
					Description: "Boundary test",
				},
			}
			_, err := svc.SetTransactions(ctx, "SMSF", txs, "")
			if tt.wantErr && err == nil {
				t.Errorf("amount %v should be rejected", tt.amount)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("amount %v should be accepted, got: %v", tt.amount, err)
			}
		})
	}
}

// =============================================================================
// 9. Account with special characters
// =============================================================================

func TestSetTransactions_SpecialCharAccountName(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	tests := []struct {
		name    string
		account string
		wantErr bool
	}{
		{"unicode", "Konto\u00e4\u00f6\u00fc", false},
		{"emoji", "Account \U0001f4b0", false},
		{"with newline", "Account\nName", false}, // odd but not rejected by validation
		{"with tab", "Account\tName", false},
		{"max length", strings.Repeat("A", 100), false},
		{"over max length", strings.Repeat("A", 101), true},
		{"only whitespace", "   ", true}, // trims to empty
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txs := []models.CashTransaction{
				{
					Account:     tt.account,
					Category:    models.CashCatContribution,
					Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Amount:      1000,
					Description: "Test",
				},
			}
			_, err := svc.SetTransactions(ctx, "SMSF", txs, "")
			if tt.wantErr && err == nil {
				t.Errorf("account %q should be rejected", tt.account)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("account %q should be accepted, got: %v", tt.account, err)
			}
		})
	}
}

// =============================================================================
// 10. Account type preservation after SetTransactions
// =============================================================================

func TestSetTransactions_PreservesAccountType(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Set up a transaction to create an account
	_, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Savings",
		Category:    models.CashCatContribution,
		Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Amount:      5000,
		Description: "Deposit",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	// Update the account type to accumulate
	_, err = svc.UpdateAccount(ctx, "SMSF", "Savings", models.CashAccountUpdate{Type: "accumulate"})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}

	// Replace all transactions — should preserve the account and its type
	newTxs := []models.CashTransaction{
		{
			Account:     "Savings",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			Amount:      10000,
			Description: "New deposit",
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", newTxs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	acct := ledger.GetAccount("Savings")
	if acct == nil {
		t.Fatal("Savings account should be preserved")
	}
	if acct.Type != "accumulate" {
		t.Errorf("account type should be preserved as 'accumulate', got %q", acct.Type)
	}
}

// =============================================================================
// 11. Idempotent set — calling twice with same data
// =============================================================================

func TestSetTransactions_Idempotent(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	makeTxs := func() []models.CashTransaction {
		return []models.CashTransaction{
			{
				Account:     "Trading",
				Category:    models.CashCatContribution,
				Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				Amount:      5000,
				Description: "Deposit",
			},
			{
				Account:     "Trading",
				Category:    models.CashCatFee,
				Date:        time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
				Amount:      -50,
				Description: "Fee",
			},
		}
	}

	ledger1, err := svc.SetTransactions(ctx, "SMSF", makeTxs(), "Notes")
	if err != nil {
		t.Fatalf("first set: %v", err)
	}
	id1 := ledger1.Transactions[0].ID

	// Second call with fresh slice of same logical data
	ledger2, err := svc.SetTransactions(ctx, "SMSF", makeTxs(), "Notes")
	if err != nil {
		t.Fatalf("second set: %v", err)
	}

	if len(ledger2.Transactions) != len(ledger1.Transactions) {
		t.Errorf("idempotent set should produce same count: %d vs %d",
			len(ledger1.Transactions), len(ledger2.Transactions))
	}

	// IDs should be different (regenerated each time)
	if id1 == ledger2.Transactions[0].ID {
		t.Error("IDs should be regenerated on each set, not reused")
	}

	// Amounts and descriptions should be the same
	for i := range ledger2.Transactions {
		if ledger2.Transactions[i].Amount != makeTxs()[i].Amount {
			t.Errorf("tx[%d] amount mismatch", i)
		}
	}
}

// SetTransactions mutates the input slice (assigns IDs, timestamps, trims
// whitespace) and the returned ledger shares the same backing array via
// ledger.Transactions = transactions. If the caller reuses the same slice,
// the previously returned ledger's transactions get silently overwritten.
// This test documents that behavior.
func TestSetTransactions_InputSliceMutation(t *testing.T) {
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

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	// The implementation either mutates the input slice OR makes a defensive copy.
	// Both are acceptable. What we require: the returned ledger has a valid ID.
	if ledger.Transactions[0].ID == "" {
		t.Error("returned ledger transaction should have an assigned ID")
	}
	if !strings.HasPrefix(ledger.Transactions[0].ID, "ct_") {
		t.Errorf("assigned ID should start with ct_, got %q", ledger.Transactions[0].ID)
	}

	// The returned ledger should NOT share backing memory with input (defensive copy).
	// Mutating txs after the call must not affect the returned ledger.
	originalID := ledger.Transactions[0].ID
	txs[0].ID = "mutated"
	if ledger.Transactions[0].ID != originalID {
		t.Error("returned ledger shares backing array with input — caller mutations affect the returned ledger")
	}
}

// =============================================================================
// 12. All invalid categories in batch
// =============================================================================

func TestSetTransactions_InvalidCategory(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	txs := []models.CashTransaction{
		{
			Account:     "Trading",
			Category:    "invalid_category",
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: "Bad category",
		},
	}

	_, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err == nil {
		t.Fatal("invalid category should be rejected")
	}
	if !strings.Contains(err.Error(), "invalid cash transaction at index 0") {
		t.Errorf("error should mention index 0, got: %v", err)
	}
}

// =============================================================================
// 13. Future date at boundary
// =============================================================================

func TestSetTransactions_FutureDate(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Just under 24h from now — should be accepted
	txs := []models.CashTransaction{
		{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Now().Add(23 * time.Hour),
			Amount:      1000,
			Description: "Near-future tx",
		},
	}

	_, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Errorf("date within 24h buffer should be accepted, got: %v", err)
	}

	// Beyond 24h from now — should be rejected
	txs[0].Date = time.Now().Add(25 * time.Hour)
	_, err = svc.SetTransactions(ctx, "SMSF", txs, "")
	if err == nil {
		t.Error("date beyond 24h in future should be rejected")
	}
}

// =============================================================================
// 14. Description edge cases
// =============================================================================

func TestSetTransactions_DescriptionEdgeCases(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	tests := []struct {
		name    string
		desc    string
		wantErr bool
	}{
		{"whitespace only", "   ", true}, // trims to empty
		{"max length", strings.Repeat("D", 500), false},
		{"over max length", strings.Repeat("D", 501), true},
		{"single char", "x", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txs := []models.CashTransaction{
				{
					Account:     "Trading",
					Category:    models.CashCatContribution,
					Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					Amount:      1000,
					Description: tt.desc,
				},
			}
			_, err := svc.SetTransactions(ctx, "SMSF", txs, "")
			if tt.wantErr && err == nil {
				t.Errorf("description %q should be rejected", tt.desc)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("description %q should be accepted, got: %v", tt.desc, err)
			}
		})
	}
}

// =============================================================================
// 15. Notes field on individual transactions
// =============================================================================

func TestSetTransactions_TransactionNotesLimit(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Transaction notes at max
	txs := []models.CashTransaction{
		{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: "Deposit",
			Notes:       strings.Repeat("N", 1000),
		},
	}

	_, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Errorf("notes at 1000 chars should be accepted, got: %v", err)
	}

	// Over limit
	txs[0].Notes = strings.Repeat("N", 1001)
	_, err = svc.SetTransactions(ctx, "SMSF", txs, "")
	if err == nil {
		t.Error("notes over 1000 chars should be rejected")
	}
}

// =============================================================================
// 16. All five valid categories
// =============================================================================

func TestSetTransactions_AllValidCategories(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	categories := []models.CashCategory{
		models.CashCatContribution,
		models.CashCatDividend,
		models.CashCatTransfer,
		models.CashCatFee,
		models.CashCatOther,
	}

	txs := make([]models.CashTransaction, len(categories))
	for i, cat := range categories {
		txs[i] = models.CashTransaction{
			Account:     "Trading",
			Category:    cat,
			Date:        time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: fmt.Sprintf("Category %s", cat),
		}
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("all valid categories should be accepted: %v", err)
	}

	if len(ledger.Transactions) != 5 {
		t.Errorf("expected 5 transactions, got %d", len(ledger.Transactions))
	}
}

// =============================================================================
// 17. Multiple same-date transactions — sort stability
// =============================================================================

func TestSetTransactions_SameDateStability(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	sameDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	txs := make([]models.CashTransaction, 10)
	for i := range txs {
		txs[i] = models.CashTransaction{
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        sameDate,
			Amount:      float64((i + 1) * 100),
			Description: fmt.Sprintf("Same date tx %d", i),
		}
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	if len(ledger.Transactions) != 10 {
		t.Errorf("expected 10 transactions, got %d", len(ledger.Transactions))
	}

	// All should have the same date
	for _, tx := range ledger.Transactions {
		if !tx.Date.Equal(sameDate) {
			t.Errorf("date should be %v, got %v", sameDate, tx.Date)
		}
	}
}

// =============================================================================
// 18. Balance correctness after bulk replace with mixed signs
// =============================================================================

func TestSetTransactions_MixedSignBalance(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	txs := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 100000, Description: "Deposit"},
		{Account: "Trading", Category: models.CashCatFee, Date: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), Amount: -500, Description: "Fee"},
		{Account: "Trading", Category: models.CashCatDividend, Date: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Amount: 2000, Description: "Dividend"},
		{Account: "Trading", Category: models.CashCatOther, Date: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), Amount: -10000, Description: "Withdrawal"},
		{Account: "Savings", Category: models.CashCatContribution, Date: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), Amount: 50000, Description: "Savings deposit"},
		{Account: "Savings", Category: models.CashCatOther, Date: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC), Amount: -5000, Description: "Savings withdrawal"},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	// Trading balance: 100000 - 500 + 2000 - 10000 = 91500
	tradingBal := ledger.AccountBalance("Trading")
	if tradingBal != 91500 {
		t.Errorf("Trading balance = %v, want 91500", tradingBal)
	}

	// Savings balance: 50000 - 5000 = 45000
	savingsBal := ledger.AccountBalance("Savings")
	if savingsBal != 45000 {
		t.Errorf("Savings balance = %v, want 45000", savingsBal)
	}

	// TotalDeposited: only contributions = 100000 + 50000 = 150000 (dividends/other don't count)
	if ledger.TotalDeposited() != 150000 {
		t.Errorf("TotalDeposited = %v, want 150000 (only contributions count)", ledger.TotalDeposited())
	}

	// TotalWithdrawn: only contribution debits = 0 (fees, other withdrawals don't count)
	if ledger.TotalWithdrawn() != 0 {
		t.Errorf("TotalWithdrawn = %v, want 0 (only contribution withdrawals count)", ledger.TotalWithdrawn())
	}

	// TotalCashBalance: 91500 + 45000 = 136500
	if ledger.TotalCashBalance() != 136500 {
		t.Errorf("TotalCashBalance = %v, want 136500", ledger.TotalCashBalance())
	}
}

// =============================================================================
// 19. Set then add — transactions accumulate correctly
// =============================================================================

func TestSetTransactions_ThenAddTransaction(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	// Bulk set 3 transactions
	txs := []models.CashTransaction{
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Amount: 10000, Description: "Deposit 1"},
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Amount: 20000, Description: "Deposit 2"},
		{Account: "Trading", Category: models.CashCatContribution, Date: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), Amount: 30000, Description: "Deposit 3"},
	}
	_, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	// Add one more
	ledger, err := svc.AddTransaction(ctx, "SMSF", models.CashTransaction{
		Account:     "Trading",
		Category:    models.CashCatDividend,
		Date:        time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
		Amount:      500,
		Description: "Dividend",
	})
	if err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}

	if len(ledger.Transactions) != 4 {
		t.Errorf("expected 4 transactions after set+add, got %d", len(ledger.Transactions))
	}

	// Balance: 10000 + 20000 + 30000 + 500 = 60500
	if ledger.TotalCashBalance() != 60500 {
		t.Errorf("TotalCashBalance = %v, want 60500", ledger.TotalCashBalance())
	}
}

// =============================================================================
// 20. User-provided IDs and timestamps are overwritten
// =============================================================================

func TestSetTransactions_OverwritesUserIDsAndTimestamps(t *testing.T) {
	svc, _ := testService()
	ctx := testContext()

	userTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	txs := []models.CashTransaction{
		{
			ID:          "user_id_1",
			Account:     "Trading",
			Category:    models.CashCatContribution,
			Date:        time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			Amount:      1000,
			Description: "Deposit",
			CreatedAt:   userTime,
			UpdatedAt:   userTime,
		},
	}

	ledger, err := svc.SetTransactions(ctx, "SMSF", txs, "")
	if err != nil {
		t.Fatalf("SetTransactions: %v", err)
	}

	tx := ledger.Transactions[0]

	// ID must be overwritten with server-generated ct_ prefix
	if tx.ID == "user_id_1" {
		t.Error("user-provided ID should be overwritten")
	}
	if !strings.HasPrefix(tx.ID, "ct_") {
		t.Errorf("generated ID should have ct_ prefix, got %q", tx.ID)
	}

	// Timestamps must be overwritten with server time (not user's year 2000)
	if tx.CreatedAt.Year() == 2000 {
		t.Error("CreatedAt should be overwritten with server time")
	}
	if tx.UpdatedAt.Year() == 2000 {
		t.Error("UpdatedAt should be overwritten with server time")
	}
}
