package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"painkiller-shell/internal/provider"
)

const (
	ipPollInterval = 5 * time.Second
	ipPollTimeout  = 2 * time.Minute
)

type ProxmoxProvider struct {
	client *Client
	config Config
}

func New(config Config) *ProxmoxProvider {
	return &ProxmoxProvider{
		client: NewClient(config),
		config: config,
	}
}

func (p *ProxmoxProvider) waitForIP(ctx context.Context, vmID int) (string, error) {
	deadline := time.Now().Add(ipPollTimeout)
	for time.Now().Before(deadline) {
		ip, err := p.client.GetVMIPAddress(ctx, vmID)
		if err == nil && ip != "" {
			return ip, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(ipPollInterval):
		}
	}
	return "", fmt.Errorf("timed out waiting for IP on VM %d", vmID)
}

func (p *ProxmoxProvider) cloudInitConfig(hostname, sshPublicKey string) map[string]string {
	cfg := map[string]string{
		"citype":    "nocloud",
		"ipconfig0": fmt.Sprintf("ip=dhcp,bridge=%s", p.config.NetworkBridge),
	}
	if sshPublicKey != "" {
		cfg["cipublickey"] = strings.TrimSpace(sshPublicKey)
	}
	if hostname != "" {
		cfg["ciname"] = hostname
	}
	return cfg
}

func (p *ProxmoxProvider) CreateEnvironment(ctx context.Context, spec provider.EnvironmentSpec) (*provider.EnvironmentResult, error) {
	result := &provider.EnvironmentResult{
		Clusters: make([]provider.ClusterResult, 0, len(spec.Clusters)),
	}

	templateID, ok := p.config.Templates[spec.Workstation.Template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", spec.Workstation.Template)
	}

	wsVMID, err := p.client.CloneVM(ctx, templateID, spec.Workstation.Hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to clone workstation: %w", err)
	}

	ciConfig := p.cloudInitConfig(spec.Workstation.Hostname, spec.Workstation.SSHPublicKey)
	if err := p.client.ConfigureVM(ctx, wsVMID, ciConfig); err != nil {
		return nil, fmt.Errorf("failed to configure workstation: %w", err)
	}

	if err := p.client.StartVM(ctx, wsVMID); err != nil {
		return nil, fmt.Errorf("failed to start workstation: %w", err)
	}

	wsIP, err := p.waitForIP(ctx, wsVMID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workstation IP: %w", err)
	}

	result.Workstation = provider.VMResult{
		ProviderVMID: fmt.Sprintf("%d", wsVMID),
		IPAddress:    wsIP,
		Hostname:     spec.Workstation.Hostname,
	}

	for _, cluster := range spec.Clusters {
		cr := provider.ClusterResult{
			Name:  cluster.Name,
			Nodes: make([]provider.VMResult, 0, len(cluster.Nodes)),
		}

		for _, node := range cluster.Nodes {
			nodeTemplateID, ok := p.config.Templates[node.Template]
			if !ok {
				return nil, fmt.Errorf("unknown template: %s", node.Template)
			}

			nodeVMID, err := p.client.CloneVM(ctx, nodeTemplateID, node.Hostname)
			if err != nil {
				return nil, fmt.Errorf("failed to clone node %s: %w", node.Hostname, err)
			}

			nodeCI := p.cloudInitConfig(node.Hostname, node.SSHPublicKey)
			if err := p.client.ConfigureVM(ctx, nodeVMID, nodeCI); err != nil {
				return nil, fmt.Errorf("failed to configure node %s: %w", node.Hostname, err)
			}

			if err := p.client.StartVM(ctx, nodeVMID); err != nil {
				return nil, fmt.Errorf("failed to start node %s: %w", node.Hostname, err)
			}

			nodeIP, err := p.waitForIP(ctx, nodeVMID)
			if err != nil {
				return nil, fmt.Errorf("failed to get IP for node %s: %w", node.Hostname, err)
			}

			cr.Nodes = append(cr.Nodes, provider.VMResult{
				ProviderVMID: fmt.Sprintf("%d", nodeVMID),
				IPAddress:    nodeIP,
				Hostname:     node.Hostname,
			})
		}

		result.Clusters = append(result.Clusters, cr)
	}

	return result, nil
}

func (p *ProxmoxProvider) DestroyEnvironment(ctx context.Context, providerMetadata json.RawMessage) error {
	var metadata struct {
		WorkstationVMID string   `json:"workstation_vm_id"`
		NodeVMIDs       []string `json:"node_vm_ids"`
	}
	if err := json.Unmarshal(providerMetadata, &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	for _, vmIDStr := range metadata.NodeVMIDs {
		vmID := 0
		fmt.Sscanf(vmIDStr, "%d", &vmID)
		if vmID > 0 {
			_ = p.client.StopVM(ctx, vmID)
			_ = p.client.DeleteVM(ctx, vmID)
		}
	}

	if metadata.WorkstationVMID != "" {
		vmID := 0
		fmt.Sscanf(metadata.WorkstationVMID, "%d", &vmID)
		if vmID > 0 {
			_ = p.client.StopVM(ctx, vmID)
			_ = p.client.DeleteVM(ctx, vmID)
		}
	}

	return nil
}

func (p *ProxmoxProvider) GetVMStatus(ctx context.Context, providerVMID string) (string, error) {
	vmID := 0
	fmt.Sscanf(providerVMID, "%d", &vmID)
	if vmID == 0 {
		return "", fmt.Errorf("invalid VM ID: %s", providerVMID)
	}
	return p.client.GetVMStatus(ctx, vmID)
}

var _ provider.Provider = (*ProxmoxProvider)(nil)
