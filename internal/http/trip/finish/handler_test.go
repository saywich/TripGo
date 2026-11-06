package finish_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	trip "github.com/saywich/tripgo/internal/domain/trip"
	handler "github.com/saywich/tripgo/internal/http/trip/finish"
)

func TestHandler(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{{"success", nil, 200}, {"business", trip.ErrTripCompleted, 409}, {"internal", errors.New("password=secret"), 500}} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			u := NewMockusecase(ctrl)
			u.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(trip.Trip{ID: uuid.New()}, tc.err)
			r := chi.NewRouter()
			r.Method("POST", "/api/v1/trips/{tripID}/finish", handler.New(u))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/trips/11111111-1111-4111-8111-111111111111/finish", strings.NewReader(`{"driver_id":"11111111-1111-4111-8111-111111111111","user_id":"22222222-2222-4222-8222-222222222222","price":10,"start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0}}`)))

			if w.Code != tc.status || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

var (
	_ context.Context
	_ http.Handler
)
