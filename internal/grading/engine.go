package grading

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"painkiller-shell/internal/attempts"
	"painkiller-shell/internal/jobs"
	"painkiller-shell/internal/models"
	"painkiller-shell/internal/store"
)

type Engine struct {
	store    *store.Store
	attempts *attempts.Service
	runner   *Runner
	queue    *jobs.Queue
	logger   *slog.Logger
}

func NewEngine(store *store.Store, attemptsSvc *attempts.Service, queue *jobs.Queue, logger *slog.Logger) *Engine {
	return &Engine{
		store:    store,
		attempts: attemptsSvc,
		runner:   NewRunner(),
		queue:    queue,
		logger:   logger,
	}
}

func (e *Engine) GradeAttempt(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		AttemptID string `json:"attempt_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	attemptID, err := uuid.Parse(p.AttemptID)
	if err != nil {
		return fmt.Errorf("invalid attempt_id: %w", err)
	}

	attempt, err := e.store.Attempts().GetByID(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("attempt not found: %w", err)
	}

	if err := e.attempts.TransitionAttempt(ctx, attemptID, models.AttemptStatusGrading); err != nil {
		return fmt.Errorf("failed to transition to grading: %w", err)
	}

	purchase, err := e.store.Purchases().GetByID(ctx, attempt.PurchasedTestID)
	if err != nil {
		return fmt.Errorf("purchase not found: %w", err)
	}

	test, err := e.store.Tests().GetByID(ctx, purchase.TestID)
	if err != nil {
		return fmt.Errorf("test not found: %w", err)
	}

	checks, err := e.store.Scenarios().ListChecksByScenarioVersion(ctx, test.ScenarioVersionID)
	if err != nil {
		return fmt.Errorf("failed to list checks: %w", err)
	}

	env, err := e.store.Environments().GetByAttemptID(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("environment not found: %w", err)
	}

	totalScore := 0
	maxScore := 0

	for _, check := range checks {
		maxScore += check.Points

		result, err := e.runner.RunCheck(ctx, *check, *env.WorkstationIP, env.SSHPrivateKey)
		if err != nil {
			e.logger.Error("check failed", "check_id", check.ID, "error", err)
			continue
		}

		totalScore += result.PointsAwarded
		e.logger.Info("check completed", "check_id", check.ID, "passed", result.Passed, "points", result.PointsAwarded)
	}

	if err := e.store.Attempts().UpdateScore(ctx, attemptID, totalScore, maxScore); err != nil {
		return fmt.Errorf("failed to update score: %w", err)
	}

	if err := e.attempts.TransitionAttempt(ctx, attemptID, models.AttemptStatusScored); err != nil {
		return fmt.Errorf("failed to transition to scored: %w", err)
	}

	cleanupPayload, _ := json.Marshal(map[string]string{"attempt_id": attemptID.String()})
	if err := e.queue.Enqueue(ctx, jobs.JobKindCleanupEnvironment, cleanupPayload, nil); err != nil {
		e.logger.Error("failed to enqueue cleanup", "attempt_id", attemptID, "error", err)
	}

	e.logger.Info("attempt graded", "attempt_id", attemptID, "score", totalScore, "max_score", maxScore)
	return nil
}
