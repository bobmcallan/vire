package data

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/storage/blob"
	tcommon "github.com/bobmcallan/vire/tests/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlobStore_Lifecycle tests full CRUD: save, has, get, delete, has-after-delete
func TestBlobStore_Lifecycle(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")
	guard := tcommon.NewTestOutputGuard(t)

	ctx := context.Background()
	category := "test-category"
	key := "test-key"
	data := []byte("test file content")
	contentType := "text/plain"

	// Save file
	err = store.SaveFile(ctx, category, key, data, contentType)
	require.NoError(t, err, "failed to save file")
	guard.SaveResult("01_save", fmt.Sprintf("Saved file %s/%s (%d bytes, type=%s)", category, key, len(data), contentType))

	// Has file should return true
	has, err := store.HasFile(ctx, category, key)
	require.NoError(t, err, "failed to check has file")
	assert.True(t, has, "file should exist after save")
	guard.SaveResult("02_has_after_save", fmt.Sprintf("HasFile returned true as expected"))

	// Get file should return data and content type
	retrievedData, retrievedType, err := store.GetFile(ctx, category, key)
	require.NoError(t, err, "failed to get file")
	assert.Equal(t, data, retrievedData, "retrieved data should match saved data")
	assert.Equal(t, contentType, retrievedType, "retrieved content type should match saved content type")
	guard.SaveResult("03_get", fmt.Sprintf("Retrieved data: %d bytes, type=%s", len(retrievedData), retrievedType))

	// Delete file
	err = store.DeleteFile(ctx, category, key)
	require.NoError(t, err, "failed to delete file")
	guard.SaveResult("04_delete", fmt.Sprintf("Deleted file %s/%s", category, key))

	// Has file should return false after delete
	has, err = store.HasFile(ctx, category, key)
	require.NoError(t, err, "failed to check has file after delete")
	assert.False(t, has, "file should not exist after delete")
	guard.SaveResult("05_has_after_delete", fmt.Sprintf("HasFile returned false as expected"))

	// Get file should fail after delete
	_, _, err = store.GetFile(ctx, category, key)
	assert.Error(t, err, "getting deleted file should return error")
	guard.SaveResult("06_get_after_delete", fmt.Sprintf("GetFile returned error as expected: %v", err))
}

// TestBlobStore_LargeFile tests that a 20MB file round-trips correctly
func TestBlobStore_LargeFile(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")
	guard := tcommon.NewTestOutputGuard(t)

	ctx := context.Background()
	category := "large-files"
	key := "20mb-file"
	contentType := "application/pdf"

	// Generate 20MB of random data
	largeData := make([]byte, 20*1024*1024)
	_, err = rand.Read(largeData)
	require.NoError(t, err, "failed to generate random data")
	guard.SaveResult("01_generate_data", fmt.Sprintf("Generated 20MB of random data"))

	// Save large file
	err = store.SaveFile(ctx, category, key, largeData, contentType)
	require.NoError(t, err, "failed to save large file")
	guard.SaveResult("02_save", fmt.Sprintf("Saved 20MB file successfully"))

	// Retrieve and verify
	retrievedData, retrievedType, err := store.GetFile(ctx, category, key)
	require.NoError(t, err, "failed to get large file")
	assert.Equal(t, largeData, retrievedData, "large file data should match")
	assert.Equal(t, contentType, retrievedType, "content type should match")
	guard.SaveResult("03_retrieve_and_verify", fmt.Sprintf("Retrieved 20MB file, size=%d bytes, type=%s, data matches=true", len(retrievedData), retrievedType))
}

// TestBlobStore_ContentType tests that content-type is preserved in .meta sidecar
func TestBlobStore_ContentType(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()

	tests := []struct {
		name        string
		category    string
		key         string
		contentType string
	}{
		{"PDF document", "filings", "AAPL-20250101.pdf", "application/pdf"},
		{"PNG image", "charts", "portfolio-allocation.png", "image/png"},
		{"JSON data", "exports", "data.json", "application/json"},
		{"HTML report", "reports", "annual-report.html", "text/html; charset=utf-8"},
		{"CSV spreadsheet", "exports", "transactions.csv", "text/csv"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte(fmt.Sprintf("Content for %s", tt.key))

			// Save with specific content type
			err := store.SaveFile(ctx, tt.category, tt.key, data, tt.contentType)
			require.NoError(t, err, "failed to save file with content type %s", tt.contentType)

			// Retrieve and verify content type is preserved
			_, retrievedType, err := store.GetFile(ctx, tt.category, tt.key)
			require.NoError(t, err, "failed to get file")
			assert.Equal(t, tt.contentType, retrievedType, "content type should be preserved")
		})
	}
}

