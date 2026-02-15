// Package userdb implements UserDataStore using BadgerHold.
// It stores all user domain data as generic UserRecord entries.
package userdb

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

// maxSearchRecords is the auto-prune limit for the "search" subject.
const maxSearchRecords = 50

// Store implements interfaces.UserDataStore using BadgerHold.
type Store struct {
	db     *badgerhold.Store
	logger *common.Logger
}

// NewStore creates a new UserDataStore backed by BadgerHold.
func NewStore(logger *common.Logger, path string) (*Store, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create userdb path %s: %w", path, err)
	}
	opts := badgerhold.DefaultOptions
	opts.Dir = path
	opts.ValueDir = path
	opts.Logger = nil
	db, err := badgerhold.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open userdb at %s: %w", path, err)
	}
	logger.Info().Str("path", path).Msg("UserDB opened")
	return &Store{db: db, logger: logger}, nil
}

// keySep is the composite key separator. Using a null byte prevents collisions
// when userID, subject, or key contain ":" characters.
const keySep = "\x00"

// compositeKey builds the storage key: user_id + \x00 + subject + \x00 + key
func compositeKey(userID, subject, key string) string {
	return userID + keySep + subject + keySep + key
}

func (s *Store) Get(_ context.Context, userID, subject, key string) (*models.UserRecord, error) {
	ck := compositeKey(userID, subject, key)
	var rec models.UserRecord
	if err := s.db.Get(ck, &rec); err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("%s '%s' not found for user '%s'", subject, key, userID)
		}
		return nil, fmt.Errorf("failed to get %s '%s': %w", subject, key, err)
	}
	return &rec, nil
}

func (s *Store) Put(_ context.Context, record *models.UserRecord) error {
	ck := compositeKey(record.UserID, record.Subject, record.Key)
	now := time.Now()

	// Read existing to increment version
	var existing models.UserRecord
	if err := s.db.Get(ck, &existing); err == nil {
		record.Version = existing.Version + 1
	} else {
		record.Version = 1
	}
	record.DateTime = now

	if err := s.db.Upsert(ck, record); err != nil {
		return fmt.Errorf("failed to put %s '%s': %w", record.Subject, record.Key, err)
	}

	// Auto-prune search history
	if record.Subject == "search" {
		s.pruneSearch(record.UserID)
	}

	return nil
}

func (s *Store) Delete(_ context.Context, userID, subject, key string) error {
	ck := compositeKey(userID, subject, key)
	if err := s.db.Delete(ck, models.UserRecord{}); err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete %s '%s': %w", subject, key, err)
	}
	return nil
}

func (s *Store) List(_ context.Context, userID, subject string) ([]*models.UserRecord, error) {
	var all []models.UserRecord
	if err := s.db.Find(&all, nil); err != nil {
		return nil, fmt.Errorf("failed to list %s records: %w", subject, err)
	}
	var result []*models.UserRecord
	for i := range all {
		if all[i].UserID == userID && all[i].Subject == subject {
			rec := all[i]
			result = append(result, &rec)
		}
	}
	return result, nil
}

func (s *Store) Query(_ context.Context, userID, subject string, opts interfaces.QueryOptions) ([]*models.UserRecord, error) {
	var all []models.UserRecord
	if err := s.db.Find(&all, nil); err != nil {
		return nil, fmt.Errorf("failed to query %s records: %w", subject, err)
	}
	var result []*models.UserRecord
	for i := range all {
		if all[i].UserID == userID && all[i].Subject == subject {
			rec := all[i]
			result = append(result, &rec)
		}
	}

	// Sort
	if opts.OrderBy == "datetime_asc" {
		sort.Slice(result, func(i, j int) bool {
			return result[i].DateTime.Before(result[j].DateTime)
		})
	} else {
		// Default: datetime_desc
		sort.Slice(result, func(i, j int) bool {
			return result[i].DateTime.After(result[j].DateTime)
		})
	}

	// Limit
	if opts.Limit > 0 && len(result) > opts.Limit {
		result = result[:opts.Limit]
	}

	return result, nil
}

func (s *Store) DeleteBySubject(_ context.Context, subject string) (int, error) {
	var all []models.UserRecord
	if err := s.db.Find(&all, nil); err != nil {
		return 0, fmt.Errorf("failed to find %s records: %w", subject, err)
	}
	count := 0
	for _, rec := range all {
		if rec.Subject == subject {
			ck := compositeKey(rec.UserID, rec.Subject, rec.Key)
			if err := s.db.Delete(ck, models.UserRecord{}); err == nil {
				count++
			}
		}
	}
	return count, nil
}

// pruneSearch trims old search records to maxSearchRecords per user.
func (s *Store) pruneSearch(userID string) {
	var all []models.UserRecord
	if err := s.db.Find(&all, nil); err != nil {
		return
	}
	var searches []models.UserRecord
	for _, rec := range all {
		if rec.UserID == userID && rec.Subject == "search" {
			searches = append(searches, rec)
		}
	}
	if len(searches) <= maxSearchRecords {
		return
	}
	// Sort by DateTime descending — keep newest
	sort.Slice(searches, func(i, j int) bool {
		return searches[i].DateTime.After(searches[j].DateTime)
	})
	// Delete the oldest entries beyond the limit
	for _, rec := range searches[maxSearchRecords:] {
		ck := compositeKey(rec.UserID, rec.Subject, rec.Key)
		_ = s.db.Delete(ck, models.UserRecord{})
	}
	pruned := len(searches) - maxSearchRecords
	if pruned > 0 {
		s.logger.Debug().
			Str("user_id", userID).
			Int("pruned", pruned).
			Msg("Pruned old search records")
	}
}

// FindAllBySubject returns all records matching a subject across all users.
// Used by migration and purge operations.
func (s *Store) FindAllBySubject(subject string) ([]models.UserRecord, error) {
	var all []models.UserRecord
	if err := s.db.Find(&all, nil); err != nil {
		return nil, fmt.Errorf("failed to find records by subject '%s': %w", subject, err)
	}
	var result []models.UserRecord
	for _, rec := range all {
		if rec.Subject == subject {
			result = append(result, rec)
		}
	}
	return result, nil
}

// DeleteBySubjects deletes records for multiple subjects. Returns total count.
func (s *Store) DeleteBySubjects(ctx context.Context, subjects ...string) (int, error) {
	total := 0
	for _, subj := range subjects {
		n, err := s.DeleteBySubject(ctx, subj)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// DB returns the underlying badgerhold store for migration use.
func (s *Store) DB() *badgerhold.Store {
	return s.db
}

// Close shuts down the BadgerHold database.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// matchSubject checks if a record has the given subject.
// Uses case-sensitive comparison, consistent with List and Query.
func matchSubject(rec *models.UserRecord, subject string) bool {
	return rec.Subject == subject
}
