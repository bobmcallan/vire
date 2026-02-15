// Package storage provides blob-based persistence with pluggable backends.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// FileStore provides file-based JSON storage with optional versioning.
type FileStore struct {
	basePath string
	versions int
	logger   *common.Logger
}

// NewFileStore creates a new FileStore and ensures the base directory exists.
// Subdirectories are created by each domain storage constructor via os.MkdirAll.
func NewFileStore(logger *common.Logger, config *common.FileConfig) (*FileStore, error) {
	versions := config.Versions
	if versions < 0 {
		versions = 0
	}

	if err := os.MkdirAll(config.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory %s: %w", config.Path, err)
	}

	fs := &FileStore{
		basePath: config.Path,
		versions: versions,
		logger:   logger,
	}

	logger.Debug().Str("path", config.Path).Int("versions", versions).Msg("FileStore opened")
	return fs, nil
}

// Close releases any resources held by the FileStore.
func (fs *FileStore) Close() error {
	return nil
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
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

// --- Market Data Storage ---

type marketDataStorage struct {
	fs     *FileStore
	dir    string
	logger *common.Logger
}

func newMarketDataStorage(fs *FileStore, logger *common.Logger) *marketDataStorage {
	dir := filepath.Join(fs.basePath, "market")
	os.MkdirAll(dir, 0755)
	return &marketDataStorage{fs: fs, dir: dir, logger: logger}
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
	dir := filepath.Join(fs.basePath, "signals")
	os.MkdirAll(dir, 0755)
	return &signalStorage{fs: fs, dir: dir, logger: logger}
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
