package blob

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *common.Logger {
	return common.NewSilentLogger()
}

func newTestStore(t *testing.T) *FileSystemStore {
	t.Helper()
	store, err := NewFileSystemStore(t.TempDir(), testLogger())
	require.NoError(t, err)
	return store
}

func TestFileSystemStore_SaveAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	data := []byte("hello world PDF content")
	err := store.SaveFile(ctx, "filing_pdf", "BHP/20250101-12345.pdf", data, "application/pdf")
	require.NoError(t, err)

	got, contentType, err := store.GetFile(ctx, "filing_pdf", "BHP/20250101-12345.pdf")
	require.NoError(t, err)
	assert.True(t, bytes.Equal(got, data), "data round-trip mismatch")
	assert.Equal(t, "application/pdf", contentType)
}

func TestFileSystemStore_HasFile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	has, err := store.HasFile(ctx, "filing_pdf", "XYZ/nonexistent.pdf")
	require.NoError(t, err)
	assert.False(t, has, "expected false for nonexistent file")

	require.NoError(t, store.SaveFile(ctx, "filing_pdf", "XYZ/exists.pdf", []byte("data"), "application/pdf"))

	has, err = store.HasFile(ctx, "filing_pdf", "XYZ/exists.pdf")
	require.NoError(t, err)
	assert.True(t, has, "expected true after save")
}

func TestFileSystemStore_DeleteFile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.SaveFile(ctx, "chart", "test/chart.png", []byte("png data"), "image/png"))

	has, err := store.HasFile(ctx, "chart", "test/chart.png")
	require.NoError(t, err)
	require.True(t, has)

	require.NoError(t, store.DeleteFile(ctx, "chart", "test/chart.png"))

	has, err = store.HasFile(ctx, "chart", "test/chart.png")
	require.NoError(t, err)
	assert.False(t, has, "expected false after delete")

	// .meta sidecar should also be gone
	_, _, err = store.GetFile(ctx, "chart", "test/chart.png")
	assert.Error(t, err, "expected error getting deleted file")
}

func TestFileSystemStore_OverwriteExisting(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.SaveFile(ctx, "chart", "port/chart.png", []byte("version1"), "image/png"))
	newData := []byte("version2 updated content")
	require.NoError(t, store.SaveFile(ctx, "chart", "port/chart.png", newData, "image/png"))

	got, _, err := store.GetFile(ctx, "chart", "port/chart.png")
	require.NoError(t, err)
	assert.True(t, bytes.Equal(got, newData), "overwrite failed")
}

func TestFileSystemStore_NestedKeys(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	key := "BHP/20250101-doc.pdf"
	data := []byte("pdf content")
	require.NoError(t, store.SaveFile(ctx, "filing_pdf", key, data, "application/pdf"))

	// Verify subdirectory was created
	p, err := store.blobPath("filing_pdf", key)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filepath.Dir(p), "BHP"), "expected BHP subdirectory")

	got, contentType, err := store.GetFile(ctx, "filing_pdf", key)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(got, data))
	assert.Equal(t, "application/pdf", contentType)
}

func TestFileSystemStore_PathTraversal(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	cases := []struct {
		category string
		key      string
	}{
		{"filing_pdf", "../etc/passwd"},
		{"../etc", "passwd"},
		{"filing_pdf", "subdir/../../etc/passwd"},
	}

	for _, tc := range cases {
		err := store.SaveFile(ctx, tc.category, tc.key, []byte("data"), "text/plain")
		assert.Error(t, err, "expected path traversal rejection for category=%q key=%q", tc.category, tc.key)
	}
}

func TestFileSystemStore_MissingFile(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, _, err := store.GetFile(ctx, "filing_pdf", "nonexistent/file.pdf")
	assert.Error(t, err, "expected error for missing file")
}
