package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/google/uuid"

	"github.com/saywich/tripgo/internal/http/trip/create"
	"github.com/saywich/tripgo/internal/model"
)

type IdempotencyMiddleware struct {
	txManager  txManager
	repository idempotencyRepository
}

var errIdempotencyResponseNotPersisted = errors.New("idempotency response is not persisted")

func NewIdempotencyMiddleware(
	txManager txManager,
	repository idempotencyRepository,
) *IdempotencyMiddleware {
	return &IdempotencyMiddleware{txManager: txManager, repository: repository}
}

type responseSnapshot struct {
	status      int
	contentType string
	location    string
	body        []byte
}

func (m *IdempotencyMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys := r.Header.Values("Idempotency-Key")

		if len(keys) == 0 {
			next.ServeHTTP(w, r)

			return
		}

		if len(keys) != 1 {
			writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid_request", "Idempotency-Key must be a single UUID")

			return
		}

		key, err := uuid.Parse(keys[0])
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid_request", "Idempotency-Key must be a valid UUID")

			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid_request", "Could not read request body")

			return
		}

		canonicalBody, err := canonicalTripRequest(body)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid_request", "Request body must be a single valid JSON value")

			return
		}

		requestHash := sha256.Sum256(canonicalBody)

		var response responseSnapshot
		err = m.txManager.Do(r.Context(), func(ctx context.Context) error {
			claimed, previous, err := m.repository.Claim(ctx, key, requestHash[:], canonicalBody)
			if err != nil {
				return err
			}

			if !claimed {
				if !bytes.Equal(previous.RequestHash, requestHash[:]) || !bytes.Equal(previous.RequestBody, canonicalBody) {
					recorder := httptest.NewRecorder()

					writeProblem(
						recorder,
						http.StatusConflict,
						"Idempotency conflict",
						"idempotency_conflict",
						"Idempotency-Key was already used with a different request body",
					)

					response = snapshotResponse(recorder)

					return nil
				}

				if previous.Status == 0 {
					return errors.New("idempotency key has no completed response")
				}

				response = responseSnapshot{
					status:      http.StatusOK,
					contentType: previous.ContentType,
					location:    previous.Location,
					body:        previous.ResponseBody,
				}

				return nil
			}

			request := r.Clone(ctx)
			request.Body = io.NopCloser(bytes.NewReader(body))

			recorder := httptest.NewRecorder()
			next.ServeHTTP(recorder, request)
			response = snapshotResponse(recorder)

			if response.status == http.StatusCreated {
				return m.repository.Complete(
					ctx,
					key,
					response.status,
					response.contentType,
					response.location,
					response.body,
				)
			}

			return errIdempotencyResponseNotPersisted
		})

		if err != nil && !errors.Is(err, errIdempotencyResponseNotPersisted) {
			writeProblem(
				w,
				http.StatusInternalServerError,
				"Internal Server Error",
				"internal_error",
				"Internal server error",
			)

			return
		}

		if response.contentType != "" {
			w.Header().Set("Content-Type", response.contentType)
		}

		if response.location != "" {
			w.Header().Set("Location", response.location)
		}

		w.WriteHeader(response.status)
		_, _ = w.Write(response.body)
	})
}

func canonicalTripRequest(body []byte) ([]byte, error) {
	tripData, err := create.DecodeTripData(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	return json.Marshal(tripData)
}

func snapshotResponse(recorder *httptest.ResponseRecorder) responseSnapshot {
	status := recorder.Code

	if status == 0 {
		status = http.StatusOK
	}

	return responseSnapshot{
		status:      status,
		contentType: recorder.Header().Get("Content-Type"),
		location:    recorder.Header().Get("Location"),
		body:        append([]byte(nil), recorder.Body.Bytes()...),
	}
}

type txManager interface {
	Do(context.Context, func(context.Context) error) error
}

type idempotencyRepository interface {
	Claim(context.Context, uuid.UUID, []byte, []byte) (bool, model.IdempotencyRecord, error)
	Complete(context.Context, uuid.UUID, int, string, string, []byte) error
}

func writeProblem(w http.ResponseWriter, status int, title, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "status": status, "title": title, "code": code, "detail": detail})
}
