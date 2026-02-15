// Package marketfs implements file-based storage for market data and signals.
// Ported from the original storage/file.go FileStore.
package marketfs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Store provides file-based JSON storage for market data and signals.
type Store struct {
	basePath   string
	marketDir  string
	signalsDir string
	logger     *common.Logger
}

// NewMarketStore creates a new market file store.
func NewMarketStore(logger *common.Logger, path string) (*Store, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create market store path %s: %w", path, err)
	}
	marketDir := filepath.Join(path, "market")
	signalsDir := filepath.Join(path, "signals")
	os.MkdirAll(marketDir, 0755)
	os.MkdirAll(signalsDir, 0755)

	logger.Info().Str("path", path).Msg("MarketFS store opened")
	return &Store{
		basePath:   path,
		marketDir:  marketDir,
		signalsDir: signalsDir,
		logger:     logger,
	}, nil
}

// DataPath returns the base data path.
func (s *Store) DataPath() string {
	return s.basePath
}

// MarketDataStorage returns the market data storage interface.
func (s *Store) MarketDataStorage() interfaces.MarketDataStorage {
	return &marketDataStorage{store: s}
}

// SignalStorage returns the signal storage interface.
func (s *Store) SignalStorage() interfaces.SignalStorage {
	return &signalStorage{store: s}
}

// WriteRaw writes arbitrary binary data to a subdirectory atomically.
func (s *Store) WriteRaw(subdir, key string, data []byte) error {
	dir := filepath.Join(s.basePath, subdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	target := filepath.Join(dir, sanitizeKey(key))

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

// PurgeMarket removes all market data files and returns the count.
func (s *Store) PurgeMarket() int {
	return purgeDir(s.marketDir)
}

// PurgeSignals removes all signal files and returns the count.
func (s *Store) PurgeSignals() int {
	return purgeDir(s.signalsDir)
}

// PurgeCharts removes all chart files and returns the count.
func (s *Store) PurgeCharts() int {
	chartsDir := filepath.Join(s.basePath, "charts")
	return purgeAllFiles(chartsDir)
}

// Close is a no-op for file-based storage.
func (s *Store) Close() error {
	return nil
}

// --- helpers ---

func sanitizeKey(key string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	return r.Replace(key)
}

func filePath(dir, key string) string {
	return filepath.Join(dir, sanitizeKey(key)+".json")
}

func readJSON(dir, key string, dest interface{}) error {
	path := filePath(dir, key)
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
	return json.Unmarshal(data, dest)
}

func writeJSON(dir, key string, data interface{}) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	target := filePath(dir, key)
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	jsonData = append(jsonData, '\n')

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

func listKeys(dir string) ([]string, error) {
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
		if strings.HasSuffix(name, ".json") && !strings.HasPrefix(name, ".tmp-") {
			key := strings.TrimSuffix(name, ".json")
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func deleteJSON(dir, key string) {
	target := filePath(dir, key)
	os.Remove(target)
}

func purgeDir(dir string) int {
	keys, err := listKeys(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, key := range keys {
		deleteJSON(dir, key)
		count++
	}
	return count
}

func purgeAllFiles(dir string) int {
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

// --- MarketDataStorage ---

type marketDataStorage struct {
	store *Store
}

func (m *marketDataStorage) GetMarketData(_ context.Context, ticker string) (*models.MarketData, error) {
	var data models.MarketData
	if err := readJSON(m.store.marketDir, ticker, &data); err != nil {
		return nil, fmt.Errorf("market data for '%s' not found", ticker)
	}
	return &data, nil
}

func (m *marketDataStorage) SaveMarketData(_ context.Context, data *models.MarketData) error {
	data.LastUpdated = time.Now()
	if err := writeJSON(m.store.marketDir, data.Ticker, data); err != nil {
		return fmt.Errorf("failed to save market data: %w", err)
	}
	m.store.logger.Debug().Str("ticker", data.Ticker).Msg("Market data saved")
	return nil
}

func (m *marketDataStorage) GetMarketDataBatch(_ context.Context, tickers []string) ([]*models.MarketData, error) {
	result := make([]*models.MarketData, 0, len(tickers))
	for _, ticker := range tickers {
		var data models.MarketData
		if err := readJSON(m.store.marketDir, ticker, &data); err == nil {
			result = append(result, &data)
		}
	}
	return result, nil
}

func (m *marketDataStorage) GetStaleTickers(_ context.Context, exchange string, maxAgeSeconds int64) ([]string, error) {
	cutoff := time.Now().Add(-time.Duration(maxAgeSeconds) * time.Second)
	keys, err := listKeys(m.store.marketDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list market data: %w", err)
	}
	var stale []string
	for _, key := range keys {
		var data models.MarketData
		if err := readJSON(m.store.marketDir, key, &data); err != nil {
			continue
		}
		if data.Exchange == exchange && data.LastUpdated.Before(cutoff) {
			stale = append(stale, data.Ticker)
		}
	}
	return stale, nil
}

// --- SignalStorage ---

type signalStorage struct {
	store *Store
}

func (ss *signalStorage) GetSignals(_ context.Context, ticker string) (*models.TickerSignals, error) {
	var signals models.TickerSignals
	if err := readJSON(ss.store.signalsDir, ticker, &signals); err != nil {
		return nil, fmt.Errorf("signals for '%s' not found", ticker)
	}
	return &signals, nil
}

func (ss *signalStorage) SaveSignals(_ context.Context, signals *models.TickerSignals) error {
	signals.ComputeTimestamp = time.Now()
	if err := writeJSON(ss.store.signalsDir, signals.Ticker, signals); err != nil {
		return fmt.Errorf("failed to save signals: %w", err)
	}
	ss.store.logger.Debug().Str("ticker", signals.Ticker).Msg("Signals saved")
	return nil
}

func (ss *signalStorage) GetSignalsBatch(_ context.Context, tickers []string) ([]*models.TickerSignals, error) {
	result := make([]*models.TickerSignals, 0, len(tickers))
	for _, ticker := range tickers {
		var signals models.TickerSignals
		if err := readJSON(ss.store.signalsDir, ticker, &signals); err == nil {
			result = append(result, &signals)
		}
	}
	return result, nil
}
