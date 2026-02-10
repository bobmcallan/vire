package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/bobmccarthy/vire/internal/app"
	"github.com/bobmccarthy/vire/internal/common"
)

func main() {
	// Resolve config path
	configPath := os.Getenv("VIRE_CONFIG")

	a, err := app.NewApp(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize app: %v\n", err)
		os.Exit(1)
	}

	// Start background services
	a.StartWarmCache()
	a.StartPriceScheduler()

	// Build HTTP mux
	mux := buildMux(a)

	// Read server config (host/port from config with env overrides applied)
	host := a.Config.Server.Host
	port := a.Config.Server.Port

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	go func() {
		a.Logger.Info().Int("port", port).Msg("Starting HTTP server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.Logger.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	a.Logger.Info().
		Str("url", fmt.Sprintf("http://localhost:%d", port)).
		Str("mcp", fmt.Sprintf("http://localhost:%d/mcp", port)).
		Msg("Server ready")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	a.Logger.Info().Msg("Shutdown signal received")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		a.Logger.Error().Err(err).Msg("HTTP server shutdown failed")
	}

	a.Close()
	a.Logger.Info().Msg("Server stopped")
}

// buildMux creates the HTTP mux with MCP and REST endpoints.
func buildMux(a *app.App) http.Handler {
	// MCP over Streamable HTTP
	httpMCP := server.NewStreamableHTTPServer(a.MCPServer,
		server.WithStateLess(true),
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", httpMCP)
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/version", versionHandler)

	return mux
}

// healthHandler responds to GET/HEAD /api/health with {"status":"ok"}.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// versionHandler responds to GET/HEAD /api/version with version info.
func versionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"version": common.GetVersion(),
		"build":   common.GetBuild(),
		"commit":  common.GetGitCommit(),
	})
}
