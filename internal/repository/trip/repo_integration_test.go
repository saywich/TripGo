//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	trip "github.com/saywich/tripgo/internal/domain/trip"
	"github.com/saywich/tripgo/internal/infra/postgres/transaction"
	"github.com/saywich/tripgo/internal/model"
	repo "github.com/saywich/tripgo/internal/repository/trip"
)

func TestRepository(t *testing.T) {
	url := os.Getenv("DATABASE_URL")

	if url == "" {
		t.Fatal("DATABASE_URL is required; start PostgreSQL and apply migrations")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)
	r := repo.NewRepository(pool, 5*time.Second)
	h := repo.NewStatusHistoryRepository(pool, 5*time.Second)
	tx := transaction.NewManager(pool)
	data := trip.Data{UserID: uuid.New(), DriverID: uuid.New(), Price: 100}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM trips WHERE driver_id=$1", data.DriverID); err != nil {
			t.Error(err)
		}
	})
	var saved trip.Trip
	err = tx.Do(ctx, func(ctx context.Context) error {
		var err error
		saved, err = r.Create(ctx, data)
		if err != nil {
			return err
		}

		return h.CreateInitial(ctx, saved.ID, trip.Active, "test")
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := r.SearchByID(ctx, saved.ID)

	if err != nil || got.DriverID != data.DriverID || got.Status != trip.Active {
		t.Fatalf("fetch: %+v %v", got, err)
	}

	if _, err = r.Create(ctx, data); !errors.Is(err, trip.ErrDriverBusy) {
		t.Fatalf("busy: %v", err)
	}

	if _, err = r.SearchByID(ctx, uuid.New()); !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("missing: %v", err)
	}

	finished, err := r.Finish(ctx, saved.ID)

	if err != nil || finished.Status != trip.Completed || finished.FinishedAt == nil {
		t.Fatalf("finish: %+v %v", finished, err)
	}
}

func TestRollbackBothTables(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("DATABASE_URL")

	if url == "" {
		t.Fatal("DATABASE_URL required")
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)
	r := repo.NewRepository(pool, 5*time.Second)
	h := repo.NewStatusHistoryRepository(pool, 5*time.Second)
	tx := transaction.NewManager(pool)
	driver := uuid.New()
	var id uuid.UUID
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DELETE FROM trips WHERE driver_id=$1", driver); err != nil {
			t.Error(err)
		}
	})
	err = tx.Do(ctx, func(ctx context.Context) error {
		v, err := r.Create(ctx, trip.Data{UserID: uuid.New(), DriverID: driver})
		if err != nil {
			return err
		}

		id = v.ID

		if err = h.CreateInitial(ctx, id, trip.Active, "before failure"); err != nil {
			return err
		}

		return h.Create(ctx, id, trip.Active, trip.Status("invalid"), "force constraint violation")
	})

	if err == nil {
		t.Fatal("expected constraint violation")
	}

	if id == uuid.Nil {
		t.Fatal("trip insert failed before rollback scenario")
	}

	for _, table := range []string{"trips", "trip_status_history"} {
		var count int
		query := "SELECT count(*) FROM trips WHERE id=$1"

		if table == "trip_status_history" {
			query = "SELECT count(*) FROM trip_status_history WHERE trip_id=$1"
		}

		if err := pool.QueryRow(ctx, query, id).Scan(&count); err != nil {
			t.Fatal(err)
		}

		if count != 0 {
			t.Fatalf("%s has %d uncommitted rows", table, count)
		}
	}
}

func TestCommitCallbacks(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx := transaction.NewManager(pool)
	for _, fail := range []bool{false, true} {
		calls := 0
		failure := errors.New("rollback")
		err := tx.Do(ctx, func(ctx context.Context) error {
			return tx.Do(ctx, func(ctx context.Context) error {
				model.AfterCommit(ctx, func() { calls++ })
				if calls != 0 {
					t.Fatal("callback ran before outer commit")
				}
				if fail {
					return failure
				}
				return nil
			})
		})
		if fail {
			if !errors.Is(err, failure) || calls != 0 {
				t.Fatalf("rollback: calls=%d err=%v", calls, err)
			}
		} else if err != nil || calls != 1 {
			t.Fatalf("commit: calls=%d err=%v", calls, err)
		}
	}
}
