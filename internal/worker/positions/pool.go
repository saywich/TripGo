package positions

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/saywich/tripgo/internal/domain/trip"
)

type Config struct {
	StaleAfter     time.Duration
	RequestTimeout time.Duration
	Workers        int
	Capacity       int
	BatchSize      int
	Priority       bool
}

type Pool struct {
	repo         repository
	push         gateway
	config       Config
	logger       *slog.Logger
	tracer       trace.Tracer
	queue        *queue
	metrics      metrics
	registration metric.Registration
	active       atomic.Int64
}

func New(repo repository, push gateway, config Config, logger *slog.Logger, tracer trace.Tracer, meter metric.Meter) (*Pool, error) {
	instruments, err := newMetrics(meter)
	if err != nil {
		return nil, err
	}

	p := &Pool{
		repo:    repo,
		push:    push,
		config:  config,
		logger:  logger,
		tracer:  tracer,
		queue:   newQueue(config.Capacity, config.Priority),
		metrics: instruments,
	}

	p.registration, err = p.registerMetrics(meter)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Pool) Cancel(id trip.ID) {
	p.queue.cancel(id)
}

func (p *Pool) Start(ctx context.Context, ticks ticker) func(context.Context) error {
	scanCtx, stopScanner := context.WithCancel(ctx)
	workCtx, stopWorkers := context.WithCancel(context.WithoutCancel(ctx))

	scanDone := make(chan struct{})
	workDone := make(chan struct{})

	go func() {
		defer close(scanDone)
		defer p.queue.close()
		defer ticks.Stop()

		p.logger.InfoContext(scanCtx, "position scanner started")
		defer p.logger.InfoContext(scanCtx, "position scanner stopped")
		
		p.scan(scanCtx, ticks)
	}()

	p.logger.InfoContext(workCtx, "position workers started", "workers", p.config.Workers)

	var workers sync.WaitGroup
	for range p.config.Workers {
		workers.Go(func() { p.work(workCtx) })
	}

	go func() {
		workers.Wait()
		p.logger.InfoContext(workCtx, "position workers stopped")
		close(workDone)
	}()

	return func(deadline context.Context) error {
		defer stopWorkers()
		stopScanner()

		select {
		case <-workDone:
			<-scanDone
			return p.registration.Unregister()
		case <-deadline.Done():
			stopWorkers()
			<-scanDone
			<-workDone
			return errors.Join(deadline.Err(), p.registration.Unregister())
		}
	}
}

func (p *Pool) scan(ctx context.Context, ticks ticker) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks.Ticks():
			if ctx.Err() == nil {
				p.scanOnce(ctx)
			}
		}
	}
}

func (p *Pool) scanOnce(ctx context.Context) {
	ctx, span := p.tracer.Start(ctx, "positions.scan")
	defer span.End()

	var after *trip.StaleTrip
	var rowsRead int64
	batch := p.config.BatchSize

	if batch > 0 {
		defer func() { p.metrics.rowsRead.Record(ctx, rowsRead) }()
	}

	for ctx.Err() == nil {
		trips, err := p.repo.StaleTrips(ctx, p.config.StaleAfter, after, batch)
		if err != nil {
			if ctx.Err() == nil {
				p.logger.ErrorContext(ctx, "select stale trips", "error", err)
				span.RecordError(err)
				span.SetStatus(codes.Error, "stale trip lookup failed")
			}
			return
		}

		rowsRead += int64(len(trips))
		if !p.enqueueTrips(ctx, trips, span.SpanContext()) || batch == 0 || len(trips) < batch {
			return
		}

		after = &trips[len(trips)-1]
	}
}

func (p *Pool) enqueueTrips(ctx context.Context, trips []trip.StaleTrip, source trace.SpanContext) bool {
	for _, candidate := range trips {
		if ctx.Err() != nil {
			return false
		}

		result, evicted := p.queue.offer(&task{trip: candidate, source: source})
		p.metrics.checks.Add(ctx, 1, resultAttribute(string(result)))

		if evicted != nil {
			p.metrics.checks.Add(ctx, 1, resultAttribute(string(dropped)))
			p.recordDrop(ctx, evicted.trip.ID)
		}

		if result == dropped {
			p.recordDrop(ctx, candidate.ID)
			if p.config.BatchSize > 0 {
				return false
			}
		}
	}

	return true
}

func (p *Pool) recordDrop(ctx context.Context, id trip.ID) {
	p.metrics.requests.Add(ctx, 1, resultAttribute(string(dropped)))
	p.logger.WarnContext(ctx, "position queue full", "trip_id", id)
}

func (p *Pool) work(ctx context.Context) {
	for {
		t := p.queue.next(ctx)
		if t == nil {
			return
		}

		p.process(ctx, t)
	}
}

func (p *Pool) process(ctx context.Context, t *task) {
	defer p.queue.finish(t)

	ctx, span := p.tracer.Start(ctx, "positions.request", trace.WithLinks(trace.Link{SpanContext: t.source}))
	defer span.End()

	p.active.Add(1)
	defer p.active.Add(-1)

	requestCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
	defer cancel()

	start := time.Now()
	err := p.push.RequestPosition(requestCtx, t.trip)
	elapsed := time.Since(start)

	result := "success"
	if err != nil {
		result = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, "position request failed")
		p.logger.WarnContext(ctx, "request driver position", "trip_id", t.trip.ID, "error", err)
	}

	p.metrics.requests.Add(ctx, 1, resultAttribute(result))
	p.metrics.duration.Record(ctx, elapsed.Seconds(), resultAttribute(result))
}
