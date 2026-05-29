package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

type WebhookHandler struct {
	service       *Service
	webhookSecret string
	logger        *slog.Logger
}

func NewWebhookHandler(service *Service, webhookSecret string, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{
		service:       service,
		webhookSecret: webhookSecret,
		logger:        logger,
	}
}

func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", "error", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), h.webhookSecret)
	if err != nil {
		h.logger.Error("failed to verify webhook signature", "error", err)
		http.Error(w, "invalid signature", http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			h.logger.Error("failed to unmarshal checkout session", "error", err)
			http.Error(w, "failed to parse event", http.StatusBadRequest)
			return
		}

		if err := h.service.HandleCheckoutCompleted(context.Background(), &sess); err != nil {
			h.logger.Error("failed to handle checkout completed", "error", err, "session_id", sess.ID)
			http.Error(w, "processing failed", http.StatusInternalServerError)
			return
		}

		h.logger.Info("checkout completed", "session_id", sess.ID)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")

	default:
		h.logger.Debug("unhandled webhook event", "type", event.Type)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	}
}
