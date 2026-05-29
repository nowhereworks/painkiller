package admin

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"painkiller-shell/internal/attempts"
	"painkiller-shell/internal/httpx"
	"painkiller-shell/internal/jobs"
	"painkiller-shell/internal/models"
	"painkiller-shell/internal/store"
)

type Handler struct {
	store    *store.Store
	attempts *attempts.Service
	queue    *jobs.Queue
}

func NewHandler(store *store.Store, attemptsSvc *attempts.Service, queue *jobs.Queue) *Handler {
	return &Handler{
		store:    store,
		attempts: attemptsSvc,
		queue:    queue,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/attempts/{attemptID}/retry-provision", h.retryProvision)
	r.Post("/attempts/{attemptID}/retry-grade", h.retryGrade)
	r.Post("/environments/{environmentID}/destroy", h.forceDestroy)
	r.Get("/attempts", h.listAttempts)
	r.Get("/environments", h.listEnvironments)
}

func (h *Handler) retryProvision(w http.ResponseWriter, r *http.Request) {
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

	if attempt.Status != models.AttemptStatusProvisionFailed {
		httpx.BadRequest(w, "attempt is not in provision_failed state")
		return
	}

	if err := h.attempts.TransitionAttempt(r.Context(), attemptID, models.AttemptStatusAttemptRequested); err != nil {
		httpx.InternalError(w, "failed to reset attempt")
		return
	}

	payload, _ := json.Marshal(map[string]string{"attempt_id": attemptID.String()})
	if err := h.queue.Enqueue(r.Context(), jobs.JobKindProvisionEnvironment, payload, nil); err != nil {
		httpx.InternalError(w, "failed to enqueue provisioning")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "retrying"})
}

func (h *Handler) retryGrade(w http.ResponseWriter, r *http.Request) {
	attemptIDStr := chi.URLParam(r, "attemptID")
	attemptID, err := uuid.Parse(attemptIDStr)
	if err != nil {
		httpx.BadRequest(w, "invalid attempt_id")
		return
	}

	payload, _ := json.Marshal(map[string]string{"attempt_id": attemptID.String()})
	if err := h.queue.Enqueue(r.Context(), jobs.JobKindGradeAttempt, payload, nil); err != nil {
		httpx.InternalError(w, "failed to enqueue grading")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "regrading"})
}

func (h *Handler) forceDestroy(w http.ResponseWriter, r *http.Request) {
	envIDStr := chi.URLParam(r, "environmentID")
	envID, err := uuid.Parse(envIDStr)
	if err != nil {
		httpx.BadRequest(w, "invalid environment_id")
		return
	}

	env, err := h.store.Environments().GetByID(r.Context(), envID)
	if err != nil {
		httpx.NotFound(w, "environment not found")
		return
	}

	payload, _ := json.Marshal(map[string]string{"attempt_id": env.AttemptID.String()})
	if err := h.queue.Enqueue(r.Context(), jobs.JobKindCleanupEnvironment, payload, nil); err != nil {
		httpx.InternalError(w, "failed to enqueue cleanup")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "destroying"})
}

type attemptResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (h *Handler) listAttempts(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	var allAttempts []*models.Attempt
	if statusFilter != "" {
		attempts, err := h.store.Attempts().ListByStatus(r.Context(), models.AttemptStatus(statusFilter))
		if err != nil {
			httpx.InternalError(w, "failed to list attempts")
			return
		}
		allAttempts = attempts
	} else {
		for _, status := range []models.AttemptStatus{
			models.AttemptStatusAttemptRequested,
			models.AttemptStatusEnvironmentProvisioning,
			models.AttemptStatusEnvironmentReady,
			models.AttemptStatusRunning,
			models.AttemptStatusProvisionFailed,
			models.AttemptStatusCleanupPending,
		} {
			attempts, err := h.store.Attempts().ListByStatus(r.Context(), status)
			if err != nil {
				continue
			}
			allAttempts = append(allAttempts, attempts...)
		}
	}

	resp := make([]attemptResponse, 0, len(allAttempts))
	for _, a := range allAttempts {
		resp = append(resp, attemptResponse{
			ID:     a.ID.String(),
			Status: string(a.Status),
		})
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"attempts": resp})
}

type environmentResponse struct {
	ID        string `json:"id"`
	AttemptID string `json:"attempt_id"`
	Status    string `json:"status"`
}

func (h *Handler) listEnvironments(w http.ResponseWriter, r *http.Request) {
	_ = httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"environments": []environmentResponse{}})
}