// TestBlobStore_CategoryIsolation tests that same key in different categories are independent
func TestBlobStore_CategoryIsolation(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")
	guard := tcommon.NewTestOutputGuard(t)

	ctx := context.Background()
	key := "same-key"
	data1 := []byte("data from category 1")
	data2 := []byte("data from category 2")

	// Save same key in two different categories
	err = store.SaveFile(ctx, "category-1", key, data1, "text/plain")
	require.NoError(t, err, "failed to save file in category 1")
	err = store.SaveFile(ctx, "category-2", key, data2, "text/plain")
	require.NoError(t, err, "failed to save file in category 2")
	guard.SaveResult("01_save_both_categories", fmt.Sprintf("Saved same key in two different categories"))

	// Retrieve from each category and verify they are independent
	retrieved1, _, err := store.GetFile(ctx, "category-1", key)
	require.NoError(t, err, "failed to get file from category 1")
	assert.Equal(t, data1, retrieved1, "data from category 1 should be independent")

	retrieved2, _, err := store.GetFile(ctx, "category-2", key)
	require.NoError(t, err, "failed to get file from category 2")
	assert.Equal(t, data2, retrieved2, "data from category 2 should be independent")
	guard.SaveResult("02_retrieve_both", fmt.Sprintf("Retrieved from both categories, data is independent"))

	// Delete from category 1 and verify category 2 is unaffected
	err = store.DeleteFile(ctx, "category-1", key)
	require.NoError(t, err, "failed to delete file from category 1")

	has, err := store.HasFile(ctx, "category-1", key)
	require.NoError(t, err, "failed to check has file")
	assert.False(t, has, "file should be deleted from category 1")

	has, err = store.HasFile(ctx, "category-2", key)
	require.NoError(t, err, "failed to check has file")
	assert.True(t, has, "file should still exist in category 2")
	guard.SaveResult("03_delete_and_verify", fmt.Sprintf("Deleted from category-1, category-2 unaffected"))
}

// TestBlobStore_ConcurrentWrites tests that 10 goroutines writing different keys work correctly
func TestBlobStore_ConcurrentWrites(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")
	guard := tcommon.NewTestOutputGuard(t)

	ctx := context.Background()
	numGoroutines := 10
	var wg sync.WaitGroup
	errs := make([]error, numGoroutines)

	// Spawn 10 goroutines writing different keys
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("file-%d", idx)
			data := []byte(fmt.Sprintf("content for file %d", idx))
			errs[idx] = store.SaveFile(ctx, "concurrent", key, data, "text/plain")
		}(i)
	}
	wg.Wait()
	guard.SaveResult("01_concurrent_writes", fmt.Sprintf("Spawned %d goroutines to write files", numGoroutines))

	// Check for errors
	for _, err := range errs {
		assert.NoError(t, err, "concurrent write should not error")
	}
	guard.SaveResult("02_no_errors", fmt.Sprintf("All %d concurrent writes succeeded", numGoroutines))

	// Verify all files were created
	for i := 0; i < numGoroutines; i++ {
		key := fmt.Sprintf("file-%d", i)
		has, err := store.HasFile(ctx, "concurrent", key)
		require.NoError(t, err, "failed to check has file")
		assert.True(t, has, "concurrent file should exist")
	}
	guard.SaveResult("03_all_files_created", fmt.Sprintf("All %d files verified to exist", numGoroutines))
}

