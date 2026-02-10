// Package storage provides blob-based persistence with pluggable backends.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

// FileStore provides file-based JSON storage with optional versioning.
type FileStore struct {
	basePath string
	versions int
	logger   *common.Logger
}

// subdirectories defines the directory layout under basePath.
var subdirectories = []string{
	"portfolios", "market", "signals", "reports",
	"strategies", "plans", "watchlists", "searches", "kv",
	"charts",
}

// NewFileStore creates a new FileStore and ensures all subdirectories exist.
func NewFileStore(logger *common.Logger, config *common.FileConfig) (*FileStore, error) {
	versions := config.Versions
	if versions < 0 {
		versions = 0
	}

	fs := &FileStore{
		basePath: config.Path,
		versions: versions,
		logger:   logger,
	}

	// Create all subdirectories
	for _, sub := range subdirectories {
		dir := filepath.Join(fs.basePath, sub)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	logger.Debug().Str("path", config.Path).Int("versions", versions).Msg("FileStore opened")
	return fs, nil
}

// sanitizeKey makes a key safe for use as a filename.
// Replaces /, \, : with _ and collapses ".." to "_" to prevent path traversal.
// Preserves single dots (safe in filenames, common in tickers like BHP.AU).
func (fs *FileStore) sanitizeKey(key string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	return r.Replace(key)
}

// filePath returns the full path for a key in a directory.
func (fs *FileStore) filePath(dir, key string) string {
	return filepath.Join(dir, fs.sanitizeKey(key)+".json")
}

// readJSON reads and unmarshals a JSON file.
func (fs *FileStore) readJSON(dir, key string, dest interface{}) error {
	path := fs.filePath(dir, key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("'%s' not found", key)
		}
		return fmt.Errorf("failed to read %s: %w", path, err)
	}
	if len(data) == 0 {
		return fmt.Errorf("'%s' is empty", key)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return nil
}

// writeJSON marshals data to indented JSON and writes it atomically.
// If versioned is true and fs.versions > 0, rotates previous versions before overwriting.
// Use versioned=true for user-authored data (portfolios, strategies, plans, watchlists).
// Use versioned=false for derived/cached data (market, signals, reports, searches, kv).
func (fs *FileStore) writeJSON(dir, key string, data interface{}, versioned bool) error {
	target := fs.filePath(dir, key)

	// Marshal to indented JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	jsonData = append(jsonData, '\n')

	// Rotate versions before overwriting (user-authored data only)
	if versioned && fs.versions > 0 {
		fs.rotateVersions(target)
	}

	// Atomic write: write to temp file in the same directory, then rename
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(jsonData); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, target); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// rotateVersions shifts existing versions up and copies current to v1.
// v{N} -> deleted, v{N-1} -> v{N}, ..., v1 -> v2, current -> v1
func (fs *FileStore) rotateVersions(target string) {
	// Delete the oldest version if it exists
	oldest := fmt.Sprintf("%s.v%d", target, fs.versions)
	os.Remove(oldest)

	// Shift versions up: v{N-1} -> v{N}, ..., v1 -> v2
	for i := fs.versions; i > 1; i-- {
		src := fmt.Sprintf("%s.v%d", target, i-1)
		dst := fmt.Sprintf("%s.v%d", target, i)
		os.Rename(src, dst) // Ignore errors (file may not exist yet)
	}

	// Copy current file to v1 (if it exists)
	if _, err := os.Stat(target); err == nil {
		v1 := fmt.Sprintf("%s.v1", target)
		os.Rename(target, v1) // Ignore errors
	}
}

// deleteJSON removes a file and all its version backups.
func (fs *FileStore) deleteJSON(dir, key string) error {
	target := fs.filePath(dir, key)

	// Remove main file
	os.Remove(target)

	// Remove all version files
	for i := 1; i <= fs.versions; i++ {
		os.Remove(fmt.Sprintf("%s.v%d", target, i))
	}

	return nil
}

// listKeys returns all keys in a directory (excluding version files and temp files).
func (fs *FileStore) listKeys(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var keys []string
	for _, e := range entries {
		name := e.Name()
		// Only include .json files, not .json.v1 versions or .tmp-* temp files
		if strings.HasSuffix(name, ".json") && !strings.HasPrefix(name, ".tmp-") {
			key := strings.TrimSuffix(name, ".json")
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// purgeDir removes all .json files and their versions from a directory.
// Returns the count of primary files removed.
func (fs *FileStore) purgeDir(dir string) int {
	keys, err := fs.listKeys(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, key := range keys {
		fs.deleteJSON(dir, key)
		count++
	}
	return count
}

// purgeAllFiles removes all files from a directory (regardless of extension).
// Returns the count of files removed. Skips temp files and subdirectories.
func (fs *FileStore) purgeAllFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".tmp-") {
			continue
		}
		os.Remove(filepath.Join(dir, e.Name()))
		count++
	}
	return count
}

// WriteRaw writes arbitrary binary data atomically using temp file + rename.
// The key is sanitized for safe filenames (e.g. "smsf-growth.png").
func (fs *FileStore) WriteRaw(subdir, key string, data []byte) error {
	dir := filepath.Join(fs.basePath, subdir)
	target := filepath.Join(dir, fs.sanitizeKey(key))

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, target); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// --- Portfolio Storage ---

type portfolioStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

func newPortfolioStorage(fs *FileStore, logger *common.Logger) *portfolioStorage {
	return &portfolioStorage{fs: fs, dir: filepath.Join(fs.basePath, "portfolios"), logger: logger}
}

func (s *portfolioStorage) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
	var portfolio models.Portfolio
	if err := s.fs.readJSON(s.dir, name, &portfolio); err != nil {
		return nil, fmt.Errorf("portfolio '%s' not found", name)
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

	if err := s.fs.writeJSON(s.dir, portfolio.ID, portfolio, true); err != nil {
		return fmt.Errorf("failed to save portfolio: %w", err)
	}
	s.logger.Debug().Str("name", portfolio.Name).Msg("Portfolio saved")
	return nil
}

func (s *portfolioStorage) ListPortfolios(ctx context.Context) ([]string, error) {
	keys, err := s.fs.listKeys(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list portfolios: %w", err)
	}
	return keys, nil
}

func (s *portfolioStorage) DeletePortfolio(ctx context.Context, name string) error {
	s.fs.deleteJSON(s.dir, name)
	s.logger.Debug().Str("name", name).Msg("Portfolio deleted")
	return nil
}

// --- Market Data Storage ---

type marketDataStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

func newMarketDataStorage(fs *FileStore, logger *common.Logger) *marketDataStorage {
	return &marketDataStorage{fs: fs, dir: filepath.Join(fs.basePath, "market"), logger: logger}
}

func (s *marketDataStorage) GetMarketData(ctx context.Context, ticker string) (*models.MarketData, error) {
	var data models.MarketData
	if err := s.fs.readJSON(s.dir, ticker, &data); err != nil {
		return nil, fmt.Errorf("market data for '%s' not found", ticker)
	}
	return &data, nil
}

func (s *marketDataStorage) SaveMarketData(ctx context.Context, data *models.MarketData) error {
	data.LastUpdated = time.Now()
	if err := s.fs.writeJSON(s.dir, data.Ticker, data, false); err != nil {
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

	keys, err := s.fs.listKeys(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list market data: %w", err)
	}

	var stale []string
	for _, key := range keys {
		var data models.MarketData
		if err := s.fs.readJSON(s.dir, key, &data); err != nil {
			continue
		}
		if data.Exchange == exchange && data.LastUpdated.Before(cutoff) {
			stale = append(stale, data.Ticker)
		}
	}
	return stale, nil
}

// --- Signal Storage ---

type signalStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

func newSignalStorage(fs *FileStore, logger *common.Logger) *signalStorage {
	return &signalStorage{fs: fs, dir: filepath.Join(fs.basePath, "signals"), logger: logger}
}

func (s *signalStorage) GetSignals(ctx context.Context, ticker string) (*models.TickerSignals, error) {
	var signals models.TickerSignals
	if err := s.fs.readJSON(s.dir, ticker, &signals); err != nil {
		return nil, fmt.Errorf("signals for '%s' not found", ticker)
	}
	return &signals, nil
}

func (s *signalStorage) SaveSignals(ctx context.Context, signals *models.TickerSignals) error {
	signals.ComputeTimestamp = time.Now()
	if err := s.fs.writeJSON(s.dir, signals.Ticker, signals, false); err != nil {
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

// --- Key-Value Storage ---

type kvStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

// kvEntry represents a key-value entry stored as JSON.
type kvEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func newKVStorage(fs *FileStore, logger *common.Logger) *kvStorage {
	return &kvStorage{fs: fs, dir: filepath.Join(fs.basePath, "kv"), logger: logger}
}

func (s *kvStorage) Get(ctx context.Context, key string) (string, error) {
	var entry kvEntry
	if err := s.fs.readJSON(s.dir, key, &entry); err != nil {
		return "", fmt.Errorf("key '%s' not found", key)
	}
	return entry.Value, nil
}

func (s *kvStorage) Set(ctx context.Context, key, value string) error {
	entry := kvEntry{Key: key, Value: value}
	if err := s.fs.writeJSON(s.dir, key, &entry, false); err != nil {
		return fmt.Errorf("failed to set key: %w", err)
	}
	return nil
}

func (s *kvStorage) Delete(ctx context.Context, key string) error {
	s.fs.deleteJSON(s.dir, key)
	return nil
}

func (s *kvStorage) GetAll(ctx context.Context) (map[string]string, error) {
	keys, err := s.fs.listKeys(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	result := make(map[string]string, len(keys))
	for _, key := range keys {
		var entry kvEntry
		if err := s.fs.readJSON(s.dir, key, &entry); err == nil {
			result[entry.Key] = entry.Value
		}
	}
	return result, nil
}

// --- Report Storage ---

type reportStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

func newReportStorage(fs *FileStore, logger *common.Logger) *reportStorage {
	return &reportStorage{fs: fs, dir: filepath.Join(fs.basePath, "reports"), logger: logger}
}

func (s *reportStorage) GetReport(ctx context.Context, portfolio string) (*models.PortfolioReport, error) {
	var report models.PortfolioReport
	if err := s.fs.readJSON(s.dir, portfolio, &report); err != nil {
		return nil, fmt.Errorf("report for '%s' not found", portfolio)
	}
	return &report, nil
}

func (s *reportStorage) SaveReport(ctx context.Context, report *models.PortfolioReport) error {
	if err := s.fs.writeJSON(s.dir, report.Portfolio, report, false); err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}
	s.logger.Debug().Str("portfolio", report.Portfolio).Msg("Report saved")
	return nil
}

func (s *reportStorage) ListReports(ctx context.Context) ([]string, error) {
	keys, err := s.fs.listKeys(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}
	return keys, nil
}

func (s *reportStorage) DeleteReport(ctx context.Context, portfolio string) error {
	s.fs.deleteJSON(s.dir, portfolio)
	s.logger.Debug().Str("portfolio", portfolio).Msg("Report deleted")
	return nil
}

// --- Strategy Storage ---

type strategyStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

func newStrategyStorage(fs *FileStore, logger *common.Logger) *strategyStorage {
	return &strategyStorage{fs: fs, dir: filepath.Join(fs.basePath, "strategies"), logger: logger}
}

func (s *strategyStorage) GetStrategy(ctx context.Context, portfolioName string) (*models.PortfolioStrategy, error) {
	var strategy models.PortfolioStrategy
	if err := s.fs.readJSON(s.dir, portfolioName, &strategy); err != nil {
		return nil, fmt.Errorf("strategy for '%s' not found", portfolioName)
	}
	return &strategy, nil
}

func (s *strategyStorage) SaveStrategy(ctx context.Context, strategy *models.PortfolioStrategy) error {
	// Read existing to preserve CreatedAt and increment Version
	var existing models.PortfolioStrategy
	err := s.fs.readJSON(s.dir, strategy.PortfolioName, &existing)
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

	if err := s.fs.writeJSON(s.dir, strategy.PortfolioName, strategy, true); err != nil {
		return fmt.Errorf("failed to save strategy: %w", err)
	}
	s.logger.Debug().Str("portfolio", strategy.PortfolioName).Int("version", strategy.Version).Msg("Strategy saved")
	return nil
}

func (s *strategyStorage) DeleteStrategy(ctx context.Context, portfolioName string) error {
	s.fs.deleteJSON(s.dir, portfolioName)
	s.logger.Debug().Str("portfolio", portfolioName).Msg("Strategy deleted")
	return nil
}

func (s *strategyStorage) ListStrategies(ctx context.Context) ([]string, error) {
	keys, err := s.fs.listKeys(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list strategies: %w", err)
	}
	return keys, nil
}

// --- Plan Storage ---

type planStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

func newPlanStorage(fs *FileStore, logger *common.Logger) *planStorage {
	return &planStorage{fs: fs, dir: filepath.Join(fs.basePath, "plans"), logger: logger}
}

func (s *planStorage) GetPlan(ctx context.Context, portfolioName string) (*models.PortfolioPlan, error) {
	var plan models.PortfolioPlan
	if err := s.fs.readJSON(s.dir, portfolioName, &plan); err != nil {
		return nil, fmt.Errorf("plan for '%s' not found", portfolioName)
	}
	return &plan, nil
}

func (s *planStorage) SavePlan(ctx context.Context, plan *models.PortfolioPlan) error {
	// Read existing to preserve CreatedAt and increment Version
	var existing models.PortfolioPlan
	err := s.fs.readJSON(s.dir, plan.PortfolioName, &existing)
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

	if err := s.fs.writeJSON(s.dir, plan.PortfolioName, plan, true); err != nil {
		return fmt.Errorf("failed to save plan: %w", err)
	}
	s.logger.Debug().Str("portfolio", plan.PortfolioName).Int("version", plan.Version).Msg("Plan saved")
	return nil
}

func (s *planStorage) DeletePlan(ctx context.Context, portfolioName string) error {
	s.fs.deleteJSON(s.dir, portfolioName)
	s.logger.Debug().Str("portfolio", portfolioName).Msg("Plan deleted")
	return nil
}

func (s *planStorage) ListPlans(ctx context.Context) ([]string, error) {
	keys, err := s.fs.listKeys(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list plans: %w", err)
	}
	return keys, nil
}

// --- Watchlist Storage ---

type watchlistStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

func newWatchlistStorage(fs *FileStore, logger *common.Logger) *watchlistStorage {
	return &watchlistStorage{fs: fs, dir: filepath.Join(fs.basePath, "watchlists"), logger: logger}
}

func (s *watchlistStorage) GetWatchlist(ctx context.Context, portfolioName string) (*models.PortfolioWatchlist, error) {
	var watchlist models.PortfolioWatchlist
	if err := s.fs.readJSON(s.dir, portfolioName, &watchlist); err != nil {
		return nil, fmt.Errorf("watchlist for '%s' not found", portfolioName)
	}
	return &watchlist, nil
}

func (s *watchlistStorage) SaveWatchlist(ctx context.Context, watchlist *models.PortfolioWatchlist) error {
	// Read existing to preserve CreatedAt and increment Version
	var existing models.PortfolioWatchlist
	err := s.fs.readJSON(s.dir, watchlist.PortfolioName, &existing)
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

	if err := s.fs.writeJSON(s.dir, watchlist.PortfolioName, watchlist, true); err != nil {
		return fmt.Errorf("failed to save watchlist: %w", err)
	}
	s.logger.Debug().Str("portfolio", watchlist.PortfolioName).Int("version", watchlist.Version).Msg("Watchlist saved")
	return nil
}

func (s *watchlistStorage) DeleteWatchlist(ctx context.Context, portfolioName string) error {
	s.fs.deleteJSON(s.dir, portfolioName)
	s.logger.Debug().Str("portfolio", portfolioName).Msg("Watchlist deleted")
	return nil
}

func (s *watchlistStorage) ListWatchlists(ctx context.Context) ([]string, error) {
	keys, err := s.fs.listKeys(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list watchlists: %w", err)
	}
	return keys, nil
}

// --- Search History Storage ---

type searchHistoryStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

func newSearchHistoryStorage(fs *FileStore, logger *common.Logger) *searchHistoryStorage {
	return &searchHistoryStorage{fs: fs, dir: filepath.Join(fs.basePath, "searches"), logger: logger}
}

func (s *searchHistoryStorage) SaveSearch(ctx context.Context, record *models.SearchRecord) error {
	if record.ID == "" {
		record.ID = fmt.Sprintf("search-%d-%s-%s", record.CreatedAt.Unix(), record.Type, record.Exchange)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	if err := s.fs.writeJSON(s.dir, record.ID, record, false); err != nil {
		return fmt.Errorf("failed to save search record: %w", err)
	}
	s.logger.Debug().Str("id", record.ID).Str("type", record.Type).Msg("Search record saved")

	// Prune oldest records if over max limit
	s.pruneOldRecords()

	return nil
}

func (s *searchHistoryStorage) pruneOldRecords() {
	const maxSearchRecords = 50

	keys, err := s.fs.listKeys(s.dir)
	if err != nil || len(keys) <= maxSearchRecords {
		return
	}

	// Load all records to sort by CreatedAt
	type recordWithKey struct {
		key       string
		createdAt time.Time
	}
	var records []recordWithKey
	for _, key := range keys {
		var r models.SearchRecord
		if err := s.fs.readJSON(s.dir, key, &r); err == nil {
			records = append(records, recordWithKey{key: key, createdAt: r.CreatedAt})
		}
	}

	if len(records) <= maxSearchRecords {
		return
	}

	// Sort by CreatedAt descending (newest first)
	sort.Slice(records, func(i, j int) bool {
		return records[i].createdAt.After(records[j].createdAt)
	})

	// Delete oldest records beyond the limit
	for _, old := range records[maxSearchRecords:] {
		s.fs.deleteJSON(s.dir, old.key)
	}
	s.logger.Debug().Int("pruned", len(records)-maxSearchRecords).Msg("Pruned old search records")
}

func (s *searchHistoryStorage) GetSearch(ctx context.Context, id string) (*models.SearchRecord, error) {
	var record models.SearchRecord
	if err := s.fs.readJSON(s.dir, id, &record); err != nil {
		return nil, fmt.Errorf("search record '%s' not found", id)
	}
	return &record, nil
}

func (s *searchHistoryStorage) ListSearches(ctx context.Context, options interfaces.SearchListOptions) ([]*models.SearchRecord, error) {
	keys, err := s.fs.listKeys(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list search records: %w", err)
	}

	var records []models.SearchRecord
	for _, key := range keys {
		var r models.SearchRecord
		if err := s.fs.readJSON(s.dir, key, &r); err != nil {
			continue
		}

		// Apply filters
		if options.Type != "" && r.Type != options.Type {
			continue
		}
		if options.Exchange != "" && r.Exchange != options.Exchange {
			continue
		}
		records = append(records, r)
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
	s.fs.deleteJSON(s.dir, id)
	s.logger.Debug().Str("id", id).Msg("Search record deleted")
	return nil
}
