package surrealdb

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// FileStore implements interfaces.FileStore using SurrealDB.
type FileStore struct {
	db     *surrealdb.DB
	logger *common.Logger
}

// fileRecord is the SurrealDB record shape for the files table.
type fileRecord struct {
	Category    string    `json:"category"`
	Key         string    `json:"key"`
	ContentType string    `json:"content_type"`
	Size        int       `json:"size"`
	Data        string    `json:"data"` // base64-encoded
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NewFileStore creates a new FileStore.
func NewFileStore(db *surrealdb.DB, logger *common.Logger) *FileStore {
	return &FileStore{db: db, logger: logger}
}

// fileRecordID builds a SurrealDB record ID from category and key.
// Sanitizes dots and slashes to underscores for safe record IDs.
func fileRecordID(category, key string) string {
	sanitized := strings.NewReplacer(".", "_", "/", "_").Replace(category + "_" + key)
	return sanitized
}

// maxCBORDocBytes is the maximum encoded document size for SurrealDB's CBOR wire format.
// Documents exceeding this limit cause opaque CBOR errors at the driver level.
const maxCBORDocBytes = 10_000_000

func (s *FileStore) SaveFile(ctx context.Context, category, key string, data []byte, contentType string) error {
	// Base64 encoding expands data by ~33%. Reject early if the encoded size
	// would exceed SurrealDB's CBOR 10MB document limit.
	encodedSize := base64.StdEncoding.EncodedLen(len(data))
	if encodedSize > maxCBORDocBytes {
		return fmt.Errorf("file %s/%s too large for storage: %d bytes encoded (limit %d)", category, key, encodedSize, maxCBORDocBytes)
	}

	now := time.Now()
	encoded := base64.StdEncoding.EncodeToString(data)

	sql := `UPSERT $rid SET
		category = $category, key = $key, content_type = $content_type,
		size = $size, data = $data, created_at = $created_at, updated_at = $updated_at`
	vars := map[string]any{
		"rid":          surrealmodels.NewRecordID("files", fileRecordID(category, key)),
		"category":     category,
		"key":          key,
		"content_type": contentType,
		"size":         len(data),
		"data":         encoded,
		"created_at":   now,
		"updated_at":   now,
	}

	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to save file %s/%s: %w", category, key, err)
	}
	return nil
}

func (s *FileStore) GetFile(ctx context.Context, category, key string) ([]byte, string, error) {
	rid := surrealmodels.NewRecordID("files", fileRecordID(category, key))
	record, err := surrealdb.Select[fileRecord](ctx, s.db, rid)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get file %s/%s: %w", category, key, err)
	}
	if record == nil {
		return nil, "", fmt.Errorf("file not found: %s/%s", category, key)
	}

	data, err := base64.StdEncoding.DecodeString(record.Data)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode file data: %w", err)
	}

	return data, record.ContentType, nil
}

func (s *FileStore) DeleteFile(ctx context.Context, category, key string) error {
	rid := surrealmodels.NewRecordID("files", fileRecordID(category, key))
	if _, err := surrealdb.Delete[fileRecord](ctx, s.db, rid); err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to delete file %s/%s: %w", category, key, err)
	}
	return nil
}

func (s *FileStore) HasFile(ctx context.Context, category, key string) (bool, error) {
	rid := surrealmodels.NewRecordID("files", fileRecordID(category, key))
	record, err := surrealdb.Select[fileRecord](ctx, s.db, rid)
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file %s/%s: %w", category, key, err)
	}
	return record != nil, nil
}

// Compile-time check
var _ interfaces.FileStore = (*FileStore)(nil)
