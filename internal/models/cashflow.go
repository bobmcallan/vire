package models

import (
	"time"
)

// CashDirection represents the direction of a cash transaction.
type CashDirection string

const (
	CashCredit CashDirection = "credit" // Money flowing into the account
	CashDebit  CashDirection = "debit"  // Money flowing out of the account
)

// validCashDirections lists all accepted directions.
var validCashDirections = map[CashDirection]bool{
	CashCredit: true,
	CashDebit:  true,
}

// ValidCashDirection returns true if d is a valid cash direction.
func ValidCashDirection(d CashDirection) bool {
	return validCashDirections[d]
}

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
	IsTransactional bool   `json:"is_transactional"`
}

// CashTransaction represents a single ledger entry: a credit or debit to a named account.
type CashTransaction struct {
	ID          string        `json:"id"`
	Direction   CashDirection `json:"direction"`           // credit or debit
	Account     string        `json:"account"`             // Named account (e.g. "Trading", "Stake Accumulate")
	Category    CashCategory  `json:"category"`            // contribution, dividend, transfer, fee, other
	Date        time.Time     `json:"date"`                // Transaction date
	Amount      float64       `json:"amount"`              // Always positive; direction determines sign
	Description string        `json:"description"`         // Required description
	LinkedID    string        `json:"linked_id,omitempty"` // Links paired transfer entries
	Notes       string        `json:"notes,omitempty"`     // Optional notes
	CreatedAt   time.Time     `json:"created_at"`          // Auto-set on creation
	UpdatedAt   time.Time     `json:"updated_at"`          // Auto-set on updates
}

// IsCredit returns true if this is a credit (money in).
func (tx CashTransaction) IsCredit() bool {
	return tx.Direction == CashCredit
}

// SignedAmount returns the amount with sign applied: positive for credits, negative for debits.
// This is the single source of truth for how a transaction affects a balance.
func (tx CashTransaction) SignedAmount() float64 {
	if tx.Direction == CashCredit {
		return tx.Amount
	}
	return -tx.Amount
}

// NetDeployedImpact returns this transaction's effect on net deployed capital.
// Contribution credits increase it. Non-dividend debits decrease it.
// Dividends are returns on investment, not capital deployment.
func (tx CashTransaction) NetDeployedImpact() float64 {
	switch tx.Category {
	case CashCatContribution:
		if tx.Direction == CashCredit {
			return tx.Amount
		}
	case CashCatOther, CashCatFee, CashCatTransfer:
		if tx.Direction == CashDebit {
			return -tx.Amount
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

// AccountBalance computes the ledger balance for a named account.
// Balance = sum of credits - sum of debits for that account.
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

// TotalDeposited returns the sum of all credit amounts.
func (l *CashFlowLedger) TotalDeposited() float64 {
	var total float64
	for _, tx := range l.Transactions {
		if tx.Direction == CashCredit {
			total += tx.Amount
		}
	}
	return total
}

// TotalWithdrawn returns the sum of all debit amounts.
func (l *CashFlowLedger) TotalWithdrawn() float64 {
	var total float64
	for _, tx := range l.Transactions {
		if tx.Direction == CashDebit {
			total += tx.Amount
		}
	}
	return total
}

// NetFlowForPeriod returns the net cash flow (credits - debits) within [from, to).
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

// CapitalPerformance contains computed capital deployment performance metrics.
type CapitalPerformance struct {
	TotalDeposited        float64                      `json:"total_deposited"`
	TotalWithdrawn        float64                      `json:"total_withdrawn"`
	NetCapitalDeployed    float64                      `json:"net_capital_deployed"`
	CurrentPortfolioValue float64                      `json:"current_portfolio_value"`
	SimpleReturnPct       float64                      `json:"simple_return_pct"`
	AnnualizedReturnPct   float64                      `json:"annualized_return_pct"`
	FirstTransactionDate  *time.Time                   `json:"first_transaction_date,omitempty"`
	TransactionCount      int                          `json:"transaction_count"`
	ExternalBalances      []ExternalBalancePerformance `json:"external_balances,omitempty"`
}

// ExternalBalancePerformance tracks transfer activity and gain/loss per external balance category.
type ExternalBalancePerformance struct {
	Category       string  `json:"category"`
	TotalOut       float64 `json:"total_out"`
	TotalIn        float64 `json:"total_in"`
	NetTransferred float64 `json:"net_transferred"`
	CurrentBalance float64 `json:"current_balance"`
	GainLoss       float64 `json:"gain_loss"`
}

// ExternalBalanceCategories are the valid external balance types.
var ExternalBalanceCategories = map[string]bool{
	"cash": true, "accumulate": true, "term_deposit": true, "offset": true,
}

// DefaultTradingAccount is the default name for the transactional account.
const DefaultTradingAccount = "Trading"
