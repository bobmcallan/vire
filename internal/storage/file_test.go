package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Test helpers ---

// newTestFileStore creates a FileStore with a temp directory and default 5 versions.
func newTestFileStore(t *testing.T) *FileStore {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewLogger("error")
	fs, err := NewFileStore(logger, &common.FileConfig{Path: dir, Versions: 5})
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}
	return fs
}

// newTestFileStoreVersions creates a FileStore with a custom version count.
func newTestFileStoreVersions(t *testing.T, versions int) *FileStore {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewLogger("error")
	fs, err := NewFileStore(logger, &common.FileConfig{Path: dir, Versions: versions})
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}
	return fs
}

// newTestFileManager creates a full Manager backed by BadgerHold (user data) and FileStore (data).
func newTestFileManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewLogger("error")
	config := &common.Config{
		Storage: common.StorageConfig{
			UserData: common.FileConfig{Path: filepath.Join(dir, "user"), Versions: 5},
			Data:     common.FileConfig{Path: filepath.Join(dir, "data"), Versions: 0},
		},
	}
	mgr, err := NewStorageManager(logger, config)
	if err != nil {
		t.Fatalf("NewStorageManager failed: %v", err)
	}
	t.Cleanup(func() { mgr.Close() })
	return mgr.(*Manager)
}

// --- FileStore core tests ---

func TestFileStore_BaseDirectoryCreation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "path")
	logger := common.NewLogger("error")
	_, err := NewFileStore(logger, &common.FileConfig{Path: dir, Versions: 5})
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	// Base directory should exist
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected base directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected base path to be a directory")
	}
}

func TestDomainStorageCreatesSubdirectories(t *testing.T) {
	m := newTestFileManager(t)

	// Data store subdirectories should exist (created by domain constructors)
	dataSubdirs := []string{"market", "signals"}
	for _, sub := range dataSubdirs {
		path := filepath.Join(m.dataStore.basePath, sub)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected data store directory %s to exist: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", sub)
		}
	}

	// BadgerDB directory should exist for user data
	if m.badgerStore == nil {
		t.Error("expected badgerStore to be initialized")
	}
}

func TestFileStore_SanitizeKey(t *testing.T) {
	fs := newTestFileStore(t)

	tests := []struct {
		input    string
		expected string
	}{
		{"BHP.AU", "BHP.AU"}, // single dots preserved
		{"simple", "simple"}, // no change needed
		{"path/with/slashes", "path_with_slashes"},
		{"back\\slashes", "back_slashes"},
		{"has:colons", "has_colons"},
		{"..", "_"},           // dot-dot collapsed to prevent path traversal
		{"../evil", "__evil"}, // path traversal attempt neutralized
		{"../../etc/passwd", "____etc_passwd"},
		{"a..b", "a_b"}, // embedded dot-dot collapsed
		{"", ""},
	}

	for _, tt := range tests {
		got := fs.sanitizeKey(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// --- JSON round-trip and atomic write tests ---

func TestFileStore_WriteAndReadJSON(t *testing.T) {
	fs := newTestFileStore(t)

	type testData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	data := testData{Name: "test", Value: 42}
	err := fs.writeJSON(filepath.Join(fs.basePath, "kv"), "test-key", &data, false)
	if err != nil {
		t.Fatalf("writeJSON failed: %v", err)
	}

	var got testData
	err = fs.readJSON(filepath.Join(fs.basePath, "kv"), "test-key", &got)
	if err != nil {
		t.Fatalf("readJSON failed: %v", err)
	}

	if got.Name != "test" || got.Value != 42 {
		t.Errorf("got %+v, want {Name:test Value:42}", got)
	}
}

func TestFileStore_HumanReadableJSON(t *testing.T) {
	fs := newTestFileStore(t)

	type testData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	data := testData{Name: "test", Value: 42}
	err := fs.writeJSON(filepath.Join(fs.basePath, "kv"), "human-readable", &data, false)
	if err != nil {
		t.Fatalf("writeJSON failed: %v", err)
	}

	// Read raw file content and verify it's indented
	filePath := filepath.Join(fs.basePath, "kv", "human-readable.json")
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	content := string(raw)
	if !strings.Contains(content, "  ") {
		t.Errorf("expected indented JSON, got:\n%s", content)
	}
	if !strings.Contains(content, "\"name\": \"test\"") {
		t.Errorf("expected human-readable JSON with field names, got:\n%s", content)
	}
}

func TestFileStore_AtomicWrite_NoTempFileLeftBehind(t *testing.T) {
	fs := newTestFileStore(t)

	type testData struct {
		Value string `json:"value"`
	}

	data := testData{Value: "hello"}
	dir := filepath.Join(fs.basePath, "kv")
	err := fs.writeJSON(dir, "atomic-test", &data, false)
	if err != nil {
		t.Fatalf("writeJSON failed: %v", err)
	}

	// Verify no .tmp files remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestFileStore_DeleteJSON(t *testing.T) {
	fs := newTestFileStore(t)

	type testData struct {
		Value string `json:"value"`
	}

	// Write then delete
	data := testData{Value: "to-delete"}
	dir := filepath.Join(fs.basePath, "kv")
	err := fs.writeJSON(dir, "delete-me", &data, false)
	if err != nil {
		t.Fatalf("writeJSON failed: %v", err)
	}

	err = fs.deleteJSON(dir, "delete-me")
	if err != nil {
		t.Fatalf("deleteJSON failed: %v", err)
	}

	// Should not be readable
	var got testData
	err = fs.readJSON(dir, "delete-me", &got)
	if err == nil {
		t.Error("expected error reading deleted key")
	}
}

func TestFileStore_DeleteJSON_RemovesVersionFiles(t *testing.T) {
	fs := newTestFileStore(t)

	type testData struct {
		Value int `json:"value"`
	}

	dir := filepath.Join(fs.basePath, "kv")

	// Write multiple times to create version files
	for i := 0; i < 3; i++ {
		data := testData{Value: i}
		err := fs.writeJSON(dir, "versioned-delete", &data, true)
		if err != nil {
			t.Fatalf("writeJSON #%d failed: %v", i, err)
		}
	}

	// Delete should remove main file and all versions
	err := fs.deleteJSON(dir, "versioned-delete")
	if err != nil {
		t.Fatalf("deleteJSON failed: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "versioned-delete") {
			t.Errorf("version file not cleaned up: %s", e.Name())
		}
	}
}

func TestFileStore_ListKeys(t *testing.T) {
	fs := newTestFileStore(t)

	type testData struct {
		Value string `json:"value"`
	}

	dir := filepath.Join(fs.basePath, "kv")

	// Write 3 items
	for _, key := range []string{"alpha", "beta", "gamma"} {
		data := testData{Value: key}
		if err := fs.writeJSON(dir, key, &data, false); err != nil {
			t.Fatalf("writeJSON %s failed: %v", key, err)
		}
	}

	keys, err := fs.listKeys(dir)
	if err != nil {
		t.Fatalf("listKeys failed: %v", err)
	}

	if len(keys) != 3 {
		t.Fatalf("listKeys returned %d keys, want 3", len(keys))
	}

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	for _, expected := range []string{"alpha", "beta", "gamma"} {
		if !keySet[expected] {
			t.Errorf("missing key %q in listKeys result", expected)
		}
	}
}

func TestFileStore_ListKeys_ExcludesVersionFiles(t *testing.T) {
	fs := newTestFileStore(t)

	type testData struct {
		Value int `json:"value"`
	}

	dir := filepath.Join(fs.basePath, "kv")

	// Write multiple times to create version files
	for i := 0; i < 3; i++ {
		data := testData{Value: i}
		if err := fs.writeJSON(dir, "only-one", &data, true); err != nil {
			t.Fatalf("writeJSON failed: %v", err)
		}
	}

	keys, err := fs.listKeys(dir)
	if err != nil {
		t.Fatalf("listKeys failed: %v", err)
	}

	if len(keys) != 1 {
		t.Errorf("listKeys returned %d keys (expected 1 - versions should be excluded): %v", len(keys), keys)
	}
}

func TestFileStore_ListKeys_EmptyDir(t *testing.T) {
	fs := newTestFileStore(t)
	dir := filepath.Join(fs.basePath, "kv")

	keys, err := fs.listKeys(dir)
	if err != nil {
		t.Fatalf("listKeys on empty dir failed: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestFileStore_ReadJSON_MissingFile(t *testing.T) {
	fs := newTestFileStore(t)

	var dest struct{}
	err := fs.readJSON(filepath.Join(fs.basePath, "kv"), "nonexistent", &dest)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestFileStore_ReadJSON_CorruptJSON(t *testing.T) {
	fs := newTestFileStore(t)
	dir := filepath.Join(fs.basePath, "kv")
	os.MkdirAll(dir, 0755)

	// Write a corrupt JSON file directly
	filePath := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(filePath, []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}

	var dest map[string]interface{}
	err := fs.readJSON(dir, "corrupt", &dest)
	if err == nil {
		t.Error("expected error for corrupt JSON")
	}
}

func TestFileStore_ReadJSON_ZeroLengthFile(t *testing.T) {
	fs := newTestFileStore(t)
	dir := filepath.Join(fs.basePath, "kv")
	os.MkdirAll(dir, 0755)

	// Write a zero-length file
	filePath := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	var dest map[string]interface{}
	err := fs.readJSON(dir, "empty", &dest)
	if err == nil {
		t.Error("expected error for zero-length JSON file")
	}
}

// --- Versioning tests ---

func TestFileStore_Versioning_RetentionLimit(t *testing.T) {
	fs := newTestFileStoreVersions(t, 3) // Keep 3 versions

	type testData struct {
		Value int `json:"value"`
	}

	dir := filepath.Join(fs.basePath, "kv")

	// Write 6 times: should keep current + 3 versions = 4 files max
	for i := 0; i < 6; i++ {
		data := testData{Value: i}
		if err := fs.writeJSON(dir, "versioned", &data, true); err != nil {
			t.Fatalf("writeJSON #%d failed: %v", i, err)
		}
	}

	// Count files matching "versioned"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	count := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "versioned.json") {
			count++
		}
	}

	// Expected: versioned.json + versioned.json.v1 + versioned.json.v2 + versioned.json.v3 = 4
	if count != 4 {
		t.Errorf("expected 4 files (current + 3 versions), got %d", count)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "versioned") {
				t.Logf("  found: %s", e.Name())
			}
		}
	}

	// Current file should have latest value
	var current testData
	if err := fs.readJSON(dir, "versioned", &current); err != nil {
		t.Fatalf("readJSON current failed: %v", err)
	}
	if current.Value != 5 {
		t.Errorf("current value = %d, want 5", current.Value)
	}
}

func TestFileStore_Versioning_ZeroDisablesVersioning(t *testing.T) {
	fs := newTestFileStoreVersions(t, 0) // No versioning

	type testData struct {
		Value int `json:"value"`
	}

	dir := filepath.Join(fs.basePath, "kv")

	for i := 0; i < 5; i++ {
		data := testData{Value: i}
		if err := fs.writeJSON(dir, "no-versions", &data, true); err != nil {
			t.Fatalf("writeJSON #%d failed: %v", i, err)
		}
	}

	// Should only have the current file, no version backups
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	count := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "no-versions") {
			count++
		}
	}

	if count != 1 {
		t.Errorf("expected 1 file (no versioning), got %d", count)
	}
}

