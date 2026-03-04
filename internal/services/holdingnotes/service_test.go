package holdingnotes

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// =============================================================================
// Test helpers
// =============================================================================

// testContext creates a context with a test user.
func testContext() context.Context {
	ctx := context.Background()
	uc := &common.UserContext{UserID: "test-user"}
	return common.WithUserContext(ctx, uc)
}

// testService creates a Service backed by an in-memory store.
func testService() *Service {
	storage := newTestStorageManager()
	logger := common.NewLogger("error")
	return NewService(storage, logger)
}

// =============================================================================
// In-memory storage stubs
// =============================================================================

type testStorageManager struct {
	userData *testUserDataStore
}

func newTestStorageManager() *testStorageManager {
	return &testStorageManager{userData: newTestUserDataStore()}
}

func (m *testStorageManager) UserDataStore() interfaces.UserDataStore         { return m.userData }
func (m *testStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return nil }
func (m *testStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (m *testStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (m *testStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (m *testStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (m *testStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (m *testStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (m *testStorageManager) ChangelogStore() interfaces.ChangelogStore       { return nil }
func (m *testStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
func (m *testStorageManager) TimelineStore() interfaces.TimelineStore         { return nil }
func (m *testStorageManager) DataPath() string                                { return "" }
func (m *testStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (m *testStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *testStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *testStorageManager) Close() error                                { return nil }

// testUserDataStore is a simple in-memory implementation.
type testUserDataStore struct {
	records map[string]*models.UserRecord
}

func newTestUserDataStore() *testUserDataStore {
	return &testUserDataStore{records: make(map[string]*models.UserRecord)}
}

func compositeKey(userID, subject, key string) string {
	return userID + ":" + subject + ":" + key
}

func (s *testUserDataStore) Get(_ context.Context, userID, subject, key string) (*models.UserRecord, error) {
	ck := compositeKey(userID, subject, key)
	if r, ok := s.records[ck]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("%s '%s' not found", subject, key)
}

func (s *testUserDataStore) Put(_ context.Context, record *models.UserRecord) error {
	ck := compositeKey(record.UserID, record.Subject, record.Key)
	if existing, ok := s.records[ck]; ok {
		record.Version = existing.Version + 1
	} else {
		record.Version = 1
	}
	record.DateTime = time.Now()
	s.records[ck] = record
	return nil
}

func (s *testUserDataStore) Delete(_ context.Context, userID, subject, key string) error {
	ck := compositeKey(userID, subject, key)
	delete(s.records, ck)
	return nil
}

func (s *testUserDataStore) List(_ context.Context, userID, subject string) ([]*models.UserRecord, error) {
	var result []*models.UserRecord
	for _, r := range s.records {
		if r.UserID == userID && r.Subject == subject {
			result = append(result, r)
		}
	}
	return result, nil
}

func (s *testUserDataStore) Query(_ context.Context, userID, subject string, _ interfaces.QueryOptions) ([]*models.UserRecord, error) {
	return s.List(context.Background(), userID, subject)
}

func (s *testUserDataStore) DeleteBySubject(_ context.Context, subject string) (int, error) {
	count := 0
	for ck, r := range s.records {
		if r.Subject == subject {
			delete(s.records, ck)
			count++
		}
	}
	return count, nil
}

func (s *testUserDataStore) Close() error { return nil }

// =============================================================================
// GetNotes tests
// =============================================================================

func TestGetNotes_NotFound(t *testing.T) {
	svc := testService()
	ctx := testContext()

	_, err := svc.GetNotes(ctx, "SMSF")
	if err == nil {
		t.Fatal("expected error when no notes exist, got nil")
	}
}

func TestGetNotes_AfterSave(t *testing.T) {
	svc := testService()
	ctx := testContext()

	notes := &models.PortfolioHoldingNotes{
		PortfolioName: "SMSF",
		Items: []models.HoldingNote{
			{Ticker: "BHP.AU", Thesis: "Mining thesis"},
		},
	}
	if err := svc.SaveNotes(ctx, notes); err != nil {
		t.Fatalf("SaveNotes: %v", err)
	}

	got, err := svc.GetNotes(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetNotes: %v", err)
	}
	if got.PortfolioName != "SMSF" {
		t.Errorf("PortfolioName = %q, want %q", got.PortfolioName, "SMSF")
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(got.Items))
	}
	if got.Items[0].Ticker != "BHP.AU" {
		t.Errorf("Ticker = %q, want BHP.AU", got.Items[0].Ticker)
	}
	if got.Items[0].Thesis != "Mining thesis" {
		t.Errorf("Thesis = %q, want %q", got.Items[0].Thesis, "Mining thesis")
	}
}

// =============================================================================
// SaveNotes tests
// =============================================================================

func TestSaveNotes_VersionIncrements(t *testing.T) {
	svc := testService()
	ctx := testContext()

	notes := &models.PortfolioHoldingNotes{
		PortfolioName: "SMSF",
		Items:         []models.HoldingNote{{Ticker: "CBA.AU"}},
	}

	if err := svc.SaveNotes(ctx, notes); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if notes.Version != 1 {
		t.Errorf("after first save: Version = %d, want 1", notes.Version)
	}

	if err := svc.SaveNotes(ctx, notes); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if notes.Version != 2 {
		t.Errorf("after second save: Version = %d, want 2", notes.Version)
	}
}

func TestSaveNotes_SetsTimestamps(t *testing.T) {
	svc := testService()
	ctx := testContext()

	before := time.Now().Add(-time.Millisecond)
	notes := &models.PortfolioHoldingNotes{
		PortfolioName: "SMSF",
		Items:         []models.HoldingNote{{Ticker: "ANZ.AU"}},
	}
	if err := svc.SaveNotes(ctx, notes); err != nil {
		t.Fatalf("SaveNotes: %v", err)
	}
	if notes.CreatedAt.Before(before) {
		t.Error("CreatedAt should be set after save")
	}
	if notes.UpdatedAt.Before(before) {
		t.Error("UpdatedAt should be set after save")
	}
}

func TestSaveNotes_CreatedAtPreservedOnUpdate(t *testing.T) {
	svc := testService()
	ctx := testContext()

	notes := &models.PortfolioHoldingNotes{
		PortfolioName: "SMSF",
		Items:         []models.HoldingNote{{Ticker: "WBC.AU"}},
	}
	if err := svc.SaveNotes(ctx, notes); err != nil {
		t.Fatalf("first save: %v", err)
	}
	createdAt := notes.CreatedAt

	time.Sleep(2 * time.Millisecond)
	if err := svc.SaveNotes(ctx, notes); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if !notes.CreatedAt.Equal(createdAt) {
		t.Error("CreatedAt should not change on second save")
	}
	if !notes.UpdatedAt.After(createdAt) {
		t.Error("UpdatedAt should be updated on second save")
	}
}

// =============================================================================
// AddOrUpdateNote tests
// =============================================================================

func TestAddOrUpdateNote_AddNew(t *testing.T) {
	svc := testService()
	ctx := testContext()

	note := &models.HoldingNote{
		Ticker: "BHP.AU",
		Thesis: "Resources thesis",
	}
	result, err := svc.AddOrUpdateNote(ctx, "SMSF", note)
	if err != nil {
		t.Fatalf("AddOrUpdateNote: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Ticker != "BHP.AU" {
		t.Errorf("Ticker = %q, want BHP.AU", result.Items[0].Ticker)
	}
	if result.Items[0].Thesis != "Resources thesis" {
		t.Errorf("Thesis = %q, want %q", result.Items[0].Thesis, "Resources thesis")
	}
	if result.Items[0].CreatedAt.IsZero() {
		t.Error("CreatedAt should be set on new note")
	}
}

func TestAddOrUpdateNote_AddMultiple(t *testing.T) {
	svc := testService()
	ctx := testContext()

	for _, ticker := range []string{"BHP.AU", "CBA.AU", "ANZ.AU"} {
		_, err := svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{Ticker: ticker})
		if err != nil {
			t.Fatalf("AddOrUpdateNote(%s): %v", ticker, err)
		}
	}

	result, err := svc.GetNotes(ctx, "SMSF")
	if err != nil {
		t.Fatalf("GetNotes: %v", err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result.Items))
	}
}

func TestAddOrUpdateNote_UpdateExisting(t *testing.T) {
	svc := testService()
	ctx := testContext()

	// Add initial note
	_, err := svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{
		Ticker: "BHP.AU",
		Thesis: "Original thesis",
	})
	if err != nil {
		t.Fatalf("initial add: %v", err)
	}

	// Get created_at before update
	notes, _ := svc.GetNotes(ctx, "SMSF")
	originalCreatedAt := notes.Items[0].CreatedAt

	time.Sleep(2 * time.Millisecond)

	// Update
	result, err := svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{
		Ticker: "BHP.AU",
		Thesis: "Updated thesis",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item after upsert, got %d", len(result.Items))
	}
	if result.Items[0].Thesis != "Updated thesis" {
		t.Errorf("Thesis = %q, want %q", result.Items[0].Thesis, "Updated thesis")
	}
	if !result.Items[0].CreatedAt.Equal(originalCreatedAt) {
		t.Error("CreatedAt should be preserved on update")
	}
	if !result.Items[0].ReviewedAt.After(originalCreatedAt) {
		t.Error("ReviewedAt should be updated")
	}
}

func TestAddOrUpdateNote_CaseInsensitiveTicker(t *testing.T) {
	svc := testService()
	ctx := testContext()

	// Add with uppercase ticker
	_, err := svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{
		Ticker: "BHP.AU",
		Thesis: "Mining",
	})
	if err != nil {
		t.Fatalf("initial add: %v", err)
	}

	// FindByTicker should match case-insensitively
	notes, _ := svc.GetNotes(ctx, "SMSF")
	got, idx := notes.FindByTicker("bhp.au")
	if idx < 0 {
		t.Fatal("FindByTicker should find BHP.AU case-insensitively")
	}
	if got.Thesis != "Mining" {
		t.Errorf("Thesis = %q, want Mining", got.Thesis)
	}
}

// =============================================================================
// UpdateNote tests
// =============================================================================

func TestUpdateNote_NotFound_NoChange(t *testing.T) {
	svc := testService()
	ctx := testContext()

	// Start with one note
	_, _ = svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{Ticker: "BHP.AU", Thesis: "Mining"})

	// Update a non-existent ticker — should return unchanged
	result, err := svc.UpdateNote(ctx, "SMSF", "XYZ.AU", &models.HoldingNote{Thesis: "New"})
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Ticker != "BHP.AU" {
		t.Errorf("existing item should be unchanged, got %q", result.Items[0].Ticker)
	}
}

func TestUpdateNote_MergeSemantics(t *testing.T) {
	svc := testService()
	ctx := testContext()

	_, _ = svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{
		Ticker:           "CBA.AU",
		Thesis:           "Banking thesis",
		LiquidityProfile: models.LiquidityHigh,
		AssetType:        models.AssetTypeASXStock,
	})

	// Only update the thesis — other fields should be preserved
	_, err := svc.UpdateNote(ctx, "SMSF", "CBA.AU", &models.HoldingNote{
		Thesis: "Updated banking thesis",
	})
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}

	result, _ := svc.GetNotes(ctx, "SMSF")
	item := result.Items[0]

	if item.Thesis != "Updated banking thesis" {
		t.Errorf("Thesis = %q, want updated", item.Thesis)
	}
	if item.LiquidityProfile != models.LiquidityHigh {
		t.Errorf("LiquidityProfile = %q, want high (should be preserved)", item.LiquidityProfile)
	}
	if item.AssetType != models.AssetTypeASXStock {
		t.Errorf("AssetType = %q, want ASX_stock (should be preserved)", item.AssetType)
	}
}

func TestUpdateNote_AllFields(t *testing.T) {
	svc := testService()
	ctx := testContext()

	_, _ = svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{
		Ticker: "ANZ.AU",
	})

	update := &models.HoldingNote{
		Name:             "ANZ Bank",
		AssetType:        models.AssetTypeASXStock,
		LiquidityProfile: models.LiquidityMedium,
		Thesis:           "Banking",
		KnownBehaviours:  "Dividend reliable",
		SignalOverrides:  "Ignore RSI in rate cycles",
		Notes:            "Watch APRA decisions",
		StaleDays:        180,
	}
	result, err := svc.UpdateNote(ctx, "SMSF", "ANZ.AU", update)
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}

	item := result.Items[0]
	if item.Name != "ANZ Bank" {
		t.Errorf("Name = %q, want ANZ Bank", item.Name)
	}
	if item.AssetType != models.AssetTypeASXStock {
		t.Errorf("AssetType = %q, want ASX_stock", item.AssetType)
	}
	if item.LiquidityProfile != models.LiquidityMedium {
		t.Errorf("LiquidityProfile = %q, want medium", item.LiquidityProfile)
	}
	if item.StaleDays != 180 {
		t.Errorf("StaleDays = %d, want 180", item.StaleDays)
	}
}