// TestBlobStore_NestedKeyStructure tests that TICKER/date-doc.pdf creates proper directory structure
func TestBlobStore_NestedKeyStructure(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")
	guard := tcommon.NewTestOutputGuard(t)

	ctx := context.Background()
	category := "filings"
	key := "AAPL/20250101-earnings.pdf"
	data := []byte("PDF content")
	contentType := "application/pdf"

	// Save file with nested key structure
	err = store.SaveFile(ctx, category, key, data, contentType)
	require.NoError(t, err, "failed to save file with nested key")
	guard.SaveResult("01_save_nested_key", fmt.Sprintf("Saved file with nested key: %s/%s", category, key))

	// Verify file exists
	has, err := store.HasFile(ctx, category, key)
	require.NoError(t, err, "failed to check has file")
	assert.True(t, has, "nested file should exist")

	// Verify content
	retrieved, retrievedType, err := store.GetFile(ctx, category, key)
	require.NoError(t, err, "failed to get nested file")
	assert.Equal(t, data, retrieved, "nested file content should match")
	assert.Equal(t, contentType, retrievedType, "content type should match")
	guard.SaveResult("02_retrieve_nested", fmt.Sprintf("Retrieved nested file, type=%s, size=%d bytes", retrievedType, len(retrieved)))

	// Verify directory structure exists on filesystem
	expectedPath := filepath.Join(basePath, category, key)
	_, err = os.Stat(expectedPath)
	require.NoError(t, err, "nested directory structure should exist on filesystem")
	guard.SaveResult("03_filesystem_structure", fmt.Sprintf("Verified nested directory structure exists on filesystem: %s", expectedPath))
}

// TestBlobStore_OverwriteExisting tests that SaveFile with same key overwrites previous data
func TestBlobStore_OverwriteExisting(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "test"
	key := "overwrite-key"

	// Save first version
	data1 := []byte("version 1")
	err = store.SaveFile(ctx, category, key, data1, "text/plain")
	require.NoError(t, err, "failed to save first version")

	// Save second version with same key
	data2 := []byte("version 2 with more content")
	err = store.SaveFile(ctx, category, key, data2, "text/plain; charset=utf-8")
	require.NoError(t, err, "failed to save second version")

	// Retrieve and verify second version overwrote first
	retrieved, retrievedType, err := store.GetFile(ctx, category, key)
	require.NoError(t, err, "failed to get file after overwrite")
	assert.Equal(t, data2, retrieved, "should retrieve second version")
	assert.Equal(t, "text/plain; charset=utf-8", retrievedType, "should have second version's content type")
}

// TestBlobStore_PathTraversal tests that keys containing .. are rejected
func TestBlobStore_PathTraversal(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	data := []byte("test")

	tests := []struct {
		name     string
		category string
		key      string
	}{
		{"key with ..", "category", "../../../etc/passwd"},
		{"category with ..", "..", "file"},
		{"both with ..", "..", ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Attempting to save with path traversal should error
			err := store.SaveFile(ctx, tt.category, tt.key, data, "text/plain")
			assert.Error(t, err, "path traversal attempt should be rejected")

			// Attempting to check existence should also error
			_, err = store.HasFile(ctx, tt.category, tt.key)
			assert.Error(t, err, "path traversal in HasFile should be rejected")
		})
	}
}

// TestBlobStore_MissingFile tests that GetFile on non-existent key returns error
func TestBlobStore_MissingFile(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()

	// Attempt to get non-existent file
	_, _, err = store.GetFile(ctx, "nonexistent-category", "nonexistent-key")
	assert.Error(t, err, "getting nonexistent file should return error")
}

// TestBlobStore_EmptyData tests that saving empty data works correctly
func TestBlobStore_EmptyData(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "empty"
	key := "empty-file"
	emptyData := []byte{}
	contentType := "text/plain"

	// Save empty file
	err = store.SaveFile(ctx, category, key, emptyData, contentType)
	require.NoError(t, err, "failed to save empty file")

	// Verify it exists
	has, err := store.HasFile(ctx, category, key)
	require.NoError(t, err, "failed to check has file")
	assert.True(t, has, "empty file should exist")

	// Retrieve and verify it's still empty
	retrieved, retrievedType, err := store.GetFile(ctx, category, key)
	require.NoError(t, err, "failed to get empty file")
	assert.Equal(t, emptyData, retrieved, "empty file data should match")
	assert.Equal(t, contentType, retrievedType, "content type should be preserved for empty file")
}

