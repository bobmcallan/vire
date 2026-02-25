package jobmanager

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- mocks ---

type mockMarketService struct {
	mu                  sync.Mutex
	collectCalls        map[string]int // job_type -> call count
	collectCoreFn       func(ctx context.Context, tickers []string, force bool) error
	collectFullFn       func(ctx context.Context, tickers []string, includeNews bool, force bool) error
	collectFilingsFn    func(ctx context.Context, ticker string, force bool) error // injectable for concurrency tests (index only)
	collectFilingPdfsFn func(ctx context.Context, ticker string, force bool) error // injectable for concurrency tests (PDFs)
}

func newMockMarketService() *mockMarketService {
	return &mockMarketService{collectCalls: make(map[string]int)}
}

func (m *mockMarketService) CollectMarketData(ctx context.Context, tickers []string, includeNews bool, force bool) error {
	if m.collectFullFn != nil {
		return m.collectFullFn(ctx, tickers, includeNews, force)
	}
	return nil
}
func (m *mockMarketService) CollectCoreMarketData(ctx context.Context, tickers []string, force bool) error {
	if m.collectCoreFn != nil {
		return m.collectCoreFn(ctx, tickers, force)
	}
	return nil
}
func (m *mockMarketService) CollectEOD(_ context.Context, ticker string, _ bool) error {
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectEOD]++
	m.mu.Unlock()
	return nil
}
func (m *mockMarketService) CollectFundamentals(_ context.Context, _ string, _ bool) error {
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectFundamentals]++
	m.mu.Unlock()
	return nil
}
func (m *mockMarketService) CollectFilingsIndex(ctx context.Context, ticker string, force bool) error {
	if m.collectFilingsFn != nil {
		return m.collectFilingsFn(ctx, ticker, force)
	}
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectFilings]++
	m.mu.Unlock()
	return nil
}
func (m *mockMarketService) CollectFilingPdfs(ctx context.Context, ticker string, force bool) error {
	if m.collectFilingPdfsFn != nil {
		return m.collectFilingPdfsFn(ctx, ticker, force)
	}
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectFilingPdfs]++
	m.mu.Unlock()
	return nil
}
func (m *mockMarketService) CollectNews(_ context.Context, _ string, _ bool) error {
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectNews]++
	m.mu.Unlock()
	return nil
}
func (m *mockMarketService) CollectFilingSummaries(_ context.Context, _ string, _ bool) error {
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectFilingSummaries]++
	m.mu.Unlock()
	return nil
}
func (m *mockMarketService) CollectTimeline(_ context.Context, _ string, _ bool) error {
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectTimeline]++
	m.mu.Unlock()
	return nil
}
func (m *mockMarketService) CollectNewsIntelligence(_ context.Context, _ string, _ bool) error {
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectNewsIntel]++
	m.mu.Unlock()
	return nil
}
func (m *mockMarketService) CollectBulkEOD(_ context.Context, exchange string, _ bool) error {
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectEODBulk]++
	m.mu.Unlock()
	return nil
}
func (m *mockMarketService) GetStockData(_ context.Context, _ string, _ interfaces.StockDataInclude) (*models.StockData, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockMarketService) FindSnipeBuys(_ context.Context, _ interfaces.SnipeOptions) ([]*models.SnipeBuy, error) {
	return nil, nil
}
func (m *mockMarketService) ScreenStocks(_ context.Context, _ interfaces.ScreenOptions) ([]*models.ScreenCandidate, error) {
	return nil, nil
}
func (m *mockMarketService) FunnelScreen(_ context.Context, _ interfaces.FunnelOptions) (*models.FunnelResult, error) {
	return nil, nil
}
func (m *mockMarketService) RefreshStaleData(_ context.Context, _ string) error { return nil }
func (m *mockMarketService) ScanMarket(_ context.Context, _ models.ScanQuery) (*models.ScanResponse, error) {
	return nil, nil
}
func (m *mockMarketService) ScanFields() *models.ScanFieldsResponse { return nil }
func (m *mockMarketService) ReadFiling(_ context.Context, _, _ string) (*models.FilingContent, error) {
	return nil, nil
}

type mockSignalService struct{}

func (m *mockSignalService) DetectSignals(_ context.Context, _ []string, _ []string, _ bool) ([]*models.TickerSignals, error) {
	return nil, nil
}
func (m *mockSignalService) ComputeSignals(_ context.Context, _ string, _ *models.MarketData) (*models.TickerSignals, error) {
	return nil, nil
}

type mockStorageManager struct {
	internal   *mockInternalStore
	market     *mockMarketDataStorage
	stockIndex *mockStockIndexStore
	jobQueue   *mockJobQueueStore
	files      *mockFileStore
}