// =============================================================================
// RemoveNote tests
// =============================================================================

func TestRemoveNote_RemovesCorrectTicker(t *testing.T) {
	svc := testService()
	ctx := testContext()

	for _, ticker := range []string{"BHP.AU", "CBA.AU", "ANZ.AU"} {
		_, _ = svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{Ticker: ticker})
	}

	result, err := svc.RemoveNote(ctx, "SMSF", "CBA.AU")
	if err != nil {
		t.Fatalf("RemoveNote: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items after removal, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if item.Ticker == "CBA.AU" {
			t.Error("CBA.AU should have been removed")
		}
	}
}

func TestRemoveNote_NotFound_IdempotentNoError(t *testing.T) {
	svc := testService()
	ctx := testContext()

	_, _ = svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{Ticker: "BHP.AU"})

	result, err := svc.RemoveNote(ctx, "SMSF", "XYZ.AU")
	if err != nil {
		t.Fatalf("RemoveNote for non-existent ticker should not error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("expected 1 item (BHP.AU unchanged), got %d", len(result.Items))
	}
}

func TestRemoveNote_LastItem_EmptyCollection(t *testing.T) {
	svc := testService()
	ctx := testContext()

	_, _ = svc.AddOrUpdateNote(ctx, "SMSF", &models.HoldingNote{Ticker: "BHP.AU"})
	result, err := svc.RemoveNote(ctx, "SMSF", "BHP.AU")
	if err != nil {
		t.Fatalf("RemoveNote: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items after removing only item, got %d", len(result.Items))
	}
}

