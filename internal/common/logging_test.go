package common

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// NOTE: The 2 .Time() call sites (market/service.go:73, portfolio/snapshot.go:87)
// are converted to .Str(key, t.Format(time.RFC3339)). No test needed — the compiler
// enforces this since arbor's ILogEvent has no .Time() method. If the conversion is
// missing, the build fails.

// --- Test 1: Logger creation ---

func TestNewLogger_ReturnsNonNil(t *testing.T) {
	logger := NewLogger("info")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}
}

func TestNewLogger_FluentAPI(t *testing.T) {
	// Must not panic — proves the fluent chain works with arbor
	logger := NewLogger("error")
	logger.Info().Str("key", "value").Msg("test message")
	logger.Warn().Int("count", 42).Msg("warning")
	logger.Error().Err(nil).Msg("error message")
	logger.Debug().Float64("rate", 3.14).Bool("ok", true).Msg("debug")
}

func TestNewLoggerWithOutput_WritesToProvidedWriter(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithOutput("info", &buf)
	logger.Info().Str("key", "value").Msg("hello")

	output := buf.String()
	if output == "" {
		t.Error("Expected output to provided writer, got empty string")
	}
}

func TestNewDefaultLogger_ReturnsNonNil(t *testing.T) {
	logger := NewDefaultLogger()
	if logger == nil {
		t.Fatal("NewDefaultLogger returned nil")
	}
}

// --- Test 2: Silent logger discards output (DA Finding #1) ---

func TestNewSilentLogger_DiscardsOutput(t *testing.T) {
	logger := NewSilentLogger()
	if logger == nil {
		t.Fatal("NewSilentLogger returned nil")
	}
	// Must not panic
	logger.Info().Str("key", "value").Msg("should be discarded")
	logger.Error().Err(nil).Msg("should be discarded")
	logger.Warn().Msg("should be discarded")
}

func TestNewSilentLogger_DoesNotWriteToGlobalWriters(t *testing.T) {
	// DA Finding #1: Silent logger must not dispatch to globally-registered writers.
	// This test is self-contained: it creates a normal logger first (which registers
	// global writers), then verifies the silent logger doesn't leak through them.
	// The test is valid regardless of execution order because it sets up its own
	// precondition (global writer registration) explicitly.
	var buf bytes.Buffer
	_ = NewLoggerWithOutput("info", &buf)

	// Reset buffer after any initialization output
	buf.Reset()

	// Now create silent logger and write through it
	silent := NewSilentLogger()
	silent.Info().Str("key", "value").Msg("this should NOT appear")
	silent.Error().Msg("this should NOT appear either")

	if buf.Len() > 0 {
		t.Errorf("Silent logger wrote %d bytes to global writer: %s", buf.Len(), buf.String())
	}
}

// --- Test 3: No stdout writes (DA Finding #2) ---

func TestNewLogger_DoesNotWriteToStdout(t *testing.T) {
	// DA Finding #2: Vire uses stdio transport. stdout IS the MCP JSON-RPC channel.
	// Console writer MUST route to stderr, never stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdout = w

	logger := NewLogger("info")
	logger.Info().Str("tool", "test").Msg("this must not go to stdout")
	logger.Error().Msg("neither should this")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	r.Close()

	if buf.Len() > 0 {
		t.Errorf("Logger wrote %d bytes to stdout (would corrupt MCP stdio): %s", buf.Len(), buf.String())
	}
}

// --- Test 4: Correlation ID ---

func TestWithCorrelationId_ReturnsNewLogger(t *testing.T) {
	logger := NewLogger("info")
	correlated := logger.WithCorrelationId("test-req-123")

	if correlated == nil {
		t.Fatal("WithCorrelationId returned nil")
	}
	// Must be a different instance
	if correlated == logger {
		t.Error("WithCorrelationId should return a new Logger instance, not the same one")
	}
}

func TestWithCorrelationId_FluentAPI(t *testing.T) {
	logger := NewLogger("error")
	correlated := logger.WithCorrelationId("test-req-456")
	// Must not panic
	correlated.Info().Str("tool", "portfolio_compliance").Msg("handler start")
	correlated.Info().Dur("elapsed", 0).Msg("handler complete")
}

// --- Test 5: Memory writer query ---

