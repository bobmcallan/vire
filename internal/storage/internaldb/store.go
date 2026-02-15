// Package internaldb implements InternalStore using BadgerHold.
// It manages user accounts, per-user key-value config, and system-level KV.
package internaldb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

// Store implements interfaces.InternalStore using BadgerHold.
type Store struct {
	db     *badgerhold.Store
	logger *common.Logger
}

// systemUserID is the sentinel UserID for system-level key-value pairs.
// Uses a prefix that cannot be a valid user ID to prevent namespace collision.
const systemUserID = "__system__"

// kvSep is the composite key separator for UserKeyValue records.
// Using a null byte prevents collisions when userID or key contain ":"
// (e.g., userID="a:b" key="c" vs userID="a" key="b:c" would both produce
// "a:b:c" with a colon separator, but produce distinct keys with \x00).
const kvSep = "\x00"

// NewStore creates a new InternalStore backed by BadgerHold.
func NewStore(logger *common.Logger, path string) (*Store, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create internal db path %s: %w", path, err)
	}
	opts := badgerhold.DefaultOptions
	opts.Dir = path
	opts.ValueDir = path
	opts.Logger = nil
	db, err := badgerhold.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open internal db at %s: %w", path, err)
	}
	logger.Info().Str("path", path).Msg("InternalDB opened")
	return &Store{db: db, logger: logger}, nil
}

// --- User accounts ---

func (s *Store) GetUser(_ context.Context, userID string) (*models.InternalUser, error) {
	var user models.InternalUser
	if err := s.db.Get(userID, &user); err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("user '%s' not found", userID)
		}
		return nil, fmt.Errorf("failed to get user '%s': %w", userID, err)
	}
	return &user, nil
}

func (s *Store) SaveUser(_ context.Context, user *models.InternalUser) error {
	if user.UserID == systemUserID {
		return fmt.Errorf("user ID '%s' is reserved for system use", systemUserID)
	}
	now := time.Now()
	var existing models.InternalUser
	if err := s.db.Get(user.UserID, &existing); err == nil {
		user.CreatedAt = existing.CreatedAt
	} else if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.ModifiedAt = now

	if err := s.db.Upsert(user.UserID, user); err != nil {
		return fmt.Errorf("failed to save user '%s': %w", user.UserID, err)
	}
	s.logger.Debug().Str("user_id", user.UserID).Msg("User saved")
	return nil
}

func (s *Store) DeleteUser(_ context.Context, userID string) error {
	if err := s.db.Delete(userID, models.InternalUser{}); err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete user '%s': %w", userID, err)
	}
	// Delete all UserKV entries for this user
	var kvs []models.UserKeyValue
	if err := s.db.Find(&kvs, badgerhold.Where("UserID").Eq(userID)); err == nil {
		for _, kv := range kvs {
			compositeKey := kv.UserID + kvSep + kv.Key
			_ = s.db.Delete(compositeKey, models.UserKeyValue{})
		}
	}
	s.logger.Debug().Str("user_id", userID).Msg("User and KV entries deleted")
	return nil
}

func (s *Store) ListUsers(_ context.Context) ([]string, error) {
	var users []models.InternalUser
	if err := s.db.Find(&users, nil); err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	ids := make([]string, len(users))
	for i, u := range users {
		ids[i] = u.UserID
	}
	return ids, nil
}

// --- Per-user key-value config ---

func (s *Store) GetUserKV(_ context.Context, userID, key string) (*models.UserKeyValue, error) {
	compositeKey := userID + kvSep + key
	var kv models.UserKeyValue
	if err := s.db.Get(compositeKey, &kv); err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("key '%s' not found for user '%s'", key, userID)
		}
		return nil, fmt.Errorf("failed to get kv '%s' for user '%s': %w", key, userID, err)
	}
	return &kv, nil
}

func (s *Store) SetUserKV(_ context.Context, userID, key, value string) error {
	compositeKey := userID + kvSep + key
	now := time.Now()

	var existing models.UserKeyValue
	version := 1
	if err := s.db.Get(compositeKey, &existing); err == nil {
		version = existing.Version + 1
	}

	kv := &models.UserKeyValue{
		UserID:   userID,
		Key:      key,
		Value:    value,
		Version:  version,
		DateTime: now,
	}
	if err := s.db.Upsert(compositeKey, kv); err != nil {
		return fmt.Errorf("failed to set kv '%s' for user '%s': %w", key, userID, err)
	}
	return nil
}

func (s *Store) DeleteUserKV(_ context.Context, userID, key string) error {
	compositeKey := userID + kvSep + key
	if err := s.db.Delete(compositeKey, models.UserKeyValue{}); err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete kv '%s' for user '%s': %w", key, userID, err)
	}
	return nil
}

func (s *Store) ListUserKV(_ context.Context, userID string) ([]*models.UserKeyValue, error) {
	var all []models.UserKeyValue
	if err := s.db.Find(&all, nil); err != nil {
		return nil, fmt.Errorf("failed to list kv for user '%s': %w", userID, err)
	}
	var result []*models.UserKeyValue
	for i := range all {
		if all[i].UserID == userID {
			kv := all[i]
			result = append(result, &kv)
		}
	}
	return result, nil
}

// --- System key-value ---

func (s *Store) GetSystemKV(ctx context.Context, key string) (string, error) {
	kv, err := s.GetUserKV(ctx, systemUserID, key)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return "", nil
		}
		return "", err
	}
	return kv.Value, nil
}

func (s *Store) SetSystemKV(ctx context.Context, key, value string) error {
	return s.SetUserKV(ctx, systemUserID, key, value)
}

// Close shuts down the BadgerHold database.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