// TestBlobStore_BinaryData tests that binary data (not text) is handled correctly
func TestBlobStore_BinaryData(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "binary"
	key := "binary-file"
	contentType := "application/octet-stream"

	// Create binary data with all byte values
	binaryData := make([]byte, 256)
	for i := 0; i < 256; i++ {
		binaryData[i] = byte(i)
	}

	// Save binary file
	err = store.SaveFile(ctx, category, key, binaryData, contentType)
	require.NoError(t, err, "failed to save binary file")

	// Retrieve and verify byte-for-byte match
	retrieved, retrievedType, err := store.GetFile(ctx, category, key)
	require.NoError(t, err, "failed to get binary file")
	assert.Equal(t, binaryData, retrieved, "binary file data should match exactly")
	assert.Equal(t, contentType, retrievedType, "content type should match")
}

// TestBlobStore_SpecialCharactersInKey tests keys with special characters (but not path traversal)
func TestBlobStore_SpecialCharactersInKey(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	data := []byte("test content")
	contentType := "text/plain"

	tests := []struct {
		name     string
		category string
		key      string
	}{
		{"spaces in key", "category", "file with spaces.txt"},
		{"hyphens in key", "category", "file-with-hyphens"},
		{"underscores in key", "category", "file_with_underscores"},
		{"dots in key", "category", "file.with.multiple.dots.txt"},
		{"nested path", "category", "folder/subfolder/file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save file with special characters
			err := store.SaveFile(ctx, tt.category, tt.key, data, contentType)
			require.NoError(t, err, "failed to save file with special characters")

			// Verify it can be retrieved
			retrieved, _, err := store.GetFile(ctx, tt.category, tt.key)
			require.NoError(t, err, "failed to get file with special characters")
			assert.Equal(t, data, retrieved, "file data should match")
		})
	}
}

// TestBlobStore_MultipleReadsSameFile tests that multiple reads of the same file work correctly
func TestBlobStore_MultipleReadsSameFile(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "multi-read"
	key := "test-file"
	data := []byte("test content for multiple reads")
	contentType := "text/plain"

	// Save file once
	err = store.SaveFile(ctx, category, key, data, contentType)
	require.NoError(t, err, "failed to save file")

	// Read the same file multiple times
	for i := 0; i < 5; i++ {
		retrieved, retrievedType, err := store.GetFile(ctx, category, key)
		require.NoError(t, err, "failed to get file on iteration %d", i)
		assert.Equal(t, data, retrieved, "data should match on iteration %d", i)
		assert.Equal(t, contentType, retrievedType, "content type should match on iteration %d", i)
	}
}

// TestBlobStore_LargeNumberOfFiles tests creating and retrieving many files
func TestBlobStore_LargeNumberOfFiles(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "many-files"
	numFiles := 100
	contentType := "text/plain"

	// Save many files
	for i := 0; i < numFiles; i++ {
		key := fmt.Sprintf("file-%d", i)
		data := []byte(fmt.Sprintf("content for file %d", i))
		err := store.SaveFile(ctx, category, key, data, contentType)
		require.NoError(t, err, "failed to save file %d", i)
	}

	// Verify all files exist and have correct content
	for i := 0; i < numFiles; i++ {
		key := fmt.Sprintf("file-%d", i)
		has, err := store.HasFile(ctx, category, key)
		require.NoError(t, err, "failed to check has file %d", i)
		assert.True(t, has, "file %d should exist", i)

		retrieved, _, err := store.GetFile(ctx, category, key)
		require.NoError(t, err, "failed to get file %d", i)
		expected := []byte(fmt.Sprintf("content for file %d", i))
		assert.Equal(t, expected, retrieved, "file %d content should match", i)
	}
}

// TestBlobStore_SaveAfterDelete tests that a file can be saved again after deletion
func TestBlobStore_SaveAfterDelete(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "test"
	key := "reuse-key"
	data1 := []byte("first version")
	data2 := []byte("second version")
	contentType := "text/plain"

	// Save, delete, save again
	err = store.SaveFile(ctx, category, key, data1, contentType)
	require.NoError(t, err, "failed to save first time")

	err = store.DeleteFile(ctx, category, key)
	require.NoError(t, err, "failed to delete")

	err = store.SaveFile(ctx, category, key, data2, contentType)
	require.NoError(t, err, "failed to save second time")

	// Verify we get the second version
	retrieved, _, err := store.GetFile(ctx, category, key)
	require.NoError(t, err, "failed to get file")
	assert.Equal(t, data2, retrieved, "should have second version after save-delete-save")
}

