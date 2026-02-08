package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

// --- mockPortfolioService ---

type mockPortfolioService struct {
	dailyGrowthFn    func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error)
	listPortfoliosFn func(ctx context.Context) ([]string, error)
}

func (m *mockPortfolioService) GetDailyGrowth(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
	if m.dailyGrowthFn != nil {
		return m.dailyGrowthFn(ctx, name, opts)
	}
	return nil, nil
}

func (m *mockPortfolioService) ListPortfolios(ctx context.Context) ([]string, error) {
	if m.listPortfoliosFn != nil {
		return m.listPortfoliosFn(ctx)
	}
	return nil, nil
}

func (m *mockPortfolioService) SyncPortfolio(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPortfolioService) GetPortfolio(ctx context.Context, name string) (*models.Portfolio, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPortfolioService) ReviewPortfolio(ctx context.Context, name string, options interfaces.ReviewOptions) (*models.PortfolioReview, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPortfolioService) GetPortfolioSnapshot(ctx context.Context, name string, asOf time.Time) (*models.PortfolioSnapshot, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockPortfolioService) GetPortfolioGrowth(ctx context.Context, name string) ([]models.GrowthDataPoint, error) {
	return nil, fmt.Errorf("not implemented")
}

// --- mockKeyValueStorage ---

type mockKeyValueStorage struct {
	mu   sync.RWMutex
	data map[string]string
}

func newMockKeyValueStorage() *mockKeyValueStorage {
	return &mockKeyValueStorage{data: make(map[string]string)}
}

func (m *mockKeyValueStorage) Get(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return v, nil
}

func (m *mockKeyValueStorage) Set(ctx context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockKeyValueStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockKeyValueStorage) GetAll(ctx context.Context) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make(map[string]string, len(m.data))
	for k, v := range m.data {
		cp[k] = v
	}
	return cp, nil
}

// --- mockStorageManager ---

type mockStorageManager struct {
	kv       *mockKeyValueStorage
	strategy *mockStrategyStorage
}

func newMockStorageManager() *mockStorageManager {
	return &mockStorageManager{
		kv:       newMockKeyValueStorage(),
		strategy: &mockStrategyStorage{},
	}
}

func (m *mockStorageManager) KeyValueStorage() interfaces.KeyValueStorage           { return m.kv }
func (m *mockStorageManager) StrategyStorage() interfaces.StrategyStorage           { return m.strategy }
func (m *mockStorageManager) PlanStorage() interfaces.PlanStorage                   { return nil }
func (m *mockStorageManager) PortfolioStorage() interfaces.PortfolioStorage         { return nil }
func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage       { return nil }
func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage               { return nil }
func (m *mockStorageManager) ReportStorage() interfaces.ReportStorage               { return nil }
func (m *mockStorageManager) SearchHistoryStorage() interfaces.SearchHistoryStorage { return nil }
func (m *mockStorageManager) PurgeDerivedData(ctx context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) Close() error { return nil }

// --- mockStrategyStorage ---

type mockStrategyStorage struct{}

func (m *mockStrategyStorage) GetStrategy(ctx context.Context, portfolioName string) (*models.PortfolioStrategy, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockStrategyStorage) SaveStrategy(ctx context.Context, strategy *models.PortfolioStrategy) error {
	return nil
}
func (m *mockStrategyStorage) DeleteStrategy(ctx context.Context, portfolioName string) error {
	return nil
}
func (m *mockStrategyStorage) ListStrategies(ctx context.Context) ([]string, error) {
	return nil, nil
}

// --- Test data generators ---

// generateTestGrowthPoints creates n daily growth data points starting from 2025-01-01.
func generateTestGrowthPoints(n int) []models.GrowthDataPoint {
	points := make([]models.GrowthDataPoint, n)
	baseDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		points[i] = models.GrowthDataPoint{
			Date:         baseDate.AddDate(0, 0, i),
			TotalValue:   100000 + float64(i)*100,
			TotalCost:    95000,
			GainLoss:     5000 + float64(i)*100,
			GainLossPct:  5.26 + float64(i)*0.1,
			HoldingCount: 12,
		}
	}
	return points
}
