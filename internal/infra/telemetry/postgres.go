package telemetry

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type QueryTracer struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
}

type (
	queryKey  struct{}
	queryInfo struct {
		start                 time.Time
		operation, collection string
	}
)

func NewQueryTracer(meter metric.Meter) (*QueryTracer, error) {
	h, err := meter.Float64Histogram("db.client.operation.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	return &QueryTracer{Tracer: otelpgx.NewTracer(otelpgx.WithMeterProvider(noop.NewMeterProvider()), otelpgx.WithDisableSQLStatementInAttributes()), duration: h}, nil
}

func queryAttributes(sql string) (string, string) {
	parts := strings.Fields(sql)

	if len(parts) == 0 {
		return "UNKNOWN", ""
	}

	op := strings.ToUpper(parts[0])

	for i, p := range parts {
		if (strings.EqualFold(p, "FROM") || strings.EqualFold(p, "INTO") || strings.EqualFold(p, "UPDATE")) && i+1 < len(parts) {
			return op, strings.Trim(parts[i+1], `"`)
		}
	}

	return op, ""
}

func (t *QueryTracer) TraceQueryStart(ctx context.Context, c *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	op, collection := queryAttributes(d.SQL)
	ctx = t.Tracer.TraceQueryStart(ctx, c, d)
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("db.operation.name", op), attribute.String("db.collection.name", collection))

	return context.WithValue(ctx, queryKey{}, queryInfo{time.Now(), op, collection})
}

func (t *QueryTracer) TraceQueryEnd(ctx context.Context, c *pgx.Conn, d pgx.TraceQueryEndData) {
	info, _ := ctx.Value(queryKey{}).(queryInfo)
	errorType := ""

	if d.Err != nil && !errors.Is(d.Err, pgx.ErrNoRows) {
		errorType = "database_error"
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("error.type", errorType))
	}

	t.duration.Record(ctx, time.Since(info.start).Seconds(), metric.WithAttributes(attribute.String("db.operation.name", info.operation), attribute.String("db.collection.name", info.collection), attribute.String("error.type", errorType)))
	t.Tracer.TraceQueryEnd(ctx, c, d)
}
