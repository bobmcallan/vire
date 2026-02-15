package badger

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

const maxSearchRecords = 50

type searchHistoryStorage struct {
	store  *Store
	logger *common.Logger
}

// NewSearchHistoryStorage creates a new SearchHistoryStorage backed by BadgerHold.
func NewSearchHistoryStorage(store *Store, logger *common.Logger) *searchHistoryStorage {
	return &searchHistoryStorage{store: store, logger: logger}
}

func (s *searchHistoryStorage) SaveSearch(_ context.Context, record *models.SearchRecord) error {
	if record.ID == "" {
		record.ID = fmt.Sprintf("search-%d-%s-%s", record.CreatedAt.Unix(), record.Type, record.Exchange)
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	if err := s.store.db.Upsert(record.ID, record); err != nil {
		return fmt.Errorf("failed to save search record: %w", err)
	}
	s.logger.Debug().Str("id", record.ID).Str("type", record.Type).Msg("Search record saved")

	// Prune oldest records if over max limit
	s.pruneOldRecords()

	return nil
}

func (s *searchHistoryStorage) pruneOldRecords() {
	var records []models.SearchRecord
	if err := s.store.db.Find(&records, nil); err != nil || len(records) <= maxSearchRecords {
		return
	}

	// Sort by CreatedAt descending (newest first)
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})

	// Delete oldest records beyond the limit
	for _, old := range records[maxSearchRecords:] {
		s.store.db.Delete(old.ID, models.SearchRecord{})
	}
	s.logger.Debug().Int("pruned", len(records)-maxSearchRecords).Msg("Pruned old search records")
}

func (s *searchHistoryStorage) GetSearch(_ context.Context, id string) (*models.SearchRecord, error) {
	var record models.SearchRecord
	err := s.store.db.Get(id, &record)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("search record '%s' not found", id)
		}
		return nil, fmt.Errorf("failed to get search record '%s': %w", id, err)
	}
	return &record, nil
}

func (s *searchHistoryStorage) ListSearches(_ context.Context, options interfaces.SearchListOptions) ([]*models.SearchRecord, error) {
	var records []models.SearchRecord

	// Build query from options
	var query *badgerhold.Query
	if options.Type != "" {
		query = badgerhold.Where("Type").Eq(options.Type)
		if options.Exchange != "" {
			query = query.And("Exchange").Eq(options.Exchange)
		}
	} else if options.Exchange != "" {
		query = badgerhold.Where("Exchange").Eq(options.Exchange)
	}

	if err := s.store.db.Find(&records, query); err != nil {
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

func (s *searchHistoryStorage) DeleteSearch(_ context.Context, id string) error {
	err := s.store.db.Delete(id, models.SearchRecord{})
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete search record '%s': %w", id, err)
	}
	s.logger.Debug().Str("id", id).Msg("Search record deleted")
	return nil
}
