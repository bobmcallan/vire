package blob

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
)

// Devils-advocate stress tests for the filesystem blob store.
// These tests exercise path traversal, injection, race conditions,
// resource leaks, and edge cases that could cause data loss or security issues.

func testFSStore(t *testing.T) *FileSystemStore {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := NewFileSystemStore(dir, logger)
	if err != nil {
		t.Fatalf("NewFileSystemStore: %v", err)
	}
	return store
}

// ============================================================================
// BFS-1. Path traversal via category — reject ".." segments
// ============================================================================

func TestBlobStress_PathTraversal_Category(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	attacks := []string{
		"../etc",
		"../../etc",
		"valid/../../../etc",
		"..%2f..%2f",
		"..\x00/etc",
	}

	for _, cat := range attacks {
		t.Run(cat, func(t *testing.T) {
			err := store.SaveFile(ctx, cat, "passwd", []byte("pwned"), "text/plain")
			if err == nil {
				t.Errorf("SaveFile should reject traversal category %q", cat)
			}
		})
	}
}

// ============================================================================
// BFS-2. Path traversal via key — reject ".." segments
// ============================================================================

func TestBlobStress_PathTraversal_Key(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	attacks := []string{
		"../../../etc/passwd",
		"BHP/../../etc/passwd",
		"..\\..\\etc\\passwd",
		"valid/../../../../tmp/evil",
	}

	for _, key := range attacks {
		t.Run(key, func(t *testing.T) {
			err := store.SaveFile(ctx, "filing_pdf", key, []byte("pwned"), "text/plain")
			if err == nil {
				t.Errorf("SaveFile should reject traversal key %q", key)
			}
		})
	}
}

// ============================================================================
// BFS-3. Symlink escape — symlink inside basePath pointing outside
// ============================================================================

func TestBlobStress_SymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	logger := common.NewSilentLogger()
	store, err := NewFileSystemStore(dir, logger)
	if err != nil {
		t.Fatalf("NewFileSystemStore: %v", err)
	}
	ctx := context.Background()

	// Create a category dir that is actually a symlink to /tmp
	catDir := filepath.Join(dir, "evil_cat")
	tmpTarget := t.TempDir()
	if err := os.Symlink(tmpTarget, catDir); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	// Attempt to write through the symlink — this writes outside basePath
	err = store.SaveFile(ctx, "evil_cat", "payload.txt", []byte("escaped"), "text/plain")
	// The blobPath check should catch that the resolved path is outside basePath.
	// If it doesn't, the file will be written to tmpTarget — that's a bug.
	if err == nil {
		// Check if file actually ended up outside basePath
		escaped := filepath.Join(tmpTarget, "payload.txt")
		if _, statErr := os.Stat(escaped); statErr == nil {
			t.Error("SECURITY: SaveFile followed symlink and wrote outside basePath")
			os.Remove(escaped)
		} else {
			// File was written but inside basePath via symlink resolution — acceptable
			// as long as blobPath validates the resolved path
			t.Log("SaveFile succeeded but file stayed within controlled directory")
		}
	}
}

// ============================================================================
// BFS-4. Null bytes in key — should not truncate path
// ============================================================================

func TestBlobStress_NullBytesInKey(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	// Null byte can truncate C-level path operations
	err := store.SaveFile(ctx, "filing_pdf", "BHP\x00/evil.pdf", []byte("data"), "application/pdf")
	if err == nil {
		// If it succeeds, verify the stored key roundtrips correctly
		data, _, getErr := store.GetFile(ctx, "filing_pdf", "BHP\x00/evil.pdf")
		if getErr != nil {
			t.Logf("SaveFile succeeded but GetFile failed (key truncated): %v", getErr)
		} else if !bytes.Equal(data, []byte("data")) {
			t.Error("data corruption with null byte key")
		}
	}
	// Either error or clean roundtrip is acceptable; data corruption is not
}

// ============================================================================
// BFS-5. Very long key — exceeds filesystem path limits
// ============================================================================

func TestBlobStress_VeryLongKey(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	// Most filesystems limit path components to 255 bytes
	longKey := strings.Repeat("a", 300) + ".pdf"
	err := store.SaveFile(ctx, "filing_pdf", longKey, []byte("data"), "application/pdf")
	if err == nil {
		// If it succeeded, verify round-trip
		data, _, getErr := store.GetFile(ctx, "filing_pdf", longKey)
		if getErr != nil {
			t.Errorf("SaveFile succeeded but GetFile failed for long key: %v", getErr)
		}
		if !bytes.Equal(data, []byte("data")) {
			t.Error("data corruption with very long key")
		}
	}
	// Error is acceptable — OS will reject the path
}

// ============================================================================
// BFS-6. Concurrent writes to same key — last writer wins, no corruption
// ============================================================================