// =============================================================================
// HoldingNote model method tests
// =============================================================================

func TestHoldingNote_IsStale_DefaultTTL(t *testing.T) {
	// Default TTL is 90 days; note reviewed > 90 days ago is stale
	note := &models.HoldingNote{
		ReviewedAt: time.Now().Add(-91 * 24 * time.Hour),
	}
	if !note.IsStale() {
		t.Error("note reviewed 91 days ago should be stale with default TTL")
	}
}

func TestHoldingNote_IsStale_NotYetStale(t *testing.T) {
	note := &models.HoldingNote{
		ReviewedAt: time.Now().Add(-10 * 24 * time.Hour),
	}
	if note.IsStale() {
		t.Error("note reviewed 10 days ago should not be stale")
	}
}

func TestHoldingNote_IsStale_CustomTTL(t *testing.T) {
	note := &models.HoldingNote{
		ReviewedAt: time.Now().Add(-31 * 24 * time.Hour),
		StaleDays:  30,
	}
	if !note.IsStale() {
		t.Error("note reviewed 31 days ago with 30-day TTL should be stale")
	}
}

func TestHoldingNote_IsStale_ZeroTTLFallsBackToDefault(t *testing.T) {
	// StaleDays=0 should use default 90
	note := &models.HoldingNote{
		ReviewedAt: time.Now().Add(-91 * 24 * time.Hour),
		StaleDays:  0,
	}
	if !note.IsStale() {
		t.Error("StaleDays=0 should fall back to 90-day default and be stale after 91 days")
	}
}