func (m *mockStorageManager) InternalStore() interfaces.InternalStore         { return m.internal }
func (m *mockStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (m *mockStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return m.market }
func (m *mockStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (m *mockStorageManager) StockIndexStore() interfaces.StockIndexStore     { return m.stockIndex }
func (m *mockStorageManager) JobQueueStore() interfaces.JobQueueStore         { return m.jobQueue }
func (m *mockStorageManager) FileStore() interfaces.FileStore                 { return m.files }
func (m *mockStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (m *mockStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
func (m *mockStorageManager) DataPath() string                                { return "" }
func (m *mockStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (m *mockStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (m *mockStorageManager) Close() error                                { return nil }

type mockInternalStore struct {
	kv map[string]string
}

func (m *mockInternalStore) GetUser(_ context.Context, _ string) (*models.InternalUser, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockInternalStore) GetUserByEmail(_ context.Context, _ string) (*models.InternalUser, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockInternalStore) SaveUser(_ context.Context, _ *models.InternalUser) error { return nil }
func (m *mockInternalStore) DeleteUser(_ context.Context, _ string) error             { return nil }
func (m *mockInternalStore) ListUsers(_ context.Context) ([]string, error)            { return nil, nil }
func (m *mockInternalStore) GetUserKV(_ context.Context, _, _ string) (*models.UserKeyValue, error) {
	return nil, fmt.Errorf("not found")
}
func (m *mockInternalStore) SetUserKV(_ context.Context, _, _, _ string) error { return nil }
func (m *mockInternalStore) DeleteUserKV(_ context.Context, _, _ string) error { return nil }
func (m *mockInternalStore) ListUserKV(_ context.Context, _ string) ([]*models.UserKeyValue, error) {
	return nil, nil
}
func (m *mockInternalStore) GetSystemKV(_ context.Context, key string) (string, error) {
	if v, ok := m.kv[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("not found")
}
func (m *mockInternalStore) SetSystemKV(_ context.Context, key, value string) error {
	m.kv[key] = value
	return nil
}
func (m *mockInternalStore) Close() error { return nil }

type mockMarketDataStorage struct {
	data map[string]*models.MarketData
}

func (m *mockMarketDataStorage) GetMarketData(_ context.Context, ticker string) (*models.MarketData, error) {
	if md, ok := m.data[ticker]; ok {
		return md, nil
	}
	return nil, fmt.Errorf("not found")
}
func (m *mockMarketDataStorage) SaveMarketData(_ context.Context, data *models.MarketData) error {
	m.data[data.Ticker] = data
	return nil
}
func (m *mockMarketDataStorage) GetMarketDataBatch(_ context.Context, _ []string) ([]*models.MarketData, error) {
	return nil, nil
}
func (m *mockMarketDataStorage) GetStaleTickers(_ context.Context, _ string, _ int64) ([]string, error) {
	return nil, nil
}

// mockStockIndexStore is an in-memory stock index store for tests.
type mockStockIndexStore struct {
	mu      sync.Mutex
	entries map[string]*models.StockIndexEntry
}

func newMockStockIndexStore() *mockStockIndexStore {
	return &mockStockIndexStore{entries: make(map[string]*models.StockIndexEntry)}
}

func (m *mockStockIndexStore) Upsert(_ context.Context, entry *models.StockIndexEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.entries[entry.Ticker]; ok {
		existing.LastSeenAt = time.Now()
		existing.Source = entry.Source
	} else {
		entry.AddedAt = time.Now()
		entry.LastSeenAt = time.Now()
		m.entries[entry.Ticker] = entry
	}
	return nil
}

func (m *mockStockIndexStore) Get(_ context.Context, ticker string) (*models.StockIndexEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[ticker]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockStockIndexStore) List(_ context.Context) ([]*models.StockIndexEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.StockIndexEntry
	for _, e := range m.entries {
		result = append(result, e)
	}
	return result, nil
}

func (m *mockStockIndexStore) UpdateTimestamp(_ context.Context, ticker, field string, ts time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[ticker]
	if !ok {
		return nil
	}
	switch field {
	case "eod_collected_at":
		e.EODCollectedAt = ts
	case "fundamentals_collected_at":
		e.FundamentalsCollectedAt = ts
	case "filings_collected_at":
		e.FilingsCollectedAt = ts
	case "filings_pdfs_collected_at":
		e.FilingsPdfsCollectedAt = ts
	case "news_collected_at":
		e.NewsCollectedAt = ts
	case "filing_summaries_collected_at":
		e.FilingSummariesCollectedAt = ts
	case "timeline_collected_at":
		e.TimelineCollectedAt = ts
	case "signals_collected_at":
		e.SignalsCollectedAt = ts
	}
	return nil
}

func (m *mockStockIndexStore) Delete(_ context.Context, ticker string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, ticker)
	return nil
}

// mockJobQueueStore is an in-memory job queue store for tests.
type mockJobQueueStore struct {
	mu   sync.Mutex
	jobs []*models.Job
}

func newMockJobQueueStore() *mockJobQueueStore {
	return &mockJobQueueStore{}
}

func (m *mockJobQueueStore) Enqueue(_ context.Context, job *models.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if job.ID == "" {
		job.ID = fmt.Sprintf("j%d", len(m.jobs)+1)
	}
	if job.Status == "" {
		job.Status = models.JobStatusPending
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	m.jobs = append(m.jobs, job)
	return nil
}

func (m *mockJobQueueStore) Dequeue(_ context.Context) (*models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find highest priority pending job
	bestIdx := -1
	bestPriority := -1
	for i, j := range m.jobs {
		if j.Status == models.JobStatusPending && j.Priority > bestPriority {
			bestIdx = i
			bestPriority = j.Priority
		}
	}
	if bestIdx < 0 {
		return nil, nil
	}

	job := m.jobs[bestIdx]
	job.Status = models.JobStatusRunning
	job.StartedAt = time.Now()
	job.Attempts++
	return job, nil
}

func (m *mockJobQueueStore) Complete(_ context.Context, id string, jobErr error, durationMS int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			if jobErr != nil {
				j.Status = models.JobStatusFailed
				j.Error = jobErr.Error()
			} else {
				j.Status = models.JobStatusCompleted
			}
			j.DurationMS = durationMS
			j.CompletedAt = time.Now()
			return nil
		}
	}
	return nil
}

func (m *mockJobQueueStore) Cancel(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id && j.Status == models.JobStatusPending {
			j.Status = models.JobStatusCancelled
		}
	}
	return nil
}

func (m *mockJobQueueStore) SetPriority(_ context.Context, id string, priority int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			j.Priority = priority
		}
	}
	return nil
}

func (m *mockJobQueueStore) GetMaxPriority(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	maxP := 0
	for _, j := range m.jobs {
		if j.Status == models.JobStatusPending && j.Priority > maxP {
			maxP = j.Priority
		}
	}
	return maxP, nil
}

func (m *mockJobQueueStore) ListPending(_ context.Context, limit int) ([]*models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Job
	for _, j := range m.jobs {
		if j.Status == models.JobStatusPending {
			result = append(result, j)
		}
	}
	return result, nil
}

func (m *mockJobQueueStore) ListAll(_ context.Context, limit int) ([]*models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs, nil
}

func (m *mockJobQueueStore) ListByTicker(_ context.Context, ticker string) ([]*models.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*models.Job
	for _, j := range m.jobs {
		if j.Ticker == ticker {
			result = append(result, j)
		}
	}
	return result, nil
}

func (m *mockJobQueueStore) CountPending(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, j := range m.jobs {
		if j.Status == models.JobStatusPending {
			count++
		}
	}
	return count, nil
}

func (m *mockJobQueueStore) HasPendingJob(_ context.Context, jobType, ticker string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.JobType == jobType && j.Ticker == ticker && j.Status == models.JobStatusPending {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockJobQueueStore) PurgeCompleted(_ context.Context, olderThan time.Time) (int, error) {
	return 0, nil
}

func (m *mockJobQueueStore) CancelByTicker(_ context.Context, ticker string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, j := range m.jobs {
		if j.Ticker == ticker && j.Status == models.JobStatusPending {
			j.Status = models.JobStatusCancelled
			count++
		}
	}
	return count, nil
}

func (m *mockJobQueueStore) ResetRunningJobs(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, j := range m.jobs {
		if j.Status == models.JobStatusRunning {
			j.Status = models.JobStatusPending
			j.StartedAt = time.Time{}
			count++
		}
	}
	return count, nil
}

// mockFileStore is an in-memory file store for tests.
type mockFileStore struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMockFileStore() *mockFileStore {
	return &mockFileStore{files: make(map[string][]byte)}
}

func (m *mockFileStore) SaveFile(_ context.Context, category, key string, data []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[category+"/"+key] = data
	return nil
}

func (m *mockFileStore) GetFile(_ context.Context, category, key string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d, ok := m.files[category+"/"+key]; ok {
		return d, "application/octet-stream", nil
	}
	return nil, "", fmt.Errorf("not found")
}

func (m *mockFileStore) DeleteFile(_ context.Context, category, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, category+"/"+key)
	return nil
}

func (m *mockFileStore) HasFile(_ context.Context, category, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[category+"/"+key]
	return ok, nil
}

// --- tests ---

func TestJobManager_StartStop(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{
		Enabled:         true,
		WatcherInterval: "1h",
		MaxConcurrent:   1,
	}

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
	}

	jm := NewJobManager(newMockMarketService(), &mockSignalService{}, store, logger, config)
	jm.Start()

	if jm.cancel == nil {
		t.Error("expected cancel to be set after Start()")
	}

	jm.Stop()
	if jm.cancel != nil {
		t.Error("expected cancel to be nil after Stop()")
	}
}

func TestJobManager_EnqueueIfNeeded_Dedup(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{MaxConcurrent: 1, MaxRetries: 3}

	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	jm := NewJobManager(newMockMarketService(), &mockSignalService{}, store, logger, config)

	ctx := context.Background()

	// First enqueue should succeed
	if err := jm.EnqueueIfNeeded(ctx, models.JobTypeCollectEOD, "BHP.AU", 10); err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}

	// Second enqueue for same type+ticker should be deduped
	if err := jm.EnqueueIfNeeded(ctx, models.JobTypeCollectEOD, "BHP.AU", 10); err != nil {
		t.Fatalf("second enqueue failed: %v", err)
	}

	pending, _ := queue.CountPending(ctx)
	if pending != 1 {
		t.Errorf("expected 1 pending job (dedup), got %d", pending)
	}

	// Different job type should not be deduped
	if err := jm.EnqueueIfNeeded(ctx, models.JobTypeCollectFundamentals, "BHP.AU", 8); err != nil {
		t.Fatalf("third enqueue failed: %v", err)
	}

	pending, _ = queue.CountPending(ctx)
	if pending != 2 {
		t.Errorf("expected 2 pending jobs, got %d", pending)
	}
}

