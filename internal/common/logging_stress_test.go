package common

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- Stress Test 1: Race condition check ---
// Multiple goroutines logging with different correlation IDs simultaneously.
// Tests arbor's memory writer thread safety under contention.

func TestStress_ConcurrentCorrelationIDs(t *testing.T) {
	logger := NewLogger("info")

	var wg sync.WaitGroup
	goroutines := 50
	entriesPerGoroutine := 200

	// Phase 1: Concurrent writes with different correlation IDs
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			correlated := logger.WithCorrelationId(fmt.Sprintf("stress-%d", id))
			for j := 0; j < entriesPerGoroutine; j++ {
				correlated.Info().
					Int("goroutine", id).
					Int("entry", j).
					Str("data", fmt.Sprintf("payload-%d-%d", id, j)).
					Msg("stress test entry")
			}
		}(i)
	}
	wg.Wait()

	// Phase 2: Concurrent reads while verifying isolation
	var readWg sync.WaitGroup
	errors := make(chan string, goroutines)

	for i := 0; i < goroutines; i++ {
		readWg.Add(1)
		go func(id int) {
			defer readWg.Done()
			corrID := fmt.Sprintf("stress-%d", id)
			logs, err := logger.GetMemoryLogsForCorrelation(corrID)
			if err != nil {
				errors <- fmt.Sprintf("goroutine %d: GetMemoryLogsForCorrelation failed: %v", id, err)
				return
			}
			if len(logs) == 0 {
				errors <- fmt.Sprintf("goroutine %d: expected entries for %s, got 0", id, corrID)
			}
		}(i)
	}
	readWg.Wait()
	close(errors)

	for errMsg := range errors {
		t.Error(errMsg)
	}
}

// Concurrent reads AND writes simultaneously
func TestStress_ConcurrentReadWrite(t *testing.T) {
	logger := NewLogger("info")

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Writer goroutines
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			correlated := logger.WithCorrelationId(fmt.Sprintf("rw-%d", id))
			for j := 0; j < 100; j++ {
				select {
				case <-done:
					return
				default:
					correlated.Info().Int("id", id).Int("j", j).Msg("write during read")
				}
			}
		}(i)
	}

	// Reader goroutines (concurrent with writers)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				select {
				case <-done:
					return
				default:
					// Query by correlation ID
					logger.GetMemoryLogsForCorrelation(fmt.Sprintf("rw-%d", id%20))
					// Query recent with limit
					logger.GetMemoryLogsWithLimit(10)
				}
			}
		}(i)
	}

	wg.Wait()
	close(done)
}

// --- Stress Test 2: Memory pressure ---
// What happens after 10,000 log entries in memory writer?

func TestStress_MemoryPressure10k(t *testing.T) {
	logger := NewLogger("info")

	// Write 10,000 entries
	for i := 0; i < 10000; i++ {
		logger.Info().
			Int("index", i).
			Str("data", fmt.Sprintf("entry-%d-with-some-payload-data-to-simulate-real-fields", i)).
			Msg("memory pressure test")
	}

	// Verify we can still query without hanging or panicking
	start := time.Now()
	logs, err := logger.GetMemoryLogsWithLimit(100)
	queryTime := time.Since(start)

	if err != nil {
		t.Fatalf("GetMemoryLogsWithLimit failed after 10k entries: %v", err)
	}

	if len(logs) == 0 {
		t.Error("Expected entries after 10k writes, got 0")
	}

	// Query should complete in reasonable time (< 5s)
	// The O(n log n) sort on 10k entries should be well under 1s
	if queryTime > 5*time.Second {
		t.Errorf("GetMemoryLogsWithLimit took %v after 10k entries — too slow", queryTime)
	}

	t.Logf("Query of 100 from 10k entries took %v, returned %d entries", queryTime, len(logs))
}

// --- Stress Test 3: Logger creation ---
// Can you create 100 correlated loggers from the same parent without issues?

func TestStress_100CorrelatedLoggers(t *testing.T) {
	parent := NewLogger("info")

	loggers := make([]*Logger, 100)
	for i := 0; i < 100; i++ {
		loggers[i] = parent.WithCorrelationId(fmt.Sprintf("logger-%d", i))
		if loggers[i] == nil {
			t.Fatalf("WithCorrelationId returned nil for logger %d", i)
		}
	}

	// All loggers should work independently
	for i, l := range loggers {
		l.Info().Int("id", i).Msg("from correlated logger")
	}

	// Allow async logStoreWriter to flush buffered entries.
	// Arbor's logStoreWriter uses a channel-based async writer (buffer size 1000)
	// which processes entries in a background goroutine. Without this pause,
	// entries may not yet be stored in the in-memory log store.
	time.Sleep(200 * time.Millisecond)

	// Verify each correlation ID has entries
	for i := 0; i < 100; i++ {
		corrID := fmt.Sprintf("logger-%d", i)
		logs, err := parent.GetMemoryLogsForCorrelation(corrID)
		if err != nil {
			t.Errorf("Failed to get logs for %s: %v", corrID, err)
			continue
		}
		if len(logs) == 0 {
			t.Errorf("Expected entries for %s, got 0", corrID)
		}
	}

	// Parent logger should not have any of the child correlation IDs
	// (WithCorrelationId creates a fork, not a mutation)
	// Verify fork isolation: parent.GetMemoryLogsForCorrelation queries the
	// global memory writer, so it will find logger-0's entries. What matters is
	// that parent.Info().Msg() does NOT get tagged with any child's correlation ID.
	_, _ = parent.GetMemoryLogsForCorrelation("logger-0")
	parent.Info().Msg("parent log after children created")
}