func TestHoldingNote_DeriveSignalConfidence_ETF(t *testing.T) {
	note := &models.HoldingNote{AssetType: models.AssetTypeETF}
	if note.DeriveSignalConfidence() != models.SignalConfidenceLow {
		t.Errorf("ETF should return low signal confidence, got %q", note.DeriveSignalConfidence())
	}
}

func TestHoldingNote_DeriveSignalConfidence_ASXHighLiquidity(t *testing.T) {
	note := &models.HoldingNote{AssetType: models.AssetTypeASXStock, LiquidityProfile: models.LiquidityHigh}
	if note.DeriveSignalConfidence() != models.SignalConfidenceHigh {
		t.Errorf("ASX stock + high liquidity should return high confidence, got %q", note.DeriveSignalConfidence())
	}
}

func TestHoldingNote_DeriveSignalConfidence_ASXMediumLiquidity(t *testing.T) {
	note := &models.HoldingNote{AssetType: models.AssetTypeASXStock, LiquidityProfile: models.LiquidityMedium}
	if note.DeriveSignalConfidence() != models.SignalConfidenceHigh {
		t.Errorf("ASX stock + medium liquidity should return high confidence, got %q", note.DeriveSignalConfidence())
	}
}

func TestHoldingNote_DeriveSignalConfidence_ASXLowLiquidity(t *testing.T) {
	note := &models.HoldingNote{AssetType: models.AssetTypeASXStock, LiquidityProfile: models.LiquidityLow}
	if note.DeriveSignalConfidence() != models.SignalConfidenceMedium {
		t.Errorf("ASX stock + low liquidity should return medium confidence, got %q", note.DeriveSignalConfidence())
	}
}

