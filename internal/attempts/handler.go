package attempts

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"painkiller-shell/internal/auth"
	"painkiller-shell/internal/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/", h.createAttempt)
	r.Get("/{attemptID}", h.getAttempt)
	r.Post("/{attemptID}/submit", h.submitAttempt)
}

type createAttemptRequest struct {
	PurchasedTestID string `json:"purchased_test_id"`
}

type createAttemptResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (h *Handler) createAttempt(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Unauthorized(w, "unauthorized")
		return
	}

	var req createAttemptRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid request body")
		return
	}

	purchasedTestID, err := uuid.Parse(req.PurchasedTestID)
	if err != nil {
		httpx.BadRequest(w, "invalid purchased_test_id")
		return
	}

	attempt, err := h.service.RequestAttempt(r.Context(), userID, purchasedTestID)
	if err != nil {
		if err == ErrPurchaseExpired {
			httpx.BadRequest(w, "purchase has expired")
			return
		}
		if err == ErrNoAttemptsRemaining {
			httpx.BadRequest(w, "no attempts remaining")
			return
		}
		httpx.InternalError(w, "failed to create attempt")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, createAttemptResponse{
		ID:     attempt.ID.String(),
		Status: string(attempt.Status),
	})
}

type getAttemptResponse struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	Score         *int    `json:"score,omitempty"`
	MaxScore      *int    `json:"max_score,omitempty"`
	TerminalToken *string `json:"terminal_token,omitempty"`
}

func (h *Handler) getAttempt(w http.ResponseWriter, r *http.Request) {
	attemptIDStr := chi.URLParam(r, "attemptID")
	attemptID, err := uuid.Parse(attemptIDStr)
	if err != nil {
		httpx.BadRequest(w, "invalid attempt_id")
		return
	}

	attempt, err := h.service.GetAttempt(r.Context(), attemptID)
	if err != nil {
		httpx.NotFound(w, "attempt not found")
		return
	}

	resp := getAttemptResponse{
		ID:       attempt.ID.String(),
		Status:   string(attempt.Status),
		Score:    attempt.Score,
		MaxScore: attempt.MaxScore,
	}

	_ = httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) submitAttempt(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.SubmitAttempt(r.Context(), userID, attemptID); err != nil {
		if err.Error() == "attempt is not running" {
			httpx.BadRequest(w, "attempt is not running")
			return
		}
		if err.Error() == "unauthorized" {
			httpx.Forbidden(w, "forbidden")
			return
		}
		httpx.InternalError(w, "failed to submit attempt")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "submitted"})
}
