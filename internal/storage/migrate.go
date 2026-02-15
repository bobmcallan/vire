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
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

// MigrateOldLayout detects pre-refactor storage layouts and migrates data
// to the new 3-area layout (internal, user, market).
//
// Detection: if data/user/badger/ exists, the old BadgerHold database is
// present. Records are read, split by type, and written to the new stores.
// Market files (data/data/market, data/data/signals, data/data/charts) are
// moved to the new market path. Old directories are renamed with a
// .migrated-{timestamp} suffix.
func MigrateOldLayout(ctx context.Context, logger *common.Logger, config *common.Config, sm interfaces.StorageManager) error {
	// Check for old badger path patterns
	oldBadgerPaths := []string{
		"data/user/badger",
		filepath.Join(filepath.Dir(config.Storage.Internal.Path), "user", "badger"),
	}

	var oldBadgerPath string
	for _, p := range oldBadgerPaths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			oldBadgerPath = p
			break
		}
	}

	if oldBadgerPath == "" {
		// Also check for old split layout (data/user/portfolios, etc.)
		migrateOldSplitLayout(ctx, logger, config, sm)
		return nil
	}

	logger.Info().Str("path", oldBadgerPath).Msg("Detected old BadgerHold database — migrating")

	// Open old database in read-only mode
	opts := badgerhold.DefaultOptions
	opts.Dir = oldBadgerPath
	opts.ValueDir = oldBadgerPath
	opts.Logger = nil
	opts.ReadOnly = true

	oldDB, err := badgerhold.Open(opts)
	if err != nil {
		return fmt.Errorf("failed to open old badger db at %s: %w", oldBadgerPath, err)
	}
	defer oldDB.Close()

	internalStore := sm.InternalStore()
	userDataStore := sm.UserDataStore()

	// Migrate users (old models.User -> InternalUser + UserKV)
	migrateUsers(ctx, logger, oldDB, internalStore)

	// Migrate domain data (portfolios, strategies, plans, watchlists, reports, searches)
	migrateDomainData(ctx, logger, oldDB, userDataStore)

	// Migrate KV entries (old kv -> system KV)
	migrateKVEntries(ctx, logger, oldDB, internalStore)

	// Rename old directory
	timestamp := time.Now().Format("20060102-150405")
	renamedPath := oldBadgerPath + ".migrated-" + timestamp
	if err := os.Rename(oldBadgerPath, renamedPath); err != nil {
		logger.Warn().Err(err).Msg("Failed to rename old badger directory")
	} else {
		logger.Info().Str("renamed_to", renamedPath).Msg("Old database renamed")
	}

	// Migrate market files
	migrateMarketFiles(logger, config)

	logger.Info().Msg("Migration from old layout complete")
	return nil
}

// oldUser matches the pre-refactor models.User struct layout.
type oldUser struct {
	Username         string   `json:"username"`
	Email            string   `json:"email"`
	PasswordHash     string   `json:"password_hash"`
	Role             string   `json:"role"`
	NavexaKey        string   `json:"navexa_key"`
	DisplayCurrency  string   `json:"display_currency"`
	DefaultPortfolio string   `json:"default_portfolio"`
	Portfolios       []string `json:"portfolios"`
}

func migrateUsers(ctx context.Context, logger *common.Logger, oldDB *badgerhold.Store, store interfaces.InternalStore) {
	var users []oldUser
	if err := oldDB.Find(&users, nil); err != nil {
		logger.Warn().Err(err).Msg("Migration: failed to read old users")
		return
	}

	for _, u := range users {
		if u.Username == "" {
			continue
		}
		user := &models.InternalUser{
			UserID:       u.Username,
			Email:        u.Email,
			PasswordHash: u.PasswordHash,
			Role:         u.Role,
			CreatedAt:    time.Now(),
		}
		if err := store.SaveUser(ctx, user); err != nil {
			logger.Warn().Err(err).Str("user", u.Username).Msg("Migration: failed to save user")
			continue
		}

		// Save preferences as UserKV
		if u.NavexaKey != "" {
			store.SetUserKV(ctx, u.Username, "navexa_key", u.NavexaKey)
		}
		if u.DisplayCurrency != "" {
			store.SetUserKV(ctx, u.Username, "display_currency", u.DisplayCurrency)
		}
		if u.DefaultPortfolio != "" {
			store.SetUserKV(ctx, u.Username, "default_portfolio", u.DefaultPortfolio)
		}
		if len(u.Portfolios) > 0 {
			store.SetUserKV(ctx, u.Username, "portfolios", strings.Join(u.Portfolios, ","))
		}
		logger.Debug().Str("user", u.Username).Msg("Migration: user migrated")
	}
	logger.Info().Int("count", len(users)).Msg("Migration: users migrated")
}

// oldKVEntry matches the old KV storage entry format.
type oldKVEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func migrateKVEntries(ctx context.Context, logger *common.Logger, oldDB *badgerhold.Store, store interfaces.InternalStore) {
	var entries []oldKVEntry
	if err := oldDB.Find(&entries, nil); err != nil {
		logger.Debug().Msg("Migration: no old KV entries found (or different schema)")
		return
	}

	for _, e := range entries {
		if e.Key == "" {
			continue
		}
		if err := store.SetSystemKV(ctx, e.Key, e.Value); err != nil {
			logger.Warn().Err(err).Str("key", e.Key).Msg("Migration: failed to migrate KV entry")
		}
	}
	logger.Info().Int("count", len(entries)).Msg("Migration: KV entries migrated")
}

