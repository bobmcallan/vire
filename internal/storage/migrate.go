package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bobmcallan/vire/internal/common"
)

// userDirs are subdirectories that belong in the user store.
var userDirs = []string{"portfolios", "strategies", "plans", "watchlists", "reports", "searches", "kv"}

// dataDirs are subdirectories that belong in the data store.
var dataDirs = []string{"market", "signals", "charts"}

// MigrateToSplitLayout detects the old flat storage layout and migrates
// subdirectories into the new split layout (user store + data store).
//
// Old layout (single base path):
//
//	data/portfolios/, data/market/, data/signals/, ...
//
// New layout (split):
//
//	data/user/portfolios/, data/user/strategies/, ...
//	data/data/market/, data/data/signals/, ...
//
// Detection: if the parent directory of UserData.Path contains a "portfolios/"
// subdirectory, the old layout is assumed. The function moves each subdirectory
// to the appropriate new location.
func MigrateToSplitLayout(logger *common.Logger, config *common.Config) error {
	userPath := config.Storage.UserData.Path
	dataPath := config.Storage.Data.Path

	if userPath == "" || dataPath == "" {
		return nil
	}

	// Derive the old base path: parent of the user path.
	// e.g., if UserData.Path = "data/user", old base = "data"
	oldBase := filepath.Dir(userPath)

	// Check if old flat layout exists by looking for "portfolios" in the old base
	oldPortfolios := filepath.Join(oldBase, "portfolios")
	if _, err := os.Stat(oldPortfolios); os.IsNotExist(err) {
		return nil // No old layout detected
	}

	// Don't migrate if old base IS one of the new paths (would be self-referential)
	absOldBase, _ := filepath.Abs(oldBase)
	absUserPath, _ := filepath.Abs(userPath)
	absDataPath, _ := filepath.Abs(dataPath)
	if absOldBase == absUserPath || absOldBase == absDataPath {
		return nil
	}

	logger.Info().
		Str("old_base", oldBase).
		Str("user_path", userPath).
		Str("data_path", dataPath).
		Msg("Detected old flat storage layout — migrating to split layout")

	// Ensure new directories exist
	if err := os.MkdirAll(userPath, 0755); err != nil {
		return fmt.Errorf("failed to create user path %s: %w", userPath, err)
	}
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return fmt.Errorf("failed to create data path %s: %w", dataPath, err)
	}

	moved := 0

	// Move user dirs
	for _, dir := range userDirs {
		src := filepath.Join(oldBase, dir)
		dst := filepath.Join(userPath, dir)
		if n, err := moveDir(src, dst); err != nil {
			logger.Warn().Err(err).Str("dir", dir).Msg("Failed to migrate directory")
		} else if n > 0 {
			logger.Info().Str("dir", dir).Str("from", src).Str("to", dst).Msg("Migrated directory")
			moved++
		}
	}

	// Move data dirs
	for _, dir := range dataDirs {
		src := filepath.Join(oldBase, dir)
		dst := filepath.Join(dataPath, dir)
		if n, err := moveDir(src, dst); err != nil {
			logger.Warn().Err(err).Str("dir", dir).Msg("Failed to migrate directory")
		} else if n > 0 {
			logger.Info().Str("dir", dir).Str("from", src).Str("to", dst).Msg("Migrated directory")
			moved++
		}
	}

	logger.Info().Int("directories_moved", moved).Msg("Storage migration complete")
	return nil
}

// moveDir moves src to dst using os.Rename. Returns 1 if moved, 0 if src didn't exist.
func moveDir(src, dst string) (int, error) {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return 0, nil
	}

	// If destination already exists, skip (don't overwrite)
	if _, err := os.Stat(dst); err == nil {
		return 0, nil
	}

	// Ensure parent of dst exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return 0, fmt.Errorf("failed to create parent for %s: %w", dst, err)
	}

	if err := os.Rename(src, dst); err != nil {
		return 0, fmt.Errorf("failed to move %s to %s: %w", src, dst, err)
	}

	return 1, nil
}
