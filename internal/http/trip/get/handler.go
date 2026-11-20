package get

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	model "github.com/saywich/tripgo/internal/domain/trip"
	"github.com/saywich/tripgo/internal/http/domainerror"
)

type Handler struct{ usecase usecase }

func New(u usecase) *Handler { return &Handler{u} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	input, err := uuid.Parse(chi.URLParam(r, "tripID"))
	if err != nil {
		domainerror.Write(w, &model.ValidationError{Field: "tripID", Message: "must be a valid UUID"})

		return
	}

	result, err := h.usecase.Execute(r.Context(), input)
	if err != nil {
		domainerror.Write(w, err)

		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(result)
}
