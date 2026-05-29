package orchestrator

import (
	"context"
	"encoding/json"
	"time"

	"painkiller-shell/internal/jobs"
	"painkiller-shell/internal/models"
)

type Reconciler struct {
	o        *Orchestrator
	interval time.Duration
}

func NewReconciler(o *Orchestrator, interval time.Duration) *Reconciler {
	return &Reconciler{
		o:        o,
		interval: interval,
	}
}

func (r *Reconciler) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	r.cleanupStaleProvisioning(ctx)
	r.cleanupPendingAttempts(ctx)
	r.cleanupTerminalStates(ctx)
}

func (r *Reconciler) cleanupStaleProvisioning(ctx context.Context) {
	attempts, err := r.o.store.Attempts().ListByStatus(ctx, models.AttemptStatusEnvironmentProvisioning)
	if err != nil {
		r.o.logger.Error("failed to list provisioning attempts", "error", err)
		return
	}

	staleThreshold := time.Now().Add(-30 * time.Minute)
	for _, attempt := range attempts {
		if attempt.CreatedAt.Before(staleThreshold) {
			r.o.logger.Warn("attempt stuck in provisioning", "attempt_id", attempt.ID)
			_ = r.o.attempts.TransitionAttempt(ctx, attempt.ID, models.AttemptStatusProvisionFailed)
			r.enqueueCleanup(ctx, attempt.ID)
		}
	}
}

func (r *Reconciler) cleanupPendingAttempts(ctx context.Context) {
	attempts, err := r.o.store.Attempts().ListByStatus(ctx, models.AttemptStatusCleanupPending)
	if err != nil {
		r.o.logger.Error("failed to list cleanup_pending attempts", "error", err)
		return
	}

	for _, attempt := range attempts {
		r.enqueueCleanup(ctx, attempt.ID)
	}
}

func (r *Reconciler) cleanupTerminalStates(ctx context.Context) {
	terminalStates := []models.AttemptStatus{
		models.AttemptStatusScored,
		models.AttemptStatusExpiredBeforeStart,
	}

	for _, status := range terminalStates {
		attempts, err := r.o.store.Attempts().ListByStatus(ctx, status)
		if err != nil {
			r.o.logger.Error("failed to list attempts", "status", status, "error", err)
			continue
		}

		for _, attempt := range attempts {
			env, err := r.o.store.Environments().GetByAttemptID(ctx, attempt.ID)
			if err != nil {
				continue
			}

			if env.Status == models.EnvironmentStatusDestroyed {
				continue
			}

			r.enqueueCleanup(ctx, attempt.ID)
		}
	}
}

func (r *Reconciler) enqueueCleanup(ctx context.Context, attemptID interface{ String() string }) {
	payload, _ := json.Marshal(map[string]string{"attempt_id": attemptID.String()})
	if err := r.o.queue.Enqueue(ctx, jobs.JobKindCleanupEnvironment, payload, nil); err != nil {
		r.o.logger.Error("failed to enqueue cleanup", "attempt_id", attemptID.String(), "error", err)
	}
}