func TestBlobStress_ConcurrentSameKey(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make([]error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("writer-%02d-payload", n))
			errors[n] = store.SaveFile(ctx, "filing_pdf", "RACE/same.pdf", data, "application/pdf")
		}(i)
	}
	wg.Wait()

	// Count errors
	var errCount int
	for _, e := range errors {
		if e != nil {
			errCount++
		}
	}
	if errCount == 20 {
		t.Fatal("all concurrent writers failed")
	}

	// Read should succeed and return one writer's data (not corrupt)
	data, _, err := store.GetFile(ctx, "filing_pdf", "RACE/same.pdf")
	if err != nil {
		t.Fatalf("GetFile after concurrent writes: %v", err)
	}
	if !strings.HasPrefix(string(data), "writer-") {
		t.Errorf("data corruption after concurrent writes: got %q", string(data))
	}
}

// ============================================================================
// BFS-7. Concurrent writes to different keys — no interference
// ============================================================================

func TestBlobStress_ConcurrentDifferentKeys(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	const count = 50
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("PAR/%04d.pdf", n)
			data := []byte(fmt.Sprintf("content-%04d", n))
			if err := store.SaveFile(ctx, "filing_pdf", key, data, "application/pdf"); err != nil {
				t.Errorf("concurrent SaveFile #%d: %v", n, err)
			}
		}(i)
	}
	wg.Wait()

	// Verify all files are independently readable
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("PAR/%04d.pdf", i)
		expected := []byte(fmt.Sprintf("content-%04d", i))
		data, _, err := store.GetFile(ctx, "filing_pdf", key)
		if err != nil {
			t.Errorf("GetFile #%d: %v", i, err)
			continue
		}
		if !bytes.Equal(data, expected) {
			t.Errorf("data mismatch file #%d: got %q, want %q", i, string(data), string(expected))
		}
	}
}

// ============================================================================
// BFS-8. Large file (20MB) — memory pressure test
// ============================================================================

func TestBlobStress_LargeFile(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	size := 20 * 1024 * 1024 // 20MB
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	if err := store.SaveFile(ctx, "filing_pdf", "BIG/huge.pdf", data, "application/pdf"); err != nil {
		t.Fatalf("SaveFile (20MB): %v", err)
	}

	got, ct, err := store.GetFile(ctx, "filing_pdf", "BIG/huge.pdf")
	if err != nil {
		t.Fatalf("GetFile (20MB): %v", err)
	}
	if len(got) != size {
		t.Errorf("size mismatch: got %d, want %d", len(got), size)
	}
	if !bytes.Equal(got, data) {
		t.Error("data corruption in 20MB round-trip")
	}
	if ct != "application/pdf" {
		t.Errorf("content type: got %q, want %q", ct, "application/pdf")
	}
}

// ============================================================================
// BFS-9. Empty data and nil data
// ============================================================================

func TestBlobStress_EmptyData(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	if err := store.SaveFile(ctx, "filing_pdf", "EMPTY/zero.pdf", []byte{}, "application/pdf"); err != nil {
		t.Fatalf("SaveFile empty: %v", err)
	}

	data, _, err := store.GetFile(ctx, "filing_pdf", "EMPTY/zero.pdf")
	if err != nil {
		t.Fatalf("GetFile empty: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected 0 bytes, got %d", len(data))
	}
}

func TestBlobStress_NilData(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	if err := store.SaveFile(ctx, "filing_pdf", "NIL/null.pdf", nil, "application/pdf"); err != nil {
		t.Fatalf("SaveFile nil: %v", err)
	}

	data, _, err := store.GetFile(ctx, "filing_pdf", "NIL/null.pdf")
	if err != nil {
		t.Fatalf("GetFile nil: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected 0 bytes for nil input, got %d", len(data))
	}
}

// ============================================================================
// BFS-10. Sidecar metadata integrity — content type preserved
// ============================================================================

func TestBlobStress_ContentTypePreserved(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	cases := []struct {
		input    string
		expected string
		desc     string
	}{
		{"application/pdf", "application/pdf", "standard PDF"},
		{"image/png", "image/png", "PNG image"},
		{"text/html; charset=utf-8", "text/html; charset=utf-8", "HTML with charset"},
		{"", "application/octet-stream", "empty content type defaults to octet-stream"},
		{"application/octet-stream", "application/octet-stream", "binary"},
	}

	for i, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			key := fmt.Sprintf("CT/%d.bin", i)
			if err := store.SaveFile(ctx, "test", key, []byte("data"), tc.input); err != nil {
				t.Fatalf("SaveFile: %v", err)
			}
			_, got, err := store.GetFile(ctx, "test", key)
			if err != nil {
				t.Fatalf("GetFile: %v", err)
			}
			if got != tc.expected {
				t.Errorf("content type: got %q, want %q", got, tc.expected)
			}
		})
	}
}

