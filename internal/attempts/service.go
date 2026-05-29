package attempts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"painkiller-shell/internal/jobs"
	"painkiller-shell/internal/models"
	"painkiller-shell/internal/store"
)

var (
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrNoAttemptsRemaining = errors.New("no attempts remaining")
	ErrPurchaseExpired     = errors.New("purchase has expired")
)

type Service struct {
	store *store.Store
	queue *jobs.Queue
}

func NewService(store *store.Store, queue *jobs.Queue) *Service {
	return &Service{
		store: store,
		queue: queue,
	}
}

func (s *Service) RequestAttempt(ctx context.Context, userID, purchasedTestID uuid.UUID) (*models.Attempt, error) {
	purchase, err := s.store.Purchases().GetByID(ctx, purchasedTestID)
	if err != nil {
		return nil, fmt.Errorf("purchase not found: %w", err)
	}

	if purchase.UserID != userID {
		return nil, errors.New("unauthorized")
	}

	if time.Now().After(purchase.ExpiresAt) {
		return nil, ErrPurchaseExpired
	}

	if purchase.AttemptsRemaining <= 0 {
		return nil, ErrNoAttemptsRemaining
	}

	if err := s.store.Purchases().DecrementAttemptsRemaining(ctx, purchase.ID); err != nil {
		return nil, fmt.Errorf("failed to decrement attempts: %w", err)
	}

	attempt := &models.Attempt{
		ID:              uuid.New(),
		PurchasedTestID: purchase.ID,
		Status:          models.AttemptStatusAttemptRequested,
		CreatedAt:       time.Now(),
	}

	if err := s.store.Attempts().Create(ctx, attempt); err != nil {
		return nil, fmt.Errorf("failed to create attempt: %w", err)
	}

	payload, _ := json.Marshal(map[string]string{"attempt_id": attempt.ID.String()})
	if err := s.queue.Enqueue(ctx, jobs.JobKindProvisionEnvironment, payload, nil); err != nil {
		return nil, fmt.Errorf("failed to enqueue provisioning job: %w", err)
	}

	return attempt, nil
}

func (s *Service) TransitionAttempt(ctx context.Context, attemptID uuid.UUID, to models.AttemptStatus) error {
	attempt, err := s.store.Attempts().GetByID(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("attempt not found: %w", err)
	}

	if !ValidTransition(attempt.Status, to) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, attempt.Status, to)
	}

	if err := s.store.Attempts().UpdateStatus(ctx, attemptID, to); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

func (s *Service) GetAttempt(ctx context.Context, attemptID uuid.UUID) (*models.Attempt, error) {
	return s.store.Attempts().GetByID(ctx, attemptID)
}