func TestGetMemoryLogsWithLimit_ReturnsEntries(t *testing.T) {
	// Self-contained: creates its own logger which registers the memory writer.
	// Valid regardless of test execution order.
	logger := NewLogger("info")

	// Write some logs
	logger.Info().Str("tool", "test1").Msg("first message")
	logger.Info().Str("tool", "test2").Msg("second message")
	logger.Warn().Msg("warning message")

	// Arbor's memory writer is async — allow buffer to flush
	time.Sleep(200 * time.Millisecond)

	// Query memory writer
	logs, err := logger.GetMemoryLogsWithLimit(10)
	if err != nil {
		t.Fatalf("GetMemoryLogsWithLimit failed: %v", err)
	}

	if len(logs) == 0 {
		t.Error("Expected memory writer to contain log entries, got 0")
	}
}

func TestGetMemoryLogsForCorrelation_FiltersById(t *testing.T) {
	logger := NewLogger("info")

	// Write logs with different correlation IDs
	c1 := logger.WithCorrelationId("req-AAA")
	c2 := logger.WithCorrelationId("req-BBB")

	c1.Info().Str("tool", "review").Msg("c1 message")
	c2.Info().Str("tool", "screen").Msg("c2 message")
	c1.Info().Msg("c1 second message")

	// Arbor's memory writer is async — allow buffer to flush
	time.Sleep(200 * time.Millisecond)

	// Query for correlation ID "req-AAA"
	logs, err := logger.GetMemoryLogsForCorrelation("req-AAA")
	if err != nil {
		t.Fatalf("GetMemoryLogsForCorrelation failed: %v", err)
	}

	if len(logs) == 0 {
		t.Error("Expected memory logs for correlation 'req-AAA', got 0")
	}

	// Verify none of the returned logs are from req-BBB
	for key, val := range logs {
		combined := key + val
		if strings.Contains(combined, "req-BBB") {
			t.Errorf("GetMemoryLogsForCorrelation returned entry from wrong correlation: %s=%s", key, val)
		}
	}
}

// --- Test 6: Level filtering ---

func TestLogLevel_DebugFilteredAtInfoLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithOutput("info", &buf)

	logger.Debug().Msg("debug message should not appear")

	if strings.Contains(buf.String(), "debug message should not appear") {
		t.Error("Debug message appeared at info level — level filtering is broken")
	}
}

func TestLogLevel_InfoVisibleAtInfoLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithOutput("info", &buf)

	logger.Info().Msg("info message should appear")

	if !strings.Contains(buf.String(), "info message should appear") {
		t.Errorf("Info message not visible at info level — got: %s", buf.String())
	}
}

func TestLogLevel_ErrorVisibleAtWarnLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithOutput("warn", &buf)

	logger.Error().Msg("error message should appear")

	if !strings.Contains(buf.String(), "error message should appear") {
		t.Errorf("Error message not visible at warn level — got: %s", buf.String())
	}
}

func TestLogLevel_InfoFilteredAtWarnLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerWithOutput("warn", &buf)

	logger.Info().Msg("info message should not appear at warn level")

	if strings.Contains(buf.String(), "info message should not appear") {
		t.Error("Info message appeared at warn level — level filtering is broken")
	}
}

// --- Test 7: Memory writer edge cases ---

func TestGetMemoryLogsWithLimit_ZeroLimit_ReturnsEmpty(t *testing.T) {
	logger := NewLogger("info")
	logger.Info().Msg("test entry")

	logs, err := logger.GetMemoryLogsWithLimit(0)
	if err != nil {
		t.Fatalf("GetMemoryLogsWithLimit(0) failed: %v", err)
	}

	if len(logs) != 0 {
		t.Errorf("Expected 0 entries for limit=0, got %d", len(logs))
	}
}

func TestGetMemoryLogsWithLimit_NegativeLimit_ReturnsEmpty(t *testing.T) {
	logger := NewLogger("info")
	logger.Info().Msg("test entry")

	logs, err := logger.GetMemoryLogsWithLimit(-1)
	if err != nil {
		t.Fatalf("GetMemoryLogsWithLimit(-1) failed: %v", err)
	}

	if len(logs) != 0 {
		t.Errorf("Expected 0 entries for limit=-1, got %d", len(logs))
	}
}

