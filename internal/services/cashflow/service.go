// Package cashflow provides cash flow tracking and capital performance calculation
package cashflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Compile-time interface check
var _ interfaces.CashFlowService = (*Service)(nil)

// Service implements CashFlowService
type Service struct {
	storage          interfaces.StorageManager
	portfolioService interfaces.PortfolioService
	logger           *common.Logger
}

// NewService creates a new cashflow service
func NewService(storage interfaces.StorageManager, portfolioService interfaces.PortfolioService, logger *common.Logger) *Service {
	return &Service{
		storage:          storage,
		portfolioService: portfolioService,
		logger:           logger,
	}
}

// generateCashTransactionID returns a unique ID with "ct_" prefix + 8 hex chars.
func generateCashTransactionID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "ct_00000000"
	}
	return "ct_" + hex.EncodeToString(b)
}

// validateCashTransaction checks that a transaction has valid field values.
func validateCashTransaction(tx models.CashTransaction) error {
	if strings.TrimSpace(tx.Account) == "" {
		return fmt.Errorf("account is required")
	}
	if len(tx.Account) > 100 {
		return fmt.Errorf("account name exceeds 100 characters")
	}
	if !models.ValidCashCategory(tx.Category) {
		return fmt.Errorf("invalid category %q; must be contribution, dividend, transfer, fee, or other", tx.Category)
	}
	if tx.Date.IsZero() {
		return fmt.Errorf("date is required")
	}
	if tx.Date.After(time.Now().Add(24 * time.Hour)) {
		return fmt.Errorf("date cannot be in the future")
	}
	if tx.Amount == 0 {
		return fmt.Errorf("amount must not be zero")
	}
	if math.IsInf(tx.Amount, 0) || math.IsNaN(tx.Amount) {
		return fmt.Errorf("amount must be finite")
	}
	if math.Abs(tx.Amount) >= 1e15 {
		return fmt.Errorf("amount exceeds maximum (1e15)")
	}
	desc := strings.TrimSpace(tx.Description)
	if desc == "" {
		return fmt.Errorf("description is required")
	}
	if len(desc) > 500 {
		return fmt.Errorf("description exceeds 500 characters")
	}
	if len(tx.Notes) > 1000 {
		return fmt.Errorf("notes exceeds 1000 characters")
	}
	return nil
}

// GetLedger retrieves the cash flow ledger for a portfolio.
func (s *Service) GetLedger(ctx context.Context, portfolioName string) (*models.CashFlowLedger, error) {
	userID := common.ResolveUserID(ctx)
	rec, err := s.storage.UserDataStore().Get(ctx, userID, "cashflow", portfolioName)
	if err != nil {
		// No existing ledger — return empty with default trading account
		return &models.CashFlowLedger{
			PortfolioName: portfolioName,
			Accounts: []models.CashAccount{
				{Name: models.DefaultTradingAccount, Type: "trading", IsTransactional: true},
			},
			Transactions: []models.CashTransaction{},
		}, nil
	}

	var ledger models.CashFlowLedger
	if err := json.Unmarshal([]byte(rec.Value), &ledger); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cashflow ledger: %w", err)
	}

	if ledger.Transactions == nil {
		ledger.Transactions = []models.CashTransaction{}
	}
	if len(ledger.Accounts) == 0 {
		ledger.Accounts = []models.CashAccount{
			{Name: models.DefaultTradingAccount, Type: "trading", IsTransactional: true},
		}
	}
	return &ledger, nil
}

