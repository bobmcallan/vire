package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/common"
)

// Server wraps the HTTP server and application reference.
type Server struct {
	app          *app.App
	server       *http.Server
	logger       *common.Logger
	shutdownChan chan struct{}
}

// SetShutdownChannel sets the channel that will be signaled when HTTP shutdown is requested.
func (s *Server) SetShutdownChannel(ch chan struct{}) {
	s.shutdownChan = ch
}

// NewServer creates a new HTTP REST API server.
func NewServer(a *app.App) *Server {
	s := &Server{
		app:    a,
		logger: a.Logger,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	handler := applyMiddleware(mux, a.Logger, a.Config, a.Storage.InternalStore())

	host := a.Config.Server.Host
	port := a.Config.Server.Port

	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Handler returns the HTTP handler for testing.
func (s *Server) Handler() http.Handler {
	return s.server.Handler
}

// Start starts the HTTP server (blocking).
func (s *Server) Start() error {
	s.logger.Info().
		Str("addr", s.server.Addr).
		Msg("Starting REST API server")
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
