package container

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"

	"github.com/saywich/tripgo/internal/config"
	"github.com/saywich/tripgo/internal/http/middleware"
	createhttp "github.com/saywich/tripgo/internal/http/trip/create"
	finishhttp "github.com/saywich/tripgo/internal/http/trip/finish"
	gethttp "github.com/saywich/tripgo/internal/http/trip/get"
	"github.com/saywich/tripgo/internal/infra/postgres/transaction"
	"github.com/saywich/tripgo/internal/infra/telemetry"
	repo "github.com/saywich/tripgo/internal/repository/trip"
	createuc "github.com/saywich/tripgo/internal/usecase/trip/create"
	finishuc "github.com/saywich/tripgo/internal/usecase/trip/finish"
	getuc "github.com/saywich/tripgo/internal/usecase/trip/get"
	"github.com/saywich/tripgo/internal/worker/idempotency"
)

type Container struct {
	Server *http.Server
	Logger *slog.Logger
	Close  func(context.Context) error
}

func New(ctx context.Context, c *config.Config) (*Container, error) {
	logger, shutdown, err := telemetry.New(ctx, c.LogLevel)
	if err != nil {
		return nil, err
	}

	pc, err := pgxpool.ParseConfig(c.DatabaseURL)
	if err != nil {
		_ = shutdown(ctx)

		return nil, err
	}

	pc.MaxConns = int32(c.DatabaseMaxConns) //nolint:gosec // Config validates the value fits int32.
	pc.MinConns = int32(c.DatabaseMinConns) //nolint:gosec // Config validates min <= max <= MaxInt32.
	pc.MaxConnLifetime = c.DatabaseMaxConnLifetime
	pc.ConnConfig.ConnectTimeout = c.DatabaseConnectTimeout
	tracer, err := telemetry.NewQueryTracer(otel.Meter("trip-service/postgres"))
	if err != nil {
		_ = shutdown(ctx)

		return nil, err
	}

	pc.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		_ = shutdown(ctx)

		return nil, err
	}

	closeAll := func(ctx context.Context) error { pool.Close(); return shutdown(ctx) }
	pingCtx, cancelPing := context.WithTimeout(ctx, c.DatabaseConnectTimeout)
	defer cancelPing()

	if err := pool.Ping(pingCtx); err != nil {
		_ = closeAll(ctx)

		return nil, err
	}

	tx := transaction.NewManager(pool)
	trips := repo.NewRepository(pool, c.DatabaseQueryTimeout)
	history := repo.NewStatusHistoryRepository(pool, c.DatabaseQueryTimeout)
	idem := repo.NewIdempotencyRepository(pool, c.DatabaseQueryTimeout, c.IdempotencyKeyRetention)
	worker := idempotency.New(idem, logger)
	go worker.Run(ctx)
	meter := otel.Meter("trip-service")
	created, err := meter.Int64Counter("tripgo.trips.created")
	if err != nil {
		_ = closeAll(ctx)

		return nil, err
	}

	completed, err := meter.Int64Counter("tripgo.trips.completed")
	if err != nil {
		_ = closeAll(ctx)

		return nil, err
	}

	router := chi.NewRouter()
	router.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	router.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		status := "ok"
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		if pool.Ping(r.Context()) != nil {
			status = "unavailable"
			w.WriteHeader(503)
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
	})
	router.Method(
		"POST",
		"/api/v1/trips",
		middleware.NewIdempotencyMiddleware(tx, idem).
			Wrap(createhttp.New(createuc.New(trips, tx, history, logger, created))),
	)
	router.Method("GET", "/api/v1/trips/{tripID}", gethttp.New(getuc.New(trips)))
	router.Method(
		"POST",
		"/api/v1/trips/{tripID}/finish",
		finishhttp.New(finishuc.New(trips, tx, history, logger, completed)))

	handler, err := middleware.Wrap(router, logger, meter)
	if err != nil {
		_ = closeAll(ctx)

		return nil, err
	}

	matched := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := chi.NewRouteContext()
		router.Match(rc, r.Method, r.URL.Path)
		r = r.WithContext(middleware.WithRoute(r.Context(), rc.RoutePattern()))
		handler.ServeHTTP(w, r)
	})

	return &Container{
		Server: &http.Server{
			Addr:              c.HTTPAddress,
			Handler:           matched,
			ReadTimeout:       c.HTTPReadTimeout,
			ReadHeaderTimeout: c.HTTPReadHeaderTimeout,
			WriteTimeout:      c.HTTPWriteTimeout,
			IdleTimeout:       c.HTTPIdleTimeout,
		},
		Logger: logger,
		Close:  closeAll,
	}, nil
}