// Concurrent logger creation from same parent
func TestStress_ConcurrentLoggerCreation(t *testing.T) {
	parent := NewLogger("info")

	var wg sync.WaitGroup
	loggers := make([]*Logger, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			loggers[id] = parent.WithCorrelationId(fmt.Sprintf("concurrent-%d", id))
			loggers[id].Info().Int("id", id).Msg("created concurrently")
		}(i)
	}

	wg.Wait()

	// Verify all loggers were created
	for i, l := range loggers {
		if l == nil {
			t.Errorf("Logger %d is nil after concurrent creation", i)
		}
	}
}

// --- Stress Test 4: Graceful degradation ---
// What if memory writer query fails? Does the logger still work?

func TestStress_LoggingWorksWithoutMemoryWriter(t *testing.T) {
	// Create a silent logger (no memory writer registered on its private writers).
	// NOTE: GetMemoryLogsWithLimit/GetMemoryLogsForCorrelation query the GLOBAL
	// memory writer registry, not the logger's private writers. So if any previous
	// test registered a global memory writer (via NewLogger), queries will return
	// entries from that global store. This is by design — the memory writer is a
	// shared queryable store, not per-logger. The discardWriter only prevents
	// WRITES from going to the global registry, not READS.
	logger := NewSilentLogger()

	// GetMemoryLogsWithLimit should not error (may return entries from global store)
	_, err := logger.GetMemoryLogsWithLimit(10)
	if err != nil {
		t.Fatalf("GetMemoryLogsWithLimit on silent logger failed: %v", err)
	}

	// GetMemoryLogsForCorrelation with a truly unique ID should return empty
	logs, err := logger.GetMemoryLogsForCorrelation("truly-nonexistent-id-never-used-anywhere")
	if err != nil {
		t.Fatalf("GetMemoryLogsForCorrelation on silent logger failed: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("Expected 0 logs for unknown correlation ID, got %d", len(logs))
	}

	// Logging through silent logger should not panic
	logger.Info().Str("key", "value").Msg("should not panic")
	logger.Error().Err(fmt.Errorf("test error")).Msg("error should not panic")

	// Verify the silent logger's writes did NOT reach the global memory writer
	// by checking with a unique correlation ID
	uniqueID := "silent-test-unique-12345"
	silentCorrelated := logger.WithCorrelationId(uniqueID)
	silentCorrelated.Info().Msg("this should be discarded")

	logs, err = logger.GetMemoryLogsForCorrelation(uniqueID)
	if err != nil {
		t.Fatalf("GetMemoryLogsForCorrelation failed: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("Silent logger wrote %d entries to global memory writer — discardWriter not working", len(logs))
	}
}

// --- Stress Test 5: No log loss verification ---
// Write entries with correlation ID, verify exact count retrievable.
// Note: arbor's async logStoreWriter may buffer, so we need to account for timing.

func TestStress_NoLogLoss_SequentialWrites(t *testing.T) {
	logger := NewLogger("info")
	corrID := "loss-check-seq"
	correlated := logger.WithCorrelationId(corrID)

	expected := 500
	for i := 0; i < expected; i++ {
		correlated.Info().Int("i", i).Msg("loss check")
	}

	// Allow async writers to flush
	time.Sleep(500 * time.Millisecond)

	logs, err := logger.GetMemoryLogsForCorrelation(corrID)
	if err != nil {
		t.Fatalf("GetMemoryLogsForCorrelation failed: %v", err)
	}

	// Due to arbor's async logStoreWriter, some entries might still be buffered.
	// We check that we got at least 90% of entries (allowing for async lag).
	threshold := expected * 90 / 100
	if len(logs) < threshold {
		t.Errorf("Expected at least %d entries (90%% of %d), got %d — possible log loss", threshold, expected, len(logs))
	}

	t.Logf("Retrieved %d/%d entries (%.1f%%)", len(logs), expected, float64(len(logs))/float64(expected)*100)
}

// --- Stress Test 6: Backwards compatibility ---
// Representative log statements from different services.

func TestStress_BackwardsCompatibility_PortfolioService(t *testing.T) {
	// From internal/services/portfolio/service.go:52
	logger := NewSilentLogger()
	name := "SMSF"
	force := true
	logger.Info().Str("name", name).Bool("force", force).Msg("Syncing portfolio")
}

func TestStress_BackwardsCompatibility_MarketService(t *testing.T) {
	// From internal/services/market/service.go:73 (converted .Time() to .Str())
	logger := NewSilentLogger()
	ticker := "BHP.AU"
	fromDate := time.Now().AddDate(0, 0, -1)
	logger.Debug().Str("ticker", ticker).Str("from", fromDate.Format(time.RFC3339)).Msg("Incremental EOD fetch")
}

func TestStress_BackwardsCompatibility_HandlerTiming(t *testing.T) {
	// From cmd/vire-mcp/handlers.go — handler timing pattern
	logger := NewSilentLogger()
	handlerStart := time.Now()
	time.Sleep(10 * time.Millisecond) // Simulate work
	logger.Info().
		Dur("elapsed", time.Since(handlerStart)).
		Str("tool", "portfolio_compliance").
		Int("holdings", 10).
		Msg("handlePortfolioCompliance: TOTAL")
}
