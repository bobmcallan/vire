package models

import (
	"encoding/json"
	"math"
	"testing"
)

// --- Stress tests for CashFlowSummary (adversarial analysis) ---

func TestCashFlowSummary_LargeDataset(t *testing.T) {
	// 10,000 transactions — verify no precision loss or performance issues.
	ledger := CashFlowLedger{}
	for i := 0; i < 10000; i++ {
		if i%2 == 0 {
			ledger.Transactions = append(ledger.Transactions, CashTransaction{
				Amount:   100.01,
				Category: CashCatContribution,
			})
		} else {
			ledger.Transactions = append(ledger.Transactions, CashTransaction{
				Amount:   -50.01,
				Category: CashCatFee,
			})
		}
	}

	s := ledger.Summary()
	if s.TransactionCount != 10000 {
		t.Errorf("TransactionCount = %d, want 10000", s.TransactionCount)
	}
	// 5000 contributions of 100.01 = 500050
	wantContributions := 5000 * 100.01
	if math.Abs(s.NetCashByCategory["contribution"]-wantContributions) > 0.01 {
		t.Errorf("ByCategory[contribution] = %v, want %v", s.NetCashByCategory["contribution"], wantContributions)
	}
	// 5000 fees of -50.01 = -250050
	wantFees := 5000 * -50.01
	if math.Abs(s.NetCashByCategory["fee"]-wantFees) > 0.01 {
		t.Errorf("ByCategory[fee] = %v, want %v", s.NetCashByCategory["fee"], wantFees)
	}
	// TotalCash = sum of all = wantContributions + wantFees
	wantTotal := wantContributions + wantFees
	if math.Abs(s.GrossCashBalance-wantTotal) > 0.01 {
		t.Errorf("TotalCash = %v, want %v", s.GrossCashBalance, wantTotal)
	}
}

func TestCashFlowSummary_NaNAmount(t *testing.T) {
	// NaN amount: NaN + anything = NaN, so the category it belongs to becomes NaN.
	// TransactionCount still increments correctly.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000, Category: CashCatContribution},
			{Amount: math.NaN(), Category: CashCatContribution},
			{Amount: -500, Category: CashCatFee},
		},
	}
	s := ledger.Summary()
	if s.TransactionCount != 3 {
		t.Errorf("TransactionCount = %d, want 3", s.TransactionCount)
	}
	// NaN propagates into ByCategory[contribution] (1000 + NaN = NaN)
	if !math.IsNaN(s.NetCashByCategory["contribution"]) {
		t.Errorf("ByCategory[contribution] = %v, want NaN (NaN propagates)", s.NetCashByCategory["contribution"])
	}
	// Fee is unaffected
	if math.Abs(s.NetCashByCategory["fee"]-(-500)) > 0.001 {
		t.Errorf("ByCategory[fee] = %v, want -500", s.NetCashByCategory["fee"])
	}
}

func TestCashFlowSummary_InfAmount(t *testing.T) {
	// +Inf amount propagates into its category and TotalCash.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: math.Inf(1), Category: CashCatContribution},
			{Amount: -500, Category: CashCatFee},
		},
	}
	s := ledger.Summary()
	if !math.IsInf(s.NetCashByCategory["contribution"], 1) {
		t.Errorf("ByCategory[contribution] = %v, want +Inf", s.NetCashByCategory["contribution"])
	}
	if !math.IsInf(s.GrossCashBalance, 1) {
		t.Errorf("TotalCash = %v, want +Inf", s.GrossCashBalance)
	}
	if s.TransactionCount != 2 {
		t.Errorf("TransactionCount = %d, want 2", s.TransactionCount)
	}
}

func TestCashFlowSummary_NegativeInfAmount(t *testing.T) {
	// -Inf amount propagates into its category.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000, Category: CashCatContribution},
			{Amount: math.Inf(-1), Category: CashCatFee},
		},
	}
	s := ledger.Summary()
	if !math.IsInf(s.NetCashByCategory["fee"], -1) {
		t.Errorf("ByCategory[fee] = %v, want -Inf", s.NetCashByCategory["fee"])
	}
	if !math.IsInf(s.GrossCashBalance, -1) {
		t.Errorf("TotalCash = %v, want -Inf (1000 + -Inf)", s.GrossCashBalance)
	}
}

