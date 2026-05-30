package billing

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"painkiller-shell/internal/auth"
	"painkiller-shell/internal/httpx"
)

type Handler struct {
	service        *Service
	webhookHandler *WebhookHandler
}

func NewHandler(service *Service, webhookHandler *WebhookHandler) *Handler {
	return &Handler{
		service:        service,
		webhookHandler: webhookHandler,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/checkout", h.checkout)
	r.Post("/acquire-free", h.acquireFree)
}

type checkoutRequest struct {
	TestID string `json:"test_id"`
}

type checkoutResponse struct {
	URL string `json:"url"`
}

func (h *Handler) checkout(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Unauthorized(w, "unauthorized")
		return
	}

	var req checkoutRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid request body")
		return
	}

	testID, err := uuid.Parse(req.TestID)
	if err != nil {
		httpx.BadRequest(w, "invalid test_id")
		return
	}

	url, err := h.service.CreateCheckoutSession(r.Context(), userID, testID)
	if err != nil {
		slog.Default().Error("failed to create checkout session", "error", err)
		httpx.InternalError(w, "failed to create checkout session")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, checkoutResponse{URL: url})
}

type acquireFreeRequest struct {
	TestID string `json:"test_id"`
}

type acquireFreeResponse struct {
	PurchaseID string `json:"purchase_id"`
}

func (h *Handler) acquireFree(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		httpx.Unauthorized(w, "unauthorized")
		return
	}

	var req acquireFreeRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid request body")
		return
	}

	testID, err := uuid.Parse(req.TestID)
	if err != nil {
		httpx.BadRequest(w, "invalid test_id")
		return
	}

	purchaseID, err := h.service.CreateFreePurchase(r.Context(), userID, testID)
	if err != nil {
		slog.Default().Error("failed to create free purchase", "error", err)
		if err.Error() == "test is not free" {
			httpx.BadRequest(w, "test is not free")
			return
		}
		httpx.InternalError(w, "failed to create free purchase")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, acquireFreeResponse{PurchaseID: purchaseID.String()})
}
