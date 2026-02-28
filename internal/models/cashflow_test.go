package models

import (
	"math"
	"testing"
)

func TestValidCashCategory(t *testing.T) {
	tests := []struct {
		name  string
		cat   CashCategory
		valid bool
	}{
		{"contribution is valid", CashCatContribution, true},
		{"dividend is valid", CashCatDividend, true},
		{"transfer is valid", CashCatTransfer, true},
		{"fee is valid", CashCatFee, true},
		{"other is valid", CashCatOther, true},
		{"empty string is invalid", "", false},
		{"uppercase CONTRIBUTION is invalid", "CONTRIBUTION", false},
		{"deposit is invalid (legacy type)", "deposit", false},
		{"withdrawal is invalid (legacy type)", "withdrawal", false},
		{"arbitrary string is invalid", "groceries", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidCashCategory(tt.cat)
			if got != tt.valid {
				t.Errorf("ValidCashCategory(%q) = %v, want %v", tt.cat, got, tt.valid)
			}
		})
	}
}

func TestCashTransaction_SignedAmount(t *testing.T) {
	tests := []struct {
		name string
		tx   CashTransaction
		want float64
	}{
		{
			name: "positive amount returns positive",
			tx:   CashTransaction{Amount: 1000},
			want: 1000,
		},
		{
			name: "negative amount returns negative",
			tx:   CashTransaction{Amount: -500},
			want: -500,
		},
		{
			name: "zero returns zero",
			tx:   CashTransaction{Amount: 0},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tx.SignedAmount()
			if got != tt.want {
				t.Errorf("SignedAmount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCashFlowLedger_AccountBalance(t *testing.T) {
	tests := []struct {
		name    string
		ledger  CashFlowLedger
		account string
		want    float64
	}{
		{
			name: "single credit to one account",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 1000},
				},
			},
			account: "Trading",
			want:    1000,
		},
		{
			name: "single debit from one account",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: -500},
				},
			},
			account: "Trading",
			want:    -500,
		},
		{
			name: "credits minus debits for same account",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 1000},
					{Account: "Trading", Amount: -300},
					{Account: "Trading", Amount: 200},
				},
			},
			account: "Trading",
			want:    900,
		},
		{
			name: "filters by account name",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 1000},
					{Account: "Stake Accumulate", Amount: 5000},
					{Account: "Trading", Amount: -200},
				},
			},
			account: "Trading",
			want:    800,
		},
		{
			name: "returns zero for account with no transactions",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 1000},
				},
			},
			account: "Savings",
			want:    0,
		},
		{
			name:    "returns zero for empty ledger",
			ledger:  CashFlowLedger{},
			account: "Trading",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ledger.AccountBalance(tt.account)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("AccountBalance(%q) = %v, want %v", tt.account, got, tt.want)
			}
		})
	}
}