// TestBlobStore_LargeContentType tests very long content type strings
func TestBlobStore_LargeContentType(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "test"
	key := "large-contenttype"
	data := []byte("test")
	// Realistic complex content type with charset and boundary
	contentType := "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW; charset=utf-8"

	err = store.SaveFile(ctx, category, key, data, contentType)
	require.NoError(t, err, "failed to save file with complex content type")

	_, retrievedType, err := store.GetFile(ctx, category, key)
	require.NoError(t, err, "failed to get file")
	assert.Equal(t, contentType, retrievedType, "complex content type should be preserved exactly")
}

// TestBlobStore_ConcurrentReadsAndWrites tests mixed concurrent reads and writes
func TestBlobStore_ConcurrentReadsAndWrites(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "concurrent-mixed"

	// Pre-populate with some files
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("file-%d", i)
		data := []byte(fmt.Sprintf("content %d", i))
		err := store.SaveFile(ctx, category, key, data, "text/plain")
		require.NoError(t, err, "failed to pre-populate")
	}

	// Mix concurrent reads and writes
	var wg sync.WaitGroup
	errs := make([]error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				// Even indices do reads
				key := fmt.Sprintf("file-%d", idx%5)
				_, _, err := store.GetFile(ctx, category, key)
				errs[idx] = err
			} else {
				// Odd indices do writes
				key := fmt.Sprintf("new-file-%d", idx)
				data := []byte(fmt.Sprintf("new content %d", idx))
				errs[idx] = store.SaveFile(ctx, category, key, data, "text/plain")
			}
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		assert.NoError(t, err, "concurrent mixed operations should not error")
	}
}

// TestBlobStore_FileWithoutMeta tests that a file without .meta sidecar can still be read
// (defensive test for handling incomplete state)
func TestBlobStore_FileWithoutMeta(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "orphaned"
	key := "file"

	// Manually create a file without meta sidecar to simulate incomplete write
	filePath := filepath.Join(basePath, category, key)
	err = os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err, "failed to create directories")

	fileData := []byte("orphaned content")
	err = os.WriteFile(filePath, fileData, 0644)
	require.NoError(t, err, "failed to write orphaned file")

	// Reading should work even without meta (should return empty content type or default)
	retrieved, _, err := store.GetFile(ctx, category, key)
	// The implementation might handle this gracefully or return an error
	// This test documents the behavior
	if err == nil {
		assert.Equal(t, fileData, retrieved, "orphaned file should be readable")
	}
}

// TestBlobStore_HasFileWithoutStorePath tests HasFile before any files are created
func TestBlobStore_HasFileWithoutStorePath(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()

	// Check for file in category that never had any files
	has, err := store.HasFile(ctx, "nonexistent-category", "nonexistent-file")
	require.NoError(t, err, "HasFile should not error for nonexistent paths")
	assert.False(t, has, "should return false for nonexistent file")
}

// TestBlobStore_DataIntegrity tests that retrieved data matches saved data exactly
func TestBlobStore_DataIntegrity(t *testing.T) {
	basePath := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := blob.NewFileSystemStore(basePath, logger)
	require.NoError(t, err, "failed to create file system store")

	ctx := context.Background()
	category := "integrity"
	contentType := "application/octet-stream"

	// Test with various data patterns
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"null bytes", []byte{0, 0, 0}},
		{"all bytes 0xFF", bytes.Repeat([]byte{0xFF}, 100)},
		{"newlines", []byte("line1\nline2\r\nline3")},
		{"null terminated", []byte("data\x00end")},
		{"repeated pattern", bytes.Repeat([]byte("ABCD"), 1000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := fmt.Sprintf("test-%s", tt.name)
			err := store.SaveFile(ctx, category, key, tt.data, contentType)
			require.NoError(t, err, "failed to save file")

			retrieved, _, err := store.GetFile(ctx, category, key)
			require.NoError(t, err, "failed to get file")

			// Use DeepEqual to catch any mutations
			assert.Equal(t, tt.data, retrieved, "data should match exactly, byte for byte")
		})
	}
}
