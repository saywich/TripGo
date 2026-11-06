package idempotency

import (
	"context"
	"log/slog"
	"time"
)

type repository interface{ CleanupExpired(context.Context) error }

type Worker struct {
	repo   repository
	logger *slog.Logger
}

func New(repo repository, logger *slog.Logger) *Worker { return &Worker{repo, logger} }

func (w *Worker) Run(ctx context.Context) {
	w.cleanup(ctx)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.cleanup(ctx)
		}
	}
}

func (w *Worker) cleanup(ctx context.Context) {
	if err := w.repo.CleanupExpired(ctx); err != nil {
		w.logger.ErrorContext(ctx, "cleanup expired idempotency keys", "error", err)
	}
}
