// Package badger provides BadgerHold-based storage implementations for user-domain data.
package badger

import (
	"fmt"
	"os"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/timshannon/badgerhold/v4"
)

// Store wraps a BadgerHold database connection.
type Store struct {
	db     *badgerhold.Store
	logger *common.Logger
}

// NewStore creates a new BadgerHold store at the given directory path.
func NewStore(logger *common.Logger, path string) (*Store, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create badger directory %s: %w", path, err)
	}

	options := badgerhold.DefaultOptions
	options.Dir = path
	options.ValueDir = path
	options.Logger = nil // Disable default badger logger

	db, err := badgerhold.Open(options)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger database: %w", err)
	}

	logger.Debug().Str("path", path).Msg("BadgerHold store opened")

	return &Store{
		db:     db,
		logger: logger,
	}, nil
}

// DB returns the underlying badgerhold store.
func (s *Store) DB() *badgerhold.Store {
	return s.db
}

// Close closes the BadgerHold database.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
