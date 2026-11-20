//go:build integration

package trip

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric/noop"

	model "github.com/saywich/tripgo/internal/domain/trip"
	"github.com/saywich/tripgo/internal/infra/postgres/transaction"
	"github.com/saywich/tripgo/internal/usecase/trip/positions"
)

func TestStaleTrips(t *testing.T) {
	pool, repo := positionRepository(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)
	old := now.Add(-time.Hour)

	missing := insertPositionTrip(t, pool, old, nil, model.Active)
	oldPoint := old.Add(-time.Minute)
	stale := insertPositionTrip(t, pool, old, &oldPoint, model.Active)
	fresh := insertPositionTrip(t, pool, old, &now, model.Active)
	completed := insertPositionTrip(t, pool, old, nil, model.Completed)
	tied := insertPositionTrip(t, pool, old, nil, model.Active)

	got, err := repo.StaleTrips(ctx, time.Minute, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	filter := map[model.ID]bool{missing: true, stale: true, fresh: true, completed: true, tied: true}
	var matched []model.StaleTrip
	for _, candidate := range got {
		if filter[candidate.ID] {
			matched = append(matched, candidate)
		}
	}

	if len(matched) != 3 || matched[0].ID != stale || !matched[1].Since.Equal(old) || !matched[2].Since.Equal(old) {
		t.Fatalf("unexpected stale trips: %+v", matched)
	}

	next, err := repo.StaleTrips(ctx, time.Minute, &matched[1], 100)
	if err != nil {
		t.Fatal(err)
	}

	seen := 0
	for _, candidate := range next {
		switch candidate.ID {
		case matched[2].ID:
			seen++
		case matched[1].ID:
			t.Fatal("keyset cursor returned the same trip")
		}
	}

	if seen != 1 {
		t.Fatalf("same-time trip returned %d times", seen)
	}
}

func TestSavePositionTransaction(t *testing.T) {
	pool, repo := positionRepository(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)
	id := insertPositionTrip(t, pool, now.Add(-time.Hour), nil, model.Active)

	pending := &cancelRecorder{}
	counter, err := noop.NewMeterProvider().Meter("test").Int64Counter("saved")
	if err != nil {
		t.Fatal(err)
	}

	tx := transaction.NewManager(pool)
	save := positions.New(repo, tx, pending, counter, func() time.Time { return now })
	fresh := model.Position{TripID: id, RecordedAt: now}
	late := model.Position{TripID: id, RecordedAt: now.Add(-time.Minute)}
	freshSaved, err := save.Save(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}

	lateSaved, err := save.Save(ctx, late)
	if err != nil {
		t.Fatal(err)
	}

	tied, err := save.Save(ctx, late)
	if err != nil {
		t.Fatal(err)
	}

	failing := positions.New(failingUpdate{Repository: repo}, tx, pending, counter, func() time.Time { return now })
	fresh.RecordedAt = now.Add(time.Second)
	if _, err := failing.Save(ctx, fresh); !errors.Is(err, errForcedRollback) {
		t.Fatalf("expected rollback, got %v", err)
	}

	current, err := repo.SearchByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	if current.LastPositionAt == nil || !current.LastPositionAt.Equal(now) {
		t.Fatalf("unexpected last position: %v", current.LastPositionAt)
	}

	route, err := repo.ListPositions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	if len(route) != 3 || route[0].ID != lateSaved.ID || route[1].ID != tied.ID || route[2].ID != freshSaved.ID {
		t.Fatalf("unexpected route after rollback: %+v", route)
	}

	if pending.calls != 3 {
		t.Fatalf("canceled %d times", pending.calls)
	}
}

type cancelRecorder struct{ calls int }

func (r *cancelRecorder) Cancel(model.ID) { r.calls++ }

var errForcedRollback = errors.New("forced rollback")

type failingUpdate struct{ *Repository }

func (r failingUpdate) UpdateLastPosition(ctx context.Context, id model.ID, at time.Time) error {
	if err := r.Repository.UpdateLastPosition(ctx, id, at); err != nil {
		return err
	}

	return errForcedRollback
}

func positionRepository(t *testing.T) (*pgxpool.Pool, *Repository) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)
	return pool, NewRepository(pool, 3*time.Second)
}

func insertPositionTrip(t *testing.T, pool *pgxpool.Pool, started time.Time, last *time.Time, status model.Status) model.ID {
	t.Helper()

	id := uuid.New()
	var finished *time.Time
	if status == model.Completed {
		finished = &started
	}

	builder := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
	query, args, err := builder.Insert("trips").
		Columns(
			"id", "user_id", "driver_id", "start_latitude", "start_longitude",
			"end_latitude", "end_longitude", "price", "status", "started_at", "finished_at", "last_position_at",
		).
		Values(id, uuid.New(), uuid.New(), 0, 0, 0, 0, 0, status, started, finished, last).
		ToSql()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		query, args, err := builder.Delete("trips").Where(sq.Eq{"id": id}).ToSql()
		if err != nil {
			t.Error(err)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Error(err)
		}
	})

	return id
}
