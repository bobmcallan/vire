package surrealdb

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// Devils-advocate stress tests for the SurrealDB FileStore implementation.
// These tests use real SurrealDB via the test container.

// ============================================================================
// FS-1. Large file round-trip (simulated 5MB PDF)
// ============================================================================

func TestFileStoreStress_LargeFile(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	// Create a 5MB file (base64 encoded will be ~6.67MB)
	size := 5 * 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := store.SaveFile(ctx, "filing_pdf", "LARGE/big.pdf", data, "application/pdf"); err != nil {
		t.Fatalf("SaveFile (5MB) failed: %v", err)
	}

	got, contentType, err := store.GetFile(ctx, "filing_pdf", "LARGE/big.pdf")
	if err != nil {
		t.Fatalf("GetFile (5MB) failed: %v", err)
	}
	if len(got) != size {
		t.Errorf("size mismatch: got %d bytes, want %d bytes", len(got), size)
	}
	if !bytes.Equal(got, data) {
		t.Error("data corruption in 5MB round-trip")
	}
	if contentType != "application/pdf" {
		t.Errorf("content type mismatch: got %q", contentType)
	}
}

// ============================================================================
// FS-2. Concurrent SaveFile for the same key — last writer wins
// ============================================================================

func TestFileStoreStress_ConcurrentSameKey(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("concurrent-write-%d", n))
			store.SaveFile(ctx, "filing_pdf", "CONC/same.pdf", data, "application/pdf")
		}(i)
	}
	wg.Wait()

	// Should read successfully — one of the writers won
	data, _, err := store.GetFile(ctx, "filing_pdf", "CONC/same.pdf")
	if err != nil {
		t.Fatalf("GetFile after concurrent writes failed: %v", err)
	}
	if !strings.HasPrefix(string(data), "concurrent-write-") {
		t.Errorf("unexpected data after concurrent writes: %q", string(data))
	}
}

// ============================================================================
// FS-3. GetFile for non-existent key
// ============================================================================

