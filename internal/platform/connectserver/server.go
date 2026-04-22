// Package connectserver wires a Connect handler set together with HTTP health
// endpoints and graceful shutdown. It is shared by BFF and any deployable that
// exposes RPCs.
package connectserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/soneda-yuya/reearth-homework/internal/platform/observability"
)

// HandlerRegistration binds a Connect handler (path, http.Handler) produced by
// a generated ServiceHandler constructor such as:
//
//	connectrpc.NewSafetyIncidentServiceHandler(impl, opts...) // returns (path, http.Handler)
type HandlerRegistration struct {
	Path    string
	Handler http.Handler
}

// Config controls the HTTP server.
type Config struct {
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ReadHeader      time.Duration
	ShutdownTimeout time.Duration
	ReadyzTimeout   time.Duration
}

// Server bundles the Connect mux, health endpoints, and the HTTP listener.
type Server struct {
	cfg          Config
	httpServer   *http.Server
	handlers     []HandlerRegistration
	interceptors []connect.Interceptor
	probers      []Prober
}

// New constructs the Server. Call Start to block until shutdown.
func New(cfg Config, handlers []HandlerRegistration, interceptors []connect.Interceptor, probers []Prober) *Server {
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	if cfg.ReadHeader == 0 {
		cfg.ReadHeader = 10 * time.Second
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second
	}
	if cfg.ReadyzTimeout == 0 {
		cfg.ReadyzTimeout = 3 * time.Second
	}
	return &Server{
		cfg:          cfg,
		handlers:     handlers,
		interceptors: interceptors,
		probers:      probers,
	}
}

// buildMux wires handlers and health endpoints.
func (s *Server) buildMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", HealthzHandler())
	mux.HandleFunc("/readyz", ReadyzHandler(s.probers, s.cfg.ReadyzTimeout))
	for _, h := range s.handlers {
		mux.Handle(h.Path, h.Handler)
	}
	return mux
}

// Start blocks until ctx is cancelled (SIGTERM/SIGINT) or the server fails.
// On cancellation it drains in-flight requests up to ShutdownTimeout.
func (s *Server) Start(ctx context.Context) error {
	mux := s.buildMux()
	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.cfg.Port),
		Handler:           mux,
		ReadTimeout:       s.cfg.ReadTimeout,
		WriteTimeout:      s.cfg.WriteTimeout,
		ReadHeaderTimeout: s.cfg.ReadHeader,
	}

	errCh := make(chan error, 1)
	go func() {
		observability.Logger(ctx).Info("http server listening", "port", s.cfg.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		observability.Logger(ctx).Info("shutdown initiated")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	}
}

// Stop triggers an immediate graceful shutdown (used in tests).
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}
