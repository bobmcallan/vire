// Package common provides shared utilities for Vire
package common

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/phuslu/log"
	"github.com/ternarybob/arbor"
	"github.com/ternarybob/arbor/models"
	"github.com/ternarybob/arbor/writers"
)

// Logger wraps arbor.ILogger to provide a consistent interface
type Logger struct {
	arbor.ILogger
}

// discardWriter implements writers.IWriter and discards all output.
// Used by NewSilentLogger to prevent dispatch to globally-registered writers.
type discardWriter struct{}

func (w *discardWriter) Write(p []byte) (int, error)          { return len(p), nil }
func (w *discardWriter) WithLevel(_ log.Level) writers.IWriter { return w }
func (w *discardWriter) GetFilePath() string                   { return "" }
func (w *discardWriter) Close() error                          { return nil }

// writerAdapter adapts an io.Writer to arbor's IWriter interface.
// Used by NewLoggerWithOutput to direct log output to a custom writer.
type writerAdapter struct {
	out   io.Writer
	level log.Level
}

func (w *writerAdapter) Write(p []byte) (int, error) {
	// Parse the JSON log event and format as text for the output writer
	var evt models.LogEvent
	if err := json.Unmarshal(p, &evt); err != nil {
		return w.out.Write(p)
	}
	if evt.Level < w.level {
		return len(p), nil
	}
	msg := evt.Message
	for k, v := range evt.Fields {
		msg += fmt.Sprintf(" %s=%v", k, v)
	}
	if evt.Error != "" {
		msg += fmt.Sprintf(" error=%s", evt.Error)
	}
	msg += "\n"
	return w.out.Write([]byte(msg))
}

func (w *writerAdapter) WithLevel(level log.Level) writers.IWriter {
	w.level = level
	return w
}

func (w *writerAdapter) GetFilePath() string { return "" }
func (w *writerAdapter) Close() error        { return nil }

// NewLogger creates a new logger with the specified level, console writer (stderr),
// and memory writer for diagnostics.
func NewLogger(level string) *Logger {
	arborLogger := arbor.NewLogger().
		WithConsoleWriter(models.WriterConfiguration{
			Type:       models.LogWriterTypeConsole,
			Writer:     os.Stderr,
			TimeFormat: "2006-01-02T15:04:05Z07:00",
		}).
		WithMemoryWriter(models.WriterConfiguration{
			Type: models.LogWriterTypeMemory,
		}).
		WithLevelFromString(level)

	return &Logger{ILogger: arborLogger}
}

// NewLoggerWithOutput creates a logger writing to a specific output.
// Registers a writerAdapter as the console writer and a memory writer for queries.
func NewLoggerWithOutput(level string, w io.Writer) *Logger {
	adapter := &writerAdapter{out: w, level: log.TraceLevel}
	// Register the adapter as the console writer in the global registry
	arbor.RegisterWriter(arbor.WRITER_CONSOLE, adapter)

	arborLogger := arbor.NewLogger().
		WithMemoryWriter(models.WriterConfiguration{
			Type: models.LogWriterTypeMemory,
		}).
		WithLevelFromString(level)

	return &Logger{ILogger: arborLogger}
}

// NewDefaultLogger creates a logger with default settings
func NewDefaultLogger() *Logger {
	return NewLogger("info")
}

// NewSilentLogger creates a logger that discards all output.
// Uses a discardWriter to prevent fallthrough to globally-registered writers.
func NewSilentLogger() *Logger {
	arborLogger := arbor.NewLogger().WithWriters([]writers.IWriter{&discardWriter{}})
	return &Logger{ILogger: arborLogger}
}

// WithCorrelationId returns a new Logger with a correlation ID set.
// Used by MCP handlers to trace a request through all layers.
func (l *Logger) WithCorrelationId(id string) *Logger {
	return &Logger{ILogger: l.ILogger.WithCorrelationId(id)}
}
