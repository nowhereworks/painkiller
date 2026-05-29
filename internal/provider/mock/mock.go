package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"painkiller-shell/internal/provider"
)

type MockProvider struct {
	mu           sync.RWMutex
	environments map[string]*provider.EnvironmentResult
	delay        time.Duration
	nextIP       int
}

func New(delay time.Duration) *MockProvider {
	return &MockProvider{
		environments: make(map[string]*provider.EnvironmentResult),
		delay:        delay,
		nextIP:       1,
	}
}

func (m *MockProvider) CreateEnvironment(ctx context.Context, spec provider.EnvironmentSpec) (*provider.EnvironmentResult, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	result := &provider.EnvironmentResult{
		Workstation: provider.VMResult{
			ProviderVMID: fmt.Sprintf("mock-vm-%d", m.nextIP),
			IPAddress:    fmt.Sprintf("127.0.0.%d", m.nextIP),
			Hostname:     spec.Workstation.Hostname,
		},
		Clusters: make([]provider.ClusterResult, 0, len(spec.Clusters)),
	}
	m.nextIP++

	for _, cluster := range spec.Clusters {
		cr := provider.ClusterResult{
			Name:  cluster.Name,
			Nodes: make([]provider.VMResult, 0, len(cluster.Nodes)),
		}
		for _, node := range cluster.Nodes {
			cr.Nodes = append(cr.Nodes, provider.VMResult{
				ProviderVMID: fmt.Sprintf("mock-vm-%d", m.nextIP),
				IPAddress:    fmt.Sprintf("127.0.0.%d", m.nextIP),
				Hostname:     node.Hostname,
			})
			m.nextIP++
		}
		result.Clusters = append(result.Clusters, cr)
	}

	m.environments[result.Workstation.ProviderVMID] = result
	return result, nil
}

func (m *MockProvider) DestroyEnvironment(ctx context.Context, providerMetadata json.RawMessage) error {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var metadata struct {
		WorkstationVMID string `json:"workstation_vm_id"`
	}
	if err := json.Unmarshal(providerMetadata, &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	delete(m.environments, metadata.WorkstationVMID)
	return nil
}

func (m *MockProvider) GetVMStatus(ctx context.Context, providerVMID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.environments[providerVMID]; ok {
		return "running", nil
	}
	return "destroyed", nil
}

func (m *MockProvider) GetEnvironment(vmID string) (*provider.EnvironmentResult, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	env, ok := m.environments[vmID]
	return env, ok
}

var _ provider.Provider = (*MockProvider)(nil)
