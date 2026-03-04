// Package trade provides trade management and position derivation for manual/snapshot portfolios.
package trade

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
var _ interfaces.TradeService = (*Service)(nil)

// Service implements TradeService
type Service struct {
	storage interfaces.StorageManager
	logger  *common.Logger
}

// NewService creates a new trade service.
func NewService(storage interfaces.StorageManager, logger *common.Logger) *Service {
	return &Service{storage: storage, logger: logger}
}

// generateTradeID returns a unique ID with "tr_" prefix + 8 hex chars.
func generateTradeID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "tr_00000000"
	}
	return "tr_" + hex.EncodeToString(b)
}

// TradeFilter is a type alias for the interface-defined filter, re-exported for test convenience.
type TradeFilter = interfaces.TradeFilter

// GetTradeBook retrieves the trade book for a portfolio.
// Returns an empty TradeBook if none exists yet.
func (s *Service) GetTradeBook(ctx context.Context, portfolioName string) (*models.TradeBook, error) {
	userID := common.ResolveUserID(ctx)
	rec, err := s.storage.UserDataStore().Get(ctx, userID, "trades", portfolioName)
	if err != nil {
		// No existing trade book — return empty
		return &models.TradeBook{
			PortfolioName: portfolioName,
			Trades:        []models.Trade{},
		}, nil
	}

	var tb models.TradeBook
	if err := json.Unmarshal([]byte(rec.Value), &tb); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trade book: %w", err)
	}

	if tb.Trades == nil {
		tb.Trades = []models.Trade{}
	}
	return &tb, nil
}

// AddTrade records a buy or sell trade and returns the trade + derived holding.
func (s *Service) AddTrade(ctx context.Context, portfolioName string, trade models.Trade) (*models.Trade, *models.DerivedHolding, error) {
	if err := validateTrade(trade); err != nil {
		return nil, nil, err
	}

	tb, err := s.GetTradeBook(ctx, portfolioName)
	if err != nil {
		return nil, nil, err
	}

	// For sells: validate units ≤ current position (use trimmed ticker)
	trimmedTicker := strings.TrimSpace(trade.Ticker)
	if trade.Action == models.TradeActionSell {
		existing := tb.TradesForTicker(trimmedTicker)
		current := DeriveHolding(existing, 0)
		if trade.Units > current.Units {
			return nil, nil, fmt.Errorf("insufficient units: attempting to sell %g but only %g held for %s", trade.Units, current.Units, trimmedTicker)
		}
	}

	now := time.Now()
	trade.ID = generateTradeID()
	trade.Ticker = strings.TrimSpace(trade.Ticker)
	trade.PortfolioName = portfolioName
	trade.CreatedAt = now
	trade.UpdatedAt = now

	tb.Trades = append(tb.Trades, trade)
	sort.Slice(tb.Trades, func(i, j int) bool {
		return tb.Trades[i].Date.Before(tb.Trades[j].Date)
	})

	if err := s.saveTradeBook(ctx, tb); err != nil {
		return nil, nil, err
	}

	// Derive holding for this ticker
	tickerTrades := tb.TradesForTicker(trade.Ticker)
	holding := DeriveHolding(tickerTrades, 0)
	holding.Ticker = trade.Ticker

	s.logger.Info().Str("portfolio", portfolioName).Str("id", trade.ID).
		Str("ticker", trade.Ticker).Str("action", string(trade.Action)).
		Float64("units", trade.Units).Float64("price", trade.Price).
		Msg("Trade added")

	return &trade, &holding, nil
}

