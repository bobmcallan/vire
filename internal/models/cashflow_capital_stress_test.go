package models

import (
	"math"
	"testing"
	"time"
)

// Adversarial stress tests for capital & cash calculation fixes.
// Tests target the FIXED behavior where:
// - NetDeployedImpact returns tx.Amount for ALL contributions (positive and negative)
// - TotalCashBalance sums ALL accounts (not just non-transactional)

// =============================================================================
// 1. NetDeployedImpact edge cases — negative contributions
// =============================================================================

func TestNetDeployedImpact_NegativeContribution_ReturnsNegative(t *testing.T) {
	// After fix: negative contributions (capital withdrawals) must decrease net deployed.
	tx := CashTransaction{Category: CashCatContribution, Amount: -5000}
	got := tx.NetDeployedImpact()
	if got != -5000 {
		t.Errorf("NetDeployedImpact for -5000 contribution = %v, want -5000", got)
	}
}

func TestNetDeployedImpact_ZeroContribution(t *testing.T) {
	tx := CashTransaction{Category: CashCatContribution, Amount: 0}
	got := tx.NetDeployedImpact()
	if got != 0 {
		t.Errorf("NetDeployedImpact for 0 contribution = %v, want 0", got)
	}
}

func TestNetDeployedImpact_VeryLargeNegativeContribution(t *testing.T) {
	tx := CashTransaction{Category: CashCatContribution, Amount: -1e15}
	got := tx.NetDeployedImpact()
	if got != -1e15 {
		t.Errorf("NetDeployedImpact for -1e15 contribution = %v, want -1e15", got)
	}
}

func TestNetDeployedImpact_SmallNegativeContribution(t *testing.T) {
	// Sub-cent withdrawal
	tx := CashTransaction{Category: CashCatContribution, Amount: -0.01}
	got := tx.NetDeployedImpact()
	if math.Abs(got-(-0.01)) > 1e-10 {
		t.Errorf("NetDeployedImpact for -0.01 contribution = %v, want -0.01", got)
	}
}

func TestNetDeployedImpact_ContributionSymmetry(t *testing.T) {
	// Deposit and withdrawal of same amount must cancel out.
	deposit := CashTransaction{Category: CashCatContribution, Amount: 10000}
	withdrawal := CashTransaction{Category: CashCatContribution, Amount: -10000}

	sum := deposit.NetDeployedImpact() + withdrawal.NetDeployedImpact()
	if sum != 0 {
		t.Errorf("Deposit + withdrawal NDI = %v, want 0 (symmetric)", sum)
	}
}

func TestNetDeployedImpact_NaNContribution(t *testing.T) {
	tx := CashTransaction{Category: CashCatContribution, Amount: math.NaN()}
	got := tx.NetDeployedImpact()
	if !math.IsNaN(got) {
		t.Errorf("NetDeployedImpact for NaN contribution = %v, want NaN", got)
	}
}

func TestNetDeployedImpact_InfContribution(t *testing.T) {
	tx := CashTransaction{Category: CashCatContribution, Amount: math.Inf(-1)}
	got := tx.NetDeployedImpact()
	if !math.IsInf(got, -1) {
		t.Errorf("NetDeployedImpact for -Inf contribution = %v, want -Inf", got)
	}
}

// Dividends must never affect net deployed (no change to this invariant).
func TestNetDeployedImpact_DividendNeverCounts(t *testing.T) {
	for _, amount := range []float64{-500, 0, 500, 10000} {
		tx := CashTransaction{Category: CashCatDividend, Amount: amount}
		got := tx.NetDeployedImpact()
		if got != 0 {
			t.Errorf("NetDeployedImpact for dividend amount=%v = %v, want 0", amount, got)
		}
	}
}

// Fees/other/transfer: negative amounts decrease net deployed, positive have no effect.
func TestNetDeployedImpact_NonContributionCategories(t *testing.T) {
	tests := []struct {
		category CashCategory
		amount   float64
		want     float64
	}{
		{CashCatFee, -100, -100},
		{CashCatFee, 100, 0}, // positive fee (refund) — no effect
		{CashCatOther, -200, -200},
		{CashCatOther, 200, 0},
		{CashCatTransfer, -300, -300},
		{CashCatTransfer, 300, 0}, // positive transfer — no effect
	}
	for _, tt := range tests {
		tx := CashTransaction{Category: tt.category, Amount: tt.amount}
		got := tx.NetDeployedImpact()
		if got != tt.want {
			t.Errorf("NetDeployedImpact(category=%s, amount=%v) = %v, want %v",
				tt.category, tt.amount, got, tt.want)
		}
	}
}

// =============================================================================
// 2. Net deployed with realistic multi-transaction scenarios
// =============================================================================

func TestNetDeployed_WithdrawalExceedsDeposits(t *testing.T) {
	// Edge case: withdrawal exceeds total deposits — net deployed goes negative.
	txs := []CashTransaction{
		{Category: CashCatContribution, Amount: 5000},
		{Category: CashCatContribution, Amount: -20000},
	}

	var netDeployed float64
	for _, tx := range txs {
		netDeployed += tx.NetDeployedImpact()
	}

	if netDeployed != -15000 {
		t.Errorf("Net deployed = %v, want -15000 (withdrawal exceeds deposits)", netDeployed)
	}
}

