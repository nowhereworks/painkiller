package noop

import (
	"context"
	"log/slog"

	"painkiller-shell/internal/provisioner"
)

type NoopProvisioner struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *NoopProvisioner {
	return &NoopProvisioner{logger: logger}
}

func (n *NoopProvisioner) Provision(ctx context.Context, spec provisioner.EnvironmentProvisionSpec) (*provisioner.ProvisionResult, error) {
	n.logger.Info("skipping provisioning (noop mode)",
		"workstation_ip", spec.WorkstationIP,
		"clusters", len(spec.Clusters),
	)
	return &provisioner.ProvisionResult{Ready: true}, nil
}

var _ provisioner.Provisioner = (*NoopProvisioner)(nil)
