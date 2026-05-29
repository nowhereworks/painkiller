package catalog

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"painkiller-shell/internal/httpx"
	"painkiller-shell/internal/store"
)

type Handler struct {
	store *store.Store
}

func NewHandler(store *store.Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/tests", h.listTests)
}

type testResponse struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	DurationMinutes   int    `json:"duration_minutes"`
	AccessWindowHours int    `json:"access_window_hours"`
	AttemptsAllowed   int    `json:"attempts_allowed"`
}

type listTestsResponse struct {
	Tests []testResponse `json:"tests"`
}

func (h *Handler) listTests(w http.ResponseWriter, r *http.Request) {
	tests, err := h.store.Tests().List(r.Context())
	if err != nil {
		httpx.InternalError(w, "failed to list tests")
		return
	}

	resp := listTestsResponse{Tests: make([]testResponse, 0, len(tests))}
	for _, t := range tests {
		resp.Tests = append(resp.Tests, testResponse{
			ID:                t.ID.String(),
			Title:             "",
			Description:       "",
			DurationMinutes:   t.DurationMinutes,
			AccessWindowHours: t.AccessWindowHours,
			AttemptsAllowed:   t.AttemptsAllowed,
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, resp)
}