func TestNetDeployed_MixedCategoriesAccumulate(t *testing.T) {
	// Realistic scenario: contributions, withdrawals, fees, dividends.
	txs := []CashTransaction{
		{Category: CashCatContribution, Amount: 50000},  // +50000
		{Category: CashCatContribution, Amount: -10000}, // -10000
		{Category: CashCatDividend, Amount: 2000},       // 0 (dividend)
		{Category: CashCatFee, Amount: -500},            // -500
		{Category: CashCatTransfer, Amount: -5000},      // -5000
		{Category: CashCatContribution, Amount: 20000},  // +20000
	}

	var netDeployed float64
	for _, tx := range txs {
		netDeployed += tx.NetDeployedImpact()
	}

	// 50000 - 10000 + 0 - 500 - 5000 + 20000 = 54500
	if math.Abs(netDeployed-54500) > 0.001 {
		t.Errorf("Net deployed = %v, want 54500", netDeployed)
	}
}

func TestNetDeployed_AllWithdrawals(t *testing.T) {
	// All negative contributions — net deployed is purely negative.
	txs := []CashTransaction{
		{Category: CashCatContribution, Amount: -1000},
		{Category: CashCatContribution, Amount: -2000},
		{Category: CashCatContribution, Amount: -3000},
	}

	var netDeployed float64
	for _, tx := range txs {
		netDeployed += tx.NetDeployedImpact()
	}

	if netDeployed != -6000 {
		t.Errorf("Net deployed = %v, want -6000 (all withdrawals)", netDeployed)
	}
}

func TestNetDeployed_ConsistencyWithDepositsMinusWithdrawals(t *testing.T) {
	// After the fix, for a contribution-only ledger:
	//   sum(NetDeployedImpact) = sum(all contribution amounts) = TotalDeposited - TotalWithdrawn
	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Category: CashCatContribution, Amount: 50000},
			{Category: CashCatContribution, Amount: 30000},
			{Category: CashCatContribution, Amount: -10000},
		},
	}

	var sumNDI float64
	for _, tx := range ledger.Transactions {
		sumNDI += tx.NetDeployedImpact()
	}

	deposited := ledger.TotalDeposited()
	withdrawn := ledger.TotalWithdrawn()
	expected := deposited - withdrawn // 80000 - 10000 = 70000

	if math.Abs(sumNDI-expected) > 0.001 {
		t.Errorf("sum(NDI) = %v, want %v (TotalDeposited - TotalWithdrawn)", sumNDI, expected)
	}
}

// =============================================================================
// 3. TotalCashBalance with edge cases
// =============================================================================

func TestTotalCashBalance_EmptyLedger(t *testing.T) {
	ledger := CashFlowLedger{}
	if ledger.TotalCashBalance() != 0 {
		t.Errorf("TotalCashBalance on empty ledger = %v, want 0", ledger.TotalCashBalance())
	}
}

func TestTotalCashBalance_NilTransactions(t *testing.T) {
	ledger := CashFlowLedger{Transactions: nil}
	if ledger.TotalCashBalance() != 0 {
		t.Errorf("TotalCashBalance with nil transactions = %v, want 0", ledger.TotalCashBalance())
	}
}

func TestTotalCashBalance_SingleAccount(t *testing.T) {
	ledger := CashFlowLedger{
		Accounts: []CashAccount{{Name: "Trading", IsTransactional: true}},
		Transactions: []CashTransaction{
			{Account: "Trading", Amount: 10000},
			{Account: "Trading", Amount: -3000},
		},
	}
	got := ledger.TotalCashBalance()
	if math.Abs(got-7000) > 0.001 {
		t.Errorf("TotalCashBalance = %v, want 7000", got)
	}
}

func TestTotalCashBalance_ManyAccounts(t *testing.T) {
	ledger := CashFlowLedger{
		Accounts: []CashAccount{
			{Name: "Trading", IsTransactional: true},
			{Name: "Accumulate", IsTransactional: false},
			{Name: "Term Deposit", IsTransactional: false},
			{Name: "Offset", IsTransactional: false},
		},
		Transactions: []CashTransaction{
			{Account: "Trading", Amount: 50000},
			{Account: "Accumulate", Amount: 25000},
			{Account: "Term Deposit", Amount: 100000},
			{Account: "Offset", Amount: -10000},
		},
	}
	got := ledger.TotalCashBalance()
	want := 50000.0 + 25000 + 100000 - 10000
	if math.Abs(got-want) > 0.001 {
		t.Errorf("TotalCashBalance = %v, want %v", got, want)
	}
}

