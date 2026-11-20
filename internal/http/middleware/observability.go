package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type (
	routeKey     struct{}
	requestKey   struct{}
	requestState struct{ ctx context.Context }
)

func WithRoute(ctx context.Context, template string) context.Context {
	return context.WithValue(ctx, routeKey{}, template)
}

func route(r *http.Request) string {
	if template, ok := r.Context().Value(routeKey{}).(string); ok && template != "" {
		return template
	}

	if c := chi.RouteContext(r.Context()); c != nil && c.RoutePattern() != "" {
		return c.RoutePattern()
	}

	return "unmatched"
}

func Wrap(next http.Handler, logger *slog.Logger, meter metric.Meter) (http.Handler, error) {
	duration, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}

	active, err := meter.Int64UpDownCounter("http.server.active_requests")
	if err != nil {
		return nil, err
	}

	metrics := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := metric.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("http.route", route(r)))
		active.Add(r.Context(), 1, a)
		defer active.Add(r.Context(), -1, a)
		start := time.Now()
		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		done := false
		defer func() {
			status := ww.Status()
			if !done {
				status = 500
			}
			if status == 0 {
				status = 200
			}
			duration.Record(r.Context(), time.Since(start).Seconds(), metric.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("http.route", route(r)), attribute.Int("http.response.status_code", status)))
		}()
		next.ServeHTTP(ww, r)
		done = true
	})
	logging := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		if state, ok := r.Context().Value(requestKey{}).(*requestState); ok {
			state.ctx = r.Context()
		}
		done := false
		defer func() {
			status := ww.Status()
			if !done {
				status = 500
			}

			if status == 0 {
				status = 200
			}

			span := trace.SpanFromContext(r.Context())
			span.SetName(r.Method + " " + route(r))
			span.SetAttributes(attribute.String("http.route", route(r)))
			level := slog.LevelInfo

			if status >= 500 {
				level = slog.LevelError
				span.SetStatus(codes.Error, http.StatusText(status))
				span.SetAttributes(attribute.String("error.type", "internal_error"))
			}

			logger.Log(r.Context(), level, "request handled", "method", r.Method, "route", route(r), "status", status, "duration_ms", time.Since(start).Milliseconds())
		}()
		metrics.ServeHTTP(ww, r)
		done = true
	})
	tracing := otelhttp.NewHandler(logging, "HTTP", otelhttp.WithMeterProvider(noop.NewMeterProvider()), otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return r.Method + " " + route(r) }))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := &requestState{ctx: r.Context()}
		r = r.WithContext(context.WithValue(r.Context(), requestKey{}, state))
		response := httptest.NewRecorder()
		defer func() {
			if p := recover(); p != nil {
				logger.ErrorContext(state.ctx, "panic recovered", "panic", p, "stack", string(debug.Stack()))
				response = httptest.NewRecorder()
				writeProblem(response, 500, "Internal Server Error", "internal_error", "Internal server error")
			}
			for key, values := range response.Header() {
				w.Header()[key] = values
			}
			w.WriteHeader(response.Code)
			_, _ = w.Write(response.Body.Bytes())
		}()
		tracing.ServeHTTP(response, r)
	}), nil
}
