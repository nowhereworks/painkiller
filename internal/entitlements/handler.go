package entitlements

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"painkiller-shell/internal/auth"
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
	r.Get("/dashboard", h.dashboard)
}

type purchaseResponse struct {
	ID                string    `json:"id"`
	TestID            string    `json:"test_id"`
	ExpiresAt         time.Time `json:"expires_at"`
	AttemptsRemaining int       `json:"attempts_remaining"`
	IsActive          bool      `json:"is_active"`
}

type dashboardResponse struct {
	Purchases []purchaseResponse `json:"purchases"`
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Unauthorized(w, "unauthorized")
		return
	}

	purchases, err := h.store.Purchases().ListByUserID(r.Context(), userID)
	if err != nil {
		httpx.InternalError(w, "failed to list purchases")
		return
	}

	now := time.Now()
	resp := dashboardResponse{Purchases: make([]purchaseResponse, 0, len(purchases))}
	for _, p := range purchases {
		resp.Purchases = append(resp.Purchases, purchaseResponse{
			ID:                p.ID.String(),
			TestID:            p.TestID.String(),
			ExpiresAt:         p.ExpiresAt,
			AttemptsRemaining: p.AttemptsRemaining,
			IsActive:          p.ExpiresAt.After(now) && p.AttemptsRemaining > 0,
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, resp)
}
