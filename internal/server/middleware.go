package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/google/uuid"
)

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// recoveryMiddleware catches panics and returns 500.
func recoveryMiddleware(logger *common.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error().
						Str("panic", fmt.Sprintf("%v", rec)).
						Str("path", r.URL.Path).
						Msg("Panic recovered in HTTP handler")
					WriteError(w, http.StatusInternalServerError, "Internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware adds CORS headers for future web UI.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-Correlation-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// correlationIDMiddleware extracts or generates a correlation ID.
func correlationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corrID := r.Header.Get("X-Request-ID")
		if corrID == "" {
			corrID = r.Header.Get("X-Correlation-ID")
		}
		if corrID == "" {
			corrID = uuid.New().String()[:8]
		}
		w.Header().Set("X-Correlation-ID", corrID)
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs HTTP requests.
func loggingMiddleware(logger *common.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			dur := time.Since(start)
			corrID := w.Header().Get("X-Correlation-ID")

			event := logger.Trace()
			if rw.statusCode >= 500 {
				event = logger.Error()
			} else if rw.statusCode >= 400 {
				event = logger.Warn()
			}

			event.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("query", r.URL.RawQuery).
				Int("status", rw.statusCode).
				Int("bytes", rw.bytesWritten).
				Dur("duration", dur).
				Str("correlation_id", corrID).
				Msg("HTTP request")
		})
	}
}

// applyMiddleware wraps a handler with the middleware stack.
func applyMiddleware(handler http.Handler, logger *common.Logger) http.Handler {
	// Apply in reverse order (last applied = first executed)
	handler = loggingMiddleware(logger)(handler)
	handler = correlationIDMiddleware(handler)
	handler = corsMiddleware(handler)
	handler = recoveryMiddleware(logger)(handler)
	return handler
}
