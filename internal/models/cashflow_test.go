package models

import "testing"

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