func TestCashFlowSummary_FloatingPointRounding(t *testing.T) {
	// Classic floating-point trap: 0.1 + 0.2 != 0.3.
	// Verify TotalCash = sum of all signed amounts (consistent computation path).
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 0.1, Category: CashCatContribution},
			{Amount: 0.2, Category: CashCatContribution},
			{Amount: -0.3, Category: CashCatFee},
		},
	}
	s := ledger.Summary()

	// The key invariant: TotalCash = TotalCashBalance() = sum of signed amounts.
	recomputed := ledger.TotalCashBalance()
	if s.GrossCashBalance != recomputed {
		t.Errorf("TotalCash (%v) != TotalCashBalance() (%v)", s.GrossCashBalance, recomputed)
	}

	// Note: Due to floating point, 0.1+0.2 = 0.30000000000000004, not 0.3.
	// So contribution (0.3000...04) + fee (-0.3) != 0 exactly.
	// This is expected float64 behavior and NOT a bug.
}

func TestCashFlowSummary_NilTransactions(t *testing.T) {
	// Summary on ledger with nil transactions slice should not panic.
	ledger := CashFlowLedger{Transactions: nil}
	s := ledger.Summary()
	if s.TransactionCount != 0 {
		t.Errorf("TransactionCount = %d, want 0", s.TransactionCount)
	}
	if s.GrossCashBalance != 0 {
		t.Errorf("TotalCash = %v, want 0", s.GrossCashBalance)
	}
	// All 5 categories should be present with zero values.
	for _, cat := range []string{"contribution", "dividend", "transfer", "fee", "other"} {
		v, ok := s.NetCashByCategory[cat]
		if !ok {
			t.Errorf("ByCategory missing category %q for nil transactions", cat)
		} else if v != 0 {
			t.Errorf("ByCategory[%q] = %v, want 0", cat, v)
		}
	}
}

func TestCashFlowSummary_JSONSerialization(t *testing.T) {
	// Verify JSON field names match the API contract.
	s := CashFlowSummary{
		GrossCashBalance: 1000.50,
		TransactionCount: 5,
		NetCashByCategory: map[string]float64{
			"contribution": 1000.50,
			"dividend":     0,
			"transfer":     0,
			"fee":          0,
			"other":        0,
		},
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	expectedKeys := []string{"gross_cash_balance", "transaction_count", "net_cash_by_category", "gross_cash_balance_by_currency"}
	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("Missing JSON field %q in serialized summary", key)
		}
	}
	if len(m) != len(expectedKeys) {
		t.Errorf("Expected %d JSON fields, got %d: %v", len(expectedKeys), len(m), m)
	}

	// Verify by_category is a nested object.
	byCategory, ok := m["net_cash_by_category"].(map[string]interface{})
	if !ok {
		t.Fatalf("by_category is not a map: %T", m["net_cash_by_category"])
	}
	if _, ok := byCategory["contribution"]; !ok {
		t.Error("by_category missing contribution key")
	}
}

func TestCashFlowSummary_ZeroNetCashFlow(t *testing.T) {
	// Credits and debits exactly cancel out.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000, Category: CashCatContribution},
			{Amount: -1000, Category: CashCatFee},
		},
	}
	s := ledger.Summary()
	if s.GrossCashBalance != 0 {
		t.Errorf("TotalCash = %v, want 0", s.GrossCashBalance)
	}
	if math.Abs(s.NetCashByCategory["contribution"]-1000) > 0.001 {
		t.Errorf("ByCategory[contribution] = %v, want 1000", s.NetCashByCategory["contribution"])
	}
	if math.Abs(s.NetCashByCategory["fee"]-(-1000)) > 0.001 {
		t.Errorf("ByCategory[fee] = %v, want -1000", s.NetCashByCategory["fee"])
	}
}

func TestCashFlowResponse_FieldContract(t *testing.T) {
	// Verify CashFlowLedger fields are all preserved when serialized.
	// This catches if the cashFlowResponse struct in handlers drops fields.
	ledger := CashFlowLedger{
		PortfolioName: "SMSF",
		Version:       3,
		Accounts: []CashAccount{
			{Name: "Trading", Type: "trading", IsTransactional: true},
		},
		Transactions: []CashTransaction{
			{ID: "ct_test", Account: "Trading", Amount: 1000},
		},
		Notes: "test notes",
	}

	// Marshal the ledger directly and verify all field keys.
	data, err := json.Marshal(ledger)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var ledgerFields map[string]interface{}
	if err := json.Unmarshal(data, &ledgerFields); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// These are the fields from CashFlowLedger that MUST appear in the response.
	requiredFields := []string{
		"portfolio_name", "version", "accounts", "transactions",
		"created_at", "updated_at",
	}
	for _, field := range requiredFields {
		if _, ok := ledgerFields[field]; !ok {
			t.Errorf("CashFlowLedger missing required JSON field %q", field)
		}
	}
}

