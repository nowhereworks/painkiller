package orchestrator

import (
	"log/slog"

	"painkiller-shell/internal/attempts"
	"painkiller-shell/internal/jobs"
	"painkiller-shell/internal/provider"
	"painkiller-shell/internal/provisioner"
	"painkiller-shell/internal/store"
)

type Orchestrator struct {
	provider    provider.Provider
	provisioner provisioner.Provisioner
	store       *store.Store
	queue       *jobs.Queue
	attempts    *attempts.Service
	worker      *jobs.GenericWorker
	logger      *slog.Logger
}

type OrchestratorConfig struct {
	Provider    provider.Provider
	Provisioner provisioner.Provisioner
	Store       *store.Store
	Queue       *jobs.Queue
	Attempts    *attempts.Service
	Logger      *slog.Logger
}

func New(cfg OrchestratorConfig) *Orchestrator {
	worker := jobs.NewGenericWorker(cfg.Logger)

	o := &Orchestrator{
		provider:    cfg.Provider,
		provisioner: cfg.Provisioner,
		store:       cfg.Store,
		queue:       cfg.Queue,
		attempts:    cfg.Attempts,
		worker:      worker,
		logger:      cfg.Logger,
	}

	o.RegisterJobs()

	return o
}

func (o *Orchestrator) RegisterJobs() {
	o.worker.RegisterHandler(jobs.JobKindProvisionEnvironment, o.handleProvisionEnvironment)
	o.worker.RegisterHandler(jobs.JobKindCleanupEnvironment, o.handleCleanupEnvironment)
}

func (o *Orchestrator) Worker() *jobs.GenericWorker {
	return o.worker
}