func TestJobManager_ScanStockIndex(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{MaxConcurrent: 1, MaxRetries: 3, PurgeAfter: "24h"}

	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	// Add a stock with all stale timestamps (zero values)
	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:   "BHP.AU",
		Code:     "BHP",
		Exchange: "AU",
		AddedAt:  time.Now().Add(-1 * time.Hour), // Not a new stock
	}

	jm := NewJobManager(newMockMarketService(), &mockSignalService{}, store, logger, config)

	ctx := context.Background()
	jm.scanStockIndex(ctx)

	// Should have enqueued jobs for all stale components (9 types):
	// fundamentals, filings_index, filing_pdfs, news, filing_summaries, timeline, news_intel, signals + 1 bulk EOD
	pending, _ := queue.CountPending(ctx)
	if pending != 9 {
		t.Errorf("expected 9 pending jobs for stale stock, got %d", pending)
	}
}

func TestJobManager_ScanStockIndex_NewStock_ElevatedPriority(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{MaxConcurrent: 1, MaxRetries: 3, PurgeAfter: "24h"}

	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	// Add a new stock (AddedAt within last 5 min, zero collection timestamps)
	stockIdx.entries["NEW.AU"] = &models.StockIndexEntry{
		Ticker:   "NEW.AU",
		Code:     "NEW",
		Exchange: "AU",
		AddedAt:  time.Now().Add(-1 * time.Minute), // Just added
	}

	jm := NewJobManager(newMockMarketService(), &mockSignalService{}, store, logger, config)

	ctx := context.Background()
	jm.scanStockIndex(ctx)

	// Per-ticker jobs should have PriorityNewStock; bulk EOD job has standard priority
	for _, job := range queue.jobs {
		if job.JobType == models.JobTypeCollectEODBulk {
			// Bulk EOD is per-exchange, not per-ticker — standard priority
			if job.Priority != models.PriorityCollectEODBulk {
				t.Errorf("expected priority %d for bulk EOD job, got %d",
					models.PriorityCollectEODBulk, job.Priority)
			}
		} else {
			if job.Priority != models.PriorityNewStock {
				t.Errorf("expected priority %d for new stock job %s, got %d",
					models.PriorityNewStock, job.JobType, job.Priority)
			}
		}
	}
}