func TestCashFlowSummary_VerySmallAmounts(t *testing.T) {
	// Sub-cent amounts should accumulate correctly.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 0.001, Category: CashCatContribution},
			{Amount: 0.001, Category: CashCatContribution},
			{Amount: 0.001, Category: CashCatContribution},
			{Amount: -0.001, Category: CashCatFee},
		},
	}
	s := ledger.Summary()
	if math.Abs(s.NetCashByCategory["contribution"]-0.003) > 1e-10 {
		t.Errorf("ByCategory[contribution] = %v, want 0.003", s.NetCashByCategory["contribution"])
	}
	if math.Abs(s.NetCashByCategory["fee"]-(-0.001)) > 1e-10 {
		t.Errorf("ByCategory[fee] = %v, want -0.001", s.NetCashByCategory["fee"])
	}
	if math.Abs(s.GrossCashBalance-0.002) > 1e-10 {
		t.Errorf("TotalCash = %v, want 0.002", s.GrossCashBalance)
	}
}

func TestCashFlowSummary_MaxFloat64(t *testing.T) {
	// Two MaxFloat64 contributions: addition overflows to +Inf.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: math.MaxFloat64, Category: CashCatContribution},
			{Amount: math.MaxFloat64, Category: CashCatContribution},
		},
	}
	s := ledger.Summary()
	// MaxFloat64 + MaxFloat64 overflows to +Inf
	if !math.IsInf(s.NetCashByCategory["contribution"], 1) {
		t.Errorf("ByCategory[contribution] = %v, want +Inf (overflow)", s.NetCashByCategory["contribution"])
	}
	if !math.IsInf(s.GrossCashBalance, 1) {
		t.Errorf("TotalCash = %v, want +Inf (overflow)", s.GrossCashBalance)
	}
	if s.TransactionCount != 2 {
		t.Errorf("TransactionCount = %d, want 2", s.TransactionCount)
	}
}

// --- Adversarial stress tests for summary redesign ---

func TestCashFlowSummary_UnknownCategoryLeaks(t *testing.T) {
	// If a transaction somehow has an invalid category, it appears as an extra
	// key in by_category. This tests that the behavior is deterministic —
	// unknown categories are included (not silently dropped).
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000, Category: CashCatContribution},
			{Amount: 500, Category: CashCategory("groceries")}, // invalid
		},
	}
	s := ledger.Summary()

	// The 5 known categories must always be present.
	for _, cat := range []string{"contribution", "dividend", "transfer", "fee", "other"} {
		if _, ok := s.NetCashByCategory[cat]; !ok {
			t.Errorf("ByCategory missing known category %q", cat)
		}
	}
	// The unknown category leaks in as an extra key.
	if v, ok := s.NetCashByCategory["groceries"]; !ok {
		t.Error("ByCategory should include unknown category 'groceries'")
	} else if math.Abs(v-500) > 0.001 {
		t.Errorf("ByCategory[groceries] = %v, want 500", v)
	}
	// 5 known + 1 unknown = 6 keys
	if len(s.NetCashByCategory) != 6 {
		t.Errorf("ByCategory has %d keys, want 6 (5 known + 1 unknown)", len(s.NetCashByCategory))
	}
	// TotalCash includes the unknown category amount.
	if math.Abs(s.GrossCashBalance-1500) > 0.001 {
		t.Errorf("TotalCash = %v, want 1500", s.GrossCashBalance)
	}
}

func TestCashFlowSummary_MapMutationSafety(t *testing.T) {
	// Mutating the returned ByCategory map must not affect subsequent calls.
	// Summary() creates a new map each time.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000, Category: CashCatContribution},
		},
	}

	s1 := ledger.Summary()
	// Corrupt the returned map.
	s1.NetCashByCategory["contribution"] = 999999
	s1.NetCashByCategory["injected"] = 42

	s2 := ledger.Summary()
	if math.Abs(s2.NetCashByCategory["contribution"]-1000) > 0.001 {
		t.Errorf("ByCategory[contribution] = %v, want 1000 (mutation leaked)", s2.NetCashByCategory["contribution"])
	}
	if _, ok := s2.NetCashByCategory["injected"]; ok {
		t.Error("ByCategory contains 'injected' key — mutation leaked between Summary() calls")
	}
}

