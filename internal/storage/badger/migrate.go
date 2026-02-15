package badger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// domainDirs maps old file-based subdirectory names to their migration handler.
var domainDirs = []string{
	"users", "portfolios", "strategies", "plans",
	"watchlists", "reports", "searches", "kv",
}

// MigrateFromFiles migrates file-based JSON user data into a BadgerHold store.
// It looks for old-layout directories under the parent of the BadgerDB path.
// For example, if badgerPath is "data/user/badger", it checks "data/user/users/".
// After successful migration, old directories are renamed to .migrated-{timestamp}.
// If no old directories are found, the function returns nil (fresh install or already migrated).
func MigrateFromFiles(logger *common.Logger, store *Store, badgerPath string) error {
	// The old file-based layout lives in the parent of the badger directory.
	// e.g., badgerPath="data/user/badger" → parentPath="data/user"
	parentPath := filepath.Dir(badgerPath)

	// Check if any old directories exist
	hasOldData := false
	for _, dir := range domainDirs {
		if info, err := os.Stat(filepath.Join(parentPath, dir)); err == nil && info.IsDir() {
			hasOldData = true
			break
		}
	}
	if !hasOldData {
		return nil // Nothing to migrate
	}

	logger.Info().Str("source", parentPath).Msg("Migrating file-based user data to BadgerDB")

	totalCounts := make(map[string]int)

	for _, dir := range domainDirs {
		dirPath := filepath.Join(parentPath, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			continue
		}

		count, err := migrateDir(store, dir, dirPath)
		if err != nil {
			logger.Warn().Err(err).Str("dir", dir).Msg("Failed to migrate directory")
			continue
		}
		totalCounts[dir] = count
	}

	// Rename old directories to .migrated-{timestamp}
	timestamp := time.Now().Format("20060102-150405")
	migratedBase := filepath.Join(parentPath, fmt.Sprintf(".migrated-%s", timestamp))
	if err := os.MkdirAll(migratedBase, 0755); err != nil {
		logger.Warn().Err(err).Msg("Failed to create migration backup directory")
	} else {
		for _, dir := range domainDirs {
			src := filepath.Join(parentPath, dir)
			if _, err := os.Stat(src); os.IsNotExist(err) {
				continue
			}
			dst := filepath.Join(migratedBase, dir)
			if err := os.Rename(src, dst); err != nil {
				logger.Warn().Err(err).Str("dir", dir).Msg("Failed to move old directory to backup")
			}
		}
	}

	logger.Info().
		Int("users", totalCounts["users"]).
		Int("portfolios", totalCounts["portfolios"]).
		Int("strategies", totalCounts["strategies"]).
		Int("plans", totalCounts["plans"]).
		Int("watchlists", totalCounts["watchlists"]).
		Int("reports", totalCounts["reports"]).
		Int("searches", totalCounts["searches"]).
		Int("kv", totalCounts["kv"]).
		Msg("File-to-BadgerDB migration complete")

	return nil
}

// migrateDir reads all .json files from a directory and inserts them into BadgerDB.
func migrateDir(store *Store, domainType, dirPath string) (int, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	count := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".tmp-") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dirPath, name))
		if err != nil || len(data) == 0 {
			continue
		}

		key := strings.TrimSuffix(name, ".json")

		if err := migrateRecord(store, domainType, key, data); err != nil {
			continue // Skip individual failures
		}
		count++
	}
	return count, nil
}

// migrateRecord inserts a single JSON record into BadgerDB based on domain type.
func migrateRecord(store *Store, domainType, key string, data []byte) error {
	switch domainType {
	case "users":
		var v models.User
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		k := v.Username
		if k == "" {
			k = key
		}
		return store.db.Upsert(k, &v)
	case "portfolios":
		var v models.Portfolio
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		id := v.ID
		if id == "" {
			id = key
		}
		return store.db.Upsert(id, &v)
	case "strategies":
		var v models.PortfolioStrategy
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		k := v.PortfolioName
		if k == "" {
			k = key
		}
		return store.db.Upsert(k, &v)
	case "plans":
		var v models.PortfolioPlan
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		k := v.PortfolioName
		if k == "" {
			k = key
		}
		return store.db.Upsert(k, &v)
	case "watchlists":
		var v models.PortfolioWatchlist
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		k := v.PortfolioName
		if k == "" {
			k = key
		}
		return store.db.Upsert(k, &v)
	case "reports":
		var v models.PortfolioReport
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		k := v.Portfolio
		if k == "" {
			k = key
		}
		return store.db.Upsert(k, &v)
	case "searches":
		var v models.SearchRecord
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		k := v.ID
		if k == "" {
			k = key
		}
		return store.db.Upsert(k, &v)
	case "kv":
		var v KVEntry
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		k := v.Key
		if k == "" {
			k = key
		}
		return store.db.Upsert(k, &v)
	default:
		return fmt.Errorf("unknown domain type: %s", domainType)
	}
}
