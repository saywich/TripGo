package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/saywich/tripgo/internal/domain/trip"
)

type Client struct {
	endpoint   string
	http       *http.Client
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

type StatusError struct {
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("push service returned HTTP %d", e.Status)
}

func New(baseURL string, timeout time.Duration, tracer trace.Tracer, propagator propagation.TextMapPropagator) (*Client, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, fmt.Errorf("PUSH_HTTP_URL must be an absolute HTTP(S) URL")
	}

	dialer := &net.Dialer{Timeout: timeout}
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: timeout,
		IdleConnTimeout:     timeout,
	}

	client := &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}

	return &Client{endpoint: base.JoinPath("api", "v1", "push").String(), http: client, tracer: tracer, propagator: propagator}, nil
}

func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

func (c *Client) RequestPosition(ctx context.Context, t trip.StaleTrip) (err error) {
	ctx, span := c.tracer.Start(ctx, "POST /api/v1/push", trace.WithSpanKind(trace.SpanKindClient))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "push request failed")
		}

		span.End()
	}()

	requestID, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("generate push request ID: %w", err)
	}

	payload := struct {
		RequestID   uuid.UUID         `json:"request_id"`
		RecipientID uuid.UUID         `json:"recipient_id"`
		Kind        string            `json:"kind"`
		Data        map[string]string `json:"data"`
	}{
		RequestID:   requestID,
		RecipientID: t.DriverID,
		Kind:        "REQUEST_POSITION",
		Data:        map[string]string{"trip_id": t.ID.String()},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode push request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))

	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send push request: %w", err)
	}

	defer response.Body.Close()
	span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))

	if response.StatusCode != http.StatusAccepted {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return &StatusError{Status: response.StatusCode}
	}

	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10)); err != nil {
		return fmt.Errorf("read push response: %w", err)
	}

	return nil
}
