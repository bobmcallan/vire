// Package storage provides BadgerDB-based persistence
package storage

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/timshannon/badgerhold/v4"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
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

// strategyStorage implements StrategyStorage using BadgerDB
type strategyStorage struct {
	db     *BadgerDB
	logger *common.Logger
}

func newStrategyStorage(db *BadgerDB, logger *common.Logger) *strategyStorage {
	return &strategyStorage{db: db, logger: logger}
}

func (s *strategyStorage) GetStrategy(ctx context.Context, portfolioName string) (*models.PortfolioStrategy, error) {
	var strategy models.PortfolioStrategy
	err := s.db.store.Get(portfolioName, &strategy)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("strategy for '%s' not found", portfolioName)
		}
		return nil, fmt.Errorf("failed to get strategy: %w", err)
	}
	return &strategy, nil
}

func (s *strategyStorage) SaveStrategy(ctx context.Context, strategy *models.PortfolioStrategy) error {
	// Read existing to preserve CreatedAt and increment Version
	var existing models.PortfolioStrategy
	err := s.db.store.Get(strategy.PortfolioName, &existing)
	if err == nil {
		// Existing strategy: preserve CreatedAt, increment Version
		strategy.CreatedAt = existing.CreatedAt
		strategy.Version = existing.Version + 1
	} else {
		// New strategy
		strategy.Version = 1
		if strategy.CreatedAt.IsZero() {
			strategy.CreatedAt = time.Now()
		}
		if strategy.Disclaimer == "" {
			strategy.Disclaimer = models.DefaultDisclaimer
		}
	}

	strategy.UpdatedAt = time.Now()

	err = s.db.store.Upsert(strategy.PortfolioName, strategy)
	if err != nil {
		return fmt.Errorf("failed to save strategy: %w", err)
	}
	s.logger.Debug().Str("portfolio", strategy.PortfolioName).Int("version", strategy.Version).Msg("Strategy saved")
	return nil
}

func (s *strategyStorage) DeleteStrategy(ctx context.Context, portfolioName string) error {
	err := s.db.store.Delete(portfolioName, models.PortfolioStrategy{})
	if err != nil {
		return fmt.Errorf("failed to delete strategy: %w", err)
	}
	s.logger.Debug().Str("portfolio", portfolioName).Msg("Strategy deleted")
	return nil
}

func (s *strategyStorage) ListStrategies(ctx context.Context) ([]string, error) {
	var strategies []models.PortfolioStrategy
	err := s.db.store.Find(&strategies, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list strategies: %w", err)
	}

	names := make([]string, len(strategies))
	for i, st := range strategies {
		names[i] = st.PortfolioName
	}
	return names, nil
}

// planStorage implements PlanStorage using BadgerDB
type planStorage struct {
	db     *BadgerDB
	logger *common.Logger
}

func newPlanStorage(db *BadgerDB, logger *common.Logger) *planStorage {
	return &planStorage{db: db, logger: logger}
}

func (s *planStorage) GetPlan(ctx context.Context, portfolioName string) (*models.PortfolioPlan, error) {
	var plan models.PortfolioPlan
	err := s.db.store.Get(portfolioName, &plan)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("plan for '%s' not found", portfolioName)
		}
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}
	return &plan, nil
}

func (s *planStorage) SavePlan(ctx context.Context, plan *models.PortfolioPlan) error {
	// Read existing to preserve CreatedAt and increment Version
	var existing models.PortfolioPlan
	err := s.db.store.Get(plan.PortfolioName, &existing)
	if err == nil {
		plan.CreatedAt = existing.CreatedAt
		plan.Version = existing.Version + 1
	} else {
		plan.Version = 1
		if plan.CreatedAt.IsZero() {
			plan.CreatedAt = time.Now()
		}
	}

	plan.UpdatedAt = time.Now()

	err = s.db.store.Upsert(plan.PortfolioName, plan)
	if err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}
	s.logger.Debug().Str("portfolio", plan.PortfolioName).Int("version", plan.Version).Msg("Plan saved")
	return nil
}

func (s *planStorage) DeletePlan(ctx context.Context, portfolioName string) error {
	err := s.db.store.Delete(portfolioName, models.PortfolioPlan{})
	if err != nil {
		return fmt.Errorf("failed to delete plan: %w", err)
	}
	s.logger.Debug().Str("portfolio", portfolioName).Msg("Plan deleted")
	return nil
}

func (s *planStorage) ListPlans(ctx context.Context) ([]string, error) {
	var plans []models.PortfolioPlan
	err := s.db.store.Find(&plans, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list plans: %w", err)
	}

	names := make([]string, len(plans))
	for i, p := range plans {
		names[i] = p.PortfolioName
	}
	return names, nil
}

// watchlistStorage implements WatchlistStorage using BadgerDB
type watchlistStorage struct {
	db     *BadgerDB
	logger *common.Logger
}

func newWatchlistStorage(db *BadgerDB, logger *common.Logger) *watchlistStorage {
	return &watchlistStorage{db: db, logger: logger}
}

func (s *watchlistStorage) GetWatchlist(ctx context.Context, portfolioName string) (*models.PortfolioWatchlist, error) {
	var watchlist models.PortfolioWatchlist
	err := s.db.store.Get(portfolioName, &watchlist)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("watchlist for '%s' not found", portfolioName)
		}
		return nil, fmt.Errorf("failed to get watchlist: %w", err)
	}
	return &watchlist, nil
}