func TestFileStoreStress_GetNonExistent(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	_, _, err := store.GetFile(ctx, "filing_pdf", "GHOST/nothing.pdf")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

// ============================================================================
// FS-4. Empty and nil data
// ============================================================================

func TestFileStoreStress_EmptyData(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	// Empty byte slice
	if err := store.SaveFile(ctx, "filing_pdf", "EMPTY/file.pdf", []byte{}, "application/pdf"); err != nil {
		t.Fatalf("SaveFile with empty data failed: %v", err)
	}

	data, _, err := store.GetFile(ctx, "filing_pdf", "EMPTY/file.pdf")
	if err != nil {
		t.Fatalf("GetFile for empty data failed: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d bytes", len(data))
	}
}

func TestFileStoreStress_NilData(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	// nil data — base64.StdEncoding.EncodeToString(nil) returns ""
	if err := store.SaveFile(ctx, "filing_pdf", "NIL/file.pdf", nil, "application/pdf"); err != nil {
		t.Fatalf("SaveFile with nil data failed: %v", err)
	}

	data, _, err := store.GetFile(ctx, "filing_pdf", "NIL/file.pdf")
	if err != nil {
		t.Fatalf("GetFile for nil data failed: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty data for nil input, got %d bytes", len(data))
	}
}

// ============================================================================
// FS-5. Record ID collision — different inputs same sanitized ID
// ============================================================================

func TestFileStoreStress_RecordIDCollision(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	// These two produce the same record ID after sanitization:
	// fileRecordID("filing_pdf", "BHP/test") -> "filing_pdf_BHP_test"
	// fileRecordID("filing", "pdf_BHP_test") -> "filing_pdf_BHP_test"

	store.SaveFile(ctx, "filing_pdf", "BHP/test", []byte("data-a"), "application/pdf")
	store.SaveFile(ctx, "filing", "pdf_BHP_test", []byte("data-b"), "application/pdf")

	// Check if the second write overwrote the first
	dataA, _, errA := store.GetFile(ctx, "filing_pdf", "BHP/test")
	dataB, _, errB := store.GetFile(ctx, "filing", "pdf_BHP_test")

	if errA != nil || errB != nil {
		t.Logf("GetFile errors: a=%v, b=%v", errA, errB)
	}

	if errA == nil && errB == nil && bytes.Equal(dataA, dataB) {
		t.Log("CONFIRMED: Record ID collision — (filing_pdf, BHP/test) and (filing, pdf_BHP_test) " +
			"map to the same SurrealDB record ID. Second write overwrote the first. " +
			"Fix: use a non-sanitizable separator like '::' between category and key in fileRecordID.")
	}
}

// ============================================================================
// FS-6. Special characters in key — SurrealDB record ID safety
// ============================================================================

func TestFileStoreStress_SpecialCharKeys(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	keys := []struct {
		key  string
		desc string
	}{
		{"BHP/2025-01-01.pdf", "normal key with slash and dots"},
		{"BHP.AU/report-2025.pdf", "key with exchange dots"},
		{"BHP/特殊文字.pdf", "unicode characters"},
		{"BHP/file name with spaces.pdf", "spaces in key"},
		{"BHP/file%20encoded.pdf", "URL-encoded characters"},
	}

	for _, tc := range keys {
		t.Run(tc.desc, func(t *testing.T) {
			data := []byte("test-" + tc.key)
			if err := store.SaveFile(ctx, "filing_pdf", tc.key, data, "application/pdf"); err != nil {
				t.Errorf("SaveFile failed for key %q: %v", tc.key, err)
				return
			}

			got, _, err := store.GetFile(ctx, "filing_pdf", tc.key)
			if err != nil {
				t.Errorf("GetFile failed for key %q: %v", tc.key, err)
				return
			}
			if !bytes.Equal(got, data) {
				t.Errorf("data mismatch for key %q", tc.key)
			}
		})
	}
}

// ============================================================================
// FS-7. HasFile after Delete returns false
// ============================================================================

func TestFileStoreStress_HasFileAfterDelete(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	store.SaveFile(ctx, "chart", "test/chart.png", []byte("png-data"), "image/png")

	has, _ := store.HasFile(ctx, "chart", "test/chart.png")
	if !has {
		t.Fatal("expected HasFile=true after save")
	}

	store.DeleteFile(ctx, "chart", "test/chart.png")

	has, _ = store.HasFile(ctx, "chart", "test/chart.png")
	if has {
		t.Error("expected HasFile=false after delete")
	}
}

// ============================================================================
// FS-8. Delete non-existent file does not error
// ============================================================================

func TestFileStoreStress_DeleteNonExistent(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	err := store.DeleteFile(ctx, "filing_pdf", "GHOST/nothing.pdf")
	if err != nil {
		t.Errorf("DeleteFile for non-existent file should not error, got: %v", err)
	}
}

// ============================================================================
// FS-9. Many files in same category — no interference
// ============================================================================

func TestFileStoreStress_ManyFiles(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	count := 50
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("BHP/%08d.pdf", i)
		data := []byte(fmt.Sprintf("content-%d", i))
		if err := store.SaveFile(ctx, "filing_pdf", key, data, "application/pdf"); err != nil {
			t.Fatalf("SaveFile #%d failed: %v", i, err)
		}
	}

	// Verify all files are independently readable
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("BHP/%08d.pdf", i)
		expected := []byte(fmt.Sprintf("content-%d", i))
		got, _, err := store.GetFile(ctx, "filing_pdf", key)
		if err != nil {
			t.Errorf("GetFile #%d failed: %v", i, err)
			continue
		}
		if !bytes.Equal(got, expected) {
			t.Errorf("data mismatch for file #%d: got %q, want %q", i, string(got), string(expected))
		}
	}
}

// ============================================================================
// FS-10. Binary data with all byte values (including null bytes)
// ============================================================================

func TestFileStoreStress_AllByteValues(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	// Create data with every possible byte value repeated
	data := make([]byte, 256*4)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := store.SaveFile(ctx, "filing_pdf", "BIN/allbytes.pdf", data, "application/octet-stream"); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	got, _, err := store.GetFile(ctx, "filing_pdf", "BIN/allbytes.pdf")
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("binary data with all byte values corrupted in round-trip")
	}
}