func TestCashFlowLedger_TotalCashBalance(t *testing.T) {
	tests := []struct {
		name   string
		ledger CashFlowLedger
		want   float64
	}{
		{
			name:   "empty ledger returns zero",
			ledger: CashFlowLedger{},
			want:   0,
		},
		{
			name: "all credits",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 1000},
					{Account: "Savings", Amount: 500},
				},
			},
			want: 1500,
		},
		{
			name: "credits minus debits across accounts",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 5000},
					{Account: "Trading", Amount: -1000},
					{Account: "Stake Accumulate", Amount: 1000},
					{Account: "Stake Accumulate", Amount: -200},
				},
			},
			want: 4800,
		},
		{
			name: "transfers cancel out (net zero)",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 5000, Category: CashCatContribution},
					{Account: "Trading", Amount: -2000, Category: CashCatTransfer},
					{Account: "Stake Accumulate", Amount: 2000, Category: CashCatTransfer},
				},
			},
			want: 5000,
		},
		{
			name: "debits can exceed credits",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 100},
					{Account: "Trading", Amount: -500},
				},
			},
			want: -400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ledger.TotalCashBalance()
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("TotalCashBalance() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCashFlowLedger_TotalContributions(t *testing.T) {
	tests := []struct {
		name   string
		ledger CashFlowLedger
		want   float64
	}{
		{
			name:   "empty ledger returns zero",
			ledger: CashFlowLedger{},
			want:   0,
		},
		{
			name: "contributions only",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 1000, Category: CashCatContribution},
					{Account: "Trading", Amount: 2000, Category: CashCatContribution},
				},
			},
			want: 3000,
		},
		{
			name: "paired transfers net to zero",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 5000, Category: CashCatContribution},
					{Account: "Trading", Amount: -2000, Category: CashCatTransfer},
					{Account: "Stake Accumulate", Amount: 2000, Category: CashCatTransfer},
				},
			},
			want: 5000,
		},
		{
			name: "includes dividends and fees",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 5000, Category: CashCatContribution},
					{Account: "Trading", Amount: 100, Category: CashCatDividend},
					{Account: "Trading", Amount: -50, Category: CashCatFee},
				},
			},
			want: 5050,
		},
		{
			name: "non-transfer debits reduce contributions",
			ledger: CashFlowLedger{
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 10000, Category: CashCatContribution},
					{Account: "Trading", Amount: -3000, Category: CashCatOther},
				},
			},
			want: 7000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ledger.TotalContributions()
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("TotalContributions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCashFlowLedger_NonTransactionalBalance(t *testing.T) {
	tests := []struct {
		name   string
		ledger CashFlowLedger
		want   float64
	}{
		{
			name:   "empty ledger returns zero",
			ledger: CashFlowLedger{},
			want:   0,
		},
		{
			name: "only non-transactional accounts counted",
			ledger: CashFlowLedger{
				Accounts: []CashAccount{
					{Name: "Trading", IsTransactional: true},
					{Name: "Accumulate", IsTransactional: false},
				},
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 5000},
					{Account: "Accumulate", Amount: 2000},
				},
			},
			want: 2000,
		},
		{
			name: "multiple non-transactional accounts",
			ledger: CashFlowLedger{
				Accounts: []CashAccount{
					{Name: "Trading", IsTransactional: true},
					{Name: "Accumulate", IsTransactional: false},
					{Name: "Term Deposit", IsTransactional: false},
				},
				Transactions: []CashTransaction{
					{Account: "Trading", Amount: 5000},
					{Account: "Accumulate", Amount: 2000},
					{Account: "Term Deposit", Amount: 10000},
					{Account: "Accumulate", Amount: -500},
				},
			},
			want: 11500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ledger.NonTransactionalBalance()
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("NonTransactionalBalance() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCashFlowLedger_HasAccount(t *testing.T) {
	ledger := CashFlowLedger{
		Accounts: []CashAccount{
			{Name: "Trading", IsTransactional: true},
			{Name: "Stake Accumulate", IsTransactional: false},
		},
	}

	tests := []struct {
		name string
		acct string
		want bool
	}{
		{"existing transactional account", "Trading", true},
		{"existing non-transactional account", "Stake Accumulate", true},
		{"non-existing account", "Savings", false},
		{"empty string", "", false},
		{"case sensitive mismatch", "trading", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ledger.HasAccount(tt.acct)
			if got != tt.want {
				t.Errorf("HasAccount(%q) = %v, want %v", tt.acct, got, tt.want)
			}
		})
	}
}

func TestCashFlowLedger_HasAccount_EmptyLedger(t *testing.T) {
	ledger := CashFlowLedger{}
	if ledger.HasAccount("Trading") {
		t.Error("HasAccount on empty ledger should return false")
	}
}

func TestCashFlowLedger_GetAccount(t *testing.T) {
	ledger := CashFlowLedger{
		Accounts: []CashAccount{
			{Name: "Trading", IsTransactional: true},
			{Name: "Stake Accumulate", IsTransactional: false},
		},
	}

	tests := []struct {
		name         string
		acct         string
		wantNil      bool
		wantTransact bool
	}{
		{"existing transactional account", "Trading", false, true},
		{"existing non-transactional account", "Stake Accumulate", false, false},
		{"non-existing account returns nil", "Savings", true, false},
		{"empty string returns nil", "", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ledger.GetAccount(tt.acct)
			if tt.wantNil {
				if got != nil {
					t.Errorf("GetAccount(%q) = %+v, want nil", tt.acct, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("GetAccount(%q) = nil, want non-nil", tt.acct)
			}
			if got.Name != tt.acct {
				t.Errorf("GetAccount(%q).Name = %q, want %q", tt.acct, got.Name, tt.acct)
			}
			if got.IsTransactional != tt.wantTransact {
				t.Errorf("GetAccount(%q).IsTransactional = %v, want %v", tt.acct, got.IsTransactional, tt.wantTransact)
			}
		})
	}
}

func TestCashFlowLedger_GetAccount_ReturnsPointer(t *testing.T) {
	ledger := CashFlowLedger{
		Accounts: []CashAccount{
			{Name: "Trading", IsTransactional: true},
		},
	}

	got := ledger.GetAccount("Trading")
	if got == nil {
		t.Fatal("GetAccount returned nil")
	}

	// Modifying via pointer should modify the original
	got.IsTransactional = false
	if ledger.Accounts[0].IsTransactional != false {
		t.Error("GetAccount should return a pointer to the underlying slice element")
	}
}
