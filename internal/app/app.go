// Package app coordinates command execution for the gateway-lens binary.
package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
)

const shutdownTimeout = 5 * time.Second

// HTTPServerRunnable is a manager runnable that serves the dashboard HTTP API/UI.
type HTTPServerRunnable struct {
	// reader supplies live Gateway API resources from the informer cache.
	reader liveResourcesReader
	// logger is the structured logger for server lifecycle events.
	logger logr.Logger
	// listenAddress is the TCP address the server binds to.
	listenAddress string
	// boundAddress stores the actual address once the listener is active.
	boundAddress atomic.Pointer[string]
}

// NewHTTPServerRunnable creates a dashboard server runnable.
func NewHTTPServerRunnable(
	liveResources liveResourcesReader,
	listenAddress string,
	logger logr.Logger,
) *HTTPServerRunnable {
	return &HTTPServerRunnable{
		reader:        liveResources,
		logger:        logger,
		listenAddress: listenAddress,
	}
}

// Addr returns the bound network address once the server has started listening.
// Returns an empty string if the server has not yet started.
func (r *HTTPServerRunnable) Addr() string {
	addr := r.boundAddress.Load()
	if addr == nil {
		return ""
	}

	return *addr
}

// Start runs the dashboard HTTP server until the context is canceled.
func (r *HTTPServerRunnable) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("preparing dashboard server: %w", err)
	}

	return r.serve(ctx)
}

// serve creates the handler, opens the listener, and runs the HTTP server.
func (r *HTTPServerRunnable) serve(ctx context.Context) error {
	handler, err := newDashboardHandler(r.reader, r.logger)
	if err != nil {
		return err
	}

	listener, err := listenDashboard(ctx, r.listenAddress)
	if err != nil {
		return fmt.Errorf("opening dashboard listener: %w", err)
	}

	addr := listener.Addr().String()
	r.boundAddress.Store(&addr)

	server := newDashboardServer(handler)

	return serveDashboard(ctx, server, listener, r.logger)
}

// listenDashboard opens a TCP listener on the given address.
func listenDashboard(ctx context.Context, address string) (net.Listener, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listening on dashboard socket: %w", err)
	}

	return listener, nil
}

// newDashboardServer creates an HTTP server with the given handler.
func newDashboardServer(handler http.Handler) *http.Server {
	return &http.Server{Handler: handler, ReadHeaderTimeout: shutdownTimeout}
}

// serveDashboard serves HTTP until context cancellation, then gracefully shuts down.
func serveDashboard(
	ctx context.Context,
	server *http.Server,
	listener net.Listener,
	logger logr.Logger,
) error {
	logger.Info("Dashboard server listening", "address", listener.Addr().String())

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		return normalizeServeError(err)
	case <-ctx.Done():
		logger.Info("Shutting down dashboard server")

		if err := shutdownDashboardServer(ctx, server); err != nil {
			return err
		}

		return normalizeServeError(<-serveErr)
	}
}

func shutdownDashboardServer(ctx context.Context, server *http.Server) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutting down dashboard server: %w", err)
	}

	return nil
}

func normalizeServeError(err error) error {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving dashboard requests: %w", err)
	}

	return nil
}
