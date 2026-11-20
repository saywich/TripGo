package positions

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type metrics struct {
	checks        metric.Int64Counter
	requests      metric.Int64Counter
	duration      metric.Float64Histogram
	rowsRead      metric.Int64Histogram
	queueSize     metric.Int64ObservableGauge
	workersActive metric.Int64ObservableGauge
}

func newMetrics(meter metric.Meter) (metrics, error) {
	checks, checksErr := meter.Int64Counter("tripgo.stale_checks")
	requests, requestsErr := meter.Int64Counter("tripgo.position_requests")
	duration, durationErr := meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"))
	rows, rowsErr := meter.Int64Histogram("tripgo.stale_checks.rows_read")
	size, sizeErr := meter.Int64ObservableGauge("tripgo.position_requests.queue.size")
	active, activeErr := meter.Int64ObservableGauge("tripgo.position_requests.workers.active")

	return metrics{
		checks:        checks,
		requests:      requests,
		duration:      duration,
		rowsRead:      rows,
		queueSize:     size,
		workersActive: active,
	}, errors.Join(checksErr, requestsErr, durationErr, rowsErr, sizeErr, activeErr)
}

func resultAttribute(value string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("result", value))
}

func (p *Pool) registerMetrics(meter metric.Meter) (metric.Registration, error) {
	return meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		observer.ObserveInt64(p.metrics.queueSize, p.queue.size())
		observer.ObserveInt64(p.metrics.workersActive, p.active.Load())
		return nil
	}, p.metrics.queueSize, p.metrics.workersActive)
}