func TestFileStore_Versioning_VersionContentIsCorrect(t *testing.T) {
	fs := newTestFileStoreVersions(t, 3)

	type testData struct {
		Value int `json:"value"`
	}

	dir := filepath.Join(fs.basePath, "kv")

	// Write values 0, 1, 2, 3
	for i := 0; i <= 3; i++ {
		data := testData{Value: i}
		if err := fs.writeJSON(dir, "content-check", &data, true); err != nil {
			t.Fatalf("writeJSON #%d failed: %v", i, err)
		}
	}

	// Current should be 3
	var current testData
	if err := fs.readJSON(dir, "content-check", &current); err != nil {
		t.Fatalf("readJSON current failed: %v", err)
	}
	if current.Value != 3 {
		t.Errorf("current = %d, want 3", current.Value)
	}

	// v1 should be the most recent backup = value 2
	raw, err := os.ReadFile(filepath.Join(dir, "content-check.json.v1"))
	if err != nil {
		t.Fatalf("read v1 failed: %v", err)
	}
	var v1 testData
	if err := json.Unmarshal(raw, &v1); err != nil {
		t.Fatalf("unmarshal v1 failed: %v", err)
	}
	if v1.Value != 2 {
		t.Errorf("v1 = %d, want 2", v1.Value)
	}
}

// --- Concurrent access tests ---

func TestFileStore_ConcurrentWrites(t *testing.T) {
	fs := newTestFileStore(t)

	type testData struct {
		Value int `json:"value"`
	}

	dir := filepath.Join(fs.basePath, "kv")
	var wg sync.WaitGroup
	const goroutines = 20

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(val int) {
			defer wg.Done()
			data := testData{Value: val}
			if err := fs.writeJSON(dir, "concurrent-key", &data, false); err != nil {
				t.Errorf("concurrent writeJSON failed: %v", err)
			}
		}(i)
	}
	wg.Wait()

	// File should exist and be valid JSON (last writer wins)
	var result testData
	err := fs.readJSON(dir, "concurrent-key", &result)
	if err != nil {
		t.Fatalf("readJSON after concurrent writes failed: %v", err)
	}

	if result.Value < 0 || result.Value >= goroutines {
		t.Errorf("unexpected value %d (expected 0-%d)", result.Value, goroutines-1)
	}

	// No temp files should remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind after concurrent writes: %s", e.Name())
		}
	}
}

func TestFileStore_ConcurrentReadWrite(t *testing.T) {
	fs := newTestFileStore(t)

	type testData struct {
		Value int `json:"value"`
	}

	dir := filepath.Join(fs.basePath, "kv")

	// Seed initial value
	initial := testData{Value: 0}
	if err := fs.writeJSON(dir, "rw-key", &initial, false); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	var wg sync.WaitGroup
	const readers = 10
	const writers = 5

	// Start readers
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				var result testData
				err := fs.readJSON(dir, "rw-key", &result)
				if err != nil {
					// File might be momentarily missing during rename; that's OK for testing
					continue
				}
				// Should always be valid JSON -- never a partial write
				if result.Value < 0 {
					t.Errorf("read invalid value: %d", result.Value)
				}
			}
		}()
	}

	// Start writers
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func(val int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				data := testData{Value: val*100 + j}
				_ = fs.writeJSON(dir, "rw-key", &data, false)
			}
		}(i)
	}

	wg.Wait()
}

// --- PortfolioStorage tests ---

