package surrealdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// TimelineStore implements interfaces.TimelineStore using SurrealDB.
type TimelineStore struct {
	db     *surrealdb.DB
	logger *common.Logger
}

// NewTimelineStore creates a new TimelineStore.
func NewTimelineStore(db *surrealdb.DB, logger *common.Logger) *TimelineStore {
	return &TimelineStore{db: db, logger: logger}
}

// timelineRecordID builds a deterministic record ID for a timeline snapshot.
// Format: {userID}:{portfolioName}:{YYYY-MM-DD}
func timelineRecordID(userID, portfolioName string, date time.Time) surrealmodels.RecordID {
	// Sanitize components: replace dots with underscores for SurrealDB compatibility
	safeUser := strings.ReplaceAll(userID, ".", "_")
	safeName := strings.ReplaceAll(portfolioName, ".", "_")
	key := fmt.Sprintf("%s:%s:%s", safeUser, safeName, date.Format("2006-01-02"))
	return surrealmodels.NewRecordID("portfolio_timeline", key)
}

func (s *TimelineStore) GetRange(ctx context.Context, userID, portfolioName string, from, to time.Time) ([]models.TimelineSnapshot, error) {
	sql := `SELECT * FROM portfolio_timeline
		WHERE user_id = $uid AND portfolio_name = $name
		AND date >= $from AND date <= $to
		ORDER BY date ASC`
	vars := map[string]any{
		"uid":  userID,
		"name": portfolioName,
		"from": from.Truncate(24 * time.Hour),
		"to":   to.Truncate(24 * time.Hour),
	}

	results, err := surrealdb.Query[[]models.TimelineSnapshot](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("timeline GetRange: %w", err)
	}

	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (s *TimelineStore) GetLatest(ctx context.Context, userID, portfolioName string) (*models.TimelineSnapshot, error) {
	sql := `SELECT * FROM portfolio_timeline
		WHERE user_id = $uid AND portfolio_name = $name
		ORDER BY date DESC LIMIT 1`
	vars := map[string]any{
		"uid":  userID,
		"name": portfolioName,
	}

	results, err := surrealdb.Query[[]models.TimelineSnapshot](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("timeline GetLatest: %w", err)
	}

	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (s *TimelineStore) SaveBatch(ctx context.Context, snapshots []models.TimelineSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	// Batch UPSERTs: build a multi-statement query for efficiency.
	// SurrealDB supports multiple statements separated by semicolons.
	const batchSize = 100
	for i := 0; i < len(snapshots); i += batchSize {
		end := i + batchSize
		if end > len(snapshots) {
			end = len(snapshots)
		}
		chunk := snapshots[i:end]

		var sb strings.Builder
		vars := make(map[string]any)
		for j, snap := range chunk {
			rid := timelineRecordID(snap.UserID, snap.PortfolioName, snap.Date)
			ridKey := fmt.Sprintf("rid%d", j)
			dataKey := fmt.Sprintf("data%d", j)
			vars[ridKey] = rid
			vars[dataKey] = snap
			fmt.Fprintf(&sb, "UPSERT $%s CONTENT $%s;", ridKey, dataKey)
		}

		if _, err := surrealdb.Query[any](ctx, s.db, sb.String(), vars); err != nil {
			return fmt.Errorf("timeline SaveBatch: %w", err)
		}
	}

	s.logger.Debug().Int("count", len(snapshots)).Msg("Timeline snapshots saved")
	return nil
}

func (s *TimelineStore) DeleteRange(ctx context.Context, userID, portfolioName string, from, to time.Time) (int, error) {
	sql := `DELETE FROM portfolio_timeline
		WHERE user_id = $uid AND portfolio_name = $name
		AND date >= $from AND date <= $to`
	vars := map[string]any{
		"uid":  userID,
		"name": portfolioName,
		"from": from.Truncate(24 * time.Hour),
		"to":   to.Truncate(24 * time.Hour),
	}

	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return 0, fmt.Errorf("timeline DeleteRange: %w", err)
	}
	return 0, nil // SurrealDB doesn't easily return affected count
}

func (s *TimelineStore) DeleteAll(ctx context.Context, userID, portfolioName string) (int, error) {
	sql := `DELETE FROM portfolio_timeline
		WHERE user_id = $uid AND portfolio_name = $name`
	vars := map[string]any{
		"uid":  userID,
		"name": portfolioName,
	}

	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return 0, fmt.Errorf("timeline DeleteAll: %w", err)
	}
	return 0, nil
}

// Compile-time check
var _ interfaces.TimelineStore = (*TimelineStore)(nil)
