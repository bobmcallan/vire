package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/server"
)

func main() {
	// Resolve config path
	configPath := os.Getenv("VIRE_CONFIG")

	a, err := app.NewApp(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize app: %v\n", err)
		os.Exit(1)
	}

	// Display startup banner
	common.PrintBanner(a.Config, a.Logger)

	// Start background services
	a.StartWarmCache()
	a.StartPriceScheduler()

	// Create shutdown channel for HTTP endpoint
	shutdownChan := make(chan struct{})

	// Build REST API server
	srv := server.NewServer(a)
	srv.SetShutdownChannel(shutdownChan)

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

	// Wait for interrupt signal or HTTP shutdown request
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		a.Logger.Info().Msg("Shutdown signal received")
	case <-shutdownChan:
		a.Logger.Info().Msg("Shutdown requested via HTTP")
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		a.Logger.Error().Err(err).Msg("HTTP server shutdown failed")
	}

	common.PrintShutdownBanner(a.Logger)
	a.Close()
	a.Logger.Info().Msg("Server stopped")
}
