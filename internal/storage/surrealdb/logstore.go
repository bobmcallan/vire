package surrealdb

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/google/uuid"
	"github.com/phuslu/log"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
	arbormodels "github.com/ternarybob/arbor/models"
	"github.com/ternarybob/arbor/writers"
)

const (
	logDefaultRetention = 24 * time.Hour
	logCleanupInterval  = 5 * time.Minute
	logTable            = "logs"
)

// logRecord is the SurrealDB representation of a log entry.
type logRecord struct {
	LogID         string                 `json:"log_id"`
	Timestamp     time.Time              `json:"timestamp"`
	Level         int                    `json:"level"`
	LevelName     string                 `json:"level_name"`
	Message       string                 `json:"message"`
	Caller        string                 `json:"caller,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Fields        map[string]interface{} `json:"fields,omitempty"`
	Source        string                 `json:"source"`
	Prefix        string                 `json:"prefix,omitempty"`
	Index         uint64                 `json:"idx"`
	TTLExpiresAt  time.Time              `json:"ttl_expires_at"`
}

// LogStore implements writers.ILogStore backed by SurrealDB.
type LogStore struct {
	db           *surrealdb.DB
	logger       *common.Logger
	source       string
	retention    time.Duration
	indexCounter atomic.Uint64
	cleanupStop  chan struct{}
	closeOnce    sync.Once
}

// NewLogStore creates a new SurrealDB-backed log store.
// source identifies the log origin (e.g. "server", "portal").
func NewLogStore(db *surrealdb.DB, logger *common.Logger, source string) *LogStore {
	s := &LogStore{
		db:          db,
		logger:      logger,
		source:      source,
		retention:   logDefaultRetention,
		cleanupStop: make(chan struct{}),
	}

	// Start TTL cleanup goroutine
	go s.cleanupLoop()

	return s
}

// Store adds a log entry to SurrealDB.
func (s *LogStore) Store(entry arbormodels.LogEvent) error {
	idx := s.indexCounter.Add(1)

	rec := logRecord{
		LogID:         fmt.Sprintf("lg_%s", uuid.New().String()[:8]),
		Timestamp:     entry.Timestamp,
		Level:         int(entry.Level),
		LevelName:     levelName(entry.Level),
		Message:       entry.Message,
		Caller:        entry.Function,
		CorrelationID: entry.CorrelationID,
		Error:         entry.Error,
		Fields:        entry.Fields,
		Source:        s.source,
		Prefix:        entry.Prefix,
		Index:         idx,
		TTLExpiresAt:  time.Now().Add(s.retention),
	}

	ctx := context.Background()
	sql := `UPSERT $rid SET
		log_id = $log_id, timestamp = $timestamp, level = $level, level_name = $level_name,
		message = $message, caller = $caller, correlation_id = $correlation_id,
		error = $error, fields = $fields, source = $source, prefix = $prefix,
		idx = $idx, ttl_expires_at = $ttl_expires_at`
	vars := map[string]any{
		"rid":            surrealmodels.NewRecordID(logTable, rec.LogID),
		"log_id":         rec.LogID,
		"timestamp":      rec.Timestamp,
		"level":          rec.Level,
		"level_name":     rec.LevelName,
		"message":        rec.Message,
		"caller":         rec.Caller,
		"correlation_id": rec.CorrelationID,
		"error":          rec.Error,
		"fields":         rec.Fields,
		"source":         rec.Source,
		"prefix":         rec.Prefix,
		"idx":            rec.Index,
		"ttl_expires_at": rec.TTLExpiresAt,
	}

	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to store log entry: %w", err)
	}
	return nil
}

// GetByCorrelation retrieves all logs for a correlation ID, ordered by timestamp.
func (s *LogStore) GetByCorrelation(correlationID string) ([]arbormodels.LogEvent, error) {
	if correlationID == "" {
		return []arbormodels.LogEvent{}, nil
	}

	ctx := context.Background()
	sql := `SELECT * FROM logs WHERE correlation_id = $cid ORDER BY timestamp ASC`
	vars := map[string]any{"cid": correlationID}

	results, err := surrealdb.Query[[]logRecord](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs by correlation: %w", err)
	}

	return recordsToEvents(results), nil
}

// GetByCorrelationWithLevel retrieves logs for a correlation ID filtered by minimum level.
func (s *LogStore) GetByCorrelationWithLevel(correlationID string, minLevel log.Level) ([]arbormodels.LogEvent, error) {
	if correlationID == "" {
		return []arbormodels.LogEvent{}, nil
	}

	ctx := context.Background()
	sql := `SELECT * FROM logs WHERE correlation_id = $cid AND level >= $min_level ORDER BY timestamp ASC`
	vars := map[string]any{
		"cid":       correlationID,
		"min_level": int(minLevel),
	}

	results, err := surrealdb.Query[[]logRecord](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs by correlation with level: %w", err)
	}

	return recordsToEvents(results), nil
}

// GetSince retrieves all logs since a given timestamp.
func (s *LogStore) GetSince(since time.Time) ([]arbormodels.LogEvent, error) {
	ctx := context.Background()
	sql := `SELECT * FROM logs WHERE timestamp > $since ORDER BY timestamp ASC`
	vars := map[string]any{"since": since}

	results, err := surrealdb.Query[[]logRecord](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs since: %w", err)
	}

	return recordsToEvents(results), nil
}

// GetRecent retrieves the N most recent log entries.
func (s *LogStore) GetRecent(limit int) ([]arbormodels.LogEvent, error) {
	if limit <= 0 {
		return []arbormodels.LogEvent{}, nil
	}

	ctx := context.Background()
	sql := `SELECT * FROM logs ORDER BY timestamp DESC, idx DESC LIMIT $limit`
	vars := map[string]any{"limit": limit}

	results, err := surrealdb.Query[[]logRecord](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent logs: %w", err)
	}

	return recordsToEvents(results), nil
}

// GetCorrelationIDs returns all distinct correlation IDs.
func (s *LogStore) GetCorrelationIDs() []string {
	ctx := context.Background()
	sql := `SELECT array::distinct(correlation_id) AS ids FROM logs GROUP ALL`
	type idsResult struct {
		IDs []string `json:"ids"`
	}

	results, err := surrealdb.Query[[]idsResult](ctx, s.db, sql, nil)
	if err != nil || results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return []string{}
	}

	return (*results)[0].Result[0].IDs
}

// Close is a no-op — the DB connection is owned by the Manager.
func (s *LogStore) Close() error {
	s.closeOnce.Do(func() {
		close(s.cleanupStop)
	})
	return nil
}

// GetRecentWithSource retrieves the N most recent log entries filtered by source.
func (s *LogStore) GetRecentWithSource(limit int, source string) ([]arbormodels.LogEvent, error) {
	if limit <= 0 {
		return []arbormodels.LogEvent{}, nil
	}

	ctx := context.Background()
	sql := `SELECT * FROM logs WHERE source = $source ORDER BY timestamp DESC, idx DESC LIMIT $limit`
	vars := map[string]any{
		"limit":  limit,
		"source": source,
	}

	results, err := surrealdb.Query[[]logRecord](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent logs by source: %w", err)
	}

	return recordsToEvents(results), nil
}

// cleanupLoop periodically deletes expired log entries.
func (s *LogStore) cleanupLoop() {
	ticker := time.NewTicker(logCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.deleteExpired()
		case <-s.cleanupStop:
			return
		}
	}
}

func (s *LogStore) deleteExpired() {
	ctx := context.Background()
	sql := `DELETE FROM logs WHERE ttl_expires_at < $now`
	vars := map[string]any{"now": time.Now()}

	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		s.logger.Warn().Str("error", err.Error()).Msg("failed to clean up expired logs")
	}
}

// recordsToEvents converts SurrealDB query results to arbor LogEvents.
func recordsToEvents(results *[]surrealdb.QueryResult[[]logRecord]) []arbormodels.LogEvent {
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return []arbormodels.LogEvent{}
	}

	records := (*results)[0].Result
	events := make([]arbormodels.LogEvent, len(records))
	for i, r := range records {
		events[i] = arbormodels.LogEvent{
			Index:         r.Index,
			Level:         log.Level(r.Level),
			Timestamp:     r.Timestamp,
			CorrelationID: r.CorrelationID,
			Prefix:        r.Prefix,
			Message:       r.Message,
			Error:         r.Error,
			Function:      r.Caller,
			Fields:        r.Fields,
		}
	}
	return events
}

func levelName(l log.Level) string {
	switch l {
	case log.TraceLevel:
		return "trace"
	case log.DebugLevel:
		return "debug"
	case log.InfoLevel:
		return "info"
	case log.WarnLevel:
		return "warn"
	case log.ErrorLevel:
		return "error"
	case log.FatalLevel:
		return "fatal"
	case log.PanicLevel:
		return "panic"
	default:
		return "info"
	}
}

// Compile-time check
var _ writers.ILogStore = (*LogStore)(nil)
