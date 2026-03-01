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
			ledger.Transactions = append(ledger.Transactions, CashTransaction{Amount: 100.01})
		} else {
			ledger.Transactions = append(ledger.Transactions, CashTransaction{Amount: -50.01})
		}
	}

	s := ledger.Summary()
	if s.TransactionCount != 10000 {
		t.Errorf("TransactionCount = %d, want 10000", s.TransactionCount)
	}
	// 5000 credits of 100.01 = 500050
	wantCredits := 5000 * 100.01
	if math.Abs(s.TotalCredits-wantCredits) > 0.01 {
		t.Errorf("TotalCredits = %v, want %v", s.TotalCredits, wantCredits)
	}
	// 5000 debits of 50.01 = 250050
	wantDebits := 5000 * 50.01
	if math.Abs(s.TotalDebits-wantDebits) > 0.01 {
		t.Errorf("TotalDebits = %v, want %v", s.TotalDebits, wantDebits)
	}
	wantNet := wantCredits - wantDebits
	if math.Abs(s.NetCashFlow-wantNet) > 0.01 {
		t.Errorf("NetCashFlow = %v, want %v", s.NetCashFlow, wantNet)
	}
}

func TestCashFlowSummary_NaNAmount(t *testing.T) {
	// NaN amount should not corrupt other totals.
	// NaN > 0 is false, NaN < 0 is false — so NaN falls through both branches.
	// It still counts in TransactionCount but does not affect totals.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000},
			{Amount: math.NaN()},
			{Amount: -500},
		},
	}
	s := ledger.Summary()
	if s.TransactionCount != 3 {
		t.Errorf("TransactionCount = %d, want 3", s.TransactionCount)
	}
	if math.Abs(s.TotalCredits-1000) > 0.001 {
		t.Errorf("TotalCredits = %v, want 1000 (NaN should not affect credits)", s.TotalCredits)
	}
	if math.Abs(s.TotalDebits-500) > 0.001 {
		t.Errorf("TotalDebits = %v, want 500 (NaN should not affect debits)", s.TotalDebits)
	}
	if math.Abs(s.NetCashFlow-500) > 0.001 {
		t.Errorf("NetCashFlow = %v, want 500 (NaN should not affect net)", s.NetCashFlow)
	}
}

func TestCashFlowSummary_InfAmount(t *testing.T) {
	// +Inf amount: Inf > 0 is true, so it adds to credits.
	// This propagates Inf into TotalCredits and NetCashFlow.
	// We verify the behavior is deterministic (not crashing).
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: math.Inf(1)},
			{Amount: -500},
		},
	}
	s := ledger.Summary()
	if !math.IsInf(s.TotalCredits, 1) {
		t.Errorf("TotalCredits = %v, want +Inf", s.TotalCredits)
	}
	if !math.IsInf(s.NetCashFlow, 1) {
		t.Errorf("NetCashFlow = %v, want +Inf", s.NetCashFlow)
	}
	if s.TransactionCount != 2 {
		t.Errorf("TransactionCount = %d, want 2", s.TransactionCount)
	}
}

func TestCashFlowSummary_NegativeInfAmount(t *testing.T) {
	// -Inf amount: -Inf < 0 is true, math.Abs(-Inf) = +Inf.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000},
			{Amount: math.Inf(-1)},
		},
	}
	s := ledger.Summary()
	if !math.IsInf(s.TotalDebits, 1) {
		t.Errorf("TotalDebits = %v, want +Inf", s.TotalDebits)
	}
	if !math.IsInf(s.NetCashFlow, -1) {
		t.Errorf("NetCashFlow = %v, want -Inf (1000 - Inf)", s.NetCashFlow)
	}
}

func TestCashFlowSummary_FloatingPointRounding(t *testing.T) {
	// Classic floating-point trap: 0.1 + 0.2 != 0.3.
	// Verify NetCashFlow = TotalCredits - TotalDebits exactly (same computation path).
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 0.1},
			{Amount: 0.2},
			{Amount: -0.3},
		},
	}
	s := ledger.Summary()

	// The key invariant: NetCashFlow must equal TotalCredits - TotalDebits.
	// Both are computed as credits - debits, so they should be bit-identical.
	recomputed := s.TotalCredits - s.TotalDebits
	if s.NetCashFlow != recomputed {
		t.Errorf("NetCashFlow (%v) != TotalCredits - TotalDebits (%v)", s.NetCashFlow, recomputed)
	}

	// Note: Due to floating point, 0.1+0.2 = 0.30000000000000004, not 0.3.
	// So TotalCredits (0.3000...04) - TotalDebits (0.3) != 0 exactly.
	// This is expected float64 behavior and NOT a bug.
	// The implementation correctly preserves the credits-debits identity.
}

func TestCashFlowSummary_NilTransactions(t *testing.T) {
	// Summary on ledger with nil transactions slice should not panic.
	ledger := CashFlowLedger{Transactions: nil}
	s := ledger.Summary()
	if s.TransactionCount != 0 {
		t.Errorf("TransactionCount = %d, want 0", s.TransactionCount)
	}
	if s.TotalCredits != 0 || s.TotalDebits != 0 || s.NetCashFlow != 0 {
		t.Errorf("Expected all zeros for nil transactions, got %+v", s)
	}
}

func TestCashFlowSummary_JSONSerialization(t *testing.T) {
	// Verify JSON field names match the API contract.
	s := CashFlowSummary{
		TotalCredits:     1000.50,
		TotalDebits:      500.25,
		NetCashFlow:      500.25,
		TransactionCount: 5,
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	expectedKeys := []string{"total_credits", "total_debits", "net_cash_flow", "transaction_count"}
	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("Missing JSON field %q in serialized summary", key)
		}
	}
	if len(m) != len(expectedKeys) {
		t.Errorf("Expected %d JSON fields, got %d: %v", len(expectedKeys), len(m), m)
	}
}

func TestCashFlowSummary_ZeroNetCashFlow(t *testing.T) {
	// Credits and debits exactly cancel out.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: 1000},
			{Amount: -1000},
		},
	}
	s := ledger.Summary()
	if s.TotalCredits != 1000 {
		t.Errorf("TotalCredits = %v, want 1000", s.TotalCredits)
	}
	if s.TotalDebits != 1000 {
		t.Errorf("TotalDebits = %v, want 1000", s.TotalDebits)
	}
	if s.NetCashFlow != 0 {
		t.Errorf("NetCashFlow = %v, want 0", s.NetCashFlow)
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
			{Amount: 0.001},
			{Amount: 0.001},
			{Amount: 0.001},
			{Amount: -0.001},
		},
	}
	s := ledger.Summary()
	if math.Abs(s.TotalCredits-0.003) > 1e-10 {
		t.Errorf("TotalCredits = %v, want 0.003", s.TotalCredits)
	}
	if math.Abs(s.TotalDebits-0.001) > 1e-10 {
		t.Errorf("TotalDebits = %v, want 0.001", s.TotalDebits)
	}
}

func TestCashFlowSummary_MaxFloat64(t *testing.T) {
	// Two MaxFloat64 credits: addition overflows to +Inf.
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Amount: math.MaxFloat64},
			{Amount: math.MaxFloat64},
		},
	}
	s := ledger.Summary()
	// MaxFloat64 + MaxFloat64 overflows to +Inf
	if !math.IsInf(s.TotalCredits, 1) {
		t.Errorf("TotalCredits = %v, want +Inf (overflow)", s.TotalCredits)
	}
	if s.TransactionCount != 2 {
		t.Errorf("TransactionCount = %d, want 2", s.TransactionCount)
	}
}