func TestPortfolioStorage_SaveAndGet(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ps := m.PortfolioStorage()

	portfolio := &models.Portfolio{
		ID:       "SMSF",
		Name:     "SMSF",
		Currency: "AUD",
		Holdings: []models.Holding{
			{Ticker: "BHP.AU", Name: "BHP Group", Units: 100, CurrentPrice: 45.50, MarketValue: 4550},
		},
		TotalValue: 4550,
	}

	err := ps.SavePortfolio(ctx, portfolio)
	if err != nil {
		t.Fatalf("SavePortfolio failed: %v", err)
	}

	got, err := ps.GetPortfolio(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetPortfolio failed: %v", err)
	}

	if got.Name != "SMSF" {
		t.Errorf("Name = %q, want %q", got.Name, "SMSF")
	}
	if got.Currency != "AUD" {
		t.Errorf("Currency = %q, want %q", got.Currency, "AUD")
	}
	if len(got.Holdings) != 1 {
		t.Fatalf("Holdings len = %d, want 1", len(got.Holdings))
	}
	if got.Holdings[0].Ticker != "BHP.AU" {
		t.Errorf("Holdings[0].Ticker = %q, want %q", got.Holdings[0].Ticker, "BHP.AU")
	}
	if got.TotalValue != 4550 {
		t.Errorf("TotalValue = %f, want 4550", got.TotalValue)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestPortfolioStorage_GetNotFound(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	_, err := m.PortfolioStorage().GetPortfolio(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent portfolio")
	}
}

func TestPortfolioStorage_ListPortfolios(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ps := m.PortfolioStorage()

	// Empty
	names, err := ps.ListPortfolios(ctx)
	if err != nil {
		t.Fatalf("ListPortfolios failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 portfolios, got %d", len(names))
	}

	// Add two
	for _, name := range []string{"SMSF", "Personal"} {
		p := &models.Portfolio{ID: name, Name: name}
		if err := ps.SavePortfolio(ctx, p); err != nil {
			t.Fatalf("SavePortfolio %s failed: %v", name, err)
		}
	}

	names, err = ps.ListPortfolios(ctx)
	if err != nil {
		t.Fatalf("ListPortfolios failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 portfolios, got %d", len(names))
	}
}

func TestPortfolioStorage_Delete(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ps := m.PortfolioStorage()

	p := &models.Portfolio{ID: "SMSF", Name: "SMSF"}
	if err := ps.SavePortfolio(ctx, p); err != nil {
		t.Fatalf("SavePortfolio failed: %v", err)
	}

	if err := ps.DeletePortfolio(ctx, "SMSF"); err != nil {
		t.Fatalf("DeletePortfolio failed: %v", err)
	}

	_, err := ps.GetPortfolio(ctx, "SMSF")
	if err == nil {
		t.Error("expected error after delete")
	}
}

// --- MarketDataStorage tests ---

func TestMarketDataStorage_SaveAndGet(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ms := m.MarketDataStorage()

	md := &models.MarketData{
		Ticker:   "BHP.AU",
		Exchange: "AU",
		Name:     "BHP Group",
		EOD: []models.EODBar{
			{Date: time.Now(), Open: 44, High: 46, Low: 43, Close: 45.50, Volume: 1000000},
		},
	}

	if err := ms.SaveMarketData(ctx, md); err != nil {
		t.Fatalf("SaveMarketData failed: %v", err)
	}

	got, err := ms.GetMarketData(ctx, "BHP.AU")
	if err != nil {
		t.Fatalf("GetMarketData failed: %v", err)
	}

	if got.Ticker != "BHP.AU" {
		t.Errorf("Ticker = %q, want %q", got.Ticker, "BHP.AU")
	}
	if got.Exchange != "AU" {
		t.Errorf("Exchange = %q, want %q", got.Exchange, "AU")
	}
	if len(got.EOD) != 1 {
		t.Fatalf("EOD len = %d, want 1", len(got.EOD))
	}
	if !got.LastUpdated.After(time.Time{}) {
		t.Error("LastUpdated should be set")
	}
}

func TestMarketDataStorage_GetBatch(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ms := m.MarketDataStorage()

	tickers := []string{"BHP.AU", "CBA.AU", "WES.AU"}
	for _, ticker := range tickers {
		md := &models.MarketData{Ticker: ticker, Exchange: "AU", Name: ticker}
		if err := ms.SaveMarketData(ctx, md); err != nil {
			t.Fatalf("SaveMarketData %s failed: %v", ticker, err)
		}
	}

	// Batch get 2 of 3
	batch, err := ms.GetMarketDataBatch(ctx, []string{"BHP.AU", "WES.AU"})
	if err != nil {
		t.Fatalf("GetMarketDataBatch failed: %v", err)
	}
	if len(batch) != 2 {
		t.Errorf("batch len = %d, want 2", len(batch))
	}

	// Batch with nonexistent should return partial results
	batch, err = ms.GetMarketDataBatch(ctx, []string{"BHP.AU", "NONEXISTENT"})
	if err != nil {
		t.Fatalf("GetMarketDataBatch with missing failed: %v", err)
	}
	if len(batch) != 1 {
		t.Errorf("batch len = %d, want 1 (should skip missing)", len(batch))
	}
}

func TestMarketDataStorage_GetStaleTickers(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ms := m.MarketDataStorage()

	// Fresh ticker (SaveMarketData always sets LastUpdated = now)
	fresh := &models.MarketData{Ticker: "BHP.AU", Exchange: "AU"}
	if err := ms.SaveMarketData(ctx, fresh); err != nil {
		t.Fatalf("SaveMarketData fresh failed: %v", err)
	}

	// Stale ticker: save first, then overwrite JSON directly with old timestamp
	stale := &models.MarketData{Ticker: "CBA.AU", Exchange: "AU"}
	if err := ms.SaveMarketData(ctx, stale); err != nil {
		t.Fatalf("SaveMarketData stale failed: %v", err)
	}
	// Overwrite with stale timestamp (bypass SaveMarketData to simulate aged data)
	stale.LastUpdated = time.Now().Add(-2 * time.Hour)
	marketDir := filepath.Join(m.dataStore.basePath, "market")
	if err := m.dataStore.writeJSON(marketDir, "CBA.AU", stale, false); err != nil {
		t.Fatalf("direct writeJSON for stale ticker failed: %v", err)
	}

	// Different exchange: also stale but on US exchange
	other := &models.MarketData{Ticker: "AAPL.US", Exchange: "US"}
	if err := ms.SaveMarketData(ctx, other); err != nil {
		t.Fatalf("SaveMarketData other failed: %v", err)
	}
	other.LastUpdated = time.Now().Add(-2 * time.Hour)
	if err := m.dataStore.writeJSON(marketDir, "AAPL.US", other, false); err != nil {
		t.Fatalf("direct writeJSON for other ticker failed: %v", err)
	}

	// Stale threshold: 1 hour
	staleTickers, err := ms.GetStaleTickers(ctx, "AU", 3600)
	if err != nil {
		t.Fatalf("GetStaleTickers failed: %v", err)
	}

	if len(staleTickers) != 1 {
		t.Fatalf("expected 1 stale AU ticker, got %d: %v", len(staleTickers), staleTickers)
	}
	if staleTickers[0] != "CBA.AU" {
		t.Errorf("stale ticker = %q, want CBA.AU", staleTickers[0])
	}
}

// --- SignalStorage tests ---

func TestSignalStorage_SaveAndGet(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ss := m.SignalStorage()

	signals := &models.TickerSignals{
		Ticker: "BHP.AU",
		Price:  models.PriceSignals{Current: 45.50, SMA20: 44.0, SMA50: 43.0},
		Technical: models.TechnicalSignals{
			RSI:       55.5,
			RSISignal: "neutral",
		},
		Trend:            models.TrendBullish,
		TrendDescription: "Uptrend confirmed",
	}

	if err := ss.SaveSignals(ctx, signals); err != nil {
		t.Fatalf("SaveSignals failed: %v", err)
	}

	got, err := ss.GetSignals(ctx, "BHP.AU")
	if err != nil {
		t.Fatalf("GetSignals failed: %v", err)
	}

	if got.Ticker != "BHP.AU" {
		t.Errorf("Ticker = %q, want %q", got.Ticker, "BHP.AU")
	}
	if got.Price.Current != 45.50 {
		t.Errorf("Price.Current = %f, want 45.50", got.Price.Current)
	}
	if got.Technical.RSI != 55.5 {
		t.Errorf("Technical.RSI = %f, want 55.5", got.Technical.RSI)
	}
	if got.Trend != models.TrendBullish {
		t.Errorf("Trend = %q, want %q", got.Trend, models.TrendBullish)
	}
	if got.ComputeTimestamp.IsZero() {
		t.Error("ComputeTimestamp should be set")
	}
}

func TestSignalStorage_GetBatch(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ss := m.SignalStorage()

	for _, ticker := range []string{"BHP.AU", "CBA.AU"} {
		sig := &models.TickerSignals{Ticker: ticker}
		if err := ss.SaveSignals(ctx, sig); err != nil {
			t.Fatalf("SaveSignals %s failed: %v", ticker, err)
		}
	}

	batch, err := ss.GetSignalsBatch(ctx, []string{"BHP.AU", "CBA.AU", "MISSING"})
	if err != nil {
		t.Fatalf("GetSignalsBatch failed: %v", err)
	}
	if len(batch) != 2 {
		t.Errorf("batch len = %d, want 2", len(batch))
	}
}

// --- KeyValueStorage tests ---

func TestKVStorage_SetAndGet(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	kv := m.KeyValueStorage()

	if err := kv.Set(ctx, "test_key", "test_value"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := kv.Get(ctx, "test_key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "test_value" {
		t.Errorf("Get = %q, want %q", val, "test_value")
	}
}

func TestKVStorage_GetNotFound(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	_, err := m.KeyValueStorage().Get(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestKVStorage_Delete(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	kv := m.KeyValueStorage()

	if err := kv.Set(ctx, "delete_me", "value"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := kv.Delete(ctx, "delete_me"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := kv.Get(ctx, "delete_me")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestKVStorage_DeleteNonexistent(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	// Deleting a non-existent key should not error
	err := m.KeyValueStorage().Delete(ctx, "nope")
	if err != nil {
		t.Errorf("Delete non-existent should not error, got: %v", err)
	}
}

func TestKVStorage_GetAll(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	kv := m.KeyValueStorage()

	if err := kv.Set(ctx, "key1", "val1"); err != nil {
		t.Fatalf("Set key1 failed: %v", err)
	}
	if err := kv.Set(ctx, "key2", "val2"); err != nil {
		t.Fatalf("Set key2 failed: %v", err)
	}

	all, err := kv.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("GetAll len = %d, want 2", len(all))
	}
	if all["key1"] != "val1" {
		t.Errorf("key1 = %q, want %q", all["key1"], "val1")
	}
	if all["key2"] != "val2" {
		t.Errorf("key2 = %q, want %q", all["key2"], "val2")
	}
}

func TestKVStorage_GetAll_Empty(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	all, err := m.KeyValueStorage().GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected empty map, got %d entries", len(all))
	}
}

// --- ReportStorage tests ---

func TestReportStorage_SaveAndGet(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	rs := m.ReportStorage()

	report := &models.PortfolioReport{
		Portfolio:       "SMSF",
		GeneratedAt:     time.Now(),
		SummaryMarkdown: "# Portfolio Summary\n\nAll good.",
		TickerReports: []models.TickerReport{
			{Ticker: "BHP.AU", Name: "BHP Group", Markdown: "## BHP\n\nStrong."},
		},
		Tickers: []string{"BHP.AU"},
	}

	if err := rs.SaveReport(ctx, report); err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}

	got, err := rs.GetReport(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetReport failed: %v", err)
	}

	if got.Portfolio != "SMSF" {
		t.Errorf("Portfolio = %q, want %q", got.Portfolio, "SMSF")
	}
	if len(got.TickerReports) != 1 {
		t.Fatalf("TickerReports len = %d, want 1", len(got.TickerReports))
	}
	if got.TickerReports[0].Ticker != "BHP.AU" {
		t.Errorf("TickerReports[0].Ticker = %q, want %q", got.TickerReports[0].Ticker, "BHP.AU")
	}
}

func TestReportStorage_ListAndDelete(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	rs := m.ReportStorage()

	for _, name := range []string{"SMSF", "Personal"} {
		report := &models.PortfolioReport{Portfolio: name, GeneratedAt: time.Now()}
		if err := rs.SaveReport(ctx, report); err != nil {
			t.Fatalf("SaveReport %s failed: %v", name, err)
		}
	}

	names, err := rs.ListReports(ctx)
	if err != nil {
		t.Fatalf("ListReports failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("ListReports len = %d, want 2", len(names))
	}

	if err := rs.DeleteReport(ctx, "SMSF"); err != nil {
		t.Fatalf("DeleteReport failed: %v", err)
	}

	names, err = rs.ListReports(ctx)
	if err != nil {
		t.Fatalf("ListReports after delete failed: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("ListReports len = %d, want 1 after delete", len(names))
	}
}

// --- StrategyStorage tests ---

func TestStrategyStorage_SaveGetUpdateDelete(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ss := m.StrategyStorage()

	strategy := &models.PortfolioStrategy{
		PortfolioName:      "SMSF",
		AccountType:        models.AccountTypeSMSF,
		InvestmentUniverse: []string{"AU", "US"},
		RiskAppetite: models.RiskAppetite{
			Level:          "moderate",
			MaxDrawdownPct: 15.0,
			Description:    "Balanced growth",
		},
		TargetReturns: models.TargetReturns{
			AnnualPct: 8.5,
			Timeframe: "3-5 years",
		},
		IncomeRequirements: models.IncomeRequirements{
			DividendYieldPct: 4.0,
			Description:      "Franked dividends",
		},
		SectorPreferences: models.SectorPreferences{
			Preferred: []string{"Financials", "Healthcare"},
			Excluded:  []string{"Gambling"},
		},
		PositionSizing: models.PositionSizing{
			MaxPositionPct: 10.0,
			MaxSectorPct:   30.0,
		},
		ReferenceStrategies: []models.ReferenceStrategy{
			{Name: "Dividend Growth", Description: "Growing dividends"},
		},
		RebalanceFrequency: "quarterly",
		Notes:              "Test strategy",
	}

	// Save new
	if err := ss.SaveStrategy(ctx, strategy); err != nil {
		t.Fatalf("SaveStrategy failed: %v", err)
	}

	got, err := ss.GetStrategy(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetStrategy failed: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if got.AccountType != models.AccountTypeSMSF {
		t.Errorf("AccountType = %q, want %q", got.AccountType, models.AccountTypeSMSF)
	}
	if got.RiskAppetite.Level != "moderate" {
		t.Errorf("RiskAppetite.Level = %q, want %q", got.RiskAppetite.Level, "moderate")
	}
	if got.TargetReturns.AnnualPct != 8.5 {
		t.Errorf("TargetReturns.AnnualPct = %f, want 8.5", got.TargetReturns.AnnualPct)
	}
	if got.Disclaimer != models.DefaultDisclaimer {
		t.Error("Disclaimer should default to DefaultDisclaimer")
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	// Update
	originalCreatedAt := got.CreatedAt
	time.Sleep(10 * time.Millisecond)

	updated := &models.PortfolioStrategy{
		PortfolioName: "SMSF",
		RiskAppetite:  models.RiskAppetite{Level: "aggressive"},
		Notes:         "Updated notes",
	}
	if err := ss.SaveStrategy(ctx, updated); err != nil {
		t.Fatalf("SaveStrategy update failed: %v", err)
	}

	got, err = ss.GetStrategy(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetStrategy after update failed: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Version = %d, want 2 after update", got.Version)
	}
	if !got.CreatedAt.Equal(originalCreatedAt) {
		t.Error("CreatedAt should be preserved on update")
	}
	if !got.UpdatedAt.After(originalCreatedAt) {
		t.Error("UpdatedAt should advance on update")
	}

	// Delete
	if err := ss.DeleteStrategy(ctx, "SMSF"); err != nil {
		t.Fatalf("DeleteStrategy failed: %v", err)
	}
	_, err = ss.GetStrategy(ctx, "SMSF")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestStrategyStorage_ListStrategies(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ss := m.StrategyStorage()

	// Empty
	names, err := ss.ListStrategies(ctx)
	if err != nil {
		t.Fatalf("ListStrategies failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 strategies, got %d", len(names))
	}

	// Add two
	for _, name := range []string{"SMSF", "Personal"} {
		s := &models.PortfolioStrategy{PortfolioName: name, AccountType: models.AccountTypeTrading}
		if err := ss.SaveStrategy(ctx, s); err != nil {
			t.Fatalf("SaveStrategy %s failed: %v", name, err)
		}
	}

	names, err = ss.ListStrategies(ctx)
	if err != nil {
		t.Fatalf("ListStrategies failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 strategies, got %d", len(names))
	}
}

// --- PlanStorage tests ---

func TestPlanStorage_SaveGetDelete(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ps := m.PlanStorage()

	deadline := time.Now().Add(30 * 24 * time.Hour)
	plan := &models.PortfolioPlan{
		PortfolioName: "SMSF",
		Items: []models.PlanItem{
			{
				ID:          "plan-1",
				Type:        models.PlanItemTypeTime,
				Description: "Rebalance portfolio",
				Status:      models.PlanItemStatusPending,
				Deadline:    &deadline,
			},
		},
		Notes: "Q2 plan",
	}

	if err := ps.SavePlan(ctx, plan); err != nil {
		t.Fatalf("SavePlan failed: %v", err)
	}

	got, err := ps.GetPlan(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetPlan failed: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items len = %d, want 1", len(got.Items))
	}
	if got.Items[0].ID != "plan-1" {
		t.Errorf("Items[0].ID = %q, want %q", got.Items[0].ID, "plan-1")
	}
	if got.Items[0].Deadline == nil {
		t.Error("Items[0].Deadline should be set")
	}

	// Version increment
	plan.Items = append(plan.Items, models.PlanItem{ID: "plan-2", Type: models.PlanItemTypeEvent, Description: "Buy dip"})
	if err := ps.SavePlan(ctx, plan); err != nil {
		t.Fatalf("SavePlan update failed: %v", err)
	}
	got, err = ps.GetPlan(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetPlan after update failed: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("Version = %d, want 2", got.Version)
	}

	// Delete
	if err := ps.DeletePlan(ctx, "SMSF"); err != nil {
		t.Fatalf("DeletePlan failed: %v", err)
	}
	_, err = ps.GetPlan(ctx, "SMSF")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestPlanStorage_ListPlans(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ps := m.PlanStorage()

	for _, name := range []string{"SMSF", "Personal"} {
		p := &models.PortfolioPlan{PortfolioName: name}
		if err := ps.SavePlan(ctx, p); err != nil {
			t.Fatalf("SavePlan %s failed: %v", name, err)
		}
	}

	names, err := ps.ListPlans(ctx)
	if err != nil {
		t.Fatalf("ListPlans failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 plans, got %d", len(names))
	}
}

// --- WatchlistStorage tests ---

func TestWatchlistStorage_SaveGetDelete(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ws := m.WatchlistStorage()

	watchlist := &models.PortfolioWatchlist{
		PortfolioName: "SMSF",
		Items: []models.WatchlistItem{
			{
				Ticker:     "SGI.AU",
				Name:       "Stealth Global",
				Verdict:    models.WatchlistVerdictWatch,
				Reason:     "Revenue growing but acquisition-driven",
				KeyMetrics: "PE 12, Rev $180M",
				ReviewedAt: time.Now(),
			},
		},
	}

	if err := ws.SaveWatchlist(ctx, watchlist); err != nil {
		t.Fatalf("SaveWatchlist failed: %v", err)
	}

	got, err := ws.GetWatchlist(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetWatchlist failed: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items len = %d, want 1", len(got.Items))
	}
	if got.Items[0].Ticker != "SGI.AU" {
		t.Errorf("Items[0].Ticker = %q, want %q", got.Items[0].Ticker, "SGI.AU")
	}
	if got.Items[0].Verdict != models.WatchlistVerdictWatch {
		t.Errorf("Items[0].Verdict = %q, want %q", got.Items[0].Verdict, models.WatchlistVerdictWatch)
	}

	// Delete
	if err := ws.DeleteWatchlist(ctx, "SMSF"); err != nil {
		t.Fatalf("DeleteWatchlist failed: %v", err)
	}
	_, err = ws.GetWatchlist(ctx, "SMSF")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestWatchlistStorage_ListWatchlists(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ws := m.WatchlistStorage()

	for _, name := range []string{"SMSF", "Personal"} {
		w := &models.PortfolioWatchlist{PortfolioName: name}
		if err := ws.SaveWatchlist(ctx, w); err != nil {
			t.Fatalf("SaveWatchlist %s failed: %v", name, err)
		}
	}

	names, err := ws.ListWatchlists(ctx)
	if err != nil {
		t.Fatalf("ListWatchlists failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 watchlists, got %d", len(names))
	}
}

// --- SearchHistoryStorage tests ---

func TestSearchHistory_SaveAndGet(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	sh := m.SearchHistoryStorage()

	record := &models.SearchRecord{
		ID:          "search-test-1",
		Type:        "screen",
		Exchange:    "AU",
		Filters:     `{"exchange":"AU","max_pe":20}`,
		ResultCount: 5,
		Results:     `[{"ticker":"BHP.AU"}]`,
		CreatedAt:   time.Now(),
	}

	if err := sh.SaveSearch(ctx, record); err != nil {
		t.Fatalf("SaveSearch failed: %v", err)
	}

	got, err := sh.GetSearch(ctx, "search-test-1")
	if err != nil {
		t.Fatalf("GetSearch failed: %v", err)
	}

	if got.Type != "screen" {
		t.Errorf("Type = %q, want %q", got.Type, "screen")
	}
	if got.Exchange != "AU" {
		t.Errorf("Exchange = %q, want %q", got.Exchange, "AU")
	}
	if got.ResultCount != 5 {
		t.Errorf("ResultCount = %d, want 5", got.ResultCount)
	}
}

func TestSearchHistory_AutoGeneratesID(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	sh := m.SearchHistoryStorage()

	record := &models.SearchRecord{
		Type:      "snipe",
		Exchange:  "US",
		Filters:   `{}`,
		Results:   `[]`,
		CreatedAt: time.Now(),
	}

	if err := sh.SaveSearch(ctx, record); err != nil {
		t.Fatalf("SaveSearch failed: %v", err)
	}

	if record.ID == "" {
		t.Fatal("expected auto-generated ID")
	}

	got, err := sh.GetSearch(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetSearch by auto-ID failed: %v", err)
	}
	if got.Exchange != "US" {
		t.Errorf("Exchange = %q, want %q", got.Exchange, "US")
	}
}

func TestSearchHistory_Delete(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	sh := m.SearchHistoryStorage()

	record := &models.SearchRecord{
		ID:        "search-delete-test",
		Type:      "funnel",
		Exchange:  "AU",
		Filters:   `{}`,
		Results:   `[]`,
		CreatedAt: time.Now(),
	}

	if err := sh.SaveSearch(ctx, record); err != nil {
		t.Fatalf("SaveSearch failed: %v", err)
	}

	if err := sh.DeleteSearch(ctx, "search-delete-test"); err != nil {
		t.Fatalf("DeleteSearch failed: %v", err)
	}

	_, err := sh.GetSearch(ctx, "search-delete-test")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestSearchHistory_ListAll(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	sh := m.SearchHistoryStorage()

	now := time.Now()
	records := []*models.SearchRecord{
		{ID: "search-1", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-3 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "search-2", Type: "snipe", Exchange: "US", CreatedAt: now.Add(-2 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "search-3", Type: "funnel", Exchange: "AU", CreatedAt: now.Add(-1 * time.Hour), Results: `[]`, Filters: `{}`},
	}

	for _, r := range records {
		if err := sh.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch failed: %v", err)
		}
	}

	results, err := sh.ListSearches(ctx, interfaces.SearchListOptions{})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Most recent first
	if results[0].ID != "search-3" {
		t.Errorf("expected most recent first (search-3), got %s", results[0].ID)
	}
	if results[2].ID != "search-1" {
		t.Errorf("expected oldest last (search-1), got %s", results[2].ID)
	}
}

func TestSearchHistory_ListFilterByType(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	sh := m.SearchHistoryStorage()

	now := time.Now()
	records := []*models.SearchRecord{
		{ID: "s-1", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-3 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "s-2", Type: "snipe", Exchange: "AU", CreatedAt: now.Add(-2 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "s-3", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-1 * time.Hour), Results: `[]`, Filters: `{}`},
	}

	for _, r := range records {
		if err := sh.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch failed: %v", err)
		}
	}

	results, err := sh.ListSearches(ctx, interfaces.SearchListOptions{Type: "screen"})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 screen results, got %d", len(results))
	}
}

func TestSearchHistory_ListFilterByExchange(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	sh := m.SearchHistoryStorage()

	now := time.Now()
	records := []*models.SearchRecord{
		{ID: "e-1", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-2 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "e-2", Type: "screen", Exchange: "US", CreatedAt: now.Add(-1 * time.Hour), Results: `[]`, Filters: `{}`},
	}

	for _, r := range records {
		if err := sh.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch failed: %v", err)
		}
	}

	results, err := sh.ListSearches(ctx, interfaces.SearchListOptions{Exchange: "AU"})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 AU result, got %d", len(results))
	}
	if results[0].ID != "e-1" {
		t.Errorf("expected e-1, got %s", results[0].ID)
	}
}

func TestSearchHistory_ListFilterCombined(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	sh := m.SearchHistoryStorage()

	now := time.Now()
	records := []*models.SearchRecord{
		{ID: "c-1", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-3 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "c-2", Type: "snipe", Exchange: "AU", CreatedAt: now.Add(-2 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "c-3", Type: "screen", Exchange: "US", CreatedAt: now.Add(-1 * time.Hour), Results: `[]`, Filters: `{}`},
	}

	for _, r := range records {
		if err := sh.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch failed: %v", err)
		}
	}

	results, err := sh.ListSearches(ctx, interfaces.SearchListOptions{Type: "screen", Exchange: "AU"})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (screen+AU), got %d", len(results))
	}
	if results[0].ID != "c-1" {
		t.Errorf("expected c-1, got %s", results[0].ID)
	}
}

func TestSearchHistory_ListWithLimit(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	sh := m.SearchHistoryStorage()

	now := time.Now()
	for i := 0; i < 10; i++ {
		r := &models.SearchRecord{
			ID:        fmt.Sprintf("lim-%d", i),
			Type:      "screen",
			Exchange:  "AU",
			CreatedAt: now.Add(time.Duration(-10+i) * time.Hour),
			Results:   `[]`,
			Filters:   `{}`,
		}
		if err := sh.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch failed: %v", err)
		}
	}

	results, err := sh.ListSearches(ctx, interfaces.SearchListOptions{Limit: 3})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results with limit, got %d", len(results))
	}
}

func TestSearchHistory_Pruning(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	sh := m.SearchHistoryStorage()

	// Save 55 records (over the 50 max limit)
	now := time.Now()
	for i := 0; i < 55; i++ {
		r := &models.SearchRecord{
			ID:        fmt.Sprintf("prune-%03d", i),
			Type:      "screen",
			Exchange:  "AU",
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
			Results:   `[]`,
			Filters:   `{}`,
		}
		if err := sh.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch %d failed: %v", i, err)
		}
	}

	// List all - should have been pruned to 50
	results, err := sh.ListSearches(ctx, interfaces.SearchListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) > 50 {
		t.Errorf("expected <= 50 results after pruning, got %d", len(results))
	}
}

// --- WriteRaw tests ---

func TestFileStore_WriteRaw(t *testing.T) {
	fs := newTestFileStore(t)

	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header
	err := fs.WriteRaw("charts", "smsf-growth.png", data)
	if err != nil {
		t.Fatalf("WriteRaw failed: %v", err)
	}

	// Read back and verify
	got, err := os.ReadFile(filepath.Join(fs.basePath, "charts", "smsf-growth.png"))
	if err != nil {
		t.Fatalf("Failed to read back written file: %v", err)
	}
	if len(got) != len(data) {
		t.Fatalf("Got %d bytes, want %d", len(got), len(data))
	}
	for i, b := range data {
		if got[i] != b {
			t.Errorf("byte %d: got 0x%02X, want 0x%02X", i, got[i], b)
		}
	}
}

func TestFileStore_WriteRaw_SanitizesKey(t *testing.T) {
	fs := newTestFileStore(t)

	// Key with path separators should be sanitized
	err := fs.WriteRaw("charts", "my/bad:key\\file.png", []byte("data"))
	if err != nil {
		t.Fatalf("WriteRaw failed: %v", err)
	}

	// Should exist with sanitized name
	sanitized := filepath.Join(fs.basePath, "charts", "my_bad_key_file.png")
	if _, err := os.Stat(sanitized); os.IsNotExist(err) {
		t.Fatalf("Expected sanitized file at %s, but it does not exist", sanitized)
	}
}

func TestFileStore_WriteRaw_Atomic_NoTempFileLeftBehind(t *testing.T) {
	fs := newTestFileStore(t)

	data := []byte("binary data")
	err := fs.WriteRaw("charts", "test.png", data)
	if err != nil {
		t.Fatalf("WriteRaw failed: %v", err)
	}

	// Verify no .tmp files remain
	dir := filepath.Join(fs.basePath, "charts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestFileStore_WriteRaw_OverwritesExisting(t *testing.T) {
	fs := newTestFileStore(t)

	// Write first version
	if err := fs.WriteRaw("charts", "test.png", []byte("version1")); err != nil {
		t.Fatalf("First WriteRaw failed: %v", err)
	}

	// Write second version
	if err := fs.WriteRaw("charts", "test.png", []byte("version2-longer")); err != nil {
		t.Fatalf("Second WriteRaw failed: %v", err)
	}

	// Should have the second version
	got, err := os.ReadFile(filepath.Join(fs.basePath, "charts", "test.png"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != "version2-longer" {
		t.Errorf("Got %q, want %q", string(got), "version2-longer")
	}
}

func TestFileStore_WriteRaw_PathTraversal(t *testing.T) {
	fs := newTestFileStore(t)

	// Attempt path traversal: "../evil" should be sanitized to "__evil"
	data := []byte("should not escape")
	err := fs.WriteRaw("charts", "../evil.png", data)
	if err != nil {
		t.Fatalf("WriteRaw failed: %v", err)
	}

	// File should be written inside the charts directory, not the parent
	chartsDir := filepath.Join(fs.basePath, "charts")
	sanitizedPath := filepath.Join(chartsDir, "__evil.png")
	if _, err := os.Stat(sanitizedPath); os.IsNotExist(err) {
		t.Fatalf("Expected sanitized file at %s, but it does not exist", sanitizedPath)
	}

	// Verify nothing was written to the parent directory
	parentEntries, err := os.ReadDir(fs.basePath)
	if err != nil {
		t.Fatalf("ReadDir parent failed: %v", err)
	}
	for _, e := range parentEntries {
		if e.Name() == "evil.png" || e.Name() == "__evil.png" {
			t.Errorf("Path traversal escaped charts dir: found %s in parent", e.Name())
		}
	}
}

func TestFileStore_PurgeAllFiles(t *testing.T) {
	fs := newTestFileStore(t)
	dir := filepath.Join(fs.basePath, "charts")
	os.MkdirAll(dir, 0755)

	// Write some files
	for _, name := range []string{"a.png", "b.png", "c.dat"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644); err != nil {
			t.Fatalf("WriteFile %s failed: %v", name, err)
		}
	}

	count := fs.purgeAllFiles(dir)
	if count != 3 {
		t.Errorf("purgeAllFiles returned %d, want 3", count)
	}

	// Verify directory is empty
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Expected empty directory after purge, got %d entries", len(entries))
	}
}

func TestFileStore_PurgeAllFiles_EmptyDir(t *testing.T) {
	fs := newTestFileStore(t)
	dir := filepath.Join(fs.basePath, "charts")
	os.MkdirAll(dir, 0755)

	count := fs.purgeAllFiles(dir)
	if count != 0 {
		t.Errorf("purgeAllFiles on empty dir returned %d, want 0", count)
	}
}

// --- PurgeDerivedData tests ---

func TestPurgeDerivedData_DeletesDerivedPreservesUserData(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	// Seed derived data
	portfolio := &models.Portfolio{ID: "SMSF", Name: "SMSF"}
	if err := m.PortfolioStorage().SavePortfolio(ctx, portfolio); err != nil {
		t.Fatalf("SavePortfolio failed: %v", err)
	}

	md := &models.MarketData{Ticker: "BHP.AU", Exchange: "AU"}
	if err := m.MarketDataStorage().SaveMarketData(ctx, md); err != nil {
		t.Fatalf("SaveMarketData failed: %v", err)
	}

	sig := &models.TickerSignals{Ticker: "BHP.AU"}
	if err := m.SignalStorage().SaveSignals(ctx, sig); err != nil {
		t.Fatalf("SaveSignals failed: %v", err)
	}

	report := &models.PortfolioReport{Portfolio: "SMSF", GeneratedAt: time.Now()}
	if err := m.ReportStorage().SaveReport(ctx, report); err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}

	search := &models.SearchRecord{ID: "search-1", Type: "screen", Exchange: "AU", CreatedAt: time.Now(), Results: `[]`, Filters: `{}`}
	if err := m.SearchHistoryStorage().SaveSearch(ctx, search); err != nil {
		t.Fatalf("SaveSearch failed: %v", err)
	}

	// Seed chart file (derived binary data)
	if err := m.WriteRaw("charts", "smsf-growth.png", []byte("fake png")); err != nil {
		t.Fatalf("WriteRaw chart failed: %v", err)
	}

	// Seed user data (should be preserved)
	strategy := &models.PortfolioStrategy{PortfolioName: "SMSF", AccountType: models.AccountTypeSMSF}
	if err := m.StrategyStorage().SaveStrategy(ctx, strategy); err != nil {
		t.Fatalf("SaveStrategy failed: %v", err)
	}

	if err := m.KeyValueStorage().Set(ctx, "test_key", "test_value"); err != nil {
		t.Fatalf("KV Set failed: %v", err)
	}

	plan := &models.PortfolioPlan{PortfolioName: "SMSF"}
	if err := m.PlanStorage().SavePlan(ctx, plan); err != nil {
		t.Fatalf("SavePlan failed: %v", err)
	}

	watchlist := &models.PortfolioWatchlist{PortfolioName: "SMSF"}
	if err := m.WatchlistStorage().SaveWatchlist(ctx, watchlist); err != nil {
		t.Fatalf("SaveWatchlist failed: %v", err)
	}

	// Purge
	counts, err := m.PurgeDerivedData(ctx)
	if err != nil {
		t.Fatalf("PurgeDerivedData failed: %v", err)
	}

	// Verify counts
	if counts["portfolios"] != 1 {
		t.Errorf("portfolios purged = %d, want 1", counts["portfolios"])
	}
	if counts["market_data"] != 1 {
		t.Errorf("market_data purged = %d, want 1", counts["market_data"])
	}
	if counts["signals"] != 1 {
		t.Errorf("signals purged = %d, want 1", counts["signals"])
	}
	if counts["reports"] != 1 {
		t.Errorf("reports purged = %d, want 1", counts["reports"])
	}
	if counts["search_history"] != 1 {
		t.Errorf("search_history purged = %d, want 1", counts["search_history"])
	}
	if counts["charts"] != 1 {
		t.Errorf("charts purged = %d, want 1", counts["charts"])
	}

	// Verify derived data is gone
	_, err = m.PortfolioStorage().GetPortfolio(ctx, "SMSF")
	if err == nil {
		t.Error("portfolio should be purged")
	}
	_, err = m.MarketDataStorage().GetMarketData(ctx, "BHP.AU")
	if err == nil {
		t.Error("market data should be purged")
	}
	_, err = m.SignalStorage().GetSignals(ctx, "BHP.AU")
	if err == nil {
		t.Error("signals should be purged")
	}
	_, err = m.ReportStorage().GetReport(ctx, "SMSF")
	if err == nil {
		t.Error("report should be purged")
	}

	// Verify chart file is purged
	chartFile := filepath.Join(m.DataPath(), "charts", "smsf-growth.png")
	if _, statErr := os.Stat(chartFile); !os.IsNotExist(statErr) {
		t.Error("chart file should be purged")
	}

	// Verify user data is preserved
	gotStrategy, err := m.StrategyStorage().GetStrategy(ctx, "SMSF")
	if err != nil {
		t.Fatalf("strategy should be preserved: %v", err)
	}
	if gotStrategy.PortfolioName != "SMSF" {
		t.Errorf("strategy name = %q, want %q", gotStrategy.PortfolioName, "SMSF")
	}

	val, err := m.KeyValueStorage().Get(ctx, "test_key")
	if err != nil {
		t.Fatalf("kv should be preserved: %v", err)
	}
	if val != "test_value" {
		t.Errorf("kv value = %q, want %q", val, "test_value")
	}

	gotPlan, err := m.PlanStorage().GetPlan(ctx, "SMSF")
	if err != nil {
		t.Fatalf("plan should be preserved: %v", err)
	}
	if gotPlan.PortfolioName != "SMSF" {
		t.Errorf("plan name = %q, want %q", gotPlan.PortfolioName, "SMSF")
	}

	gotWatchlist, err := m.WatchlistStorage().GetWatchlist(ctx, "SMSF")
	if err != nil {
		t.Fatalf("watchlist should be preserved: %v", err)
	}
	if gotWatchlist.PortfolioName != "SMSF" {
		t.Errorf("watchlist name = %q, want %q", gotWatchlist.PortfolioName, "SMSF")
	}
}

func TestPurgeDerivedData_PurgesCharts(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	// Write a chart file via WriteRaw
	if err := m.WriteRaw("charts", "smsf-growth.png", []byte("fake-png-data")); err != nil {
		t.Fatalf("WriteRaw failed: %v", err)
	}

	// Verify chart exists
	chartPath := filepath.Join(m.DataPath(), "charts", "smsf-growth.png")
	if _, err := os.Stat(chartPath); os.IsNotExist(err) {
		t.Fatalf("Chart file should exist before purge")
	}

	// Purge
	counts, err := m.PurgeDerivedData(ctx)
	if err != nil {
		t.Fatalf("PurgeDerivedData failed: %v", err)
	}

	if counts["charts"] != 1 {
		t.Errorf("charts purged = %d, want 1", counts["charts"])
	}

	// Verify chart is gone
	if _, err := os.Stat(chartPath); !os.IsNotExist(err) {
		t.Error("Chart file should be purged")
	}
}

func TestPurgeDerivedData_EmptyDB(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	counts, err := m.PurgeDerivedData(ctx)
	if err != nil {
		t.Fatalf("PurgeDerivedData on empty store failed: %v", err)
	}

	for typ, count := range counts {
		if count != 0 {
			t.Errorf("%s = %d, want 0 on empty store", typ, count)
		}
	}
}

// --- Config tests ---

func TestFileConfig_DefaultVersions(t *testing.T) {
	dir := t.TempDir()
	logger := common.NewLogger("error")

	// Versions = 0 should be accepted (means no versioning)
	fs, err := NewFileStore(logger, &common.FileConfig{Path: dir, Versions: 0})
	if err != nil {
		t.Fatalf("NewFileStore with Versions=0 failed: %v", err)
	}
	if fs.versions != 0 {
		t.Errorf("versions = %d, want 0", fs.versions)
	}
}

func TestFileConfig_NegativeVersionsTreatedAsZero(t *testing.T) {
	dir := t.TempDir()
	logger := common.NewLogger("error")

	fs, err := NewFileStore(logger, &common.FileConfig{Path: dir, Versions: -1})
	if err != nil {
		t.Fatalf("NewFileStore with Versions=-1 failed: %v", err)
	}
	if fs.versions != 0 {
		t.Errorf("versions = %d, want 0 (negative should be treated as 0)", fs.versions)
	}
}

// --- JSON round-trip tests for complex model types ---

func TestJSONRoundTrip_Portfolio(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ps := m.PortfolioStorage()

	portfolio := &models.Portfolio{
		ID:           "SMSF",
		Name:         "SMSF",
		NavexaID:     "abc-123",
		Currency:     "AUD",
		TotalValue:   50000.50,
		TotalCost:    40000.00,
		TotalGain:    10000.50,
		TotalGainPct: 25.001,
		Holdings: []models.Holding{
			{
				Ticker:       "BHP.AU",
				Exchange:     "AU",
				Name:         "BHP Group",
				Units:        100,
				AvgCost:      40.00,
				CurrentPrice: 45.50,
				MarketValue:  4550.00,
				GainLoss:     550.00,
				GainLossPct:  13.75,
				Weight:       9.1,
			},
		},
		LastSynced: time.Now().Truncate(time.Second),
		CreatedAt:  time.Now().Add(-24 * time.Hour).Truncate(time.Second),
	}

	if err := ps.SavePortfolio(ctx, portfolio); err != nil {
		t.Fatalf("SavePortfolio failed: %v", err)
	}

	got, err := ps.GetPortfolio(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetPortfolio failed: %v", err)
	}

	if got.NavexaID != "abc-123" {
		t.Errorf("NavexaID = %q, want %q", got.NavexaID, "abc-123")
	}
	if got.TotalGainPct != 25.001 {
		t.Errorf("TotalGainPct = %f, want 25.001", got.TotalGainPct)
	}
	if got.Holdings[0].Weight != 9.1 {
		t.Errorf("Holdings[0].Weight = %f, want 9.1", got.Holdings[0].Weight)
	}
}

func TestJSONRoundTrip_MarketDataWithFundamentals(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ms := m.MarketDataStorage()

	md := &models.MarketData{
		Ticker:   "BHP.AU",
		Exchange: "AU",
		Name:     "BHP Group",
		EOD: []models.EODBar{
			{Date: time.Now().Truncate(time.Second), Open: 44, High: 46, Low: 43, Close: 45.50, AdjClose: 45.50, Volume: 1000000},
		},
		Fundamentals: &models.Fundamentals{
			Ticker:        "BHP.AU",
			MarketCap:     150000000000,
			PE:            12.5,
			EPS:           3.64,
			DividendYield: 5.2,
			Sector:        "Materials",
			Industry:      "Mining",
		},
	}

	if err := ms.SaveMarketData(ctx, md); err != nil {
		t.Fatalf("SaveMarketData failed: %v", err)
	}

	got, err := ms.GetMarketData(ctx, "BHP.AU")
	if err != nil {
		t.Fatalf("GetMarketData failed: %v", err)
	}

	if got.Fundamentals == nil {
		t.Fatal("Fundamentals should not be nil")
	}
	if got.Fundamentals.PE != 12.5 {
		t.Errorf("PE = %f, want 12.5", got.Fundamentals.PE)
	}
	if got.Fundamentals.MarketCap != 150000000000 {
		t.Errorf("MarketCap = %f, want 150000000000", got.Fundamentals.MarketCap)
	}
	if got.EOD[0].Volume != 1000000 {
		t.Errorf("Volume = %d, want 1000000", got.EOD[0].Volume)
	}
}

func TestJSONRoundTrip_TickerSignals(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	ss := m.SignalStorage()

	signals := &models.TickerSignals{
		Ticker: "BHP.AU",
		Price: models.PriceSignals{
			Current: 45.50,
			SMA20:   44.0,
			SMA50:   43.0,
			SMA200:  42.0,
		},
		Technical: models.TechnicalSignals{
			RSI:             55.5,
			RSISignal:       "neutral",
			MACD:            0.5,
			MACDSignal:      0.3,
			MACDHistogram:   0.2,
			VolumeRatio:     1.2,
			NearSupport:     true,
			SupportLevel:    43.0,
			ResistanceLevel: 47.0,
		},
		PBAS: models.PBASSignal{
			Score:          0.7,
			Interpretation: "underpriced",
		},
		VLI: models.VLISignal{
			Score:          0.6,
			Interpretation: "accumulating",
		},
		Regime: models.RegimeSignal{
			Current:      models.RegimeTrendUp,
			Previous:     models.RegimeAccumulation,
			DaysInRegime: 15,
			Confidence:   0.85,
		},
		Trend:     models.TrendBullish,
		RiskFlags: []string{"near_resistance"},
	}

	if err := ss.SaveSignals(ctx, signals); err != nil {
		t.Fatalf("SaveSignals failed: %v", err)
	}

	got, err := ss.GetSignals(ctx, "BHP.AU")
	if err != nil {
		t.Fatalf("GetSignals failed: %v", err)
	}

	if got.PBAS.Score != 0.7 {
		t.Errorf("PBAS.Score = %f, want 0.7", got.PBAS.Score)
	}
	if got.Regime.Current != models.RegimeTrendUp {
		t.Errorf("Regime.Current = %q, want %q", got.Regime.Current, models.RegimeTrendUp)
	}
	if !got.Technical.NearSupport {
		t.Error("Technical.NearSupport should be true")
	}
	if len(got.RiskFlags) != 1 || got.RiskFlags[0] != "near_resistance" {
		t.Errorf("RiskFlags = %v, want [near_resistance]", got.RiskFlags)
	}
}

// --- Manager interface compliance ---

func TestManagerImplementsStorageManager(t *testing.T) {
	m := newTestFileManager(t)

	// Verify all accessors return non-nil
	if m.PortfolioStorage() == nil {
		t.Error("PortfolioStorage() returned nil")
	}
	if m.MarketDataStorage() == nil {
		t.Error("MarketDataStorage() returned nil")
	}
	if m.SignalStorage() == nil {
		t.Error("SignalStorage() returned nil")
	}
	if m.KeyValueStorage() == nil {
		t.Error("KeyValueStorage() returned nil")
	}
	if m.ReportStorage() == nil {
		t.Error("ReportStorage() returned nil")
	}
	if m.StrategyStorage() == nil {
		t.Error("StrategyStorage() returned nil")
	}
	if m.PlanStorage() == nil {
		t.Error("PlanStorage() returned nil")
	}
	if m.SearchHistoryStorage() == nil {
		t.Error("SearchHistoryStorage() returned nil")
	}
	if m.WatchlistStorage() == nil {
		t.Error("WatchlistStorage() returned nil")
	}
}

func TestManagerClose(t *testing.T) {
	m := newTestFileManager(t)
	err := m.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

// --- Two-store separation tests ---

func TestTwoStoreSeparation_UserDataWritesToBadger(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	// Save a portfolio (user data — stored in BadgerDB)
	portfolio := &models.Portfolio{ID: "SMSF", Name: "SMSF"}
	if err := m.PortfolioStorage().SavePortfolio(ctx, portfolio); err != nil {
		t.Fatalf("SavePortfolio failed: %v", err)
	}

	// Verify retrievable via BadgerHold (not on filesystem)
	got, err := m.PortfolioStorage().GetPortfolio(ctx, "SMSF")
	if err != nil {
		t.Fatalf("expected portfolio to be stored in BadgerDB: %v", err)
	}
	if got.Name != "SMSF" {
		t.Errorf("unexpected portfolio name: %s", got.Name)
	}

	// Verify file does NOT exist under dataStore
	dataFile := filepath.Join(m.dataStore.basePath, "portfolios", "SMSF.json")
	if _, err := os.Stat(dataFile); !os.IsNotExist(err) {
		t.Errorf("portfolio file should NOT exist in data store at %s", dataFile)
	}
}

func TestTwoStoreSeparation_MarketDataWritesToDataStore(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	// Save market data (shared data)
	md := &models.MarketData{Ticker: "BHP.AU", Exchange: "AU"}
	if err := m.MarketDataStorage().SaveMarketData(ctx, md); err != nil {
		t.Fatalf("SaveMarketData failed: %v", err)
	}

	// Verify file exists under dataStore
	dataFile := filepath.Join(m.dataStore.basePath, "market", "BHP.AU.json")
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		t.Errorf("expected market data file in data store at %s", dataFile)
	}
}

func TestTwoStoreSeparation_SignalsWriteToDataStore(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	sig := &models.TickerSignals{Ticker: "BHP.AU"}
	if err := m.SignalStorage().SaveSignals(ctx, sig); err != nil {
		t.Fatalf("SaveSignals failed: %v", err)
	}

	dataFile := filepath.Join(m.dataStore.basePath, "signals", "BHP.AU.json")
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		t.Errorf("expected signals file in data store at %s", dataFile)
	}
}

func TestTwoStoreSeparation_StrategyWritesToBadger(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()

	strategy := &models.PortfolioStrategy{PortfolioName: "SMSF", AccountType: models.AccountTypeSMSF}
	if err := m.StrategyStorage().SaveStrategy(ctx, strategy); err != nil {
		t.Fatalf("SaveStrategy failed: %v", err)
	}

	// Verify retrievable via BadgerHold (not on filesystem)
	got, err := m.StrategyStorage().GetStrategy(ctx, "SMSF")
	if err != nil {
		t.Fatalf("expected strategy to be stored in BadgerDB: %v", err)
	}
	if got.PortfolioName != "SMSF" {
		t.Errorf("unexpected strategy portfolio name: %s", got.PortfolioName)
	}
}

func TestTwoStoreSeparation_WriteRawUsesDataStore(t *testing.T) {
	m := newTestFileManager(t)

	// WriteRaw should route through dataStore
	if err := m.WriteRaw("charts", "test.png", []byte("fake png")); err != nil {
		t.Fatalf("WriteRaw failed: %v", err)
	}

	dataFile := filepath.Join(m.dataStore.basePath, "charts", "test.png")
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		t.Errorf("expected chart file in data store at %s", dataFile)
	}
}

func TestTwoStoreSeparation_DataPathReturnsDataStore(t *testing.T) {
	m := newTestFileManager(t)

	if m.DataPath() != m.dataStore.basePath {
		t.Errorf("DataPath() = %q, want %q", m.DataPath(), m.dataStore.basePath)
	}
}

// --- Migration tests ---

func TestMigrateToSplitLayout(t *testing.T) {
	dir := t.TempDir()
	logger := common.NewLogger("error")

	// Create old flat layout
	oldDirs := []string{"portfolios", "strategies", "plans", "watchlists", "reports", "searches", "kv", "market", "signals", "charts"}
	for _, sub := range oldDirs {
		subDir := filepath.Join(dir, sub)
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("Failed to create old dir %s: %v", sub, err)
		}
		// Write a marker file
		if err := os.WriteFile(filepath.Join(subDir, "marker.json"), []byte(`{"test":true}`), 0644); err != nil {
			t.Fatalf("Failed to write marker in %s: %v", sub, err)
		}
	}

	config := &common.Config{
		Storage: common.StorageConfig{
			UserData: common.FileConfig{Path: filepath.Join(dir, "user"), Versions: 5},
			Data:     common.FileConfig{Path: filepath.Join(dir, "data"), Versions: 0},
		},
	}

	err := MigrateToSplitLayout(logger, config)
	if err != nil {
		t.Fatalf("MigrateToSplitLayout failed: %v", err)
	}

	// Verify user dirs moved
	for _, sub := range []string{"portfolios", "strategies", "plans", "watchlists", "reports", "searches", "kv"} {
		marker := filepath.Join(dir, "user", sub, "marker.json")
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			t.Errorf("expected migrated user dir %s to exist", sub)
		}
		// Old location should be gone
		oldDir := filepath.Join(dir, sub)
		if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
			t.Errorf("expected old dir %s to be removed after migration", sub)
		}
	}

	// Verify data dirs moved
	for _, sub := range []string{"market", "signals", "charts"} {
		marker := filepath.Join(dir, "data", sub, "marker.json")
		if _, err := os.Stat(marker); os.IsNotExist(err) {
			t.Errorf("expected migrated data dir %s to exist", sub)
		}
		oldDir := filepath.Join(dir, sub)
		if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
			t.Errorf("expected old dir %s to be removed after migration", sub)
		}
	}
}

