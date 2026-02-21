package surrealdb

import (
	"bytes"
	"context"
	"testing"
)

func TestFileStore_SaveAndGet(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	data := []byte("hello world PDF content")
	if err := store.SaveFile(ctx, "filing_pdf", "BHP/20250101-12345.pdf", data, "application/pdf"); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	got, contentType, err := store.GetFile(ctx, "filing_pdf", "BHP/20250101-12345.pdf")
	if err != nil {
		t.Fatalf("GetFile failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("data mismatch: got %d bytes, want %d bytes", len(got), len(data))
	}
	if contentType != "application/pdf" {
		t.Errorf("content type mismatch: got %q, want %q", contentType, "application/pdf")
	}
}

func TestFileStore_Delete(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	data := []byte("temporary data")
	store.SaveFile(ctx, "chart", "test/chart.png", data, "image/png")

	if err := store.DeleteFile(ctx, "chart", "test/chart.png"); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	_, _, err := store.GetFile(ctx, "chart", "test/chart.png")
	if err == nil {
		t.Error("expected error getting deleted file")
	}
}

func TestFileStore_HasFile(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	// Should not exist initially
	has, err := store.HasFile(ctx, "filing_pdf", "XYZ/nonexistent.pdf")
	if err != nil {
		t.Fatalf("HasFile failed: %v", err)
	}
	if has {
		t.Error("expected HasFile=false for nonexistent file")
	}

	// Save and check
	store.SaveFile(ctx, "filing_pdf", "XYZ/exists.pdf", []byte("data"), "application/pdf")

	has, err = store.HasFile(ctx, "filing_pdf", "XYZ/exists.pdf")
	if err != nil {
		t.Fatalf("HasFile failed: %v", err)
	}
	if !has {
		t.Error("expected HasFile=true after save")
	}
}

func TestFileStore_Overwrite(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	// Save initial data
	store.SaveFile(ctx, "chart", "port/chart.png", []byte("version1"), "image/png")

	// Overwrite with new data
	newData := []byte("version2 - updated content")
	if err := store.SaveFile(ctx, "chart", "port/chart.png", newData, "image/png"); err != nil {
		t.Fatalf("SaveFile (overwrite) failed: %v", err)
	}

	got, _, err := store.GetFile(ctx, "chart", "port/chart.png")
	if err != nil {
		t.Fatalf("GetFile after overwrite failed: %v", err)
	}
	if !bytes.Equal(got, newData) {
		t.Errorf("overwrite failed: got %q, want %q", string(got), string(newData))
	}
}

func TestFileStore_BinaryData(t *testing.T) {
	db := testDB(t)
	store := NewFileStore(db, testLogger())
	ctx := context.Background()

	// Create binary data with all byte values
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}

	if err := store.SaveFile(ctx, "filing_pdf", "BIN/test.pdf", data, "application/pdf"); err != nil {
		t.Fatalf("SaveFile (binary) failed: %v", err)
	}

	got, _, err := store.GetFile(ctx, "filing_pdf", "BIN/test.pdf")
	if err != nil {
		t.Fatalf("GetFile (binary) failed: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("binary data round-trip failed: got %d bytes, want %d bytes", len(got), len(data))
	}
}
