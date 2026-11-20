package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/saywich/tripgo/internal/config"
	"github.com/saywich/tripgo/internal/container"
)

func main() {
	if err := run(); err != nil {
		slog.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.ParseEnv()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := container.New(ctx, cfg)
	if err != nil {
		return err
	}

	app.Logger.Info("trip service started", "address", cfg.HTTPAddress)
	defer app.Logger.Info("trip service stopped")

	errs := make(chan error, 1)
	go func() { errs <- app.Server.ListenAndServe() }()

	var serveErr error
	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	case <-ctx.Done():
	}

	shutdown, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	serverErr := shutdownServer(shutdown, app.Server)
	closeErr := app.Close(shutdown)

	return errors.Join(serveErr, serverErr, closeErr)
}

func shutdownServer(ctx context.Context, server *http.Server) error {
	if err := server.Shutdown(ctx); err != nil {
		return errors.Join(err, server.Close())
	}

	return nil
}
