package provider

import (
	"context"
	"encoding/json"
)

type Provider interface {
	CreateEnvironment(ctx context.Context, spec EnvironmentSpec) (*EnvironmentResult, error)
	DestroyEnvironment(ctx context.Context, providerMetadata json.RawMessage) error
	GetVMStatus(ctx context.Context, providerVMID string) (string, error)
}