func TestGetMemoryLogsForCorrelation_UnknownId_ReturnsEmpty(t *testing.T) {
	logger := NewLogger("info")
	logger.Info().Msg("test entry with no correlation")

	logs, err := logger.GetMemoryLogsForCorrelation("nonexistent-id-12345")
	if err != nil {
		t.Fatalf("GetMemoryLogsForCorrelation with unknown ID failed: %v", err)
	}

	if len(logs) != 0 {
		t.Errorf("Expected 0 entries for unknown correlation ID, got %d", len(logs))
	}
}

// --- Test 8: Concurrent access ---

func TestConcurrentLogging_NoRaceOrPanic(t *testing.T) {
	logger := NewLogger("info")

	var wg sync.WaitGroup
	goroutines := 10
	entriesPerGoroutine := 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			correlated := logger.WithCorrelationId(fmt.Sprintf("goroutine-%d", id))
			for j := 0; j < entriesPerGoroutine; j++ {
				correlated.Info().
					Int("goroutine", id).
					Int("entry", j).
					Msg("concurrent log entry")
			}
		}(i)
	}

	wg.Wait()

	// Arbor's memory writer is async — allow buffer to flush
	time.Sleep(500 * time.Millisecond)

	// Verify entries are retrievable per correlation ID and no cross-contamination
	for i := 0; i < goroutines; i++ {
		corrID := fmt.Sprintf("goroutine-%d", i)
		logs, err := logger.GetMemoryLogsForCorrelation(corrID)
		if err != nil {
			t.Errorf("GetMemoryLogsForCorrelation(%s) failed: %v", corrID, err)
			continue
		}
		if len(logs) == 0 {
			t.Errorf("Expected entries for correlation %s, got 0", corrID)
		}
	}
}

func TestConcurrentLogging_SilentLoggerSafe(t *testing.T) {
	// Silent logger must be safe under concurrent use
	logger := NewSilentLogger()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				logger.Info().Int("id", id).Int("j", j).Msg("concurrent silent")
			}
		}(i)
	}

	wg.Wait()
	// Test passes if no panic or race detected (run with -race)
}

// --- Test 9: Output format regression ---

func TestOutputFormat_ContainsExpectedFields(t *testing.T) {
	// Verifies that realistic multi-field log statements produce output
	// containing the expected field names and values. Catches format regressions
	// when switching from zerolog to arbor.
	var buf bytes.Buffer
	logger := NewLoggerWithOutput("info", &buf)

	elapsed := 150 * time.Millisecond
	logger.Info().
		Dur("elapsed", elapsed).
		Str("tool", "portfolio_compliance").
		Int("holdings", 10).
		Msg("Handler complete")

	output := buf.String()
	if !strings.Contains(output, "Handler complete") {
		t.Errorf("Output missing message — got: %s", output)
	}
	if !strings.Contains(output, "elapsed") {
		t.Errorf("Output missing 'elapsed' field — got: %s", output)
	}
	if !strings.Contains(output, "portfolio_compliance") {
		t.Errorf("Output missing 'portfolio_compliance' value — got: %s", output)
	}
}

// --- Test 10: Fluent API completeness (all methods used by Vire) ---

func TestFluentAPI_AllMethodsUsedByVire(t *testing.T) {
	logger := NewSilentLogger()

	// These are all the ILogEvent methods used across 214 log statements in Vire.
	// Each must compile and not panic.
	logger.Info().Str("key", "val").Msg("str")
	logger.Info().Int("key", 1).Msg("int")
	logger.Info().Int64("key", int64(1)).Msg("int64")
	logger.Info().Float64("key", 1.0).Msg("float64")
	logger.Info().Bool("key", true).Msg("bool")
	logger.Info().Err(nil).Msg("err")
	logger.Info().Dur("key", 0).Msg("dur")
	logger.Info().Msgf("formatted %s %d", "string", 42)
	logger.Info().Strs("key", []string{"a", "b"}).Msg("strs")

	// Chained calls (common pattern)
	logger.Info().Str("a", "1").Str("b", "2").Int("c", 3).Msg("chained")
	logger.Error().Err(nil).Str("ticker", "BHP.AU").Msg("error with context")

	// All log levels
	logger.Debug().Msg("debug")
	logger.Info().Msg("info")
	logger.Warn().Msg("warn")
	logger.Error().Msg("error")
}
