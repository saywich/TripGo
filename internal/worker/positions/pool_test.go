package positions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/saywich/tripgo/internal/domain/trip"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func candidate() trip.StaleTrip {
	return trip.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
}

func TestQueueDeduplicationAndCancellation(t *testing.T) {
	q := newQueue(3, false)
	trip := candidate()
	original := &task{trip: trip}
	replacement := &task{trip: trip}

	if result, _ := q.offer(original); result != queued {
		t.Fatal(result)
	}

	if result, _ := q.offer(replacement); result != duplicate {
		t.Fatal(result)
	}

	q.cancel(trip.ID)
	if result, _ := q.offer(replacement); result != queued {
		t.Fatal(result)
	}

	if got := q.take(); got != replacement {
		t.Fatalf("picked canceled task: %p", got)
	}

	if result, _ := q.offer(original); result != duplicate {
		t.Fatal("in-flight task must remain deduplicated")
	}

	q.finish(replacement)
	if result, _ := q.offer(original); result != queued {
		t.Fatal(result)
	}
}

func TestPriorityQueueEvictsNewestAndRemovesCanceled(t *testing.T) {
	q := newQueue(3, true)
	old := &task{trip: candidate()}
	middle := &task{trip: candidate()}
	newest := &task{trip: candidate()}
	replacement := &task{trip: candidate()}
	middle.trip.Since = time.Unix(2, 0)
	newest.trip.Since = time.Unix(3, 0)
	old.trip.Since = time.Unix(1, 0)
	replacement.trip.Since = time.Unix(0, 0)

	q.offer(middle)
	q.offer(newest)
	q.offer(old)

	if result, _ := q.offer(&task{trip: newest.trip}); result != duplicate {
		t.Fatal(result)
	}

	if result, _ := q.offer(&task{trip: trip.StaleTrip{ID: uuid.New(), Since: time.Unix(4, 0)}}); result != dropped {
		t.Fatal(result)
	}

	if result, evicted := q.offer(replacement); result != queued || evicted != newest {
		t.Fatalf("unexpected result %s, evicted %p", result, evicted)
	}

	q.cancel(old.trip.ID)
	if q.take() != replacement || q.take() != middle || q.take() != nil {
		t.Fatal("priority order or cancellation is incorrect")
	}
}

func TestScannerBatches(t *testing.T) {
	for _, tc := range []struct {
		name     string
		capacity int
		want     int64
	}{
		{"three batches", 250, 250},
		{"full first batch", 50, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockrepository(ctrl)
			rows := make([]trip.StaleTrip, 250)
			for i := range rows {
				rows[i] = candidate()
			}

			cfg := Config{Capacity: tc.capacity, BatchSize: 100, StaleAfter: time.Minute}
			expect := repo.EXPECT()
			first := expect.StaleTrips(gomock.Any(), cfg.StaleAfter, (*trip.StaleTrip)(nil), cfg.BatchSize).Return(rows[:100], nil)
			if tc.capacity == len(rows) {
				second := expect.StaleTrips(gomock.Any(), cfg.StaleAfter, &rows[99], cfg.BatchSize).Return(rows[100:200], nil)
				third := expect.StaleTrips(gomock.Any(), cfg.StaleAfter, &rows[199], cfg.BatchSize).Return(rows[200:], nil)
				gomock.InOrder(first, second, third)
			}

			p := testPool(t, repo, nil, cfg)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()

			finished := make(chan struct{})
			go func() {
				defer close(finished)
				p.scanOnce(ctx)
			}()

			wait(t, ctx, finished)
			if got := p.queue.size(); got != tc.want {
				t.Fatalf("queued %d tasks, want %d", got, tc.want)
			}
		})
	}
}

func TestPoolDrainsWithFixedWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	ctrl := gomock.NewController(t)
	repo := NewMockrepository(ctrl)
	push := NewMockgateway(ctrl)
	ticker := NewMockticker(ctrl)

	first, second, third, canceled := candidate(), candidate(), candidate(), candidate()
	ticks := make(chan time.Time)
	ticker.EXPECT().Ticks().Return(ticks).AnyTimes()
	ticker.EXPECT().Stop()

	repo.EXPECT().StaleTrips(gomock.Any(), time.Minute, (*trip.StaleTrip)(nil), 0).
		Return([]trip.StaleTrip{first, second, third, canceled}, nil)

	started := make(chan struct{}, 3)
	release := make(chan struct{})

	for _, task := range []trip.StaleTrip{first, second} {
		push.EXPECT().RequestPosition(gomock.Any(), task).DoAndReturn(func(ctx context.Context, _ trip.StaleTrip) error {
			started <- struct{}{}
			select {
			case <-release:
				return errors.New("push service unavailable")
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}

	push.EXPECT().RequestPosition(gomock.Any(), third).DoAndReturn(func(context.Context, trip.StaleTrip) error {
		started <- struct{}{}
		return nil
	})

	cfg := Config{StaleAfter: time.Minute, RequestTimeout: 5 * time.Second, Workers: 2, Capacity: 4}
	p := testPool(t, repo, push, cfg)
	stop := p.Start(ctx, ticker)

	sendTick(t, ctx, ticks)
	wait(t, ctx, started)
	wait(t, ctx, started)

	if p.active.Load() != 2 {
		t.Fatal("expected two workers in flight")
	}

	repo.EXPECT().StaleTrips(gomock.Any(), time.Minute, (*trip.StaleTrip)(nil), 0).
		DoAndReturn(func(context.Context, time.Duration, *trip.StaleTrip, int) ([]trip.StaleTrip, error) {
			p.Cancel(canceled.ID)
			return []trip.StaleTrip{first, second, third}, nil
		})

	scanAgain := make(chan struct{})
	repo.EXPECT().StaleTrips(gomock.Any(), time.Minute, (*trip.StaleTrip)(nil), 0).
		DoAndReturn(func(context.Context, time.Duration, *trip.StaleTrip, int) ([]trip.StaleTrip, error) {
			close(scanAgain)
			return nil, nil
		})

	sendTick(t, ctx, ticks)
	sendTick(t, ctx, ticks)
	wait(t, ctx, scanAgain)
	close(release)

	if err := stop(ctx); err != nil {
		t.Fatal(err)
	}

	wait(t, ctx, started)
	if p.queue.size() != 0 || p.active.Load() != 0 {
		t.Fatal("pool did not drain")
	}
}

func TestPoolStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	ctrl := gomock.NewController(t)
	ticker := NewMockticker(ctrl)
	stopped := make(chan struct{})
	ticker.EXPECT().Ticks().Return(make(chan time.Time)).AnyTimes()
	ticker.EXPECT().Stop().Do(func() { close(stopped) })

	p := testPool(t, nil, nil, Config{Workers: 4, Capacity: 1})
	stop := p.Start(ctx, ticker)
	cancel()

	deadline, end := context.WithTimeout(t.Context(), time.Second)
	defer end()
	wait(t, deadline, stopped)
	if err := stop(deadline); err != nil {
		t.Fatal(err)
	}
}

func testPool(t *testing.T, repo repository, push gateway, cfg Config) *Pool {
	t.Helper()

	p, err := New(repo, push, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), tracenoop.NewTracerProvider().Tracer("test"), noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatal(err)
	}

	return p
}

func wait(t *testing.T, ctx context.Context, event <-chan struct{}) {
	t.Helper()
	select {
	case <-event:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func sendTick(t *testing.T, ctx context.Context, ticks chan<- time.Time) {
	t.Helper()
	select {
	case ticks <- time.Time{}:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestQueueOverflowDoesNotBlockAndIsCounted(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.WithoutCancel(t.Context())); err != nil {
			t.Error(err)
		}
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tracer := tracenoop.NewTracerProvider().Tracer("test")
	p, err := New(nil, nil, Config{Capacity: 1}, logger, tracer, provider.Meter("test"))
	if err != nil {
		t.Fatal(err)
	}

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		p.enqueueTrips(ctx, []trip.StaleTrip{candidate(), candidate()}, trace.SpanContext{})
	}()

	wait(t, ctx, finished)
	if p.queue.size() != 1 {
		t.Fatal("queue exceeded capacity")
	}

	var measurements metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &measurements); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"tripgo.stale_checks", "tripgo.position_requests"} {
		if countDrops(measurements, name) != 1 {
			t.Fatalf("%s did not record one drop", name)
		}
	}
}

func countDrops(measurements metricdata.ResourceMetrics, name string) int64 {
	for _, scope := range measurements.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			if instrument.Name != name {
				continue
			}

			sum, ok := instrument.Data.(metricdata.Sum[int64])
			if !ok {
				return 0
			}

			for _, point := range sum.DataPoints {
				if result, _ := point.Attributes.Value("result"); result.AsString() == string(dropped) {
					return point.Value
				}
			}
		}
	}

	return 0
}
