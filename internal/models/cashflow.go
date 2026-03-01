package models

import (
	"math"
	"time"
)

// CashCategory categorizes the purpose of a cash transaction.
type CashCategory string

const (
	CashCatContribution CashCategory = "contribution"
	CashCatDividend     CashCategory = "dividend"
	CashCatTransfer     CashCategory = "transfer"
	CashCatFee          CashCategory = "fee"
	CashCatOther        CashCategory = "other"
)

// validCashCategories lists all accepted categories.
var validCashCategories = map[CashCategory]bool{
	CashCatContribution: true,
	CashCatDividend:     true,
	CashCatTransfer:     true,
	CashCatFee:          true,
	CashCatOther:        true,
}

// ValidCashCategory returns true if c is a valid cash category.
func ValidCashCategory(c CashCategory) bool {
	return validCashCategories[c]
}

// CashAccount represents a named cash account within the portfolio ledger.
// Transactional accounts have Navexa trade settlements (buys as debits,
// sells as credits) flow into their balance automatically.
type CashAccount struct {
	Name            string `json:"name"`
	Type            string `json:"type"` // trading (default), accumulate, term_deposit, offset
	IsTransactional bool   `json:"is_transactional"`
}

// CashAccountUpdate is the update payload for UpdateAccount.
// IsTransactional uses *bool so callers can distinguish "not provided" from "false".
type CashAccountUpdate struct {
	Type            string `json:"type,omitempty"`
	IsTransactional *bool  `json:"is_transactional,omitempty"`
}

// ValidAccountTypes are the valid values for CashAccount.Type.
var ValidAccountTypes = map[string]bool{
	"trading": true, "accumulate": true, "term_deposit": true, "offset": true, "other": true,
}

// CashTransaction represents a single ledger entry.
// Positive Amount = money in (credit), negative Amount = money out (debit).
type CashTransaction struct {
	ID          string       `json:"id"`
	Account     string       `json:"account"`             // Named account (e.g. "Trading", "Stake Accumulate")
	Category    CashCategory `json:"category"`            // contribution, dividend, transfer, fee, other
	Date        time.Time    `json:"date"`                // Transaction date
	Amount      float64      `json:"amount"`              // Positive = money in (credit), negative = money out (debit)
	Description string       `json:"description"`         // Required description
	LinkedID    string       `json:"linked_id,omitempty"` // Links paired transfer entries
	Notes       string       `json:"notes,omitempty"`     // Optional notes
	CreatedAt   time.Time    `json:"created_at"`          // Auto-set on creation
	UpdatedAt   time.Time    `json:"updated_at"`          // Auto-set on updates
}

// SignedAmount returns the amount with sign applied.
// With signed amounts, this simply returns tx.Amount (already signed).
func (tx CashTransaction) SignedAmount() float64 {
	return tx.Amount
}

// NetDeployedImpact returns this transaction's effect on net deployed capital.
// Positive contributions increase it. Negative non-dividend transactions decrease it.
// Dividends are returns on investment, not capital deployment.
func (tx CashTransaction) NetDeployedImpact() float64 {
	switch tx.Category {
	case CashCatContribution:
		if tx.Amount > 0 {
			return tx.Amount
		}
	case CashCatOther, CashCatFee, CashCatTransfer:
		if tx.Amount < 0 {
			return tx.Amount
		}
	}
	return 0
}