func migrateDomainData(ctx context.Context, logger *common.Logger, oldDB *badgerhold.Store, store interfaces.UserDataStore) {
	// The old storage used per-type JSON files. Try to read them from
	// the badgerhold DB if they were stored there.
	// Map of old type name -> subject name for UserRecord
	typeMap := map[string]string{
		"Portfolio":          "portfolio",
		"PortfolioStrategy":  "strategy",
		"PortfolioPlan":      "plan",
		"PortfolioWatchlist": "watchlist",
		"PortfolioReport":    "report",
		"SearchRecord":       "search",
	}

	for typeName, subject := range typeMap {
		migrateGenericRecords(ctx, logger, oldDB, store, typeName, subject)
	}
}

func migrateGenericRecords(ctx context.Context, logger *common.Logger, oldDB *badgerhold.Store, store interfaces.UserDataStore, typeName, subject string) {
	// Try to iterate all records as raw JSON.
	// Since the old DB may have different struct layouts, we use a generic map approach.
	var records []map[string]interface{}
	if err := oldDB.Find(&records, nil); err != nil {
		logger.Debug().Str("type", typeName).Msg("Migration: could not read records (may not exist)")
		return
	}

	migrated := 0
	for _, rec := range records {
		data, err := json.Marshal(rec)
		if err != nil {
			continue
		}

		// Determine key based on subject type
		key := ""
		switch subject {
		case "portfolio":
			if name, ok := rec["name"].(string); ok {
				key = name
			}
		case "strategy", "plan", "watchlist", "report":
			if name, ok := rec["portfolio_name"].(string); ok {
				key = name
			}
		case "search":
			if id, ok := rec["id"].(string); ok {
				key = id
			}
		}

		if key == "" {
			continue
		}

		if err := store.Put(ctx, &models.UserRecord{
			UserID:  "default",
			Subject: subject,
			Key:     key,
			Value:   string(data),
		}); err != nil {
			logger.Warn().Err(err).Str("type", typeName).Str("key", key).Msg("Migration: failed to save record")
			continue
		}
		migrated++
	}

	if migrated > 0 {
		logger.Info().Str("subject", subject).Int("count", migrated).Msg("Migration: domain records migrated")
	}
}

func migrateMarketFiles(logger *common.Logger, config *common.Config) {
	// Move market-related directories to the new market path
	oldDataPaths := []string{
		"data/data",
		filepath.Join(filepath.Dir(config.Storage.Internal.Path), "data"),
	}

	for _, oldDataPath := range oldDataPaths {
		for _, dir := range []string{"market", "signals", "charts"} {
			src := filepath.Join(oldDataPath, dir)
			dst := filepath.Join(config.Storage.Market.Path, dir)

			if _, err := os.Stat(src); os.IsNotExist(err) {
				continue
			}
			if _, err := os.Stat(dst); err == nil {
				continue // Already exists
			}

			os.MkdirAll(filepath.Dir(dst), 0755)
			if err := os.Rename(src, dst); err != nil {
				logger.Warn().Err(err).Str("src", src).Str("dst", dst).Msg("Migration: failed to move market directory")
			} else {
				logger.Info().Str("src", src).Str("dst", dst).Msg("Migration: market directory moved")
			}
		}
	}
}

// migrateOldSplitLayout handles the pre-3-area split layout where domain data
// was stored as JSON files under data/user/{portfolios,strategies,...}.
func migrateOldSplitLayout(ctx context.Context, logger *common.Logger, config *common.Config, sm interfaces.StorageManager) {
	// Check for old JSON-file domain data under the user path
	oldUserBase := filepath.Dir(config.Storage.User.Path)
	oldPortfoliosDir := filepath.Join(oldUserBase, "user", "portfolios")

	if _, err := os.Stat(oldPortfoliosDir); os.IsNotExist(err) {
		return // No old split layout
	}

	logger.Info().Msg("Detected old split JSON layout — migrating domain data")

	// Map of old directory names to new subject names
	dirMap := map[string]string{
		"portfolios": "portfolio",
		"strategies": "strategy",
		"plans":      "plan",
		"watchlists": "watchlist",
		"reports":    "report",
		"searches":   "search",
	}

	userBase := filepath.Join(oldUserBase, "user")
	for dirName, subject := range dirMap {
		dir := filepath.Join(userBase, dirName)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		migrated := 0
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}

			key := strings.TrimSuffix(e.Name(), ".json")
			if err := sm.UserDataStore().Put(ctx, &models.UserRecord{
				UserID:  "default",
				Subject: subject,
				Key:     key,
				Value:   string(data),
			}); err != nil {
				logger.Warn().Err(err).Str("subject", subject).Str("key", key).Msg("Migration: failed to import JSON file")
				continue
			}
			migrated++
		}

		if migrated > 0 {
			logger.Info().Str("subject", subject).Int("count", migrated).Msg("Migration: JSON files migrated")
		}

		// Rename old directory
		timestamp := time.Now().Format("20060102-150405")
		renamedDir := dir + ".migrated-" + timestamp
		os.Rename(dir, renamedDir)
	}

	// Also migrate KV entries
	kvDir := filepath.Join(userBase, "kv")
	if entries, err := os.ReadDir(kvDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(kvDir, e.Name()))
			if err != nil {
				continue
			}
			key := strings.TrimSuffix(e.Name(), ".json")
			var val string
			if err := json.Unmarshal(data, &val); err != nil {
				val = strings.TrimSpace(string(data))
			}
			sm.InternalStore().SetSystemKV(ctx, key, val)
		}
		timestamp := time.Now().Format("20060102-150405")
		os.Rename(kvDir, kvDir+".migrated-"+timestamp)
	}

	// Move market files
	migrateMarketFiles(logger, config)

	logger.Info().Msg("Migration from old split layout complete")
}