// ============================================================================
// BFS-11. Delete removes both data file and .meta sidecar
// ============================================================================

func TestBlobStress_DeleteCleanup(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	if err := store.SaveFile(ctx, "test", "DEL/file.pdf", []byte("data"), "application/pdf"); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}

	// Verify file exists
	has, _ := store.HasFile(ctx, "test", "DEL/file.pdf")
	if !has {
		t.Fatal("expected HasFile=true after save")
	}

	// Delete
	if err := store.DeleteFile(ctx, "test", "DEL/file.pdf"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	// Verify both data and meta are gone
	has, _ = store.HasFile(ctx, "test", "DEL/file.pdf")
	if has {
		t.Error("HasFile should be false after delete")
	}

	// Verify .meta sidecar is also gone (check filesystem directly)
	p, err := store.blobPath("test", "DEL/file.pdf")
	if err != nil {
		t.Fatalf("blobPath: %v", err)
	}
	metaPath := p + ".meta"
	if _, err := os.Stat(metaPath); err == nil {
		t.Error("LEAK: .meta sidecar file was not removed on delete")
	}
}

// ============================================================================
// BFS-12. Delete non-existent file — should not error
// ============================================================================

func TestBlobStress_DeleteNonExistent(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	err := store.DeleteFile(ctx, "test", "GHOST/nothing.pdf")
	if err != nil {
		t.Errorf("DeleteFile for non-existent should not error: %v", err)
	}
}

// ============================================================================
// BFS-13. GetFile for non-existent key — returns error
// ============================================================================

func TestBlobStress_GetNonExistent(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	_, _, err := store.GetFile(ctx, "test", "GHOST/nothing.pdf")
	if err == nil {
		t.Error("GetFile should return error for non-existent file")
	}
}

// ============================================================================
// BFS-14. Category isolation — same key in different categories are independent
// ============================================================================

func TestBlobStress_CategoryIsolation(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	store.SaveFile(ctx, "filing_pdf", "BHP/doc.pdf", []byte("pdf-data"), "application/pdf")
	store.SaveFile(ctx, "chart", "BHP/doc.pdf", []byte("chart-data"), "image/png")

	pdfData, pdfCT, err := store.GetFile(ctx, "filing_pdf", "BHP/doc.pdf")
	if err != nil {
		t.Fatalf("GetFile filing_pdf: %v", err)
	}
	chartData, chartCT, err := store.GetFile(ctx, "chart", "BHP/doc.pdf")
	if err != nil {
		t.Fatalf("GetFile chart: %v", err)
	}

	if bytes.Equal(pdfData, chartData) {
		t.Error("categories not isolated: same data returned for different categories")
	}
	if string(pdfData) != "pdf-data" {
		t.Errorf("filing_pdf data: got %q", string(pdfData))
	}
	if string(chartData) != "chart-data" {
		t.Errorf("chart data: got %q", string(chartData))
	}
	if pdfCT != "application/pdf" {
		t.Errorf("filing_pdf ct: got %q", pdfCT)
	}
	if chartCT != "image/png" {
		t.Errorf("chart ct: got %q", chartCT)
	}
}

// ============================================================================
// BFS-15. Nested key creates subdirectories
// ============================================================================

func TestBlobStress_NestedKeySubdirs(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	key := "BHP/2025/01/01-12345-annual-report.pdf"
	data := []byte("deeply nested PDF")

	if err := store.SaveFile(ctx, "filing_pdf", key, data, "application/pdf"); err != nil {
		t.Fatalf("SaveFile nested: %v", err)
	}

	got, _, err := store.GetFile(ctx, "filing_pdf", key)
	if err != nil {
		t.Fatalf("GetFile nested: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("data mismatch for deeply nested key")
	}
}

// ============================================================================
// BFS-16. Overwrite existing file — data and metadata updated
// ============================================================================

func TestBlobStress_OverwriteUpdatesAll(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	// Save with one content type
	store.SaveFile(ctx, "test", "OW/file.bin", []byte("v1"), "text/plain")

	// Overwrite with different data AND different content type
	store.SaveFile(ctx, "test", "OW/file.bin", []byte("version-2-longer"), "application/octet-stream")

	data, ct, err := store.GetFile(ctx, "test", "OW/file.bin")
	if err != nil {
		t.Fatalf("GetFile after overwrite: %v", err)
	}
	if string(data) != "version-2-longer" {
		t.Errorf("data not overwritten: got %q", string(data))
	}
	if ct != "application/octet-stream" {
		t.Errorf("content type not overwritten: got %q", ct)
	}
}

// ============================================================================
// BFS-17. Special characters in category and key
// ============================================================================

func TestBlobStress_SpecialCharKeys(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	cases := []struct {
		category string
		key      string
		desc     string
	}{
		{"filing_pdf", "BHP.AU/report-2025.pdf", "dots in key"},
		{"filing_pdf", "BHP/file name with spaces.pdf", "spaces in key"},
		{"filing_pdf", "BHP/file%20encoded.pdf", "percent-encoded"},
		{"filing-pdf", "BHP/report.pdf", "hyphen in category"},
		{"test_cat", "a/b/c/d/e/f.txt", "deeply nested slashes"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			data := []byte("content-" + tc.key)
			if err := store.SaveFile(ctx, tc.category, tc.key, data, "text/plain"); err != nil {
				t.Errorf("SaveFile failed: %v", err)
				return
			}
			got, _, err := store.GetFile(ctx, tc.category, tc.key)
			if err != nil {
				t.Errorf("GetFile failed: %v", err)
				return
			}
			if !bytes.Equal(got, data) {
				t.Errorf("data mismatch: got %q", string(got))
			}
		})
	}
}