func TestJobManager_ExecuteJob(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{MaxConcurrent: 1, MaxRetries: 3}

	market := newMockMarketService()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
	}

	jm := NewJobManager(market, &mockSignalService{}, store, logger, config)

	ctx := context.Background()

	tests := []struct {
		jobType     string
		expectedKey string
	}{
		{models.JobTypeCollectEOD, models.JobTypeCollectEOD},
		{models.JobTypeCollectEODBulk, models.JobTypeCollectEODBulk},
		{models.JobTypeCollectFundamentals, models.JobTypeCollectFundamentals},
		{models.JobTypeCollectFilings, models.JobTypeCollectFilings},
		{models.JobTypeCollectNews, models.JobTypeCollectNews},
		{models.JobTypeCollectFilingSummaries, models.JobTypeCollectFilingSummaries},
		{models.JobTypeCollectTimeline, models.JobTypeCollectTimeline},
		{models.JobTypeCollectNewsIntel, models.JobTypeCollectNewsIntel},
	}

	for _, tt := range tests {
		job := &models.Job{ID: "test", JobType: tt.jobType, Ticker: "BHP.AU"}
		err := jm.executeJob(ctx, job)
		if err != nil {
			t.Errorf("executeJob(%s) failed: %v", tt.jobType, err)
		}
		if market.collectCalls[tt.expectedKey] != 1 {
			t.Errorf("expected 1 call for %s, got %d", tt.expectedKey, market.collectCalls[tt.expectedKey])
		}
		// Reset for next iteration
		market.collectCalls[tt.expectedKey] = 0
	}
}

func TestJobManager_LastJobRun_Empty(t *testing.T) {
	logger := common.NewLogger("error")

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
	}

	config := common.JobManagerConfig{}
	jm := NewJobManager(nil, nil, store, logger, config)

	run := jm.LastJobRun(context.Background())
	if run != nil {
		t.Error("expected nil LastJobRun when no jobs have run")
	}
}

func TestJobManager_PushToTop(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{MaxConcurrent: 1}

	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	jm := NewJobManager(newMockMarketService(), &mockSignalService{}, store, logger, config)
	ctx := context.Background()

	// Enqueue two jobs
	queue.Enqueue(ctx, &models.Job{ID: "a", JobType: models.JobTypeCollectEOD, Ticker: "A.AU", Priority: 10})
	queue.Enqueue(ctx, &models.Job{ID: "b", JobType: models.JobTypeCollectTimeline, Ticker: "B.AU", Priority: 2})

	// Push job "b" to top
	if err := jm.PushToTop(ctx, "b"); err != nil {
		t.Fatalf("PushToTop failed: %v", err)
	}

	// Job "b" should now have highest priority
	queue.mu.Lock()
	var jobB *models.Job
	for _, j := range queue.jobs {
		if j.ID == "b" {
			jobB = j
		}
	}
	queue.mu.Unlock()

	if jobB == nil {
		t.Fatal("job b not found")
	}
	if jobB.Priority <= 10 {
		t.Errorf("expected job b priority > 10 after PushToTop, got %d", jobB.Priority)
	}
}

