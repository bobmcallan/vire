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
	mu            sync.Mutex
	collectCalls  map[string]int // job_type -> call count
	collectCoreFn func(ctx context.Context, tickers []string, force bool) error
	collectFullFn func(ctx context.Context, tickers []string, includeNews bool, force bool) error
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
func (m *mockMarketService) CollectFilings(_ context.Context, _ string, _ bool) error {
	m.mu.Lock()
	m.collectCalls[models.JobTypeCollectFilings]++
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
	if err := jm.enqueueIfNeeded(ctx, models.JobTypeCollectEOD, "BHP.AU", 10); err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}

	// Second enqueue for same type+ticker should be deduped
	if err := jm.enqueueIfNeeded(ctx, models.JobTypeCollectEOD, "BHP.AU", 10); err != nil {
		t.Fatalf("second enqueue failed: %v", err)
	}

	pending, _ := queue.CountPending(ctx)
	if pending != 1 {
		t.Errorf("expected 1 pending job (dedup), got %d", pending)
	}

	// Different job type should not be deduped
	if err := jm.enqueueIfNeeded(ctx, models.JobTypeCollectFundamentals, "BHP.AU", 8); err != nil {
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

	// Should have enqueued jobs for all stale components (8 types)
	pending, _ := queue.CountPending(ctx)
	if pending != 8 {
		t.Errorf("expected 8 pending jobs for stale stock, got %d", pending)
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