// RemoveTrade deletes a trade by ID and returns the updated trade book.
func (s *Service) RemoveTrade(ctx context.Context, portfolioName string, tradeID string) (*models.TradeBook, error) {
	tb, err := s.GetTradeBook(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	idx := -1
	for i, t := range tb.Trades {
		if t.ID == tradeID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("trade not found: %q", tradeID)
	}

	tb.Trades = append(tb.Trades[:idx], tb.Trades[idx+1:]...)

	if err := s.saveTradeBook(ctx, tb); err != nil {
		return nil, err
	}

	s.logger.Info().Str("portfolio", portfolioName).Str("id", tradeID).Msg("Trade removed")
	return tb, nil
}

// UpdateTrade updates a trade by ID using merge semantics.
func (s *Service) UpdateTrade(ctx context.Context, portfolioName string, tradeID string, update models.Trade) (*models.Trade, error) {
	tb, err := s.GetTradeBook(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	idx := -1
	for i, t := range tb.Trades {
		if t.ID == tradeID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("trade not found: %q", tradeID)
	}

	t := &tb.Trades[idx]
	// Merge: only update non-zero/non-empty fields
	if update.Ticker != "" {
		t.Ticker = update.Ticker
	}
	if update.Action != "" {
		t.Action = update.Action
	}
	if update.Units > 0 {
		t.Units = update.Units
	}
	if update.Price > 0 {
		t.Price = update.Price
	}
	if update.Fees > 0 {
		t.Fees = update.Fees
	}
	if !update.Date.IsZero() {
		t.Date = update.Date
	}
	if update.Notes != "" {
		t.Notes = update.Notes
	}
	if update.SettleDate != "" {
		t.SettleDate = update.SettleDate
	}
	if update.SourceRef != "" {
		t.SourceRef = update.SourceRef
	}
	t.UpdatedAt = time.Now()

	// Re-sort by date
	sort.Slice(tb.Trades, func(i, j int) bool {
		return tb.Trades[i].Date.Before(tb.Trades[j].Date)
	})

	// Validate that the update doesn't create a negative position for any ticker
	for _, ticker := range tb.UniqueTickers() {
		h := DeriveHolding(tb.TradesForTicker(ticker), 0)
		if h.Units < -1e-9 {
			return nil, fmt.Errorf("update would create negative position for %s (%.6f units)", ticker, h.Units)
		}
	}

	if err := s.saveTradeBook(ctx, tb); err != nil {
		return nil, err
	}

	// Find the updated trade (index may have changed after re-sort)
	for i := range tb.Trades {
		if tb.Trades[i].ID == tradeID {
			return &tb.Trades[i], nil
		}
	}
	return t, nil
}

// ListTrades returns trades matching the filter criteria.
func (s *Service) ListTrades(ctx context.Context, portfolioName string, filter TradeFilter) ([]models.Trade, int, error) {
	tb, err := s.GetTradeBook(ctx, portfolioName)
	if err != nil {
		return nil, 0, err
	}

	// Apply filters
	var filtered []models.Trade
	for _, t := range tb.Trades {
		if filter.Ticker != "" && t.Ticker != filter.Ticker {
			continue
		}
		if filter.Action != "" && t.Action != filter.Action {
			continue
		}
		if !filter.DateFrom.IsZero() && t.Date.Before(filter.DateFrom) {
			continue
		}
		if !filter.DateTo.IsZero() && t.Date.After(filter.DateTo) {
			continue
		}
		if filter.SourceType != "" && t.SourceType != filter.SourceType {
			continue
		}
		filtered = append(filtered, t)
	}

	total := len(filtered)

	// Apply pagination
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	if offset >= len(filtered) {
		return []models.Trade{}, total, nil
	}

	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[offset:end], total, nil
}

// SnapshotPositions bulk-imports positions for snapshot-type portfolios.
func (s *Service) SnapshotPositions(ctx context.Context, portfolioName string, positions []models.SnapshotPosition, mode string, sourceRef string, snapshotDate string) (*models.TradeBook, error) {
	tb, err := s.GetTradeBook(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	if mode == "replace" {
		// Clear existing and replace with new
		for i := range positions {
			positions[i].CreatedAt = now
			positions[i].UpdatedAt = now
			if sourceRef != "" {
				positions[i].SourceRef = sourceRef
			}
			if snapshotDate != "" {
				positions[i].SnapshotDate = snapshotDate
			}
		}
		tb.SnapshotPositions = positions
	} else if mode == "merge" {
		// Update matching tickers, add new ones, leave unmatched
		existing := make(map[string]int) // ticker → index in tb.SnapshotPositions
		for i, sp := range tb.SnapshotPositions {
			existing[sp.Ticker] = i
		}
		for _, p := range positions {
			p.UpdatedAt = now
			if sourceRef != "" {
				p.SourceRef = sourceRef
			}
			if snapshotDate != "" {
				p.SnapshotDate = snapshotDate
			}
			if idx, ok := existing[p.Ticker]; ok {
				// Update existing
				p.CreatedAt = tb.SnapshotPositions[idx].CreatedAt
				tb.SnapshotPositions[idx] = p
			} else {
				// Add new
				p.CreatedAt = now
				tb.SnapshotPositions = append(tb.SnapshotPositions, p)
				existing[p.Ticker] = len(tb.SnapshotPositions) - 1
			}
		}
	}

	if err := s.saveTradeBook(ctx, tb); err != nil {
		return nil, err
	}

	return tb, nil
}

// DeriveHolding computes a DerivedHolding from a slice of trades for a single ticker.
// currentPrice is used to compute market value and unrealized return.
// Pass 0 for currentPrice to skip market value computation.
func DeriveHolding(trades []models.Trade, currentPrice float64) models.DerivedHolding {
	var (
		runningUnits  float64
		runningCost   float64
		realizedPnL   float64
		grossInvested float64
		grossProceeds float64
	)

	// Sort trades by date ascending
	sorted := make([]models.Trade, len(trades))
	copy(sorted, trades)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date.Before(sorted[j].Date)
	})

	for _, t := range sorted {
		if t.Action == models.TradeActionBuy {
			cost := (t.Units * t.Price) + t.Fees
			runningCost += cost
			runningUnits += t.Units
			grossInvested += cost
		} else if t.Action == models.TradeActionSell {
			if runningUnits > 0 {
				avgCostAtSell := runningCost / runningUnits
				costOfSold := avgCostAtSell * t.Units
				proceeds := (t.Units * t.Price) - t.Fees
				realizedPnL += proceeds - costOfSold
				runningCost -= costOfSold
				runningUnits -= t.Units
				grossProceeds += proceeds
			}
		}
	}

	h := models.DerivedHolding{
		Units:          runningUnits,
		CostBasis:      runningCost,
		RealizedReturn: realizedPnL,
		GrossInvested:  grossInvested,
		GrossProceeds:  grossProceeds,
		TradeCount:     len(trades),
	}

	if runningUnits > 0 {
		h.AvgCost = runningCost / runningUnits
		h.MarketValue = currentPrice * runningUnits
		h.UnrealizedReturn = h.MarketValue - runningCost
	}

	return h
}

// DeriveHoldings computes DerivedHolding for all tickers in the trade book.
func (s *Service) DeriveHoldings(ctx context.Context, portfolioName string) ([]models.DerivedHolding, error) {
	tb, err := s.GetTradeBook(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	tickers := tb.UniqueTickers()
	holdings := make([]models.DerivedHolding, 0, len(tickers))
	for _, ticker := range tickers {
		trades := tb.TradesForTicker(ticker)
		h := DeriveHolding(trades, 0) // caller enriches with current price
		h.Ticker = ticker
		holdings = append(holdings, h)
	}
	return holdings, nil
}

// saveTradeBook persists the trade book to UserDataStore.
func (s *Service) saveTradeBook(ctx context.Context, tb *models.TradeBook) error {
	tb.Version++
	tb.UpdatedAt = time.Now()
	if tb.CreatedAt.IsZero() {
		tb.CreatedAt = tb.UpdatedAt
	}

	data, err := json.Marshal(tb)
	if err != nil {
		return fmt.Errorf("failed to marshal trade book: %w", err)
	}

	userID := common.ResolveUserID(ctx)
	return s.storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "trades",
		Key:     tb.PortfolioName,
		Value:   string(data),
	})
}

