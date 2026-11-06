package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/saywich/tripgo/internal/http/middleware"
)

func TestRecovery(t *testing.T) {
	var logs bytes.Buffer
	handler, err := middleware.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }), slog.New(slog.NewJSONHandler(&logs, nil)), noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/trips/secret-id", nil).WithContext(middleware.WithRoute(context.Background(), "/api/v1/trips/{tripId}"))
	handler.ServeHTTP(w, r)

	if w.Code != 500 || !bytes.Contains(w.Body.Bytes(), []byte(`"code":"internal_error"`)) {
		t.Fatalf("response %d %s", w.Code, w.Body.String())
	}

	decoder := json.NewDecoder(&logs)
	handled := 0

	for decoder.More() {
		var entry map[string]any

		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}

		if entry["msg"] == "request handled" {
			handled++

			if entry["route"] != "/api/v1/trips/{tripId}" || entry["level"] != "ERROR" {
				t.Fatalf("log %+v", entry)
			}
		}
	}

	if handled != 1 {
		t.Fatalf("access records: %d", handled)
	}
}
