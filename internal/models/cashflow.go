package models

import "time"

// CashTransactionType categorizes the direction and purpose of a cash transaction.
type CashTransactionType string

const (
	CashTxDeposit      CashTransactionType = "deposit"
	CashTxWithdrawal   CashTransactionType = "withdrawal"
	CashTxContribution CashTransactionType = "contribution"
	CashTxTransferIn   CashTransactionType = "transfer_in"
	CashTxTransferOut  CashTransactionType = "transfer_out"
	CashTxDividend     CashTransactionType = "dividend"
)

// validCashTransactionTypes lists all accepted transaction types.
var validCashTransactionTypes = map[CashTransactionType]bool{
	CashTxDeposit:      true,
	CashTxWithdrawal:   true,
	CashTxContribution: true,
	CashTxTransferIn:   true,
	CashTxTransferOut:  true,
	CashTxDividend:     true,
}

// ValidCashTransactionType returns true if t is a valid cash transaction type.
func ValidCashTransactionType(t CashTransactionType) bool {
	return validCashTransactionTypes[t]
}

// IsInflowType returns true if the transaction type represents money flowing in.
// Inflows: deposit, contribution, transfer_in, dividend.
// Outflows: withdrawal, transfer_out.
func IsInflowType(t CashTransactionType) bool {
	switch t {
	case CashTxDeposit, CashTxContribution, CashTxTransferIn, CashTxDividend:
		return true
	default:
		return false
	}
}

// CashTransaction represents a single cash flow event (deposit, withdrawal, etc.).
type CashTransaction struct {
	ID          string              `json:"id"`
	Type        CashTransactionType `json:"type"`
	Date        time.Time           `json:"date"`
	Amount      float64             `json:"amount"`
	Description string              `json:"description"`
	Category    string              `json:"category,omitempty"`
	Notes       string              `json:"notes,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// CashFlowLedger stores all cash transactions for a portfolio.
type CashFlowLedger struct {
	PortfolioName string            `json:"portfolio_name"`
	Version       int               `json:"version"`
	Transactions  []CashTransaction `json:"transactions"`
	Notes         string            `json:"notes,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
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