func TestHoldingNote_DeriveSignalConfidence_USEquity(t *testing.T) {
	note := &models.HoldingNote{AssetType: models.AssetTypeUSEquity}
	if note.DeriveSignalConfidence() != models.SignalConfidenceHigh {
		t.Errorf("US equity should return high confidence, got %q", note.DeriveSignalConfidence())
	}
}

func TestHoldingNote_DeriveSignalConfidence_Unknown(t *testing.T) {
	note := &models.HoldingNote{}
	if note.DeriveSignalConfidence() != models.SignalConfidenceMedium {
		t.Errorf("empty asset type should return medium confidence, got %q", note.DeriveSignalConfidence())
	}
}

func TestHoldingNote_DeriveSignalConfidence_NilReceiver(t *testing.T) {
	var note *models.HoldingNote
	if note.DeriveSignalConfidence() != models.SignalConfidenceMedium {
		t.Errorf("nil note should return medium confidence, got %q", note.DeriveSignalConfidence())
	}
}

// =============================================================================
// PortfolioHoldingNotes model method tests
// =============================================================================

func TestPortfolioHoldingNotes_FindByTicker_Found(t *testing.T) {
	phn := &models.PortfolioHoldingNotes{
		Items: []models.HoldingNote{
			{Ticker: "BHP.AU"},
			{Ticker: "CBA.AU"},
		},
	}
	note, idx := phn.FindByTicker("CBA.AU")
	if idx != 1 {
		t.Errorf("expected index 1, got %d", idx)
	}
	if note.Ticker != "CBA.AU" {
		t.Errorf("expected CBA.AU, got %q", note.Ticker)
	}
}

func TestPortfolioHoldingNotes_FindByTicker_CaseInsensitive(t *testing.T) {
	phn := &models.PortfolioHoldingNotes{
		Items: []models.HoldingNote{{Ticker: "BHP.AU"}},
	}
	_, idx := phn.FindByTicker("bhp.au")
	if idx < 0 {
		t.Error("FindByTicker should be case-insensitive")
	}
}

func TestPortfolioHoldingNotes_FindByTicker_NotFound(t *testing.T) {
	phn := &models.PortfolioHoldingNotes{
		Items: []models.HoldingNote{{Ticker: "BHP.AU"}},
	}
	note, idx := phn.FindByTicker("XYZ.AU")
	if idx != -1 {
		t.Errorf("expected -1 index, got %d", idx)
	}
	if note != nil {
		t.Error("expected nil note for not-found ticker")
	}
}

func TestPortfolioHoldingNotes_NoteMap(t *testing.T) {
	phn := &models.PortfolioHoldingNotes{
		Items: []models.HoldingNote{
			{Ticker: "BHP.AU", Thesis: "Mining"},
			{Ticker: "CBA.AU", Thesis: "Banking"},
		},
	}
	m := phn.NoteMap()
	if len(m) != 2 {
		t.Fatalf("expected 2 entries in map, got %d", len(m))
	}
	if m["BHP.AU"] == nil {
		t.Error("BHP.AU should be in NoteMap")
	}
	if m["BHP.AU"].Thesis != "Mining" {
		t.Errorf("BHP.AU thesis = %q, want Mining", m["BHP.AU"].Thesis)
	}
	if m["CBA.AU"] == nil {
		t.Error("CBA.AU should be in NoteMap")
	}
}

func TestPortfolioHoldingNotes_NoteMap_UppercasesKeys(t *testing.T) {
	phn := &models.PortfolioHoldingNotes{
		Items: []models.HoldingNote{{Ticker: "bhp.au"}},
	}
	m := phn.NoteMap()
	if m["BHP.AU"] == nil {
		t.Error("NoteMap keys should be uppercased")
	}
}

func TestPortfolioHoldingNotes_NoteMap_Empty(t *testing.T) {
	phn := &models.PortfolioHoldingNotes{Items: []models.HoldingNote{}}
	m := phn.NoteMap()
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}
}