func TestWebSocketHub_BroadcastNoClients(t *testing.T) {
	logger := common.NewLogger("error")
	hub := NewJobWSHub(logger)
	go hub.Run()

	// Should not panic when broadcasting with no clients
	hub.Broadcast(models.JobEvent{
		Type:      "job_queued",
		Timestamp: time.Now(),
	})

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestTimestampFieldForJobType(t *testing.T) {
	tests := []struct {
		jobType  string
		expected string
	}{
		{models.JobTypeCollectEOD, "eod_collected_at"},
		{models.JobTypeCollectEODBulk, ""},
		{models.JobTypeCollectFundamentals, "fundamentals_collected_at"},
		{models.JobTypeCollectFilings, "filings_collected_at"},
		{models.JobTypeCollectNews, "news_collected_at"},
		{models.JobTypeCollectFilingSummaries, "filing_summaries_collected_at"},
		{models.JobTypeCollectTimeline, "timeline_collected_at"},
		{models.JobTypeComputeSignals, "signals_collected_at"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		got := models.TimestampFieldForJobType(tt.jobType)
		if got != tt.expected {
			t.Errorf("TimestampFieldForJobType(%s) = %s, want %s", tt.jobType, got, tt.expected)
		}
	}
}

func TestDefaultPriority(t *testing.T) {
	tests := []struct {
		jobType  string
		expected int
	}{
		{models.JobTypeCollectEOD, 10},
		{models.JobTypeCollectEODBulk, 10},
		{models.JobTypeComputeSignals, 9},
		{models.JobTypeCollectFundamentals, 8},
		{models.JobTypeCollectNews, 7},
		{models.JobTypeCollectFilings, 5},
		{models.JobTypeCollectNewsIntel, 4},
		{models.JobTypeCollectFilingSummaries, 3},
		{models.JobTypeCollectTimeline, 2},
		{"unknown", 0},
	}

	for _, tt := range tests {
		got := models.DefaultPriority(tt.jobType)
		if got != tt.expected {
			t.Errorf("DefaultPriority(%s) = %d, want %d", tt.jobType, got, tt.expected)
		}
	}
}

func TestIsHeavyJob(t *testing.T) {
	tests := []struct {
		jobType  string
		expected bool
	}{
		{models.JobTypeCollectFilings, false},        // Index only - not heavy
		{models.JobTypeCollectFilingPdfs, true},      // PDF downloads - heavy
		{models.JobTypeCollectFilingSummaries, true}, // AI summaries - heavy
		{models.JobTypeCollectEOD, false},
		{models.JobTypeCollectEODBulk, false},
		{models.JobTypeCollectFundamentals, false},
		{models.JobTypeCollectNews, false},
		{models.JobTypeCollectTimeline, false},
		{models.JobTypeCollectNewsIntel, false},
		{models.JobTypeComputeSignals, false},
		{"unknown", false},
	}

	for _, tt := range tests {
		got := isHeavyJob(tt.jobType)
		if got != tt.expected {
			t.Errorf("isHeavyJob(%s) = %v, want %v", tt.jobType, got, tt.expected)
		}
	}
}

func TestJobManager_HeavySemaphore_LimitsConcurrency(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{
		MaxConcurrent: 3,
		HeavyJobLimit: 1,
		MaxRetries:    3,
	}

	market := newMockMarketService()
	// Make filing PDF jobs take some time so we can detect concurrency
	var heavyRunning int
	var maxHeavyRunning int
	var mu sync.Mutex
	market.collectFilingPdfsFn = func(_ context.Context, _ string, _ bool) error {
		mu.Lock()
		heavyRunning++
		if heavyRunning > maxHeavyRunning {
			maxHeavyRunning = heavyRunning
		}
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		heavyRunning--
		mu.Unlock()
		return nil
	}

	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	jm := NewJobManager(market, &mockSignalService{}, store, logger, config)

	// Verify semaphore was created with correct capacity
	if cap(jm.heavySem) != 1 {
		t.Fatalf("heavySem capacity = %d, want 1", cap(jm.heavySem))
	}

	ctx := context.Background()

	// Enqueue 3 filing PDF jobs (all heavy)
	for i := 0; i < 3; i++ {
		queue.Enqueue(ctx, &models.Job{
			ID:          fmt.Sprintf("heavy-%d", i),
			JobType:     models.JobTypeCollectFilingPdfs,
			Ticker:      fmt.Sprintf("T%d.AU", i),
			Priority:    10,
			Status:      models.JobStatusPending,
			MaxAttempts: 3,
		})
	}

	// Start the job manager and wait for jobs to complete
	jm.Start()
	time.Sleep(500 * time.Millisecond)
	jm.Stop()

	mu.Lock()
	maxConcurrent := maxHeavyRunning
	mu.Unlock()

	// With heavy_job_limit=1, at most 1 heavy job should run at a time
	if maxConcurrent > 1 {
		t.Errorf("max concurrent heavy jobs = %d, want <= 1", maxConcurrent)
	}
}

func TestJobManager_HeavySemaphore_DefaultCapacity(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{
		MaxConcurrent: 5,
		// HeavyJobLimit not set — should default to 1
	}

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
	}

	jm := NewJobManager(newMockMarketService(), &mockSignalService{}, store, logger, config)
	if cap(jm.heavySem) != 1 {
		t.Errorf("heavySem capacity = %d, want 1 (default)", cap(jm.heavySem))
	}
}

func TestJobManager_WatcherStartupDelay(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{
		Enabled:             true,
		WatcherInterval:     "1h", // long interval so it doesn't re-scan
		WatcherStartupDelay: "200ms",
		MaxConcurrent:       1,
	}

	stockIdx := newMockStockIndexStore()
	// Add a stock so we can detect when the scan actually runs
	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:   "BHP.AU",
		Code:     "BHP",
		Exchange: "AU",
		AddedAt:  time.Now().Add(-1 * time.Hour),
	}

	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	jm := NewJobManager(newMockMarketService(), &mockSignalService{}, store, logger, config)

	start := time.Now()
	jm.Start()

	// Immediately after start, there should be no jobs (startup delay hasn't elapsed)
	time.Sleep(50 * time.Millisecond)
	pending, _ := queue.CountPending(context.Background())
	if pending > 0 {
		elapsed := time.Since(start)
		t.Errorf("expected 0 pending jobs before startup delay, got %d (elapsed %v)", pending, elapsed)
	}

	// Wait for startup delay + a bit for the scan to complete
	time.Sleep(300 * time.Millisecond)
	pending, _ = queue.CountPending(context.Background())
	if pending == 0 {
		t.Error("expected pending jobs after startup delay elapsed")
	}

	jm.Stop()
}

