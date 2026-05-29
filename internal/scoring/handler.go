package scoring

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"painkiller-shell/internal/auth"
	"painkiller-shell/internal/httpx"
	"painkiller-shell/internal/models"
	"painkiller-shell/internal/store"
)

type Handler struct {
	store *store.Store
}

func NewHandler(store *store.Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/attempts/{attemptID}/score", h.getScore)
}

type scoreResponse struct {
	TotalScore int    `json:"total_score"`
	MaxScore   int    `json:"max_score"`
	Percentage int    `json:"percentage"`
	Status     string `json:"status"`
}

func (h *Handler) getScore(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Unauthorized(w, "unauthorized")
		return
	}

	attemptIDStr := chi.URLParam(r, "attemptID")
	attemptID, err := uuid.Parse(attemptIDStr)
	if err != nil {
		httpx.BadRequest(w, "invalid attempt_id")
		return
	}

	attempt, err := h.store.Attempts().GetByID(r.Context(), attemptID)
	if err != nil {
		httpx.NotFound(w, "attempt not found")
		return
	}

	purchase, err := h.store.Purchases().GetByID(r.Context(), attempt.PurchasedTestID)
	if err != nil {
		httpx.NotFound(w, "purchase not found")
		return
	}

	if purchase.UserID != userID {
		httpx.Forbidden(w, "forbidden")
		return
	}

	if attempt.Status != models.AttemptStatusScored {
		httpx.NotFound(w, "score not available")
		return
	}

	totalScore := 0
	maxScore := 0
	if attempt.Score != nil {
		totalScore = *attempt.Score
	}
	if attempt.MaxScore != nil {
		maxScore = *attempt.MaxScore
	}

	percentage := 0
	if maxScore > 0 {
		percentage = (totalScore * 100) / maxScore
	}

	_ = httpx.WriteJSON(w, http.StatusOK, scoreResponse{
		TotalScore: totalScore,
		MaxScore:   maxScore,
		Percentage: percentage,
		Status:     string(attempt.Status),
	})
}
