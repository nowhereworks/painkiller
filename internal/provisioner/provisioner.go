package provisioner

import "context"

type Provisioner interface {
	Provision(ctx context.Context, spec EnvironmentProvisionSpec) (*ProvisionResult, error)
}
