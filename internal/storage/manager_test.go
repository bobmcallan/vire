package storage

import (
	"context"
	"testing"
	"time"

	"github.com/timshannon/badgerhold/v4"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/models"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	opts := badgerhold.DefaultOptions
	opts.Dir = t.TempDir()
	opts.ValueDir = opts.Dir
	opts.Logger = nil

	store, err := badgerhold.Open(opts)
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	logger := common.NewLogger("error")
	db := &BadgerDB{store: store, logger: logger}

	return &Manager{
		db:         db,
		portfolio:  newPortfolioStorage(db, logger),
		marketData: newMarketDataStorage(db, logger),
		signal:     newSignalStorage(db, logger),
		kv:         newKVStorage(db, logger),
		report:     newReportStorage(db, logger),
		strategy:   newStrategyStorage(db, logger),
		logger:     logger,
	}
}

func TestPurgeDerivedData_DeletesDerivedPreservesUserData(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	store := m.db.Store()

	// Seed derived data
	portfolio := &models.Portfolio{
		ID:        "SMSF",
		Name:      "SMSF",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := store.Upsert(portfolio.ID, portfolio); err != nil {
		t.Fatalf("failed to seed portfolio: %v", err)
	}

	md := &models.MarketData{
		Ticker:      "BHP.AU",
		Exchange:    "AU",
		LastUpdated: time.Now(),
	}
	if err := store.Upsert(md.Ticker, md); err != nil {
		t.Fatalf("failed to seed market data: %v", err)
	}

	sig := &models.TickerSignals{
		Ticker:           "BHP.AU",
		ComputeTimestamp: time.Now(),
	}
	if err := store.Upsert(sig.Ticker, sig); err != nil {
		t.Fatalf("failed to seed signals: %v", err)
	}

	report := &models.PortfolioReport{
		Portfolio:   "SMSF",
		GeneratedAt: time.Now(),
	}
	if err := store.Upsert(report.Portfolio, report); err != nil {
		t.Fatalf("failed to seed report: %v", err)
	}

	// Seed user data (should be preserved)
	strategy := &models.PortfolioStrategy{
		PortfolioName: "SMSF",
		AccountType:   models.AccountTypeSMSF,
		Version:       1,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Disclaimer:    models.DefaultDisclaimer,
	}
	if err := store.Upsert(strategy.PortfolioName, strategy); err != nil {
		t.Fatalf("failed to seed strategy: %v", err)
	}

	kv := &kvEntry{Key: "test_key", Value: "test_value"}
	if err := store.Upsert(kv.Key, kv); err != nil {
		t.Fatalf("failed to seed kv entry: %v", err)
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

	// Verify derived data is gone
	var portfolios []models.Portfolio
	if err := store.Find(&portfolios, nil); err != nil {
		t.Fatalf("find portfolios failed: %v", err)
	}
	if len(portfolios) != 0 {
		t.Errorf("expected 0 portfolios after purge, got %d", len(portfolios))
	}

	var marketDataList []models.MarketData
	if err := store.Find(&marketDataList, nil); err != nil {
		t.Fatalf("find market data failed: %v", err)
	}
	if len(marketDataList) != 0 {
		t.Errorf("expected 0 market data after purge, got %d", len(marketDataList))
	}

	var signalsList []models.TickerSignals
	if err := store.Find(&signalsList, nil); err != nil {
		t.Fatalf("find signals failed: %v", err)
	}
	if len(signalsList) != 0 {
		t.Errorf("expected 0 signals after purge, got %d", len(signalsList))
	}

	var reportsList []models.PortfolioReport
	if err := store.Find(&reportsList, nil); err != nil {
		t.Fatalf("find reports failed: %v", err)
	}
	if len(reportsList) != 0 {
		t.Errorf("expected 0 reports after purge, got %d", len(reportsList))
	}

	// Verify user data is preserved
	var strategies []models.PortfolioStrategy
	if err := store.Find(&strategies, nil); err != nil {
		t.Fatalf("find strategies failed: %v", err)
	}
	if len(strategies) != 1 {
		t.Errorf("expected 1 strategy preserved, got %d", len(strategies))
	}

	var kvEntries []kvEntry
	if err := store.Find(&kvEntries, nil); err != nil {
		t.Fatalf("find kv entries failed: %v", err)
	}
	if len(kvEntries) != 1 {
		t.Errorf("expected 1 kv entry preserved, got %d", len(kvEntries))
	}
	if len(kvEntries) > 0 && kvEntries[0].Value != "test_value" {
		t.Errorf("kv entry value = %q, want %q", kvEntries[0].Value, "test_value")
	}
}

func TestPurgeDerivedData_EmptyDB(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	counts, err := m.PurgeDerivedData(ctx)
	if err != nil {
		t.Fatalf("PurgeDerivedData on empty DB failed: %v", err)
	}

	for typ, count := range counts {
		if count != 0 {
			t.Errorf("%s = %d, want 0 on empty DB", typ, count)
		}
	}
}
