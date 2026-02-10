package app

import (
	"context"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
)

const schemaVersionKey = "vire_schema_version"

// checkSchemaVersion compares the stored schema version against the code's
// SchemaVersion constant. On mismatch (or missing version), it purges all
// derived data and stores the new version. Returns true if a rebuild occurred.
func checkSchemaVersion(ctx context.Context, sm interfaces.StorageManager, logger *common.Logger) bool {
	kv := sm.KeyValueStorage()

	stored, err := kv.Get(ctx, schemaVersionKey)
	if err == nil && stored == common.SchemaVersion {
		logger.Info().
			Str("version", common.SchemaVersion).
			Msg("Schema version matches — no rebuild needed")
		return false
	}

	if err != nil {
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

	total := counts["portfolios"] + counts["market_data"] + counts["signals"] + counts["reports"]
	logger.Info().
		Int("portfolios", counts["portfolios"]).
		Int("market_data", counts["market_data"]).
		Int("signals", counts["signals"]).
		Int("reports", counts["reports"]).
		Int("total", total).
		Str("new_version", common.SchemaVersion).
		Msg("Schema migration complete — derived data purged")

	if err := kv.Set(ctx, schemaVersionKey, common.SchemaVersion); err != nil {
		logger.Error().Err(err).Msg("Failed to store new schema version")
	}

	return true
}