// --- demand-driven collection tests ---

func newTestJobManager(queue *mockJobQueueStore, stockIdx *mockStockIndexStore) *JobManager {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{MaxConcurrent: 1, MaxRetries: 3, PurgeAfter: "24h"}
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}
	return NewJobManager(newMockMarketService(), &mockSignalService{}, store, logger, config)
}

func TestEnqueueTickerJobs_StaleData(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	// Add a stock with all zero timestamps (all stale) that is not new
	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:   "BHP.AU",
		Code:     "BHP",
		Exchange: "AU",
		AddedAt:  time.Now().Add(-1 * time.Hour),
	}

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	n := jm.EnqueueTickerJobs(ctx, []string{"BHP.AU"})

	// Should enqueue jobs for all stale components:
	// 8 per-ticker jobs (fundamentals, filings_index, filing_pdfs, news, filing_summaries, timeline, news_intel, signals)
	// + 1 bulk EOD job for AU exchange = 9 total
	if n != 9 {
		t.Errorf("expected 9 jobs enqueued for stale data, got %d", n)
	}

	pending, _ := queue.CountPending(ctx)
	if pending != 9 {
		t.Errorf("expected 9 pending jobs, got %d", pending)
	}

	// Verify bulk EOD job exists for AU exchange
	found := false
	for _, j := range queue.jobs {
		if j.JobType == models.JobTypeCollectEODBulk && j.Ticker == "AU" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a collect_eod_bulk job for AU exchange")
	}
}

func TestEnqueueTickerJobs_FreshData(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	now := time.Now()
	// Add a stock with all fresh timestamps
	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:                     "BHP.AU",
		Code:                       "BHP",
		Exchange:                   "AU",
		AddedAt:                    now.Add(-1 * time.Hour),
		EODCollectedAt:             now,
		FundamentalsCollectedAt:    now,
		FilingsCollectedAt:         now,
		FilingsPdfsCollectedAt:     now,
		NewsCollectedAt:            now,
		FilingSummariesCollectedAt: now,
		TimelineCollectedAt:        now,
		SignalsCollectedAt:         now,
		NewsIntelCollectedAt:       now,
	}

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	n := jm.EnqueueTickerJobs(ctx, []string{"BHP.AU"})

	if n != 0 {
		t.Errorf("expected 0 jobs enqueued for fresh data, got %d", n)
	}

	pending, _ := queue.CountPending(ctx)
	if pending != 0 {
		t.Errorf("expected 0 pending jobs, got %d", pending)
	}
}

func TestEnqueueTickerJobs_MissingTicker(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()
	// Stock index is empty — tickers not in index

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	n := jm.EnqueueTickerJobs(ctx, []string{"MISSING.AU", "GONE.US"})

	if n != 0 {
		t.Errorf("expected 0 jobs for missing tickers, got %d", n)
	}

	pending, _ := queue.CountPending(ctx)
	if pending != 0 {
		t.Errorf("expected 0 pending jobs, got %d", pending)
	}
}

func TestEnqueueTickerJobs_BulkEODGrouping(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	now := time.Now()
	// Add 3 AU tickers with stale EOD but fresh everything else
	for _, ticker := range []string{"BHP.AU", "CBA.AU", "WBC.AU"} {
		stockIdx.entries[ticker] = &models.StockIndexEntry{
			Ticker:                     ticker,
			Code:                       ticker[:3],
			Exchange:                   "AU",
			AddedAt:                    now.Add(-1 * time.Hour),
			EODCollectedAt:             time.Time{}, // stale
			FundamentalsCollectedAt:    now,
			FilingsCollectedAt:         now,
			FilingsPdfsCollectedAt:     now, // fresh
			NewsCollectedAt:            now,
			FilingSummariesCollectedAt: now,
			TimelineCollectedAt:        now,
			SignalsCollectedAt:         now,
			NewsIntelCollectedAt:       now,
		}
	}

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	n := jm.EnqueueTickerJobs(ctx, []string{"BHP.AU", "CBA.AU", "WBC.AU"})

	// Only 1 bulk EOD job should exist for AU exchange (not 3)
	bulkEODCount := 0
	for _, j := range queue.jobs {
		if j.JobType == models.JobTypeCollectEODBulk {
			bulkEODCount++
		}
	}
	if bulkEODCount != 1 {
		t.Errorf("expected 1 bulk EOD job for AU exchange, got %d", bulkEODCount)
	}

	// Should be exactly 1 job total (just the bulk EOD)
	if n != 1 {
		t.Errorf("expected 1 job enqueued (1 bulk EOD), got %d", n)
	}
}