// AddTransaction adds a new cash transaction to the ledger.
// Auto-creates the account if it doesn't exist yet.
func (s *Service) AddTransaction(ctx context.Context, portfolioName string, tx models.CashTransaction) (*models.CashFlowLedger, error) {
	if err := validateCashTransaction(tx); err != nil {
		return nil, fmt.Errorf("invalid cash transaction: %w", err)
	}

	ledger, err := s.GetLedger(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	tx.ID = generateCashTransactionID()
	tx.Account = strings.TrimSpace(tx.Account)
	tx.Description = strings.TrimSpace(tx.Description)
	tx.CreatedAt = now
	tx.UpdatedAt = now

	// Auto-create account if not present (non-transactional by default)
	if !ledger.HasAccount(tx.Account) {
		ledger.Accounts = append(ledger.Accounts, models.CashAccount{
			Name:            tx.Account,
			Type:            "other",
			IsTransactional: false,
		})
	}

	ledger.Transactions = append(ledger.Transactions, tx)
	sortTransactionsByDate(ledger)

	if err := s.saveLedger(ctx, ledger); err != nil {
		return nil, err
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("id", tx.ID).
		Str("account", tx.Account).Float64("amount", tx.Amount).
		Str("category", string(tx.Category)).Msg("Cash transaction added")
	return ledger, nil
}

// AddTransfer creates paired entries for a transfer between two accounts.
// from_account gets -abs(amount), to_account gets +abs(amount).
func (s *Service) AddTransfer(ctx context.Context, portfolioName string, fromAccount, toAccount string, amount float64, date time.Time, description string) (*models.CashFlowLedger, error) {
	if strings.TrimSpace(fromAccount) == "" || strings.TrimSpace(toAccount) == "" {
		return nil, fmt.Errorf("invalid cash transaction: both from_account and to_account are required for transfers")
	}
	if fromAccount == toAccount {
		return nil, fmt.Errorf("invalid cash transaction: from_account and to_account must be different")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("invalid cash transaction: amount must be positive")
	}
	if date.IsZero() {
		return nil, fmt.Errorf("invalid cash transaction: date is required")
	}
	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("invalid cash transaction: description is required")
	}

	ledger, err := s.GetLedger(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	fromID := generateCashTransactionID()
	toID := generateCashTransactionID()
	absAmount := math.Abs(amount)

	fromTx := models.CashTransaction{
		ID:          fromID,
		Account:     strings.TrimSpace(fromAccount),
		Category:    models.CashCatTransfer,
		Date:        date,
		Amount:      -absAmount, // money out of from_account
		Description: strings.TrimSpace(description),
		LinkedID:    toID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	toTx := models.CashTransaction{
		ID:          toID,
		Account:     strings.TrimSpace(toAccount),
		Category:    models.CashCatTransfer,
		Date:        date,
		Amount:      absAmount, // money into to_account
		Description: strings.TrimSpace(description),
		LinkedID:    fromID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Auto-create accounts if not present (non-transactional by default)
	for _, name := range []string{fromTx.Account, toTx.Account} {
		if !ledger.HasAccount(name) {
			ledger.Accounts = append(ledger.Accounts, models.CashAccount{
				Name:            name,
				Type:            "other",
				IsTransactional: false,
			})
		}
	}

	ledger.Transactions = append(ledger.Transactions, fromTx, toTx)
	sortTransactionsByDate(ledger)

	if err := s.saveLedger(ctx, ledger); err != nil {
		return nil, err
	}

	s.logger.Info().Str("portfolio", portfolioName).
		Str("fromID", fromID).Str("toID", toID).
		Str("from", fromAccount).Str("to", toAccount).
		Float64("amount", absAmount).Msg("Transfer added")
	return ledger, nil
}

// UpdateTransaction updates an existing transaction by ID (merge semantics).
func (s *Service) UpdateTransaction(ctx context.Context, portfolioName string, txID string, update models.CashTransaction) (*models.CashFlowLedger, error) {
	ledger, err := s.GetLedger(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	idx := -1
	for i, tx := range ledger.Transactions {
		if tx.ID == txID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("transaction %q not found", txID)
	}

	existing := &ledger.Transactions[idx]

	// Merge: only overwrite non-zero fields
	if update.Account != "" {
		acct := strings.TrimSpace(update.Account)
		if len(acct) > 100 {
			return nil, fmt.Errorf("account name exceeds 100 characters")
		}
		existing.Account = acct
		// Auto-create account (non-transactional by default)
		if !ledger.HasAccount(acct) {
			ledger.Accounts = append(ledger.Accounts, models.CashAccount{
				Name:            acct,
				Type:            "other",
				IsTransactional: false,
			})
		}
	}
	if update.Category != "" {
		if !models.ValidCashCategory(update.Category) {
			return nil, fmt.Errorf("invalid category %q", update.Category)
		}
		existing.Category = update.Category
	}
	if !update.Date.IsZero() {
		if update.Date.After(time.Now().Add(24 * time.Hour)) {
			return nil, fmt.Errorf("date cannot be in the future")
		}
		existing.Date = update.Date
	}
	if update.Amount != 0 {
		if math.IsInf(update.Amount, 0) || math.IsNaN(update.Amount) {
			return nil, fmt.Errorf("amount must be finite")
		}
		if math.Abs(update.Amount) >= 1e15 {
			return nil, fmt.Errorf("amount exceeds maximum (1e15)")
		}
		existing.Amount = update.Amount
	}
	if update.Description != "" {
		desc := strings.TrimSpace(update.Description)
		if len(desc) > 500 {
			return nil, fmt.Errorf("description exceeds 500 characters")
		}
		existing.Description = desc
	}
	if update.Notes != "" {
		if len(update.Notes) > 1000 {
			return nil, fmt.Errorf("notes exceeds 1000 characters")
		}
		existing.Notes = update.Notes
	}
	existing.UpdatedAt = time.Now()

	sortTransactionsByDate(ledger)

	if err := s.saveLedger(ctx, ledger); err != nil {
		return nil, err
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("id", txID).Msg("Cash transaction updated")
	return ledger, nil
}

// RemoveTransaction removes a transaction by ID.
// If the transaction has a linked pair (transfer), the linked entry is also removed.
func (s *Service) RemoveTransaction(ctx context.Context, portfolioName string, txID string) (*models.CashFlowLedger, error) {
	ledger, err := s.GetLedger(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	idx := -1
	for i, tx := range ledger.Transactions {
		if tx.ID == txID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("transaction %q not found", txID)
	}

	// If this is a transfer with a linked pair, remove both
	linkedID := ledger.Transactions[idx].LinkedID
	ledger.Transactions = append(ledger.Transactions[:idx], ledger.Transactions[idx+1:]...)

	if linkedID != "" {
		for i, tx := range ledger.Transactions {
			if tx.ID == linkedID {
				ledger.Transactions = append(ledger.Transactions[:i], ledger.Transactions[i+1:]...)
				break
			}
		}
	}

	if err := s.saveLedger(ctx, ledger); err != nil {
		return nil, err
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("id", txID).Msg("Cash transaction removed")
	return ledger, nil
}

// UpdateAccount updates account properties (type, is_transactional).
func (s *Service) UpdateAccount(ctx context.Context, portfolioName string, accountName string, update models.CashAccountUpdate) (*models.CashFlowLedger, error) {
	ledger, err := s.GetLedger(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	acct := ledger.GetAccount(accountName)
	if acct == nil {
		return nil, fmt.Errorf("account %q not found", accountName)
	}

	if update.Type != "" {
		if !models.ValidAccountTypes[update.Type] {
			return nil, fmt.Errorf("invalid account type %q; must be trading, accumulate, term_deposit, offset, or other", update.Type)
		}
		acct.Type = update.Type
	}
	if update.IsTransactional != nil {
		acct.IsTransactional = *update.IsTransactional
	}

	if err := s.saveLedger(ctx, ledger); err != nil {
		return nil, err
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("account", accountName).
		Str("type", acct.Type).Bool("is_transactional", acct.IsTransactional).
		Msg("Account updated")
	return ledger, nil
}

// CalculatePerformance computes capital deployment performance metrics.
func (s *Service) CalculatePerformance(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
	ledger, err := s.GetLedger(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cashflow ledger: %w", err)
	}

	if len(ledger.Transactions) == 0 {
		// Try deriving from trade history
		derived, err := s.deriveFromTrades(ctx, portfolioName)
		if err != nil || derived == nil {
			return &models.CapitalPerformance{}, nil
		}
		return derived, nil
	}

	// Get current portfolio value (equity holdings only)
	portfolio, err := s.portfolioService.GetPortfolio(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get portfolio: %w", err)
	}

	currentValue := portfolio.TotalValueHoldings

	totalDeposited := ledger.TotalDeposited()
	totalWithdrawn := ledger.TotalWithdrawn()
	firstDate := ledger.FirstTransactionDate()
	netCapital := totalDeposited - totalWithdrawn

	var simpleReturnPct float64
	if netCapital > 0 {
		simpleReturnPct = (currentValue - netCapital) / netCapital * 100
	}

	// XIRR from actual investment activity (buy/sell trades), not cash transactions
	annualizedPct := computeXIRRFromTrades(portfolio.Holdings, currentValue)

	return &models.CapitalPerformance{
		TotalDeposited:        totalDeposited,
		TotalWithdrawn:        totalWithdrawn,
		NetCapitalDeployed:    netCapital,
		CurrentPortfolioValue: currentValue,
		SimpleReturnPct:       simpleReturnPct,
		AnnualizedReturnPct:   annualizedPct,
		FirstTransactionDate:  firstDate,
		TransactionCount:      len(ledger.Transactions),
	}, nil
}

// deriveFromTrades computes capital performance from portfolio trade history
// when no manual cash transactions exist. Sums buy trades as deposits and
// sell trades as withdrawals, then computes simple return and XIRR.
func (s *Service) deriveFromTrades(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
	portfolio, err := s.portfolioService.GetPortfolio(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get portfolio: %w", err)
	}

	var totalDeposited, totalWithdrawn float64
	var firstDate *time.Time
	var syntheticTx []models.CashTransaction

	for _, h := range portfolio.Holdings {
		for _, t := range h.Trades {
			tradeDate := parseTradeDate(t.Date)
			if tradeDate.IsZero() {
				continue
			}
			if firstDate == nil || tradeDate.Before(*firstDate) {
				d := tradeDate
				firstDate = &d
			}

			tt := strings.ToLower(t.Type)
			switch tt {
			case "buy", "opening balance":
				cost := t.Units*t.Price + t.Fees
				totalDeposited += cost
				syntheticTx = append(syntheticTx, models.CashTransaction{
					Date:   tradeDate,
					Amount: -cost, // money out (buying)
				})
			case "sell":
				proceeds := t.Units*t.Price - t.Fees
				if proceeds < 0 {
					proceeds = 0
				}
				totalWithdrawn += proceeds
				syntheticTx = append(syntheticTx, models.CashTransaction{
					Date:   tradeDate,
					Amount: proceeds, // money in (selling)
				})
			}
		}
	}

	if len(syntheticTx) == 0 {
		return nil, nil
	}

	currentValue := portfolio.TotalValueHoldings
	netCapital := totalDeposited - totalWithdrawn

	var simpleReturnPct float64
	if netCapital > 0 {
		simpleReturnPct = (currentValue - netCapital) / netCapital * 100
	}

	annualizedPct := computeXIRR(syntheticTx, currentValue)

	return &models.CapitalPerformance{
		TotalDeposited:        totalDeposited,
		TotalWithdrawn:        totalWithdrawn,
		NetCapitalDeployed:    netCapital,
		CurrentPortfolioValue: currentValue,
		SimpleReturnPct:       simpleReturnPct,
		AnnualizedReturnPct:   annualizedPct,
		FirstTransactionDate:  firstDate,
		TransactionCount:      len(syntheticTx),
	}, nil
}

// parseTradeDate parses a trade date string ("2006-01-02" or "2006-01-02T15:04:05").
func parseTradeDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// saveLedger persists the ledger to storage with version increment.
func (s *Service) saveLedger(ctx context.Context, ledger *models.CashFlowLedger) error {
	userID := common.ResolveUserID(ctx)
	ledger.Version++
	ledger.UpdatedAt = time.Now()
	if ledger.CreatedAt.IsZero() {
		ledger.CreatedAt = ledger.UpdatedAt
	}

	data, err := json.Marshal(ledger)
	if err != nil {
		return fmt.Errorf("failed to marshal cashflow ledger: %w", err)
	}
	return s.storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "cashflow",
		Key:     ledger.PortfolioName,
		Value:   string(data),
	})
}

// sortTransactionsByDate sorts transactions by date ascending.
func sortTransactionsByDate(ledger *models.CashFlowLedger) {
	sort.Slice(ledger.Transactions, func(i, j int) bool {
		return ledger.Transactions[i].Date.Before(ledger.Transactions[j].Date)
	})
}

// computeXIRRFromTrades computes XIRR from actual buy/sell trades in portfolio
// holdings, not from cash transactions. This measures investment performance
// (what you bought and sold) rather than capital management (deposits/withdrawals).
func computeXIRRFromTrades(holdings []models.Holding, currentValue float64) float64 {
	var syntheticTx []models.CashTransaction
	for _, h := range holdings {
		for _, t := range h.Trades {
			tradeDate := parseTradeDate(t.Date)
			if tradeDate.IsZero() {
				continue
			}
			tt := strings.ToLower(t.Type)
			switch tt {
			case "buy", "opening balance":
				cost := t.Units*t.Price + t.Fees
				syntheticTx = append(syntheticTx, models.CashTransaction{
					Date:   tradeDate,
					Amount: -cost, // money out (buying)
				})
			case "sell":
				proceeds := t.Units*t.Price - t.Fees
				if proceeds < 0 {
					proceeds = 0
				}
				syntheticTx = append(syntheticTx, models.CashTransaction{
					Date:   tradeDate,
					Amount: proceeds, // money in (selling)
				})
			}
		}
	}
	if len(syntheticTx) == 0 {
		return 0
	}
	return computeXIRR(syntheticTx, currentValue)
}

// cashFlow is a local type for XIRR calculation.
type cashFlow struct {
	date   time.Time
	amount float64
}

// computeXIRR calculates annualized return using Newton-Raphson XIRR.
// Negative amounts are outflows (buys), positive amounts are inflows (sells).
// Terminal value (current portfolio value) is positive at today's date.
func computeXIRR(transactions []models.CashTransaction, currentValue float64) float64 {
	if len(transactions) == 0 {
		return 0
	}

	var flows []cashFlow
	for _, tx := range transactions {
		if tx.Date.IsZero() {
			continue
		}
		flows = append(flows, cashFlow{date: tx.Date, amount: tx.SignedAmount()})
	}

	if len(flows) == 0 {
		return 0
	}

	// Terminal value: current portfolio value as positive at today
	if currentValue > 0 {
		flows = append(flows, cashFlow{date: time.Now(), amount: currentValue})
	}

	// Sort by date
	sort.Slice(flows, func(i, j int) bool {
		return flows[i].date.Before(flows[j].date)
	})

	// Need at least one negative and one positive flow
	hasNeg, hasPos := false, false
	for _, f := range flows {
		if f.amount < 0 {
			hasNeg = true
		}
		if f.amount > 0 {
			hasPos = true
		}
	}
	if !hasNeg || !hasPos {
		return 0
	}

	rate := solveXIRR(flows)
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0
	}
	return rate * 100
}

// solveXIRR uses Newton-Raphson to find the rate r such that NPV(r) = 0.
func solveXIRR(flows []cashFlow) float64 {
	const (
		maxIter = 100
		tol     = 1e-7
		minRate = -0.999
	)

	baseDate := flows[0].date

	years := make([]float64, len(flows))
	for i, f := range flows {
		days := f.date.Sub(baseDate).Hours() / 24
		years[i] = days / 365.25
	}

	// Initial guess from simple return
	totalInvested := 0.0
	totalReceived := 0.0
	for _, f := range flows {
		if f.amount < 0 {
			totalInvested -= f.amount
		} else {
			totalReceived += f.amount
		}
	}

	guess := 0.1
	if totalInvested > 0 {
		simpleReturn := totalReceived/totalInvested - 1
		if simpleReturn > -0.9 && simpleReturn < 10 {
			guess = simpleReturn
		}
	}

	rate := guess

	for iter := 0; iter < maxIter; iter++ {
		npv := 0.0
		dnpv := 0.0

		for i, f := range flows {
			y := years[i]
			base := 1 + rate
			if base <= 0 {
				rate = minRate
				base = 1 + rate
			}
			discount := math.Pow(base, y)
			if discount == 0 {
				continue
			}
			npv += f.amount / discount
			if y != 0 {
				dnpv -= y * f.amount / (discount * base)
			}
		}

		if math.Abs(npv) < tol {
			return rate
		}

		if dnpv == 0 {
			break
		}

		newRate := rate - npv/dnpv
		if newRate < minRate {
			newRate = minRate
		}
		if newRate > 100 {
			newRate = 100
		}
		rate = newRate
	}

	// Fallback: bisection
	return bisectXIRR(flows, years)
}

// bisectXIRR uses bisection as a fallback solver.
func bisectXIRR(flows []cashFlow, years []float64) float64 {
	const (
		maxIter = 200
		tol     = 1e-6
	)

	npvAt := func(rate float64) float64 {
		sum := 0.0
		for i, f := range flows {
			base := 1 + rate
			if base <= 0 {
				return math.NaN()
			}
			sum += f.amount / math.Pow(base, years[i])
		}
		return sum
	}

	lo, hi := -0.99, 10.0
	npvLo := npvAt(lo)
	npvHi := npvAt(hi)

	if math.IsNaN(npvLo) || math.IsNaN(npvHi) {
		return math.NaN()
	}
	if npvLo*npvHi > 0 {
		return math.NaN()
	}

	for iter := 0; iter < maxIter; iter++ {
		mid := (lo + hi) / 2
		npvMid := npvAt(mid)
		if math.IsNaN(npvMid) {
			return math.NaN()
		}
		if math.Abs(npvMid) < tol {
			return mid
		}
		if npvMid*npvLo < 0 {
			hi = mid
		} else {
			lo = mid
			npvLo = npvMid
		}
	}

	return (lo + hi) / 2
}
