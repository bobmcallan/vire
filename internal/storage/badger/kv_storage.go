package badger

import (
	"context"
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/timshannon/badgerhold/v4"
)

// KVEntry represents a key-value pair stored in BadgerDB.
type KVEntry struct {
	Key   string `badgerhold:"key"`
	Value string
}

type kvStorage struct {
	store  *Store
	logger *common.Logger
}

// NewKVStorage creates a new KeyValueStorage backed by BadgerHold.
func NewKVStorage(store *Store, logger *common.Logger) *kvStorage {
	return &kvStorage{store: store, logger: logger}
}

func (s *kvStorage) Get(_ context.Context, key string) (string, error) {
	var entry KVEntry
	err := s.store.db.Get(key, &entry)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return "", fmt.Errorf("key '%s' not found", key)
		}
		return "", fmt.Errorf("failed to get key '%s': %w", key, err)
	}
	return entry.Value, nil
}

func (s *kvStorage) Set(_ context.Context, key, value string) error {
	entry := KVEntry{Key: key, Value: value}
	if err := s.store.db.Upsert(key, &entry); err != nil {
		return fmt.Errorf("failed to set key '%s': %w", key, err)
	}
	return nil
}

func (s *kvStorage) Delete(_ context.Context, key string) error {
	err := s.store.db.Delete(key, KVEntry{})
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete key '%s': %w", key, err)
	}
	return nil
}

func (s *kvStorage) GetAll(_ context.Context) (map[string]string, error) {
	var entries []KVEntry
	if err := s.store.db.Find(&entries, nil); err != nil {
		return nil, fmt.Errorf("failed to get all keys: %w", err)
	}
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		result[entry.Key] = entry.Value
	}
	return result, nil
}