func TestEnqueueTickerJobs_BulkEODGrouping_MultipleExchanges(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	now := time.Now()
	// Add tickers across two exchanges with stale EOD
	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:                     "BHP.AU",
		Code:                       "BHP",
		Exchange:                   "AU",
		AddedAt:                    now.Add(-1 * time.Hour),
		EODCollectedAt:             time.Time{}, // stale
		FundamentalsCollectedAt:    now,
		FilingsCollectedAt:         now,
		FilingsPdfsCollectedAt:     now, // fresh
		NewsCollectedAt:            now,
		FilingSummariesCollectedAt: now,
		TimelineCollectedAt:        now,
		SignalsCollectedAt:         now,
		NewsIntelCollectedAt:       now,
	}
	stockIdx.entries["AAPL.US"] = &models.StockIndexEntry{
		Ticker:                     "AAPL.US",
		Code:                       "AAPL",
		Exchange:                   "US",
		AddedAt:                    now.Add(-1 * time.Hour),
		EODCollectedAt:             time.Time{}, // stale
		FundamentalsCollectedAt:    now,
		FilingsCollectedAt:         now,
		FilingsPdfsCollectedAt:     now, // fresh
		NewsCollectedAt:            now,
		FilingSummariesCollectedAt: now,
		TimelineCollectedAt:        now,
		SignalsCollectedAt:         now,
		NewsIntelCollectedAt:       now,
	}

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	n := jm.EnqueueTickerJobs(ctx, []string{"BHP.AU", "AAPL.US"})

	// Should have 2 bulk EOD jobs: one for AU, one for US
	bulkEODJobs := make(map[string]bool)
	for _, j := range queue.jobs {
		if j.JobType == models.JobTypeCollectEODBulk {
			bulkEODJobs[j.Ticker] = true
		}
	}
	if len(bulkEODJobs) != 2 {
		t.Errorf("expected 2 bulk EOD jobs (AU+US), got %d", len(bulkEODJobs))
	}
	if !bulkEODJobs["AU"] {
		t.Error("expected bulk EOD job for AU exchange")
	}
	if !bulkEODJobs["US"] {
		t.Error("expected bulk EOD job for US exchange")
	}

	if n != 2 {
		t.Errorf("expected 2 jobs enqueued, got %d", n)
	}
}

func TestEnqueueTickerJobs_Dedup(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	// Add a stock with stale fundamentals
	now := time.Now()
	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:                     "BHP.AU",
		Code:                       "BHP",
		Exchange:                   "AU",
		AddedAt:                    now.Add(-1 * time.Hour),
		EODCollectedAt:             now,
		FundamentalsCollectedAt:    time.Time{}, // stale
		FilingsCollectedAt:         now,
		FilingsPdfsCollectedAt:     now,
		NewsCollectedAt:            now,
		FilingSummariesCollectedAt: now,
		TimelineCollectedAt:        now,
		SignalsCollectedAt:         now,
		NewsIntelCollectedAt:       now,
	}

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	// Pre-enqueue a pending fundamentals job
	queue.Enqueue(ctx, &models.Job{
		JobType: models.JobTypeCollectFundamentals,
		Ticker:  "BHP.AU",
		Status:  models.JobStatusPending,
	})

	initialPending, _ := queue.CountPending(ctx)
	if initialPending != 1 {
		t.Fatalf("expected 1 pre-existing job, got %d", initialPending)
	}

	n := jm.EnqueueTickerJobs(ctx, []string{"BHP.AU"})

	// EnqueueIfNeeded returns nil for deduped jobs, so the count includes them.
	// The important assertion is that no duplicate job was created in the queue.
	if n != 1 {
		t.Errorf("expected 1 from EnqueueTickerJobs (deduped but counted), got %d", n)
	}

	// Critical: only 1 fundamentals job should exist (no duplicate)
	pending, _ := queue.CountPending(ctx)
	if pending != 1 {
		t.Errorf("expected 1 pending job (original, no duplicate), got %d", pending)
	}

	fundCount := 0
	for _, j := range queue.jobs {
		if j.JobType == models.JobTypeCollectFundamentals && j.Ticker == "BHP.AU" {
			fundCount++
		}
	}
	if fundCount != 1 {
		t.Errorf("expected 1 fundamentals job (deduped), got %d", fundCount)
	}
}

func TestEnqueueSlowDataJobs_EnqueuesAllTypes(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	n := jm.EnqueueSlowDataJobs(ctx, "BHP.AU")

	// Should enqueue 6 slow data job types:
	// collect_filing_pdfs, collect_filing_summaries, collect_timeline,
	// collect_news, collect_news_intel, compute_signals
	if n != 6 {
		t.Errorf("expected 6 slow data jobs, got %d", n)
	}

	pending, _ := queue.CountPending(ctx)
	if pending != 6 {
		t.Errorf("expected 6 pending jobs, got %d", pending)
	}

	// Verify all expected job types are present
	expectedTypes := map[string]bool{
		models.JobTypeCollectFilingPdfs:      false,
		models.JobTypeCollectFilingSummaries: false,
		models.JobTypeCollectTimeline:        false,
		models.JobTypeCollectNews:            false,
		models.JobTypeCollectNewsIntel:       false,
		models.JobTypeComputeSignals:         false,
	}

	for _, j := range queue.jobs {
		if _, ok := expectedTypes[j.JobType]; ok {
			expectedTypes[j.JobType] = true
		}
		// All jobs should be for the correct ticker
		if j.Ticker != "BHP.AU" {
			t.Errorf("expected ticker BHP.AU, got %q", j.Ticker)
		}
	}

	for jt, found := range expectedTypes {
		if !found {
			t.Errorf("expected job type %s not found in queue", jt)
		}
	}
}

