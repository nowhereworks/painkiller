package jobs

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/riverqueue/river"
)

type HandlerFunc func(ctx context.Context, payload json.RawMessage) error

type GenericWorker struct {
	river.WorkerDefaults[GenericArgs]
	handlers map[JobKind]HandlerFunc
	logger   *slog.Logger
}

func NewGenericWorker(logger *slog.Logger) *GenericWorker {
	return &GenericWorker{
		handlers: make(map[JobKind]HandlerFunc),
		logger:   logger,
	}
}

func (w *GenericWorker) RegisterHandler(kind JobKind, handler HandlerFunc) {
	w.handlers[kind] = handler
}

func (w *GenericWorker) Work(ctx context.Context, job *river.Job[GenericArgs]) error {
	handler, ok := w.handlers[job.Args.JobKind]
	if !ok {
		w.logger.Error("no handler registered for job kind", "kind", job.Args.JobKind)
		return nil
	}

	w.logger.Info("processing job", "kind", job.Args.JobKind, "job_id", job.ID)

	if err := handler(ctx, job.Args.Payload); err != nil {
		w.logger.Error("job handler failed", "kind", job.Args.JobKind, "job_id", job.ID, "error", err)
		return err
	}

	w.logger.Info("job completed", "kind", job.Args.JobKind, "job_id", job.ID)
	return nil
}