func TestMigrateToSplitLayout_NoOldLayout(t *testing.T) {
	dir := t.TempDir()
	logger := common.NewLogger("error")

	config := &common.Config{
		Storage: common.StorageConfig{
			UserData: common.FileConfig{Path: filepath.Join(dir, "user"), Versions: 5},
			Data:     common.FileConfig{Path: filepath.Join(dir, "data"), Versions: 0},
		},
	}

	// Should be a no-op when old layout doesn't exist
	err := MigrateToSplitLayout(logger, config)
	if err != nil {
		t.Fatalf("MigrateToSplitLayout should not fail when no old layout: %v", err)
	}
}

func TestMigrateToSplitLayout_SkipsIfDestinationExists(t *testing.T) {
	dir := t.TempDir()
	logger := common.NewLogger("error")

	// Create old flat layout with portfolios
	oldPortfolios := filepath.Join(dir, "portfolios")
	os.MkdirAll(oldPortfolios, 0755)
	os.WriteFile(filepath.Join(oldPortfolios, "old.json"), []byte(`{"old":true}`), 0644)

	// Create new layout with existing portfolios dir (pre-existing data)
	newPortfolios := filepath.Join(dir, "user", "portfolios")
	os.MkdirAll(newPortfolios, 0755)
	os.WriteFile(filepath.Join(newPortfolios, "new.json"), []byte(`{"new":true}`), 0644)

	config := &common.Config{
		Storage: common.StorageConfig{
			UserData: common.FileConfig{Path: filepath.Join(dir, "user"), Versions: 5},
			Data:     common.FileConfig{Path: filepath.Join(dir, "data"), Versions: 0},
		},
	}

	err := MigrateToSplitLayout(logger, config)
	if err != nil {
		t.Fatalf("MigrateToSplitLayout failed: %v", err)
	}

	// New data should be preserved (not overwritten)
	newFile := filepath.Join(newPortfolios, "new.json")
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Error("existing new data should be preserved")
	}
}