func TestEnqueueSlowDataJobs_Dedup(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	// Pre-enqueue a pending filing PDFs job
	queue.Enqueue(ctx, &models.Job{
		JobType: models.JobTypeCollectFilingPdfs,
		Ticker:  "BHP.AU",
		Status:  models.JobStatusPending,
	})

	n := jm.EnqueueSlowDataJobs(ctx, "BHP.AU")

	// EnqueueIfNeeded returns nil for deduped jobs, so the count includes them.
	// All 6 calls succeed (no errors), so count is 6.
	if n != 6 {
		t.Errorf("expected 6 from EnqueueSlowDataJobs (deduped but counted), got %d", n)
	}

	// Critical: only 1 filing PDFs job should exist (no duplicate created)
	filingsCount := 0
	for _, j := range queue.jobs {
		if j.JobType == models.JobTypeCollectFilingPdfs && j.Ticker == "BHP.AU" {
			filingsCount++
		}
	}
	if filingsCount != 1 {
		t.Errorf("expected 1 filing PDFs job (deduped), got %d", filingsCount)
	}

	// Total jobs: 1 pre-existing filing PDFs + 5 newly created = 6
	totalJobs := len(queue.jobs)
	if totalJobs != 6 {
		t.Errorf("expected 6 total jobs in queue, got %d", totalJobs)
	}
}

func TestEnqueueSlowDataJobs_Priorities(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	jm.EnqueueSlowDataJobs(ctx, "BHP.AU")

	// Verify each job type has the correct default priority
	expectedPriorities := map[string]int{
		models.JobTypeCollectFilingPdfs:      models.PriorityCollectFilingPdfs,
		models.JobTypeCollectFilingSummaries: models.PriorityCollectFilingSummaries,
		models.JobTypeCollectTimeline:        models.PriorityCollectTimeline,
		models.JobTypeCollectNews:            models.PriorityCollectNews,
		models.JobTypeCollectNewsIntel:       models.PriorityCollectNewsIntel,
		models.JobTypeComputeSignals:         models.PriorityComputeSignals,
	}

	for _, j := range queue.jobs {
		if expected, ok := expectedPriorities[j.JobType]; ok {
			if j.Priority != expected {
				t.Errorf("job type %s: expected priority %d, got %d", j.JobType, expected, j.Priority)
			}
		}
	}
}

func TestEnqueueTickerJobs_EmptySlice(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	n := jm.EnqueueTickerJobs(ctx, []string{})
	if n != 0 {
		t.Errorf("expected 0 jobs for empty ticker slice, got %d", n)
	}

	n = jm.EnqueueTickerJobs(ctx, nil)
	if n != 0 {
		t.Errorf("expected 0 jobs for nil ticker slice, got %d", n)
	}
}

func TestEnqueueTickerJobs_PartialStaleness(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	now := time.Now()
	// Only fundamentals and news are stale; everything else is fresh
	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:                     "BHP.AU",
		Code:                       "BHP",
		Exchange:                   "AU",
		AddedAt:                    now.Add(-1 * time.Hour),
		EODCollectedAt:             now,
		FundamentalsCollectedAt:    time.Time{}, // stale
		FilingsCollectedAt:         now,
		FilingsPdfsCollectedAt:     now,
		NewsCollectedAt:            now.Add(-7 * time.Hour), // stale (TTL is 6h)
		FilingSummariesCollectedAt: now,
		TimelineCollectedAt:        now,
		SignalsCollectedAt:         now,
		NewsIntelCollectedAt:       now,
	}

	jm := newTestJobManager(queue, stockIdx)
	ctx := context.Background()

	n := jm.EnqueueTickerJobs(ctx, []string{"BHP.AU"})

	// Should enqueue exactly 2 jobs: fundamentals + news
	if n != 2 {
		t.Errorf("expected 2 jobs for partially stale data, got %d", n)
	}

	// Verify the correct job types
	jobTypes := make(map[string]bool)
	for _, j := range queue.jobs {
		jobTypes[j.JobType] = true
	}
	if !jobTypes[models.JobTypeCollectFundamentals] {
		t.Error("expected collect_fundamentals job for stale fundamentals")
	}
	if !jobTypes[models.JobTypeCollectNews] {
		t.Error("expected collect_news job for stale news")
	}
}

func TestEohdExchangeFromTicker(t *testing.T) {
	tests := []struct {
		ticker   string
		expected string
	}{
		{"BHP.AU", "AU"},
		{"AAPL.US", "US"},
		{"MSFT.US", "US"},
		{"NODOT", ""},
		{"", ""},
		{"A.B.C", "C"},
	}

	for _, tt := range tests {
		got := eohdExchangeFromTicker(tt.ticker)
		if got != tt.expected {
			t.Errorf("eohdExchangeFromTicker(%q) = %q, want %q", tt.ticker, got, tt.expected)
		}
	}
}
