package models

import (
	"testing"
)

func TestValidCashTransactionType(t *testing.T) {
	valid := []CashTransactionType{
		CashTxDeposit, CashTxWithdrawal, CashTxContribution,
		CashTxTransferIn, CashTxTransferOut, CashTxDividend,
	}
	for _, tt := range valid {
		if !ValidCashTransactionType(tt) {
			t.Errorf("ValidCashTransactionType(%q) = false, want true", tt)
		}
	}

	invalid := []CashTransactionType{"", "refund", "DEPOSIT", "unknown"}
	for _, tt := range invalid {
		if ValidCashTransactionType(tt) {
			t.Errorf("ValidCashTransactionType(%q) = true, want false", tt)
		}
	}
}

func TestIsInflowType(t *testing.T) {
	inflows := []CashTransactionType{CashTxDeposit, CashTxContribution, CashTxTransferIn, CashTxDividend}
	for _, tt := range inflows {
		if !IsInflowType(tt) {
			t.Errorf("IsInflowType(%q) = false, want true", tt)
		}
	}

	outflows := []CashTransactionType{CashTxWithdrawal, CashTxTransferOut}
	for _, tt := range outflows {
		if IsInflowType(tt) {
			t.Errorf("IsInflowType(%q) = true, want false", tt)
		}
	}
}

func TestIsInternalTransfer(t *testing.T) {
	tests := []struct {
		name string
		tx   CashTransaction
		want bool
	}{
		{
			name: "transfer_out with accumulate category",
			tx:   CashTransaction{Type: CashTxTransferOut, Category: "accumulate"},
			want: true,
		},
		{
			name: "transfer_out with cash category",
			tx:   CashTransaction{Type: CashTxTransferOut, Category: "cash"},
			want: true,
		},
		{
			name: "transfer_out with term_deposit category",
			tx:   CashTransaction{Type: CashTxTransferOut, Category: "term_deposit"},
			want: true,
		},
		{
			name: "transfer_out with offset category",
			tx:   CashTransaction{Type: CashTxTransferOut, Category: "offset"},
			want: true,
		},
		{
			name: "transfer_in with accumulate category",
			tx:   CashTransaction{Type: CashTxTransferIn, Category: "accumulate"},
			want: true,
		},
		{
			name: "transfer_in with cash category",
			tx:   CashTransaction{Type: CashTxTransferIn, Category: "cash"},
			want: true,
		},
		{
			name: "transfer_out with empty category",
			tx:   CashTransaction{Type: CashTxTransferOut, Category: ""},
			want: false,
		},
		{
			name: "transfer_out with unknown category",
			tx:   CashTransaction{Type: CashTxTransferOut, Category: "groceries"},
			want: false,
		},
		{
			name: "deposit with accumulate category",
			tx:   CashTransaction{Type: CashTxDeposit, Category: "accumulate"},
			want: false,
		},
		{
			name: "withdrawal with cash category",
			tx:   CashTransaction{Type: CashTxWithdrawal, Category: "cash"},
			want: false,
		},
		{
			name: "contribution with offset category",
			tx:   CashTransaction{Type: CashTxContribution, Category: "offset"},
			want: false,
		},
		{
			name: "dividend with term_deposit category",
			tx:   CashTransaction{Type: CashTxDividend, Category: "term_deposit"},
			want: false,
		},
		{
			name: "empty type with accumulate category",
			tx:   CashTransaction{Type: "", Category: "accumulate"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.tx.IsInternalTransfer()
			if got != tt.want {
				t.Errorf("IsInternalTransfer() = %v, want %v (type=%q, category=%q)",
					got, tt.want, tt.tx.Type, tt.tx.Category)
			}
		})
	}
}

func TestExternalBalanceCategories(t *testing.T) {
	expected := []string{"cash", "accumulate", "term_deposit", "offset"}
	for _, cat := range expected {
		if !ExternalBalanceCategories[cat] {
			t.Errorf("ExternalBalanceCategories[%q] = false, want true", cat)
		}
	}

	invalid := []string{"", "equity", "stock", "bond"}
	for _, cat := range invalid {
		if ExternalBalanceCategories[cat] {
			t.Errorf("ExternalBalanceCategories[%q] = true, want false", cat)
		}
	}
}
