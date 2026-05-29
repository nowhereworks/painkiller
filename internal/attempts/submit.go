package attempts

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"painkiller-shell/internal/jobs"
	"painkiller-shell/internal/models"
)

func (s *Service) SubmitAttempt(ctx context.Context, userID, attemptID uuid.UUID) error {
	attempt, err := s.store.Attempts().GetByID(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("attempt not found: %w", err)
	}

	purchase, err := s.store.Purchases().GetByID(ctx, attempt.PurchasedTestID)
	if err != nil {
		return fmt.Errorf("purchase not found: %w", err)
	}

	if purchase.UserID != userID {
		return fmt.Errorf("unauthorized")
	}

	if attempt.Status != models.AttemptStatusRunning {
		return fmt.Errorf("attempt is not running")
	}

	if err := s.TransitionAttempt(ctx, attemptID, models.AttemptStatusSubmitted); err != nil {
		return fmt.Errorf("failed to submit attempt: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{"attempt_id": attemptID.String()})
	if err := s.queue.Enqueue(ctx, jobs.JobKindGradeAttempt, payload, nil); err != nil {
		return fmt.Errorf("failed to enqueue grading job: %w", err)
	}

	return nil
}
