package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestBlobLogger creates a logger for blob tests.
func newTestBlobLogger() *common.Logger {
	return common.NewLogger("error")
}

func TestFileBlobStore_PutGet(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	key := "test/data.json"
	data := []byte(`{"foo": "bar"}`)

	// Put
	err = store.Put(ctx, key, data)
	require.NoError(t, err)

	// Get
	got, err := store.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, data, got)

	// Verify file was created
	path := filepath.Join(tmpDir, "test", "data.json")
	assert.FileExists(t, path)
}

func TestFileBlobStore_GetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	_, err = store.Get(ctx, "nonexistent.json")
	assert.ErrorIs(t, err, ErrBlobNotFound)
}

func TestFileBlobStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	key := "delete-me.json"
	data := []byte(`test`)

	// Create
	err = store.Put(ctx, key, data)
	require.NoError(t, err)

	// Verify exists
	exists, err := store.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete
	err = store.Delete(ctx, key)
	require.NoError(t, err)

	// Verify gone
	exists, err = store.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestFileBlobStore_DeleteNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	// Should not error on delete of nonexistent key
	err = store.Delete(ctx, "nonexistent.json")
	assert.NoError(t, err)
}

func TestFileBlobStore_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	key := "exists-test.json"

	// Should not exist initially
	exists, err := store.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	// Create
	err = store.Put(ctx, key, []byte("test"))
	require.NoError(t, err)

	// Should exist now
	exists, err = store.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestFileBlobStore_Metadata(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	key := "metadata-test.json"
	data := []byte(`{"test": true}`)

	err = store.Put(ctx, key, data)
	require.NoError(t, err)

	meta, err := store.Metadata(ctx, key)
	require.NoError(t, err)

	assert.Equal(t, key, meta.Key)
	assert.Equal(t, int64(len(data)), meta.Size)
	assert.NotEmpty(t, meta.ETag)
	assert.False(t, meta.LastModified.IsZero())
}

func TestFileBlobStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create some test blobs
	testData := map[string][]byte{
		"portfolios/SMSF.json":     []byte(`{"name": "SMSF"}`),
		"portfolios/Personal.json": []byte(`{"name": "Personal"}`),
		"strategies/SMSF.json":     []byte(`{"type": "growth"}`),
		"market/BHP.AU.json":       []byte(`{"ticker": "BHP.AU"}`),
	}

	for key, data := range testData {
		err := store.Put(ctx, key, data)
		require.NoError(t, err)
	}

	// List all
	result, err := store.List(ctx, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, result.Blobs, 4)

	// List with prefix
	result, err = store.List(ctx, ListOptions{Prefix: "portfolios/"})
	require.NoError(t, err)
	assert.Len(t, result.Blobs, 2)

	// List with prefix (no results)
	result, err = store.List(ctx, ListOptions{Prefix: "nonexistent/"})
	require.NoError(t, err)
	assert.Len(t, result.Blobs, 0)
}

func TestFileBlobStore_ListWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Create test blobs
	for i := 0; i < 5; i++ {
		key := filepath.Join("test", "file"+string(rune('0'+i))+".json")
		err := store.Put(ctx, key, []byte(`{}`))
		require.NoError(t, err)
	}

	// List with limit
	result, err := store.List(ctx, ListOptions{MaxKeys: 2})
	require.NoError(t, err)
	assert.Len(t, result.Blobs, 2)
	assert.True(t, result.Truncated)
}

func TestFileBlobStore_SanitizeKey(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	// Test path traversal protection
	tests := []struct {
		input    string
		expected string
	}{
		{"normal/key.json", "normal/key.json"},
		{"../escape.json", "escape.json"},
		{"foo/../bar.json", "foo/bar.json"},
		{"foo/../../bar.json", "bar.json"},
		{"/absolute/path.json", "absolute/path.json"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := store.sanitizeKey(tc.input)
			// The result should not allow escaping the base path
			assert.NotContains(t, result, "..")
		})
	}
}

func TestFileBlobStore_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	store, err := NewFileBlobStore(logger, &FileBlobConfig{BasePath: tmpDir})
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	key := "atomic-test.json"

	// Write initial data
	err = store.Put(ctx, key, []byte(`{"version": 1}`))
	require.NoError(t, err)

	// Overwrite with new data
	err = store.Put(ctx, key, []byte(`{"version": 2}`))
	require.NoError(t, err)

	// Verify final content
	data, err := store.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, `{"version": 2}`, string(data))

	// Verify no temp files left behind
	entries, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, filepath.HasPrefix(e.Name(), ".tmp-"))
	}
}

func TestNewBlobStore_FileBackend(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	config := &BlobStoreConfig{
		Backend: "file",
		File:    FileBlobConfig{BasePath: tmpDir},
	}

	store, err := NewBlobStore(logger, config)
	require.NoError(t, err)
	defer store.Close()

	// Verify it works
	ctx := context.Background()
	err = store.Put(ctx, "test.json", []byte(`{"ok": true}`))
	require.NoError(t, err)

	data, err := store.Get(ctx, "test.json")
	require.NoError(t, err)
	assert.Equal(t, `{"ok": true}`, string(data))
}

func TestNewBlobStore_DefaultBackend(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestBlobLogger()

	// Empty backend should default to "file"
	config := &BlobStoreConfig{
		Backend: "",
		File:    FileBlobConfig{BasePath: tmpDir},
	}

	store, err := NewBlobStore(logger, config)
	require.NoError(t, err)
	defer store.Close()

	// Should work just like file backend
	ctx := context.Background()
	err = store.Put(ctx, "default.json", []byte(`test`))
	require.NoError(t, err)
}

func TestNewBlobStore_UnsupportedBackend(t *testing.T) {
	logger := newTestBlobLogger()

	config := &BlobStoreConfig{
		Backend: "mongodb",
	}

	_, err := NewBlobStore(logger, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown storage backend")
}

func TestNewBlobStore_GCSNotImplemented(t *testing.T) {
	logger := newTestBlobLogger()

	config := &BlobStoreConfig{
		Backend: "gcs",
		GCS:     GCSBlobConfig{Bucket: "test-bucket"},
	}

	_, err := NewBlobStore(logger, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestNewBlobStore_S3NotImplemented(t *testing.T) {
	logger := newTestBlobLogger()

	config := &BlobStoreConfig{
		Backend: "s3",
		S3:      S3BlobConfig{Bucket: "test-bucket", Region: "us-east-1"},
	}

	_, err := NewBlobStore(logger, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}
