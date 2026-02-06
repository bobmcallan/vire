// Package storage provides BadgerDB-based persistence
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/timshannon/badgerhold/v4"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/models"
)

// BadgerDB wraps badgerhold for typed storage
type BadgerDB struct {
	store  *badgerhold.Store
	logger *common.Logger
}

// NewBadgerDB creates a new BadgerDB instance
func NewBadgerDB(logger *common.Logger, config *common.BadgerConfig) (*BadgerDB, error) {
	opts := badgerhold.DefaultOptions
	opts.Dir = config.Path
	opts.ValueDir = config.Path
	opts.Logger = nil // Disable badger's internal logging

	store, err := badgerhold.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger store: %w", err)
	}

	logger.Debug().Str("path", config.Path).Msg("BadgerDB opened")

	return &BadgerDB{
		store:  store,
		logger: logger,
	}, nil
}

// Close closes the database
func (db *BadgerDB) Close() error {
	if db.store != nil {
		return db.store.Close()
	}
	return nil
}

// Store returns the underlying badgerhold store
func (db *BadgerDB) Store() *badgerhold.Store {
	return db.store
}

// portfolioStorage implements PortfolioStorage using BadgerDB
type portfolioStorage struct {
	db     *BadgerDB
	logger *common.Logger
}

func newPortfolioStorage(db *BadgerDB, logger *common.Logger) *portfolioStorage {
	return &portfolioStorage{db: db, logger: logger}
}

func (s *portfolioStorage) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
	var portfolio models.Portfolio
	err := s.db.store.Get(name, &portfolio)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("portfolio '%s' not found", name)
		}
		return nil, fmt.Errorf("failed to get portfolio: %w", err)
	}
	return &portfolio, nil
}

func (s *portfolioStorage) SavePortfolio(ctx context.Context, portfolio *models.Portfolio) error {
	portfolio.UpdatedAt = time.Now()
	if portfolio.CreatedAt.IsZero() {
		portfolio.CreatedAt = time.Now()
	}
	if portfolio.ID == "" {
		portfolio.ID = portfolio.Name
	}

	err := s.db.store.Upsert(portfolio.ID, portfolio)
	if err != nil {
		return fmt.Errorf("failed to save portfolio: %w", err)
	}
	s.logger.Debug().Str("name", portfolio.Name).Msg("Portfolio saved")
	return nil
}

func (s *portfolioStorage) ListPortfolios(ctx context.Context) ([]string, error) {
	var portfolios []models.Portfolio
	err := s.db.store.Find(&portfolios, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list portfolios: %w", err)
	}

	names := make([]string, len(portfolios))
	for i, p := range portfolios {
		names[i] = p.Name
	}
	return names, nil
}

func (s *portfolioStorage) DeletePortfolio(ctx context.Context, name string) error {
	err := s.db.store.Delete(name, models.Portfolio{})
	if err != nil {
		return fmt.Errorf("failed to delete portfolio: %w", err)
	}
	s.logger.Debug().Str("name", name).Msg("Portfolio deleted")
	return nil
}

// marketDataStorage implements MarketDataStorage using BadgerDB
type marketDataStorage struct {
	db     *BadgerDB
	logger *common.Logger
}

func newMarketDataStorage(db *BadgerDB, logger *common.Logger) *marketDataStorage {
	return &marketDataStorage{db: db, logger: logger}
}

func (s *marketDataStorage) GetMarketData(ctx context.Context, ticker string) (*models.MarketData, error) {
	var data models.MarketData
	err := s.db.store.Get(ticker, &data)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("market data for '%s' not found", ticker)
		}
		return nil, fmt.Errorf("failed to get market data: %w", err)
	}
	return &data, nil
}

func (s *marketDataStorage) SaveMarketData(ctx context.Context, data *models.MarketData) error {
	data.LastUpdated = time.Now()
	err := s.db.store.Upsert(data.Ticker, data)
	if err != nil {
		return fmt.Errorf("failed to save market data: %w", err)
	}
	s.logger.Debug().Str("ticker", data.Ticker).Msg("Market data saved")
	return nil
}

func (s *marketDataStorage) GetMarketDataBatch(ctx context.Context, tickers []string) ([]*models.MarketData, error) {
	result := make([]*models.MarketData, 0, len(tickers))
	for _, ticker := range tickers {
		data, err := s.GetMarketData(ctx, ticker)
		if err == nil {
			result = append(result, data)
		}
	}
	return result, nil
}

func (s *marketDataStorage) GetStaleTickers(ctx context.Context, exchange string, maxAgeSeconds int64) ([]string, error) {
	cutoff := time.Now().Add(-time.Duration(maxAgeSeconds) * time.Second)

	var allData []models.MarketData
	query := badgerhold.Where("Exchange").Eq(exchange).And("LastUpdated").Lt(cutoff)
	err := s.db.store.Find(&allData, query)
	if err != nil {
		return nil, fmt.Errorf("failed to find stale tickers: %w", err)
	}

	tickers := make([]string, len(allData))
	for i, d := range allData {
		tickers[i] = d.Ticker
	}
	return tickers, nil
}