// ============================================================================
// BFS-18. All byte values in data (including null bytes)
// ============================================================================

func TestBlobStress_AllByteValues(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	data := make([]byte, 256*4)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := store.SaveFile(ctx, "test", "BIN/allbytes.bin", data, "application/octet-stream"); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}

	got, _, err := store.GetFile(ctx, "test", "BIN/allbytes.bin")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("binary data with all byte values corrupted in round-trip")
	}
}

// ============================================================================
// BFS-19. HasFile with path traversal — should reject
// ============================================================================

func TestBlobStress_HasFile_PathTraversal(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	_, err := store.HasFile(ctx, "filing_pdf", "../../../etc/passwd")
	if err == nil {
		t.Error("HasFile should reject path traversal keys")
	}
}

// ============================================================================
// BFS-20. GetFile with path traversal — should reject
// ============================================================================

func TestBlobStress_GetFile_PathTraversal(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	_, _, err := store.GetFile(ctx, "../etc", "passwd")
	if err == nil {
		t.Error("GetFile should reject path traversal categories")
	}
}

// ============================================================================
// BFS-21. DeleteFile with path traversal — should reject
// ============================================================================

func TestBlobStress_DeleteFile_PathTraversal(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	err := store.DeleteFile(ctx, "filing_pdf", "../../etc/passwd")
	if err == nil {
		t.Error("DeleteFile should reject path traversal keys")
	}
}

// ============================================================================
// BFS-22. Atomic write — partial write should not leave corrupt data
// ============================================================================

func TestBlobStress_AtomicWrite(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	// Write initial data
	store.SaveFile(ctx, "test", "ATOM/file.pdf", []byte("original-content"), "application/pdf")

	// Verify original is readable
	data, _, _ := store.GetFile(ctx, "test", "ATOM/file.pdf")
	if string(data) != "original-content" {
		t.Fatalf("initial content wrong: %q", string(data))
	}

	// Overwrite — if atomic (write-to-tmp-then-rename), even a crash
	// during write wouldn't corrupt the original
	store.SaveFile(ctx, "test", "ATOM/file.pdf", []byte("updated-content"), "application/pdf")

	data, _, err := store.GetFile(ctx, "test", "ATOM/file.pdf")
	if err != nil {
		t.Fatalf("GetFile after overwrite: %v", err)
	}
	if string(data) != "updated-content" {
		t.Errorf("expected updated-content, got %q", string(data))
	}

	// Verify no .tmp files are left behind
	p, _ := store.blobPath("test", "ATOM/file.pdf")
	dir := filepath.Dir(p)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("LEAK: temporary file %q left behind after write", e.Name())
		}
	}
}

// ============================================================================
// BFS-23. Empty category name — edge case
// ============================================================================

func TestBlobStress_EmptyCategory(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	err := store.SaveFile(ctx, "", "file.pdf", []byte("data"), "application/pdf")
	// Either error (rejecting empty category) or success is acceptable
	// but it should not panic
	_ = err
}

// ============================================================================
// BFS-24. Empty key — edge case
// ============================================================================

func TestBlobStress_EmptyKey(t *testing.T) {
	store := testFSStore(t)
	ctx := context.Background()

	err := store.SaveFile(ctx, "test", "", []byte("data"), "application/pdf")
	// Either error (rejecting empty key) or success is acceptable
	// but it should not panic
	_ = err
}

// ============================================================================
// BFS-25. Constructor with empty basePath — uses default
// ============================================================================

func TestBlobStress_EmptyBasePath(t *testing.T) {
	// The constructor should default to "data/blobs" when basePath is empty
	// We can't easily test this without side effects, so just verify it doesn't panic
	logger := common.NewSilentLogger()
	store, err := NewFileSystemStore("", logger)
	if err != nil {
		// Acceptable — might fail if default path can't be created
		return
	}
	if store.basePath == "" {
		t.Error("basePath should not remain empty after construction")
	}
}
