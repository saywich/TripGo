package create_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	handler "github.com/saywich/tripgo/internal/http/trip/create"
)

func TestInvalidBody(t *testing.T) {
	for _, body := range []string{`{`, `null`, `{}`, `{} {}`, `{"price":null}`, `{"unknown":1}`} {
		t.Run(body, func(t *testing.T) {
			u := NewMockusecase(gomock.NewController(t))
			w := httptest.NewRecorder()
			handler.New(u).ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/trips", strings.NewReader(body)))

			if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"invalid_request"`) {
				t.Fatalf("response: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
