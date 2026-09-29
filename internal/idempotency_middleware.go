package tripservice

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
	api "github.com/saywich/tripgo/internal/generated"
)

type IdempotencyMiddleware struct {
	txManager  TxManager
	repository *IdempotencyRepository
}

var errIdempotencyResponseNotPersisted = errors.New("idempotency response is not persisted")

func NewIdempotencyMiddleware(
	txManager TxManager,
	repository *IdempotencyRepository,
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
			r.Body = io.NopCloser(bytes.NewReader(body))
			next.ServeHTTP(w, r)
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
					status: http.StatusOK,
					contentType: previous.ContentType,
					location: previous.Location,
					body: previous.ResponseBody,
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
	var data api.TripData

	decoder := json.NewDecoder(bytes.NewReader(body))

	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}

	return json.Marshal(data)
}

func snapshotResponse(recorder *httptest.ResponseRecorder) responseSnapshot {
	status := recorder.Code

	if status == 0 {
		status = http.StatusOK
	}

	return responseSnapshot{
		status: status,
		contentType: recorder.Header().Get("Content-Type"),
		location: recorder.Header().Get("Location"),
		body: append([]byte(nil), recorder.Body.Bytes()...),
	}
}
