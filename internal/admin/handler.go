package admin

import (
	"encoding/json"
	"net/http"
	"time"

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

	r.Get("/tests", h.listTests)
	r.Get("/tests/{testID}", h.getTest)
	r.Post("/tests", h.createTest)
	r.Put("/tests/{testID}", h.updateTest)
	r.Delete("/tests/{testID}", h.deleteTest)
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

type adminTestResponse struct {
	ID                string  `json:"id"`
	ProductID         string  `json:"product_id"`
	Title             string  `json:"title"`
	Description       string  `json:"description"`
	StripePriceID     *string `json:"stripe_price_id"`
	IsFree            bool    `json:"is_free"`
	DurationMinutes   int     `json:"duration_minutes"`
	AccessWindowHours int     `json:"access_window_hours"`
	AttemptsAllowed   int     `json:"attempts_allowed"`
}

func testWithProductToResponse(t *store.TestWithProduct) adminTestResponse {
	return adminTestResponse{
		ID:                t.ID.String(),
		ProductID:         t.ProductID.String(),
		Title:             t.ProductTitle,
		Description:       t.ProductDescription,
		StripePriceID:     t.ProductStripePriceID,
		IsFree:            t.ProductIsFree,
		DurationMinutes:   t.DurationMinutes,
		AccessWindowHours: t.AccessWindowHours,
		AttemptsAllowed:   t.AttemptsAllowed,
	}
}

func (h *Handler) listTests(w http.ResponseWriter, r *http.Request) {
	tests, err := h.store.Tests().ListWithProduct(r.Context())
	if err != nil {
		httpx.InternalError(w, "failed to list tests")
		return
	}

	resp := make([]adminTestResponse, 0, len(tests))
	for _, t := range tests {
		resp = append(resp, testWithProductToResponse(t))
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"tests": resp})
}

func (h *Handler) getTest(w http.ResponseWriter, r *http.Request) {
	testIDStr := chi.URLParam(r, "testID")
	testID, err := uuid.Parse(testIDStr)
	if err != nil {
		httpx.BadRequest(w, "invalid test_id")
		return
	}

	test, err := h.store.Tests().GetWithProduct(r.Context(), testID)
	if err != nil {
		httpx.NotFound(w, "test not found")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, testWithProductToResponse(test))
}

type createTestRequest struct {
	Title             string  `json:"title"`
	Description       string  `json:"description"`
	StripePriceID     *string `json:"stripe_price_id"`
	IsFree            bool    `json:"is_free"`
	DurationMinutes   int     `json:"duration_minutes"`
	AccessWindowHours int     `json:"access_window_hours"`
	AttemptsAllowed   int     `json:"attempts_allowed"`
}

