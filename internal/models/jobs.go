package models

import "time"

// StockIndexEntry represents a stock in the shared cross-user index.
// The stock index is a user-agnostic registry of all stocks the system should track.
// Any user process that touches a ticker upserts it to the index.
type StockIndexEntry struct {
	Ticker   string `json:"ticker"`   // EODHD format: "BHP.AU"
	Code     string `json:"code"`     // Base code: "BHP"
	Exchange string `json:"exchange"` // Exchange: "AU"
	Name     string `json:"name"`     // Company name (populated from fundamentals)
	Source   string `json:"source"`   // How it was added: "portfolio", "watchlist", "search", "manual"

	// Data freshness timestamps — updated when corresponding job completes
	EODCollectedAt             time.Time `json:"eod_collected_at"`
	FundamentalsCollectedAt    time.Time `json:"fundamentals_collected_at"`
	FilingsCollectedAt         time.Time `json:"filings_collected_at"`      // Index collection (fast path)
	FilingsPdfsCollectedAt     time.Time `json:"filings_pdfs_collected_at"` // PDF download (slow path)
	NewsCollectedAt            time.Time `json:"news_collected_at"`
	FilingSummariesCollectedAt time.Time `json:"filing_summaries_collected_at"`
	TimelineCollectedAt        time.Time `json:"timeline_collected_at"`
	SignalsCollectedAt         time.Time `json:"signals_collected_at"`
	NewsIntelCollectedAt       time.Time `json:"news_intel_collected_at"`

	// Lifecycle
	AddedAt    time.Time `json:"added_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// Job represents a unit of work in the job queue.
type Job struct {
	ID          string    `json:"id"`
	JobType     string    `json:"job_type"`
	Ticker      string    `json:"ticker"`
	Priority    int       `json:"priority"`
	Status      string    `json:"status"` // "pending", "running", "completed", "failed", "cancelled"
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Error       string    `json:"error,omitempty"`
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"max_attempts"`
	DurationMS  int64     `json:"duration_ms"`
}

// Job type constants
const (
	JobTypeCollectEOD             = "collect_eod"
	JobTypeCollectEODBulk         = "collect_eod_bulk"
	JobTypeCollectFundamentals    = "collect_fundamentals"
	JobTypeCollectFilings         = "collect_filings"
	JobTypeCollectFilingPdfs      = "collect_filing_pdfs" // NEW: Download PDFs only (index is fast path)
	JobTypeCollectNews            = "collect_news"
	JobTypeCollectFilingSummaries = "collect_filing_summaries"
	JobTypeCollectTimeline        = "collect_timeline"
	JobTypeCollectNewsIntel       = "collect_news_intel"
	JobTypeComputeSignals         = "compute_signals"
)

// Job status constants
const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"
)

// Default priorities (higher = processed first)
const (
	PriorityCollectEOD             = 10
	PriorityCollectEODBulk         = 10
	PriorityComputeSignals         = 9
	PriorityCollectFundamentals    = 8
	PriorityCollectNews            = 7
	PriorityCollectFilings         = 5
	PriorityCollectFilingPdfs      = 4 // Same level as summaries (heavy job)
	PriorityCollectNewsIntel       = 4
	PriorityCollectFilingSummaries = 3
	PriorityCollectTimeline        = 2
	PriorityNewStock               = 15 // New stocks get elevated priority
)

// DefaultPriority returns the default priority for a job type.
func DefaultPriority(jobType string) int {
	switch jobType {
	case JobTypeCollectEOD:
		return PriorityCollectEOD
	case JobTypeCollectEODBulk:
		return PriorityCollectEODBulk
	case JobTypeCollectFundamentals:
		return PriorityCollectFundamentals
	case JobTypeCollectFilings:
		return PriorityCollectFilings
	case JobTypeCollectFilingPdfs:
		return PriorityCollectFilingPdfs
	case JobTypeCollectNews:
		return PriorityCollectNews
	case JobTypeCollectFilingSummaries:
		return PriorityCollectFilingSummaries
	case JobTypeCollectTimeline:
		return PriorityCollectTimeline
	case JobTypeCollectNewsIntel:
		return PriorityCollectNewsIntel
	case JobTypeComputeSignals:
		return PriorityComputeSignals
	default:
		return 0
	}
}

// TimestampFieldForJobType maps a job type to the StockIndexEntry timestamp field name.
// Returns "" for bulk jobs — timestamps are updated per-ticker by the market service.
func TimestampFieldForJobType(jobType string) string {
	switch jobType {
	case JobTypeCollectEOD:
		return "eod_collected_at"
	case JobTypeCollectEODBulk:
		return "" // handled per-ticker by CollectBulkEOD
	case JobTypeCollectFundamentals:
		return "fundamentals_collected_at"
	case JobTypeCollectFilings:
		return "filings_collected_at" // Index collection (fast path)
	case JobTypeCollectFilingPdfs:
		return "filings_pdfs_collected_at" // PDF download (slow path)
	case JobTypeCollectNews:
		return "news_collected_at"
	case JobTypeCollectFilingSummaries:
		return "filing_summaries_collected_at"
	case JobTypeCollectTimeline:
		return "timeline_collected_at"
	case JobTypeCollectNewsIntel:
		return "news_intel_collected_at"
	case JobTypeComputeSignals:
		return "signals_collected_at"
	default:
		return ""
	}
}

// JobEvent is broadcast via WebSocket when job state changes.
type JobEvent struct {
	Type      string    `json:"type"` // "job_queued", "job_started", "job_completed", "job_failed"
	Job       *Job      `json:"job"`
	Timestamp time.Time `json:"timestamp"`
	QueueSize int       `json:"queue_size"` // Current pending count
}