// validateTrade checks that a trade has valid field values.
func validateTrade(t models.Trade) error {
	ticker := strings.TrimSpace(t.Ticker)
	if ticker == "" {
		return fmt.Errorf("ticker is required")
	}
	if len(ticker) > 20 {
		return fmt.Errorf("ticker exceeds maximum length (20 chars)")
	}
	if t.Action != models.TradeActionBuy && t.Action != models.TradeActionSell {
		return fmt.Errorf("action must be 'buy' or 'sell', got %q", t.Action)
	}
	if math.IsNaN(t.Units) || math.IsInf(t.Units, 0) {
		return fmt.Errorf("units must be finite")
	}
	if t.Units <= 0 {
		return fmt.Errorf("units must be greater than zero")
	}
	if t.Units >= 1e15 {
		return fmt.Errorf("units exceeds maximum (1e15)")
	}
	if math.IsNaN(t.Price) || math.IsInf(t.Price, 0) {
		return fmt.Errorf("price must be finite")
	}
	if t.Price < 0 {
		return fmt.Errorf("price must be non-negative")
	}
	if t.Price >= 1e15 {
		return fmt.Errorf("price exceeds maximum (1e15)")
	}
	if math.IsNaN(t.Fees) || math.IsInf(t.Fees, 0) {
		return fmt.Errorf("fees must be finite")
	}
	if t.Fees < 0 {
		return fmt.Errorf("fees must be non-negative")
	}
	if t.Date.IsZero() {
		return fmt.Errorf("date is required")
	}
	if len(t.Notes) > 5000 {
		return fmt.Errorf("notes exceeds maximum length (5000 chars)")
	}
	if len(t.SourceRef) > 200 {
		return fmt.Errorf("source_ref exceeds maximum length (200 chars)")
	}
	return nil
}
