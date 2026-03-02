package blob

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// FileSystemStore implements interfaces.FileStore using local filesystem.
type FileSystemStore struct {
	basePath string
	logger   *common.Logger
}

// fileMeta is the sidecar metadata stored alongside each blob file.
type fileMeta struct {
	ContentType string `json:"content_type"`
}

// Compile-time check
var _ interfaces.FileStore = (*FileSystemStore)(nil)

// NewFileSystemStore creates a FileSystemStore rooted at basePath.
// The directory is created if it does not exist.
func NewFileSystemStore(basePath string, logger *common.Logger) (*FileSystemStore, error) {
	if basePath == "" {
		basePath = "data/blobs"
	}
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create blob base path %s: %w", basePath, err)
	}
	// Resolve symlinks at construction time so blobPath comparisons are reliable
	resolved, err := filepath.EvalSymlinks(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve blob base path %s: %w", basePath, err)
	}
	return &FileSystemStore{basePath: resolved, logger: logger}, nil
}

// blobPath returns the safe filesystem path for a category/key pair.
// Returns error if the key or category attempts path traversal.
func (s *FileSystemStore) blobPath(category, key string) (string, error) {
	if strings.Contains(category, "..") || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid path: directory traversal not allowed")
	}
	p := filepath.Join(s.basePath, category, key)
	// Verify resolved path is within basePath.
	// basePath was resolved via EvalSymlinks at construction time.
	// For the target path, try to resolve symlinks on the parent directory
	// (the file itself may not exist yet for SaveFile).
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	parentDir := filepath.Dir(abs)
	if resolvedParent, err := filepath.EvalSymlinks(parentDir); err == nil {
		abs = filepath.Join(resolvedParent, filepath.Base(abs))
	}
	if !strings.HasPrefix(abs, s.basePath+string(os.PathSeparator)) && abs != s.basePath {
		return "", fmt.Errorf("invalid path: resolved outside base directory")
	}
	return p, nil
}

func (s *FileSystemStore) SaveFile(ctx context.Context, category, key string, data []byte, contentType string) error {
	p, err := s.blobPath(category, key)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s/%s: %w", category, key, err)
	}

	// Write data file atomically: write to .tmp then rename
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("failed to write blob %s/%s: %w", category, key, err)
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to persist blob %s/%s: %w", category, key, err)
	}

	// Write sidecar metadata
	meta := fileMeta{ContentType: contentType}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata for %s/%s: %w", category, key, err)
	}
	metaTmp := p + ".meta.tmp"
	if err := os.WriteFile(metaTmp, metaBytes, 0644); err != nil {
		return fmt.Errorf("failed to write metadata for %s/%s: %w", category, key, err)
	}
	if err := os.Rename(metaTmp, p+".meta"); err != nil {
		os.Remove(metaTmp)
		return fmt.Errorf("failed to persist metadata for %s/%s: %w", category, key, err)
	}

	return nil
}

func (s *FileSystemStore) GetFile(ctx context.Context, category, key string) ([]byte, string, error) {
	p, err := s.blobPath(category, key)
	if err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("file not found: %s/%s", category, key)
		}
		return nil, "", fmt.Errorf("failed to read blob %s/%s: %w", category, key, err)
	}

	// Read sidecar metadata for content type
	contentType := "application/octet-stream"
	metaBytes, err := os.ReadFile(p + ".meta")
	if err == nil {
		var meta fileMeta
		if json.Unmarshal(metaBytes, &meta) == nil && meta.ContentType != "" {
			contentType = meta.ContentType
		}
	}

	return data, contentType, nil
}

func (s *FileSystemStore) DeleteFile(ctx context.Context, category, key string) error {
	p, err := s.blobPath(category, key)
	if err != nil {
		return err
	}

	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete blob %s/%s: %w", category, key, err)
	}
	// Remove sidecar — ignore not-found
	os.Remove(p + ".meta")

	return nil
}

func (s *FileSystemStore) HasFile(ctx context.Context, category, key string) (bool, error) {
	p, err := s.blobPath(category, key)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(p)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("failed to stat blob %s/%s: %w", category, key, err)
}