// CashFlowLedger stores all cash accounts and transactions for a portfolio.
type CashFlowLedger struct {
	PortfolioName string            `json:"portfolio_name"`
	Version       int               `json:"version"`
	Accounts      []CashAccount     `json:"accounts"`
	Transactions  []CashTransaction `json:"transactions"`
	Notes         string            `json:"notes,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// CashFlowSummary contains server-computed aggregate totals for the ledger.
type CashFlowSummary struct {
	TotalCash        float64            `json:"total_cash"`        // Sum of all account balances
	TransactionCount int                `json:"transaction_count"` // Total number of transactions
	ByCategory       map[string]float64 `json:"by_category"`       // Net amount per category
}

// Summary computes aggregate totals across all transactions in the ledger.
func (l *CashFlowLedger) Summary() CashFlowSummary {
	byCategory := make(map[string]float64)
	for _, tx := range l.Transactions {
		byCategory[string(tx.Category)] += tx.Amount
	}
	// Ensure all known categories are present (even if zero).
	for _, cat := range []CashCategory{CashCatContribution, CashCatDividend, CashCatTransfer, CashCatFee, CashCatOther} {
		if _, ok := byCategory[string(cat)]; !ok {
			byCategory[string(cat)] = 0
		}
	}
	return CashFlowSummary{
		TotalCash:        l.TotalCashBalance(),
		TransactionCount: len(l.Transactions),
		ByCategory:       byCategory,
	}
}

// AccountBalance computes the ledger balance for a named account.
func (l *CashFlowLedger) AccountBalance(accountName string) float64 {
	var balance float64
	for _, tx := range l.Transactions {
		if tx.Account == accountName {
			balance += tx.SignedAmount()
		}
	}
	return balance
}

// TotalCashBalance returns the sum of all account balances.
func (l *CashFlowLedger) TotalCashBalance() float64 {
	var total float64
	for _, tx := range l.Transactions {
		total += tx.SignedAmount()
	}
	return total
}

// TotalContributions returns the sum of all credits minus all debits.
func (l *CashFlowLedger) TotalContributions() float64 {
	return l.TotalCashBalance()
}

// TotalDeposited returns the sum of all positive contribution amounts.
// Only category=contribution counts as capital deposited into the fund.
// Dividends, transfers, and fees are not deposits.
func (l *CashFlowLedger) TotalDeposited() float64 {
	var total float64
	for _, tx := range l.Transactions {
		if tx.Category == CashCatContribution && tx.Amount > 0 {
			total += tx.Amount
		}
	}
	return total
}

// TotalWithdrawn returns the sum of absolute values of negative contribution amounts.
// Only category=contribution counts as capital withdrawn from the fund.
// Transfer debits, fees, and dividends are not withdrawals of capital.
func (l *CashFlowLedger) TotalWithdrawn() float64 {
	var total float64
	for _, tx := range l.Transactions {
		if tx.Category == CashCatContribution && tx.Amount < 0 {
			total += math.Abs(tx.Amount)
		}
	}
	return total
}

// NetFlowForPeriod returns the net cash flow within [from, to).
// Optionally excludes specified categories (e.g. dividends).
func (l *CashFlowLedger) NetFlowForPeriod(from, to time.Time, excludeCategories ...CashCategory) float64 {
	exclude := make(map[CashCategory]bool, len(excludeCategories))
	for _, c := range excludeCategories {
		exclude[c] = true
	}
	var total float64
	for _, tx := range l.Transactions {
		txDate := tx.Date.Truncate(24 * time.Hour)
		if txDate.Before(from) || !txDate.Before(to) {
			continue
		}
		if exclude[tx.Category] {
			continue
		}
		total += tx.SignedAmount()
	}
	return total
}

// FirstTransactionDate returns the earliest transaction date, or nil if empty.
func (l *CashFlowLedger) FirstTransactionDate() *time.Time {
	var first *time.Time
	for _, tx := range l.Transactions {
		if first == nil || tx.Date.Before(*first) {
			d := tx.Date
			first = &d
		}
	}
	return first
}

// HasAccount returns true if the ledger has an account with the given name.
func (l *CashFlowLedger) HasAccount(name string) bool {
	for _, a := range l.Accounts {
		if a.Name == name {
			return true
		}
	}
	return false
}

// GetAccount returns the account with the given name, or nil.
func (l *CashFlowLedger) GetAccount(name string) *CashAccount {
	for i := range l.Accounts {
		if l.Accounts[i].Name == name {
			return &l.Accounts[i]
		}
	}
	return nil
}

// NonTransactionalBalance returns the sum of balances for all non-transactional accounts.
func (l *CashFlowLedger) NonTransactionalBalance() float64 {
	nonTx := make(map[string]bool)
	for _, a := range l.Accounts {
		if !a.IsTransactional {
			nonTx[a.Name] = true
		}
	}
	var total float64
	for _, tx := range l.Transactions {
		if nonTx[tx.Account] {
			total += tx.SignedAmount()
		}
	}
	return total
}

// CapitalPerformance contains computed capital deployment performance metrics.
type CapitalPerformance struct {
	TotalDeposited        float64    `json:"total_deposited"`
	TotalWithdrawn        float64    `json:"total_withdrawn"`
	NetCapitalDeployed    float64    `json:"net_capital_deployed"`
	CurrentPortfolioValue float64    `json:"current_portfolio_value"`
	SimpleReturnPct       float64    `json:"simple_return_pct"`
	AnnualizedReturnPct   float64    `json:"annualized_return_pct"`
	FirstTransactionDate  *time.Time `json:"first_transaction_date,omitempty"`
	TransactionCount      int        `json:"transaction_count"`
}

// DefaultTradingAccount is the default name for the transactional account.
const DefaultTradingAccount = "Trading"
