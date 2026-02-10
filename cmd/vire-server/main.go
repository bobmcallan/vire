package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bobmccarthy/vire/internal/app"
	"github.com/bobmccarthy/vire/internal/server"
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

	// Build REST API server
	srv := server.NewServer(a)

	// Start HTTP server
	go func() {
		port := a.Config.Server.Port
		a.Logger.Info().Int("port", port).Msg("Starting HTTP server")
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			a.Logger.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	port := a.Config.Server.Port
	a.Logger.Info().
		Str("url", fmt.Sprintf("http://localhost:%d", port)).
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
