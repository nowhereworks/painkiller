package jobs

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type Queue struct {
	client *river.Client[pgx.Tx]
	logger *slog.Logger
}

type QueueConfig struct {
	DBPool *pgxpool.Pool
	Logger *slog.Logger
}

func NewQueue(cfg QueueConfig) (*Queue, error) {
	driver := riverpgxv5.New(cfg.DBPool)

	workers := river.NewWorkers()

	client, err := river.NewClient(driver, &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
		Workers: workers,
		Logger:  cfg.Logger,
	})
	if err != nil {
		return nil, err
	}

	return &Queue{
		client: client,
		logger: cfg.Logger,
	}, nil
}

func (q *Queue) Start(ctx context.Context) error {
	return q.client.Start(ctx)
}

func (q *Queue) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return q.client.Stop(ctx)
}

func (q *Queue) Client() *river.Client[pgx.Tx] {
	return q.client
}

type GenericArgs struct {
	JobKind JobKind         `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func (GenericArgs) Kind() string { return "generic" }

type EnqueueOpts struct {
	RunAt time.Time
}

func (q *Queue) Enqueue(ctx context.Context, kind JobKind, payload json.RawMessage, opts *EnqueueOpts) error {
	args := GenericArgs{
		JobKind: kind,
		Payload: payload,
	}

	insertOpts := &river.InsertOpts{}
	if opts != nil && !opts.RunAt.IsZero() {
		insertOpts.ScheduledAt = opts.RunAt
	}

	_, err := q.client.Insert(ctx, args, insertOpts)
	return err
}

func (q *Queue) EnqueueAt(ctx context.Context, kind JobKind, payload json.RawMessage, runAt time.Time) error {
	return q.Enqueue(ctx, kind, payload, &EnqueueOpts{RunAt: runAt})
}
