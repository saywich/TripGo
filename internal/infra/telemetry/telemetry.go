package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type correlated struct{ slog.Handler }

func (h correlated) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)

	if sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}

	return h.Handler.Handle(ctx, r)
}

func (h correlated) WithAttrs(a []slog.Attr) slog.Handler { return correlated{h.Handler.WithAttrs(a)} }

func (h correlated) WithGroup(g string) slog.Handler { return correlated{h.Handler.WithGroup(g)} }

func New(ctx context.Context, level string) (*slog.Logger, func(context.Context) error, error) {
	var l slog.Level

	if err := l.UnmarshalText([]byte(level)); err != nil {
		return nil, nil, err
	}

	logger := slog.New(correlated{slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})}).With("service", "trip-service")
	var te sdktrace.SpanExporter
	var err error
	protocol := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")

	switch protocol {
	case "grpc":
		te, err = otlptracegrpc.New(ctx)
	case "", "http/protobuf":
		te, err = otlptracehttp.New(ctx)
	default:
		return nil, nil, fmt.Errorf("unsupported OTLP protocol %q", protocol)
	}

	if err != nil {
		return nil, nil, err
	}

	var me sdkmetric.Exporter

	if protocol == "grpc" {
		me, err = otlpmetricgrpc.New(ctx)
	} else {
		me, err = otlpmetrichttp.New(ctx)
	}

	if err != nil {
		_ = te.Shutdown(ctx)

		return nil, nil, err
	}

	res := resource.NewSchemaless(attribute.String("service.name", "trip-service"))
	sampler, err := configuredSampler()
	if err != nil {
		_ = te.Shutdown(ctx)
		_ = me.Shutdown(ctx)

		return nil, nil, err
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler), sdktrace.WithResource(res), sdktrace.WithBatcher(te))
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(me)))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return logger, func(ctx context.Context) error {
		a := tp.Shutdown(ctx)
		b := mp.Shutdown(ctx)

		if a != nil {
			return a
		}

		return b
	}, nil
}

func configuredSampler() (sdktrace.Sampler, error) {
	ratio := 1.0

	if arg := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); arg != "" {
		v, err := strconv.ParseFloat(arg, 64)

		if err != nil || v < 0 || v > 1 {
			return nil, fmt.Errorf("invalid OTEL_TRACES_SAMPLER_ARG")
		}

		ratio = v
	}

	switch name := os.Getenv("OTEL_TRACES_SAMPLER"); name {
	case "", "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample()), nil
	case "always_on":
		return sdktrace.AlwaysSample(), nil
	case "always_off":
		return sdktrace.NeverSample(), nil
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample()), nil
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(ratio), nil
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio)), nil
	default:
		return nil, fmt.Errorf("unsupported sampler %q", name)
	}
}