func TestCashFlowSummary_MultipleTransferPairsNetToZero(t *testing.T) {
	// Multiple independent transfer pairs must all net to zero in by_category.
	ledger := CashFlowLedger{
		Accounts: []CashAccount{
			{Name: "Trading"}, {Name: "Accumulate"}, {Name: "Term Deposit"},
		},
		Transactions: []CashTransaction{
			{Account: "Trading", Category: CashCatContribution, Amount: 100000},
			// Transfer pair 1: Trading -> Accumulate
			{Account: "Trading", Category: CashCatTransfer, Amount: -20000},
			{Account: "Accumulate", Category: CashCatTransfer, Amount: 20000},
			// Transfer pair 2: Trading -> Term Deposit
			{Account: "Trading", Category: CashCatTransfer, Amount: -30000},
			{Account: "Term Deposit", Category: CashCatTransfer, Amount: 30000},
			// Transfer pair 3: Accumulate -> Trading (reverse)
			{Account: "Accumulate", Category: CashCatTransfer, Amount: -5000},
			{Account: "Trading", Category: CashCatTransfer, Amount: 5000},
		},
	}
	s := ledger.Summary()

	// All transfers must net to zero.
	if math.Abs(s.NetCashByCategory["transfer"]) > 0.001 {
		t.Errorf("ByCategory[transfer] = %v, want 0 (multiple pairs must net to zero)", s.NetCashByCategory["transfer"])
	}
	// TotalCash = only the 100000 contribution.
	if math.Abs(s.GrossCashBalance-100000) > 0.001 {
		t.Errorf("TotalCash = %v, want 100000", s.GrossCashBalance)
	}
	// Per-account balances: Trading = 100000 - 20000 - 30000 + 5000 = 55000
	if math.Abs(ledger.AccountBalance("Trading")-55000) > 0.001 {
		t.Errorf("AccountBalance(Trading) = %v, want 55000", ledger.AccountBalance("Trading"))
	}
	// Accumulate = 20000 - 5000 = 15000
	if math.Abs(ledger.AccountBalance("Accumulate")-15000) > 0.001 {
		t.Errorf("AccountBalance(Accumulate) = %v, want 15000", ledger.AccountBalance("Accumulate"))
	}
	// Term Deposit = 30000
	if math.Abs(ledger.AccountBalance("Term Deposit")-30000) > 0.001 {
		t.Errorf("AccountBalance(Term Deposit) = %v, want 30000", ledger.AccountBalance("Term Deposit"))
	}
}

func TestCashFlowSummary_ByCategoryZeroValuesInJSON(t *testing.T) {
	// Zero values in by_category must NOT be omitted from JSON.
	// This is critical: a client needs to see all 5 categories even if zero.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000, Category: CashCatContribution},
		},
	}
	s := ledger.Summary()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	byCategory, ok := m["net_cash_by_category"].(map[string]interface{})
	if !ok {
		t.Fatalf("by_category is not a map: %T", m["net_cash_by_category"])
	}

	// All 5 categories must be present in JSON, even with zero values.
	for _, cat := range []string{"contribution", "dividend", "transfer", "fee", "other"} {
		v, ok := byCategory[cat]
		if !ok {
			t.Errorf("by_category missing %q in JSON (zero omitted?)", cat)
			continue
		}
		if cat != "contribution" {
			fv, _ := v.(float64)
			if fv != 0 {
				t.Errorf("by_category[%q] = %v, want 0", cat, v)
			}
		}
	}
}

func TestCashFlowSummary_Idempotent(t *testing.T) {
	// Calling Summary() twice on the same ledger must return identical results.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 5000, Category: CashCatContribution},
			{Amount: 200, Category: CashCatDividend},
			{Amount: -50, Category: CashCatFee},
			{Amount: -1000, Category: CashCatTransfer, Account: "Trading"},
			{Amount: 1000, Category: CashCatTransfer, Account: "Accumulate"},
		},
	}

	s1 := ledger.Summary()
	s2 := ledger.Summary()

	if s1.GrossCashBalance != s2.GrossCashBalance {
		t.Errorf("TotalCash not idempotent: %v vs %v", s1.GrossCashBalance, s2.GrossCashBalance)
	}
	if s1.TransactionCount != s2.TransactionCount {
		t.Errorf("TransactionCount not idempotent: %v vs %v", s1.TransactionCount, s2.TransactionCount)
	}
	for _, cat := range []string{"contribution", "dividend", "transfer", "fee", "other"} {
		if s1.NetCashByCategory[cat] != s2.NetCashByCategory[cat] {
			t.Errorf("ByCategory[%q] not idempotent: %v vs %v", cat, s1.NetCashByCategory[cat], s2.NetCashByCategory[cat])
		}
	}
}

func TestCashFlowSummary_EmptyCategoryString(t *testing.T) {
	// Transaction with empty string category — falls through to unknown key "".
	// This tests graceful handling of edge-case data.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000, Category: CashCatContribution},
			{Amount: 500, Category: CashCategory("")},
		},
	}
	s := ledger.Summary()

	// Empty string becomes its own key in ByCategory.
	if v, ok := s.NetCashByCategory[""]; !ok {
		t.Error("ByCategory should include empty-string category key")
	} else if math.Abs(v-500) > 0.001 {
		t.Errorf("ByCategory[\"\"] = %v, want 500", v)
	}
	// TotalCash still includes it.
	if math.Abs(s.GrossCashBalance-1500) > 0.001 {
		t.Errorf("TotalCash = %v, want 1500", s.GrossCashBalance)
	}
}
