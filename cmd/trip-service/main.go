package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saywich/tripgo/internal"
)

func main() {
	config, err := tripservice.ParseEnv()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	poolConfig, err := pgxpool.ParseConfig(config.DatabaseUrl)
	if err != nil {
		log.Printf("parse database configuration: %v", err)
		return
	}
	poolConfig.MaxConns = int32(config.DatabaseMaxConns)
	poolConfig.MinConns = int32(config.DatabaseMinConns)
	poolConfig.MaxConnLifetime = config.DatabaseMaxConnLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Printf("create database pool: %v", err)
		return
	}
	defer pool.Close()

	pingCtx, cancelPing := context.WithTimeout(ctx, config.DatabaseConnectTimeout)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		log.Printf("connect to database: %v", err)
		return
	}

	txManager := tripservice.NewTransactionManager(pool)
	tripRepository := tripservice.NewTripRepository(pool)
	tripStatusHistoryRepository := tripservice.NewTripStatusHistoryRepository(pool)
	tripService := tripservice.NewTripService(
		txManager,
		tripRepository,
		tripStatusHistoryRepository,
	)

	handler := tripservice.NewHTTPHandler(tripService, pool)

	server := &http.Server{
		Addr: config.HttpAddress,
		Handler: handler.Routes(),
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server failed: %v", err)
			
			return
		}

		return
	case <-ctx.Done():
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), config.ShutdownTimeout)
		defer cancelShutdown()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful HTTP shutdown failed: %v; forcing shutdown", err)
			if closeErr := server.Close(); closeErr != nil {
				log.Printf("force HTTP shutdown failed: %v", closeErr)
			}
		}

		return
	}
}