// signalStorage implements SignalStorage using BadgerDB
type signalStorage struct {
	db     *BadgerDB
	logger *common.Logger
}

func newSignalStorage(db *BadgerDB, logger *common.Logger) *signalStorage {
	return &signalStorage{db: db, logger: logger}
}

func (s *signalStorage) GetSignals(ctx context.Context, ticker string) (*models.TickerSignals, error) {
	var signals models.TickerSignals
	err := s.db.store.Get(ticker, &signals)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("signals for '%s' not found", ticker)
		}
		return nil, fmt.Errorf("failed to get signals: %w", err)
	}
	return &signals, nil
}

func (s *signalStorage) SaveSignals(ctx context.Context, signals *models.TickerSignals) error {
	signals.ComputeTimestamp = time.Now()
	err := s.db.store.Upsert(signals.Ticker, signals)
	if err != nil {
		return fmt.Errorf("failed to save signals: %w", err)
	}
	s.logger.Debug().Str("ticker", signals.Ticker).Msg("Signals saved")
	return nil
}

func (s *signalStorage) GetSignalsBatch(ctx context.Context, tickers []string) ([]*models.TickerSignals, error) {
	result := make([]*models.TickerSignals, 0, len(tickers))
	for _, ticker := range tickers {
		signals, err := s.GetSignals(ctx, ticker)
		if err == nil {
			result = append(result, signals)
		}
	}
	return result, nil
}

// kvStorage implements KeyValueStorage using BadgerDB
type kvStorage struct {
	db     *BadgerDB
	logger *common.Logger
}

// kvEntry represents a key-value entry in the store
type kvEntry struct {
	Key   string `badgerhold:"key"`
	Value string
}

func newKVStorage(db *BadgerDB, logger *common.Logger) *kvStorage {
	return &kvStorage{db: db, logger: logger}
}

func (s *kvStorage) Get(ctx context.Context, key string) (string, error) {
	var entry kvEntry
	err := s.db.store.Get(key, &entry)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return "", fmt.Errorf("key '%s' not found", key)
		}
		return "", fmt.Errorf("failed to get key: %w", err)
	}
	return entry.Value, nil
}

func (s *kvStorage) Set(ctx context.Context, key, value string) error {
	entry := kvEntry{Key: key, Value: value}
	err := s.db.store.Upsert(key, &entry)
	if err != nil {
		return fmt.Errorf("failed to set key: %w", err)
	}
	return nil
}

func (s *kvStorage) Delete(ctx context.Context, key string) error {
	err := s.db.store.Delete(key, kvEntry{})
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete key: %w", err)
	}
	return nil
}

func (s *kvStorage) GetAll(ctx context.Context) (map[string]string, error) {
	var entries []kvEntry
	err := s.db.store.Find(&entries, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get all keys: %w", err)
	}

	result := make(map[string]string, len(entries))
	for _, e := range entries {
		result[e.Key] = e.Value
	}
	return result, nil
}

// reportStorage implements ReportStorage using BadgerDB
type reportStorage struct {
	db     *BadgerDB
	logger *common.Logger
}

func newReportStorage(db *BadgerDB, logger *common.Logger) *reportStorage {
	return &reportStorage{db: db, logger: logger}
}

func (s *reportStorage) GetReport(ctx context.Context, portfolio string) (*models.PortfolioReport, error) {
	var report models.PortfolioReport
	err := s.db.store.Get(portfolio, &report)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("report for '%s' not found", portfolio)
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}
	return &report, nil
}

func (s *reportStorage) SaveReport(ctx context.Context, report *models.PortfolioReport) error {
	err := s.db.store.Upsert(report.Portfolio, report)
	if err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}
	s.logger.Debug().Str("portfolio", report.Portfolio).Msg("Report saved")
	return nil
}

func (s *reportStorage) ListReports(ctx context.Context) ([]string, error) {
	var reports []models.PortfolioReport
	err := s.db.store.Find(&reports, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}

	names := make([]string, len(reports))
	for i, r := range reports {
		names[i] = r.Portfolio
	}
	return names, nil
}

func (s *reportStorage) DeleteReport(ctx context.Context, portfolio string) error {
	err := s.db.store.Delete(portfolio, models.PortfolioReport{})
	if err != nil {
		return fmt.Errorf("failed to delete report: %w", err)
	}
	s.logger.Debug().Str("portfolio", portfolio).Msg("Report deleted")
	return nil
}

// Needed for badger to avoid panics from concurrent access during tests
func init() {
	// Ensure types are registered
	_ = models.Portfolio{}
	_ = models.MarketData{}
	_ = models.TickerSignals{}
	_ = models.PortfolioReport{}
}
