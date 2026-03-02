package storage

import (
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/storage/blob"
	"github.com/bobmcallan/vire/internal/storage/surrealdb"
)

// NewManager creates a new StorageManager connected to SurrealDB.
func NewManager(logger *common.Logger, config *common.Config) (interfaces.StorageManager, error) {
	// Create blob-backed FileStore from config
	fileStore, err := blob.NewFileStore(config.Storage.Blob, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create blob store: %w", err)
	}

	manager, err := surrealdb.NewManager(logger, config, fileStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create surrealdb storage manager: %w", err)
	}

	return manager, nil
}