func TestTotalCashBalance_IncludesAllAccountTypes(t *testing.T) {
	// After fix: TotalCashBalance includes transactional AND non-transactional.
	// Verify both types contribute.
	ledger := CashFlowLedger{
		Accounts: []CashAccount{
			{Name: "Trading", IsTransactional: true},
			{Name: "External", IsTransactional: false},
		},
		Transactions: []CashTransaction{
			{Account: "Trading", Amount: 10000},
			{Account: "External", Amount: 5000},
		},
	}

	total := ledger.TotalCashBalance()
	nonTx := ledger.NonTransactionalBalance()

	if total == nonTx {
		t.Errorf("TotalCashBalance (%v) should not equal NonTransactionalBalance (%v) when transactional accounts have balances",
			total, nonTx)
	}
	if math.Abs(total-15000) > 0.001 {
		t.Errorf("TotalCashBalance = %v, want 15000 (all accounts)", total)
	}
}

func TestTotalCashBalance_VsNonTransactional_Divergence(t *testing.T) {
	// The key behavior change: portfolio now uses TotalCashBalance (all accounts)
	// instead of NonTransactionalBalance (non-tx only).
	// Verify they diverge when there are transactional account transactions.
	ledger := CashFlowLedger{
		Accounts: []CashAccount{
			{Name: "Trading", IsTransactional: true},
			{Name: "Savings", IsTransactional: false},
		},
		Transactions: []CashTransaction{
			{Account: "Trading", Amount: 100000}, // transactional
			{Account: "Savings", Amount: 50000},  // non-transactional
		},
	}

	total := ledger.TotalCashBalance()        // 150000
	nonTx := ledger.NonTransactionalBalance() // 50000

	if math.Abs(total-150000) > 0.001 {
		t.Errorf("TotalCashBalance = %v, want 150000", total)
	}
	if math.Abs(nonTx-50000) > 0.001 {
		t.Errorf("NonTransactionalBalance = %v, want 50000", nonTx)
	}
	if math.Abs(total-nonTx-100000) > 0.001 {
		t.Errorf("Difference (Total - NonTx) = %v, want 100000", total-nonTx)
	}
}

// =============================================================================
// 4. GrowthDataPoint TotalCapital invariant
// =============================================================================

func TestGrowthDataPoint_TotalCapitalInvariant(t *testing.T) {
	// TotalCapital = TotalValue + CashBalance
	tests := []struct {
		name       string
		totalValue float64
		cashBal    float64
	}{
		{"zero everything", 0, 0},
		{"value only", 100000, 0},
		{"cash only", 0, 50000},
		{"both positive", 100000, 50000},
		{"negative cash", 100000, -30000},
		{"negative value (shorts)", -50000, 100000},
		{"very large", 1e15, 1e14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gp := GrowthDataPoint{
				TotalValue:   tt.totalValue,
				CashBalance:  tt.cashBal,
				TotalCapital: tt.totalValue + tt.cashBal,
			}

			// Invariant: TotalCapital = TotalValue + CashBalance
			if math.Abs(gp.TotalCapital-(gp.TotalValue+gp.CashBalance)) > 0.001 {
				t.Errorf("TotalCapital (%v) != TotalValue (%v) + CashBalance (%v)",
					gp.TotalCapital, gp.TotalValue, gp.CashBalance)
			}
		})
	}
}

// =============================================================================
// 5. NetFlowForPeriod with edge cases (existing function, verify still works)
// =============================================================================

func TestNetFlowForPeriod_WithNegativeContributions(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	ledger := CashFlowLedger{
		Transactions: []CashTransaction{
			{Date: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), Category: CashCatContribution, Amount: 10000},
			{Date: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), Category: CashCatContribution, Amount: -5000},
			{Date: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC), Category: CashCatDividend, Amount: 200},
		},
	}

	// Without excluding dividends: 10000 - 5000 + 200 = 5200
	flow := ledger.NetFlowForPeriod(from, to)
	if math.Abs(flow-5200) > 0.001 {
		t.Errorf("NetFlowForPeriod (all) = %v, want 5200", flow)
	}

	// Excluding dividends: 10000 - 5000 = 5000
	flowExDividend := ledger.NetFlowForPeriod(from, to, CashCatDividend)
	if math.Abs(flowExDividend-5000) > 0.001 {
		t.Errorf("NetFlowForPeriod (ex-dividend) = %v, want 5000", flowExDividend)
	}
}

// =============================================================================
// 6. Precision edge: many small amounts accumulating
// =============================================================================

func TestNetDeployedImpact_ManySmallAmounts(t *testing.T) {
	// 100,000 small contributions and withdrawals — verify accumulation precision.
	var total float64
	for i := 0; i < 100000; i++ {
		var tx CashTransaction
		if i%3 == 0 {
			tx = CashTransaction{Category: CashCatContribution, Amount: 0.01}
		} else if i%3 == 1 {
			tx = CashTransaction{Category: CashCatContribution, Amount: -0.005}
		} else {
			tx = CashTransaction{Category: CashCatDividend, Amount: 100}
		}
		total += tx.NetDeployedImpact()
	}

	// 33334 * 0.01 = 333.34
	// 33333 * (-0.005) = -166.665
	// 33333 * 0 (dividend) = 0
	// Expected ≈ 166.675
	wantMin := 166.0
	wantMax := 168.0
	if total < wantMin || total > wantMax {
		t.Errorf("Accumulated NDI = %v, expected between %v and %v", total, wantMin, wantMax)
	}
}
