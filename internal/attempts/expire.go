package attempts

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"painkiller-shell/internal/jobs"
	"painkiller-shell/internal/models"
)

type expirePayload struct {
	AttemptID string `json:"attempt_id"`
}

func (s *Service) HandleExpireAttempt(ctx context.Context, payload json.RawMessage) error {
	var p expirePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	attemptID, err := uuid.Parse(p.AttemptID)
	if err != nil {
		return fmt.Errorf("invalid attempt_id: %w", err)
	}

	attempt, err := s.store.Attempts().GetByID(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("attempt not found: %w", err)
	}

	if attempt.Status != models.AttemptStatusRunning {
		return nil
	}

	if err := s.TransitionAttempt(ctx, attemptID, models.AttemptStatusExpired); err != nil {
		return fmt.Errorf("failed to expire attempt: %w", err)
	}

	gradePayload, _ := json.Marshal(map[string]string{"attempt_id": attemptID.String()})
	if err := s.queue.Enqueue(ctx, jobs.JobKindGradeAttempt, gradePayload, nil); err != nil {
		return fmt.Errorf("failed to enqueue grading job: %w", err)
	}

	return nil
}
