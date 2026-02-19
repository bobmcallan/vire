package surrealdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type InternalStore struct {
	db     *surrealdb.DB
	logger *common.Logger
}

func NewInternalStore(db *surrealdb.DB, logger *common.Logger) *InternalStore {
	return &InternalStore{
		db:     db,
		logger: logger,
	}
}

func (s *InternalStore) GetUser(ctx context.Context, userID string) (*models.InternalUser, error) {
	user, err := surrealdb.Select[models.InternalUser](ctx, s.db, surrealmodels.NewRecordID("user", userID))
	if err != nil {
		return nil, fmt.Errorf("failed to select user: %w", err)
	}
	if user == nil {
		return nil, errors.New("user not found")
	}
	return user, nil
}

func (s *InternalStore) SaveUser(ctx context.Context, user *models.InternalUser) error {
	sql := "UPSERT type::record('user', $id) CONTENT $user"
	vars := map[string]any{"id": user.UserID, "user": user}

	for attempt := 1; attempt <= 3; attempt++ {
		_, err := surrealdb.Query[[]models.InternalUser](ctx, s.db, sql, vars)
		if err == nil {
			return nil
		}
		if attempt == 3 {
			return fmt.Errorf("failed to save user after retries: %w", err)
		}
	}
	return nil
}

func (s *InternalStore) DeleteUser(ctx context.Context, userID string) error {
	_, err := surrealdb.Delete[models.InternalUser](ctx, s.db, surrealmodels.NewRecordID("user", userID))
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

func (s *InternalStore) ListUsers(ctx context.Context) ([]string, error) {
	list, err := surrealdb.Select[[]models.InternalUser](ctx, s.db, surrealmodels.Table("user"))
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	var userIDs []string
	if list != nil {
		for _, u := range *list {
			if u.UserID != "" {
				userIDs = append(userIDs, u.UserID)
			}
		}
	}
	return userIDs, nil
}

// UserKeyValue ID format: user_kv:<userID>_<key>
func kvID(userID, key string) string {
	return userID + "_" + key
}

func (s *InternalStore) GetUserKV(ctx context.Context, userID, key string) (*models.UserKeyValue, error) {
	kv, err := surrealdb.Select[models.UserKeyValue](ctx, s.db, surrealmodels.NewRecordID("user_kv", kvID(userID, key)))
	if err != nil {
		return nil, fmt.Errorf("failed to select user KV: %w", err)
	}
	if kv == nil {
		return nil, errors.New("user KV not found")
	}
	return kv, nil
}

func (s *InternalStore) SetUserKV(ctx context.Context, userID, key, value string) error {
	kv := models.UserKeyValue{
		UserID: userID,
		Key:    key,
		Value:  value,
	}

	sql := "UPSERT type::record('user_kv', $id) CONTENT $kv"
	vars := map[string]any{"id": kvID(userID, key), "kv": kv}

	for attempt := 1; attempt <= 3; attempt++ {
		_, err := surrealdb.Query[[]models.UserKeyValue](ctx, s.db, sql, vars)
		if err == nil {
			return nil
		}
		if attempt == 3 {
			return fmt.Errorf("failed to set user KV after retries: %w", err)
		}
	}
	return nil
}

func (s *InternalStore) DeleteUserKV(ctx context.Context, userID, key string) error {
	_, err := surrealdb.Delete[models.UserKeyValue](ctx, s.db, surrealmodels.NewRecordID("user_kv", kvID(userID, key)))
	if err != nil {
		return fmt.Errorf("failed to delete user KV: %w", err)
	}
	return nil
}

func (s *InternalStore) ListUserKV(ctx context.Context, userID string) ([]*models.UserKeyValue, error) {
	// To list all KVs for a user, we can query by UserID
	sql := "SELECT * FROM user_kv WHERE user_id = $user_id"
	vars := map[string]any{"user_id": userID}

	results, err := surrealdb.Query[[]models.UserKeyValue](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to list user KV: %w", err)
	}

	if results != nil && len(*results) > 0 {
		var mapped []*models.UserKeyValue
		for i := range (*results)[0].Result {
			mapped = append(mapped, &(*results)[0].Result[i])
		}
		return mapped, nil
	}
	return nil, nil
}

func (s *InternalStore) GetSystemKV(ctx context.Context, key string) (string, error) {
	// System KV doesn't have a specific model, we can just use a simple struct
	type SysKV struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	kv, err := surrealdb.Select[SysKV](ctx, s.db, surrealmodels.NewRecordID("system_kv", key))
	if err != nil || kv == nil {
		return "", errors.New("system KV not found")
	}
	return kv.Value, nil
}

func (s *InternalStore) SetSystemKV(ctx context.Context, key, value string) error {
	type SysKV struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	kv := SysKV{Key: key, Value: value}

	sql := "UPSERT type::record('system_kv', $id) CONTENT $kv"
	vars := map[string]any{"id": key, "kv": kv}

	for attempt := 1; attempt <= 3; attempt++ {
		_, err := surrealdb.Query[[]SysKV](ctx, s.db, sql, vars)
		if err == nil {
			return nil
		}
		if attempt == 3 {
			return fmt.Errorf("failed to set system KV after retries: %w", err)
		}
	}
	return nil
}

func (s *InternalStore) Close() error {
	return nil
}
