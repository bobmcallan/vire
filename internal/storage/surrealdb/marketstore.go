package surrealdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type MarketStore struct {
	db       *surrealdb.DB
	logger   *common.Logger
	dataPath string
}

func NewMarketStore(db *surrealdb.DB, logger *common.Logger, dataPath string) *MarketStore {
	return &MarketStore{
		db:       db,
		logger:   logger,
		dataPath: dataPath,
	}
}

// --- MarketDataStorage ---

func (s *MarketStore) GetMarketData(ctx context.Context, ticker string) (*models.MarketData, error) {
	data, err := surrealdb.Select[models.MarketData](ctx, s.db, surrealmodels.NewRecordID("market_data", ticker))
	if err != nil {
		return nil, fmt.Errorf("failed to select market data: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("market data not found")
	}
	return data, nil
}

func (s *MarketStore) SaveMarketData(ctx context.Context, data *models.MarketData) error {
	sql := "UPSERT $rid CONTENT $data"
	vars := map[string]any{"rid": surrealmodels.NewRecordID("market_data", data.Ticker), "data": data}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		_, err := surrealdb.Query[[]models.MarketData](ctx, s.db, sql, vars)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("failed to save market data after retries: %w", lastErr)
}

func (s *MarketStore) GetMarketDataBatch(ctx context.Context, tickers []string) ([]*models.MarketData, error) {
	if len(tickers) == 0 {
		return nil, nil
	}

	sql := "SELECT * FROM market_data WHERE ticker IN $tickers"
	vars := map[string]any{"tickers": tickers}

	results, err := surrealdb.Query[[]models.MarketData](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to get market data batch: %w", err)
	}

	if results != nil && len(*results) > 0 {
		var mapped []*models.MarketData
		for i := range (*results)[0].Result {
			mapped = append(mapped, &(*results)[0].Result[i])
		}
		return mapped, nil
	}
	return nil, nil
}

func (s *MarketStore) GetStaleTickers(ctx context.Context, exchange string, maxAge int64) ([]string, error) {
	cutoff := time.Now().Add(-time.Duration(maxAge) * time.Second)

	sql := "SELECT ticker FROM market_data WHERE exchange = $exchange AND last_updated < $cutoff"
	vars := map[string]any{
		"exchange": exchange,
		"cutoff":   cutoff,
	}

	type TickerResult struct {
		Ticker string `json:"ticker"`
	}

	results, err := surrealdb.Query[[]TickerResult](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to get stale tickers: %w", err)
	}

	var stale []string
	if results != nil && len(*results) > 0 {
		for _, res := range (*results)[0].Result {
			stale = append(stale, res.Ticker)
		}
	}
	return stale, nil
}

// --- SignalStorage ---

func (s *MarketStore) GetSignals(ctx context.Context, ticker string) (*models.TickerSignals, error) {
	data, err := surrealdb.Select[models.TickerSignals](ctx, s.db, surrealmodels.NewRecordID("signals", ticker))
	if err != nil {
		return nil, fmt.Errorf("failed to select signals: %w", err)
	}
	if data == nil {
		return nil, fmt.Errorf("signals not found")
	}
	return data, nil
}

func (s *MarketStore) SaveSignals(ctx context.Context, signals *models.TickerSignals) error {
	sql := "UPSERT $rid CONTENT $signals"
	vars := map[string]any{"rid": surrealmodels.NewRecordID("signals", signals.Ticker), "signals": signals}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		_, err := surrealdb.Query[[]models.TickerSignals](ctx, s.db, sql, vars)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("failed to save signals after retries: %w", lastErr)
}

func (s *MarketStore) GetSignalsBatch(ctx context.Context, tickers []string) ([]*models.TickerSignals, error) {
	if len(tickers) == 0 {
		return nil, nil
	}

	sql := "SELECT * FROM signals WHERE ticker IN $tickers"
	vars := map[string]any{"tickers": tickers}

	results, err := surrealdb.Query[[]models.TickerSignals](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to get signals batch: %w", err)
	}

	if results != nil && len(*results) > 0 {
		var mapped []*models.TickerSignals
		for i := range (*results)[0].Result {
			mapped = append(mapped, &(*results)[0].Result[i])
		}
		return mapped, nil
	}
	return nil, nil
}

// --- Purging ---

func (s *MarketStore) PurgeMarketData(ctx context.Context) (int, error) {
	sql := "DELETE market_data RETURN BEFORE"
	results, err := surrealdb.Query[[]models.MarketData](ctx, s.db, sql, nil)
	if err != nil {
		return 0, err
	}
	if results != nil && len(*results) > 0 {
		return len((*results)[0].Result), nil
	}
	return 0, nil
}

func (s *MarketStore) PurgeSignalsData(ctx context.Context) (int, error) {
	sql := "DELETE signals RETURN BEFORE"
	results, err := surrealdb.Query[[]models.TickerSignals](ctx, s.db, sql, nil)
	if err != nil {
		return 0, err
	}
	if results != nil && len(*results) > 0 {
		return len((*results)[0].Result), nil
	}
	return 0, nil
}

func (s *MarketStore) PurgeCharts() (int, error) {
	chartsDir := filepath.Join(s.dataPath, "charts")
	if _, err := os.Stat(chartsDir); os.IsNotExist(err) {
		return 0, nil
	}

	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			if err := os.Remove(filepath.Join(chartsDir, entry.Name())); err == nil {
				count++
			}
		}
	}
	return count, nil
}
