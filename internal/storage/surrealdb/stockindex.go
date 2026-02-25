package surrealdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// StockIndexStore implements interfaces.StockIndexStore using SurrealDB.
type StockIndexStore struct {
	db     *surrealdb.DB
	logger *common.Logger
}

// NewStockIndexStore creates a new StockIndexStore.
func NewStockIndexStore(db *surrealdb.DB, logger *common.Logger) *StockIndexStore {
	return &StockIndexStore{db: db, logger: logger}
}

// tickerToID converts a ticker like "BHP.AU" to a safe SurrealDB record ID.
// SurrealDB record IDs cannot contain dots, so we replace them with underscores.
func tickerToID(ticker string) string {
	return strings.ReplaceAll(ticker, ".", "_")
}

func (s *StockIndexStore) Upsert(ctx context.Context, entry *models.StockIndexEntry) error {
	now := time.Now()

	// Check if entry exists
	existing, err := s.Get(ctx, entry.Ticker)
	if err != nil || existing == nil {
		// New entry
		entry.AddedAt = now
		entry.LastSeenAt = now

		sql := "UPSERT $rid CONTENT $entry"
		vars := map[string]any{
			"rid":   surrealmodels.NewRecordID("stock_index", tickerToID(entry.Ticker)),
			"entry": entry,
		}
		if _, err := surrealdb.Query[[]models.StockIndexEntry](ctx, s.db, sql, vars); err != nil {
			return fmt.Errorf("failed to upsert stock index entry: %w", err)
		}
		return nil
	}

	// Existing entry — update LastSeenAt and Source
	sql := "UPDATE $rid SET last_seen_at = $now, source = $source"
	vars := map[string]any{
		"rid":    surrealmodels.NewRecordID("stock_index", tickerToID(entry.Ticker)),
		"now":    now,
		"source": entry.Source,
	}
	// Also update Name if provided and the existing one is empty
	if entry.Name != "" && existing.Name == "" {
		sql = "UPDATE $rid SET last_seen_at = $now, source = $source, name = $name"
		vars["name"] = entry.Name
	}

	if _, err := surrealdb.Query[[]models.StockIndexEntry](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to update stock index entry: %w", err)
	}
	return nil
}

func (s *StockIndexStore) Get(ctx context.Context, ticker string) (*models.StockIndexEntry, error) {
	entry, err := surrealdb.Select[models.StockIndexEntry](ctx, s.db, surrealmodels.NewRecordID("stock_index", tickerToID(ticker)))
	if err != nil {
		return nil, fmt.Errorf("failed to get stock index entry: %w", err)
	}
	if entry == nil {
		return nil, fmt.Errorf("stock index entry not found: %s", ticker)
	}
	return entry, nil
}

func (s *StockIndexStore) List(ctx context.Context) ([]*models.StockIndexEntry, error) {
	sql := "SELECT * FROM stock_index ORDER BY ticker ASC"
	results, err := surrealdb.Query[[]models.StockIndexEntry](ctx, s.db, sql, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list stock index: %w", err)
	}

	var entries []*models.StockIndexEntry
	if results != nil && len(*results) > 0 {
		for i := range (*results)[0].Result {
			entries = append(entries, &(*results)[0].Result[i])
		}
	}
	return entries, nil
}

func (s *StockIndexStore) UpdateTimestamp(ctx context.Context, ticker, field string, ts time.Time) error {
	// Validate field name to prevent injection
	validFields := map[string]bool{
		"eod_collected_at":              true,
		"fundamentals_collected_at":     true,
		"filings_collected_at":          true,
		"filings_pdfs_collected_at":     true, // NEW
		"news_collected_at":             true,
		"filing_summaries_collected_at": true,
		"timeline_collected_at":         true,
		"signals_collected_at":          true,
		"news_intel_collected_at":       true,
	}
	if !validFields[field] {
		return fmt.Errorf("invalid timestamp field: %s", field)
	}

	sql := fmt.Sprintf("UPDATE $rid SET %s = $ts", field)
	vars := map[string]any{
		"rid": surrealmodels.NewRecordID("stock_index", tickerToID(ticker)),
		"ts":  ts,
	}

	if _, err := surrealdb.Query[[]models.StockIndexEntry](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to update stock index timestamp: %w", err)
	}
	return nil
}

func (s *StockIndexStore) Delete(ctx context.Context, ticker string) error {
	_, err := surrealdb.Delete[models.StockIndexEntry](ctx, s.db, surrealmodels.NewRecordID("stock_index", tickerToID(ticker)))
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to delete stock index entry: %w", err)
	}
	return nil
}

// Compile-time check
var _ interfaces.StockIndexStore = (*StockIndexStore)(nil)