// --- User Storage CRUD tests ---

func TestUserStorage_SaveAndGet(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	store := m.UserStorage()

	user := &models.User{
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "$2a$10$somehashvalue",
		Role:         "admin",
		NavexaKey:    "nk-12345678",
	}

	if err := store.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser failed: %v", err)
	}

	got, err := store.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	if got.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", got.Username)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("expected email 'alice@example.com', got %q", got.Email)
	}
	if got.PasswordHash != "$2a$10$somehashvalue" {
		t.Errorf("expected password hash preserved, got %q", got.PasswordHash)
	}
	if got.Role != "admin" {
		t.Errorf("expected role 'admin', got %q", got.Role)
	}
	if got.NavexaKey != "nk-12345678" {
		t.Errorf("expected navexa key 'nk-12345678', got %q", got.NavexaKey)
	}
}

func TestUserStorage_GetNotFound(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	store := m.UserStorage()

	_, err := store.GetUser(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestUserStorage_Delete(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	store := m.UserStorage()

	user := &models.User{
		Username:     "bob",
		Email:        "bob@example.com",
		PasswordHash: "$2a$10$hash",
		Role:         "user",
	}

	store.SaveUser(ctx, user)

	if err := store.DeleteUser(ctx, "bob"); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	_, err := store.GetUser(ctx, "bob")
	if err == nil {
		t.Fatal("expected user to be deleted")
	}
}

func TestUserStorage_ListUsers(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	store := m.UserStorage()

	// Empty list initially
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}

	// Add two users
	store.SaveUser(ctx, &models.User{Username: "alice", Email: "a@x.com", PasswordHash: "h1", Role: "admin"})
	store.SaveUser(ctx, &models.User{Username: "bob", Email: "b@x.com", PasswordHash: "h2", Role: "user"})

	users, err = store.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestUserStorage_Update(t *testing.T) {
	m := newTestFileManager(t)
	ctx := context.Background()
	store := m.UserStorage()

	user := &models.User{
		Username:     "carol",
		Email:        "carol@old.com",
		PasswordHash: "$2a$10$oldhash",
		Role:         "user",
	}
	store.SaveUser(ctx, user)

	// Update email and role
	user.Email = "carol@new.com"
	user.Role = "admin"
	if err := store.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser (update) failed: %v", err)
	}

	got, _ := store.GetUser(ctx, "carol")
	if got.Email != "carol@new.com" {
		t.Errorf("expected updated email, got %q", got.Email)
	}
	if got.Role != "admin" {
		t.Errorf("expected updated role, got %q", got.Role)
	}
}
