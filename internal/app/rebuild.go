package app

import (
	"context"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

const schemaVersionKey = "vire_schema_version"
const buildTimestampKey = "vire_build_timestamp"

// checkSchemaVersion compares the stored schema version against the code's
// SchemaVersion constant. On mismatch (or missing version), it purges all
// derived data and stores the new version. Returns true if a rebuild occurred.
func checkSchemaVersion(ctx context.Context, sm interfaces.StorageManager, logger *common.Logger) bool {
	store := sm.InternalStore()

	stored, err := store.GetSystemKV(ctx, schemaVersionKey)
	if err == nil && stored == common.SchemaVersion {
		logger.Info().
			Str("version", common.SchemaVersion).
			Msg("Schema version matches — no rebuild needed")
		return false
	}

	if stored == "" {
		logger.Info().
			Str("current", common.SchemaVersion).
			Msg("Schema version not found — initializing (first run or pre-versioning)")
	} else {
		logger.Warn().
			Str("stored", stored).
			Str("current", common.SchemaVersion).
			Msg("Schema version mismatch — purging derived data")
	}

	counts, purgeErr := sm.PurgeDerivedData(ctx)
	if purgeErr != nil {
		logger.Error().Err(purgeErr).Msg("Failed to purge derived data during schema migration")
		return false
	}

	total := 0
	for _, v := range counts {
		total += v
	}
	logger.Info().
		Int("total", total).
		Str("new_version", common.SchemaVersion).
		Msg("Schema migration complete — derived data purged")

	if err := store.SetSystemKV(ctx, schemaVersionKey, common.SchemaVersion); err != nil {
		logger.Error().Err(err).Msg("Failed to store new schema version")
	}

	return true
}

// checkDevBuildChange detects if the build timestamp has changed since last startup.
// In non-production environments, a build change triggers a cache purge so that
// code changes (e.g. formatter updates) are immediately visible without manual rebuild.
// Returns true if a purge occurred.
func checkDevBuildChange(ctx context.Context, sm interfaces.StorageManager, config *common.Config, logger *common.Logger) bool {
	// Only purge on build change in non-production environments
	if !config.IsDevelopment() {
		return false
	}

	store := sm.InternalStore()
	currentBuild := common.GetBuild()

	// Skip if build is unknown (local dev without ldflags)
	if currentBuild == "unknown" {
		return false
	}

	storedBuild, err := store.GetSystemKV(ctx, buildTimestampKey)
	if err == nil && storedBuild == currentBuild {
		logger.Debug().
			Str("build", currentBuild).
			Msg("Build timestamp unchanged — skipping dev cache purge")
		return false
	}

	if storedBuild != "" {
		logger.Info().
			Str("previous_build", storedBuild).
			Str("current_build", currentBuild).
			Msg("Dev mode: build changed — purging cached reports")

		// Only purge reports (not market data) for faster startup
		counts, purgeErr := sm.PurgeReports(ctx)
		if purgeErr != nil {
			logger.Error().Err(purgeErr).Msg("Failed to purge reports on build change")
		} else {
			logger.Info().
				Int("reports", counts).
				Msg("Dev mode: reports purged due to build change")
		}
	}

	// Store current build timestamp
	if err := store.SetSystemKV(ctx, buildTimestampKey, currentBuild); err != nil {
		logger.Error().Err(err).Msg("Failed to store build timestamp")
	}

	return storedBuild != ""
}
