// Package serverapp owns the PaperSage HTTP process lifecycle shared by the
// standalone binary and platform-specific entrypoints.
package serverapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/0verL1nk/GraduateProject/server/internal/bootstrap"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
)

// Run loads configuration, assembles the application, serves HTTP, and shuts
// down gracefully when the process receives SIGINT or SIGTERM.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	app, err := bootstrap.Build(context.Background(), cfg)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler:           app.API,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", srv.Addr, "debug", cfg.Debug)
		errCh <- srv.ListenAndServe()
	}()

	var serveErr error
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr = fmt.Errorf("serve: %w", err)
		}
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	// Operational drain budget; AI final persistence has a separate 5s window.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = shutdown(shutdownCtx, srv, app)
	logger.Info("server stopped")
	return errors.Join(serveErr, err)
}

func shutdown(ctx context.Context, srv *http.Server, app *bootstrap.App) error {
	httpDone := make(chan error, 1)
	go func() {
		err := srv.Shutdown(ctx)
		if err != nil {
			err = errors.Join(err, srv.Close()) // release subscriptions after the drain deadline
		}
		httpDone <- err
	}()
	// Close seals Run admission synchronously while HTTP stops accepting connections.
	agentErr := app.Close(ctx)
	httpErr := <-httpDone
	// Neither subscriptions nor terminal writes may touch a closed DB.
	db, dbErr := app.DB.DB()
	if dbErr == nil {
		dbErr = db.Close()
	}
	return errors.Join(agentErr, httpErr, dbErr)
}
