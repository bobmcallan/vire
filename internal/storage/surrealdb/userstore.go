package surrealdb

import (
	"context"
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type UserStore struct {
	db     *surrealdb.DB
	logger *common.Logger
}

func NewUserStore(db *surrealdb.DB, logger *common.Logger) *UserStore {
	return &UserStore{
		db:     db,
		logger: logger,
	}
}

func recordID(userID, subject, key string) string {
	return userID + "_" + subject + "_" + key
}

func (s *UserStore) Get(ctx context.Context, userID, subject, key string) (*models.UserRecord, error) {
	record, err := surrealdb.Select[models.UserRecord](ctx, s.db, surrealmodels.NewRecordID("user_data", recordID(userID, subject, key)))
	if err != nil {
		return nil, fmt.Errorf("failed to select user record: %w", err)
	}
	if record == nil {
		return nil, fmt.Errorf("user record not found")
	}
	return record, nil
}

func (s *UserStore) Put(ctx context.Context, record *models.UserRecord) error {
	id := recordID(record.UserID, record.Subject, record.Key)
	sql := "UPSERT $rid CONTENT $record"
	vars := map[string]any{"rid": surrealmodels.NewRecordID("user_data", id), "record": record}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		_, err := surrealdb.Query[[]models.UserRecord](ctx, s.db, sql, vars)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("failed to put user record after retries: %w", lastErr)
}

func (s *UserStore) Delete(ctx context.Context, userID, subject, key string) error {
	_, err := surrealdb.Delete[models.UserRecord](ctx, s.db, surrealmodels.NewRecordID("user_data", recordID(userID, subject, key)))
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to delete user record: %w", err)
	}
	return nil
}

func (s *UserStore) List(ctx context.Context, userID, subject string) ([]*models.UserRecord, error) {
	sql := "SELECT * FROM user_data WHERE user_id = $user_id AND subject = $subject"
	vars := map[string]any{
		"user_id": userID,
		"subject": subject,
	}

	results, err := surrealdb.Query[[]models.UserRecord](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to list user records: %w", err)
	}

	if results != nil && len(*results) > 0 {
		var mapped []*models.UserRecord
		for i := range (*results)[0].Result {
			mapped = append(mapped, &(*results)[0].Result[i])
		}
		return mapped, nil
	}
	return nil, nil
}

func (s *UserStore) Query(ctx context.Context, userID, subject string, opts interfaces.QueryOptions) ([]*models.UserRecord, error) {
	sql := "SELECT * FROM user_data WHERE user_id = $user_id AND subject = $subject"

	if opts.OrderBy == "datetime_asc" {
		sql += " ORDER BY datetime ASC"
	} else {
		sql += " ORDER BY datetime DESC"
	}

	if opts.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	vars := map[string]any{
		"user_id": userID,
		"subject": subject,
	}

	results, err := surrealdb.Query[[]models.UserRecord](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to query user records: %w", err)
	}

	if results != nil && len(*results) > 0 {
		var mapped []*models.UserRecord
		for i := range (*results)[0].Result {
			mapped = append(mapped, &(*results)[0].Result[i])
		}
		return mapped, nil
	}
	return nil, nil
}

func (s *UserStore) DeleteBySubject(ctx context.Context, subject string) (int, error) {
	sql := "DELETE user_data WHERE subject = $subject RETURN BEFORE"
	vars := map[string]any{"subject": subject}

	results, err := surrealdb.Query[[]models.UserRecord](ctx, s.db, sql, vars)
	if err != nil {
		return 0, fmt.Errorf("failed to delete by subject: %w", err)
	}

	count := 0
	if results != nil && len(*results) > 0 {
		count = len((*results)[0].Result)
	}
	return count, nil
}

func (s *UserStore) DeleteBySubjects(ctx context.Context, subjects ...string) (int, error) {
	total := 0
	for _, sub := range subjects {
		count, err := s.DeleteBySubject(ctx, sub)
		if err != nil {
			return total, err
		}
		total += count
	}
	return total, nil
}

func (s *UserStore) Close() error {
	return nil
}