func (h *Handler) createTest(w http.ResponseWriter, r *http.Request) {
	var req createTestRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid request body")
		return
	}

	if req.Title == "" {
		httpx.BadRequest(w, "title is required")
		return
	}

	if req.DurationMinutes <= 0 || req.AccessWindowHours <= 0 || req.AttemptsAllowed <= 0 {
		httpx.BadRequest(w, "duration_minutes, access_window_hours, and attempts_allowed must be positive")
		return
	}

	if !req.IsFree && (req.StripePriceID == nil || *req.StripePriceID == "") {
		httpx.BadRequest(w, "stripe_price_id is required for paid tests")
		return
	}

	now := time.Now()
	productID := uuid.New()
	testID := uuid.New()

	product := &models.Product{
		ID:            productID,
		StripePriceID: req.StripePriceID,
		Title:         req.Title,
		Description:   req.Description,
		IsFree:        req.IsFree,
		CreatedAt:     now,
	}

	test := &models.Test{
		ID:                testID,
		ProductID:         productID,
		DurationMinutes:   req.DurationMinutes,
		AccessWindowHours: req.AccessWindowHours,
		AttemptsAllowed:   req.AttemptsAllowed,
		CreatedAt:         now,
	}

	tx, err := h.store.DB().Beginx()
	if err != nil {
		httpx.InternalError(w, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(),
		`INSERT INTO products (id, stripe_price_id, title, description, is_free, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		product.ID, product.StripePriceID, product.Title, product.Description, product.IsFree, product.CreatedAt,
	); err != nil {
		httpx.InternalError(w, "failed to create product")
		return
	}

	if _, err := tx.ExecContext(r.Context(),
		`INSERT INTO tests (id, product_id, scenario_version_id, duration_minutes, access_window_hours, attempts_allowed, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		test.ID, test.ProductID, test.ScenarioVersionID, test.DurationMinutes, test.AccessWindowHours, test.AttemptsAllowed, test.CreatedAt,
	); err != nil {
		httpx.InternalError(w, "failed to create test")
		return
	}

	if err := tx.Commit(); err != nil {
		httpx.InternalError(w, "failed to commit transaction")
		return
	}

	result, err := h.store.Tests().GetWithProduct(r.Context(), testID)
	if err != nil {
		httpx.InternalError(w, "failed to fetch created test")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusCreated, testWithProductToResponse(result))
}

type updateTestRequest struct {
	Title             *string `json:"title"`
	Description       *string `json:"description"`
	StripePriceID     *string `json:"stripe_price_id"`
	IsFree            *bool   `json:"is_free"`
	DurationMinutes   *int    `json:"duration_minutes"`
	AccessWindowHours *int    `json:"access_window_hours"`
	AttemptsAllowed   *int    `json:"attempts_allowed"`
}

func (h *Handler) updateTest(w http.ResponseWriter, r *http.Request) {
	testIDStr := chi.URLParam(r, "testID")
	testID, err := uuid.Parse(testIDStr)
	if err != nil {
		httpx.BadRequest(w, "invalid test_id")
		return
	}

	var req updateTestRequest
	if err := httpx.ReadJSON(r, &req); err != nil {
		httpx.BadRequest(w, "invalid request body")
		return
	}

	existing, err := h.store.Tests().GetWithProduct(r.Context(), testID)
	if err != nil {
		httpx.NotFound(w, "test not found")
		return
	}

	product, err := h.store.Products().GetByID(r.Context(), existing.ProductID)
	if err != nil {
		httpx.InternalError(w, "failed to fetch product")
		return
	}

	if req.Title != nil {
		product.Title = *req.Title
	}
	if req.Description != nil {
		product.Description = *req.Description
	}
	if req.StripePriceID != nil {
		if *req.StripePriceID == "" {
			product.StripePriceID = nil
		} else {
			product.StripePriceID = req.StripePriceID
		}
	}
	if req.IsFree != nil {
		product.IsFree = *req.IsFree
	}

	if !product.IsFree && (product.StripePriceID == nil || *product.StripePriceID == "") {
		httpx.BadRequest(w, "stripe_price_id is required for paid tests")
		return
	}

	if err := h.store.Products().Update(r.Context(), product); err != nil {
		httpx.InternalError(w, "failed to update product")
		return
	}

	test := existing.Test
	if req.DurationMinutes != nil {
		if *req.DurationMinutes <= 0 {
			httpx.BadRequest(w, "duration_minutes must be positive")
			return
		}
		test.DurationMinutes = *req.DurationMinutes
	}
	if req.AccessWindowHours != nil {
		if *req.AccessWindowHours <= 0 {
			httpx.BadRequest(w, "access_window_hours must be positive")
			return
		}
		test.AccessWindowHours = *req.AccessWindowHours
	}
	if req.AttemptsAllowed != nil {
		if *req.AttemptsAllowed <= 0 {
			httpx.BadRequest(w, "attempts_allowed must be positive")
			return
		}
		test.AttemptsAllowed = *req.AttemptsAllowed
	}

	if err := h.store.Tests().Update(r.Context(), &test); err != nil {
		httpx.InternalError(w, "failed to update test")
		return
	}

	result, err := h.store.Tests().GetWithProduct(r.Context(), testID)
	if err != nil {
		httpx.InternalError(w, "failed to fetch updated test")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, testWithProductToResponse(result))
}

func (h *Handler) deleteTest(w http.ResponseWriter, r *http.Request) {
	testIDStr := chi.URLParam(r, "testID")
	testID, err := uuid.Parse(testIDStr)
	if err != nil {
		httpx.BadRequest(w, "invalid test_id")
		return
	}

	existing, err := h.store.Tests().GetByID(r.Context(), testID)
	if err != nil {
		httpx.NotFound(w, "test not found")
		return
	}

	hasPurchases, err := h.store.Tests().HasPurchases(r.Context(), testID)
	if err != nil {
		httpx.InternalError(w, "failed to check purchases")
		return
	}
	if hasPurchases {
		httpx.BadRequest(w, "cannot delete test with existing purchases")
		return
	}

	tx, err := h.store.DB().Beginx()
	if err != nil {
		httpx.InternalError(w, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(r.Context(), `DELETE FROM tests WHERE id = $1`, testID); err != nil {
		httpx.InternalError(w, "failed to delete test")
		return
	}

	if _, err := tx.ExecContext(r.Context(), `DELETE FROM products WHERE id = $1`, existing.ProductID); err != nil {
		httpx.InternalError(w, "failed to delete product")
		return
	}

	if err := tx.Commit(); err != nil {
		httpx.InternalError(w, "failed to commit transaction")
		return
	}

	_ = httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