func (s *watchlistStorage) SaveWatchlist(ctx context.Context, watchlist *models.PortfolioWatchlist) error {
	// Read existing to preserve CreatedAt and increment Version
	var existing models.PortfolioWatchlist
	err := s.db.store.Get(watchlist.PortfolioName, &existing)
	if err == nil {
		watchlist.CreatedAt = existing.CreatedAt
		watchlist.Version = existing.Version + 1
	} else {
		watchlist.Version = 1
		if watchlist.CreatedAt.IsZero() {
			watchlist.CreatedAt = time.Now()
		}
	}

	watchlist.UpdatedAt = time.Now()

	err = s.db.store.Upsert(watchlist.PortfolioName, watchlist)
	if err != nil {
		return fmt.Errorf("failed to save watchlist: %w", err)
	}
	s.logger.Debug().Str("portfolio", watchlist.PortfolioName).Int("version", watchlist.Version).Msg("Watchlist saved")
	return nil
}

func (s *watchlistStorage) DeleteWatchlist(ctx context.Context, portfolioName string) error {
	err := s.db.store.Delete(portfolioName, models.PortfolioWatchlist{})
	if err != nil {
		return fmt.Errorf("failed to delete watchlist: %w", err)
	}
	s.logger.Debug().Str("portfolio", portfolioName).Msg("Watchlist deleted")
	return nil
}

func (s *watchlistStorage) ListWatchlists(ctx context.Context) ([]string, error) {
	var watchlists []models.PortfolioWatchlist
	err := s.db.store.Find(&watchlists, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list watchlists: %w", err)
	}

	names := make([]string, len(watchlists))
	for i, w := range watchlists {
		names[i] = w.PortfolioName
	}
	return names, nil
}

// searchHistoryStorage implements SearchHistoryStorage using BadgerDB
type searchHistoryStorage struct {
	db     *BadgerDB
	logger *common.Logger
}

func newSearchHistoryStorage(db *BadgerDB, logger *common.Logger) *searchHistoryStorage {
	return &searchHistoryStorage{db: db, logger: logger}
}

func (s *searchHistoryStorage) SaveSearch(ctx context.Context, record *models.SearchRecord) error {
	if record.ID == "" {
		record.ID = fmt.Sprintf("search-%d-%s-%s", record.CreatedAt.Unix(), record.Type, record.Exchange)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	err := s.db.store.Upsert(record.ID, record)
	if err != nil {
		return fmt.Errorf("failed to save search record: %w", err)
	}
	s.logger.Debug().Str("id", record.ID).Str("type", record.Type).Msg("Search record saved")

	// Prune oldest records if over max limit
	const maxSearchRecords = 50
	var all []models.SearchRecord
	if err := s.db.store.Find(&all, nil); err == nil && len(all) > maxSearchRecords {
		sort.Slice(all, func(i, j int) bool {
			return all[i].CreatedAt.After(all[j].CreatedAt)
		})
		for _, old := range all[maxSearchRecords:] {
			_ = s.db.store.Delete(old.ID, models.SearchRecord{})
		}
		s.logger.Debug().Int("pruned", len(all)-maxSearchRecords).Msg("Pruned old search records")
	}

	return nil
}

func (s *searchHistoryStorage) GetSearch(ctx context.Context, id string) (*models.SearchRecord, error) {
	var record models.SearchRecord
	err := s.db.store.Get(id, &record)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("search record '%s' not found", id)
		}
		return nil, fmt.Errorf("failed to get search record: %w", err)
	}
	return &record, nil
}

func (s *searchHistoryStorage) ListSearches(ctx context.Context, options interfaces.SearchListOptions) ([]*models.SearchRecord, error) {
	var records []models.SearchRecord

	// Build query based on filters
	var query *badgerhold.Query
	if options.Type != "" && options.Exchange != "" {
		query = badgerhold.Where("Type").Eq(options.Type).And("Exchange").Eq(options.Exchange)
	} else if options.Type != "" {
		query = badgerhold.Where("Type").Eq(options.Type)
	} else if options.Exchange != "" {
		query = badgerhold.Where("Exchange").Eq(options.Exchange)
	}

	// Find records
	err := s.db.store.Find(&records, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list search records: %w", err)
	}

	// Sort by CreatedAt descending (most recent first)
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})

	// Apply limit
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if len(records) > limit {
		records = records[:limit]
	}

	result := make([]*models.SearchRecord, len(records))
	for i := range records {
		result[i] = &records[i]
	}
	return result, nil
}

func (s *searchHistoryStorage) DeleteSearch(ctx context.Context, id string) error {
	err := s.db.store.Delete(id, models.SearchRecord{})
	if err != nil {
		return fmt.Errorf("failed to delete search record: %w", err)
	}
	s.logger.Debug().Str("id", id).Msg("Search record deleted")
	return nil
}

// Needed for badger to avoid panics from concurrent access during tests
func init() {
	// Ensure types are registered
	_ = models.Portfolio{}
	_ = models.MarketData{}
	_ = models.TickerSignals{}
	_ = models.PortfolioReport{}
	_ = models.PortfolioStrategy{}
	_ = models.PortfolioPlan{}
	_ = models.PortfolioWatchlist{}
	_ = models.SearchRecord{}
}
