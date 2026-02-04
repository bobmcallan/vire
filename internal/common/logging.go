// Package common provides shared utilities for Vire
package common

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// Logger wraps zerolog.Logger to provide a consistent interface
type Logger struct {
	zerolog.Logger
}

// NewLogger creates a new logger with the specified level
func NewLogger(level string) *Logger {
	var lvl zerolog.Level
	switch level {
	case "debug":
		lvl = zerolog.DebugLevel
	case "info":
		lvl = zerolog.InfoLevel
	case "warn":
		lvl = zerolog.WarnLevel
	case "error":
		lvl = zerolog.ErrorLevel
	default:
		lvl = zerolog.InfoLevel
	}

	output := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	}

	logger := zerolog.New(output).
		Level(lvl).
		With().
		Timestamp().
		Logger()

	return &Logger{Logger: logger}
}

// NewLoggerWithOutput creates a logger writing to a specific output
func NewLoggerWithOutput(level string, w io.Writer) *Logger {
	var lvl zerolog.Level
	switch level {
	case "debug":
		lvl = zerolog.DebugLevel
	case "info":
		lvl = zerolog.InfoLevel
	case "warn":
		lvl = zerolog.WarnLevel
	case "error":
		lvl = zerolog.ErrorLevel
	default:
		lvl = zerolog.InfoLevel
	}

	logger := zerolog.New(w).
		Level(lvl).
		With().
		Timestamp().
		Logger()

	return &Logger{Logger: logger}
}

// NewDefaultLogger creates a logger with default settings
func NewDefaultLogger() *Logger {
	return NewLogger("info")
}

// NewSilentLogger creates a logger that discards all output
func NewSilentLogger() *Logger {
	logger := zerolog.New(io.Discard)
	return &Logger{Logger: logger}
}
