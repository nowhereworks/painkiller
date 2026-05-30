package mock

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"painkiller-shell/internal/provider"
)

func TestCreateEnvironment(t *testing.T) {
	p := New(0)

	spec := provider.EnvironmentSpec{
		Workstation: provider.VMRequest{
			Hostname: "ws-1",
			Role:     "workstation",
			Template: "workstation",
		},
		Clusters: []provider.ClusterRequest{
			{
				Name: "cluster-a",
				Nodes: []provider.VMRequest{
					{Hostname: "cp-1", Role: "control-plane", Template: "cp"},
					{Hostname: "worker-1", Role: "worker", Template: "worker"},
				},
			},
		},
	}

	result, err := p.CreateEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("CreateEnvironment failed: %v", err)
	}

	if result.Workstation.Hostname != "ws-1" {
		t.Errorf("expected workstation hostname 'ws-1', got %q", result.Workstation.Hostname)
	}
	if result.Workstation.IPAddress == "" {
		t.Error("expected workstation to have an IP address")
	}
	if result.Workstation.ProviderVMID == "" {
		t.Error("expected workstation to have a provider VM ID")
	}

	if len(result.Clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(result.Clusters))
	}
	if result.Clusters[0].Name != "cluster-a" {
		t.Errorf("expected cluster name 'cluster-a', got %q", result.Clusters[0].Name)
	}
	if len(result.Clusters[0].Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Clusters[0].Nodes))
	}

	for i, node := range result.Clusters[0].Nodes {
		if node.IPAddress == "" {
			t.Errorf("node %d: expected IP address", i)
		}
		if node.ProviderVMID == "" {
			t.Errorf("node %d: expected provider VM ID", i)
		}
	}
}

func TestCreateEnvironmentIPsAreUnique(t *testing.T) {
	p := New(0)

	spec := provider.EnvironmentSpec{
		Workstation: provider.VMRequest{Hostname: "ws", Template: "ws"},
		Clusters: []provider.ClusterRequest{
			{
				Name: "c1",
				Nodes: []provider.VMRequest{
					{Hostname: "n1", Template: "t"},
					{Hostname: "n2", Template: "t"},
				},
			},
		},
	}

	result, err := p.CreateEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("CreateEnvironment failed: %v", err)
	}

	seen := map[string]bool{result.Workstation.IPAddress: true}
	for _, cluster := range result.Clusters {
		for _, node := range cluster.Nodes {
			if seen[node.IPAddress] {
				t.Errorf("duplicate IP address: %s", node.IPAddress)
			}
			seen[node.IPAddress] = true
		}
	}
}

func TestDestroyEnvironment(t *testing.T) {
	p := New(0)

	spec := provider.EnvironmentSpec{
		Workstation: provider.VMRequest{Hostname: "ws", Template: "ws"},
		Clusters:    []provider.ClusterRequest{},
	}

	result, err := p.CreateEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("CreateEnvironment failed: %v", err)
	}

	metadata, _ := json.Marshal(map[string]string{
		"workstation_vm_id": result.Workstation.ProviderVMID,
	})

	if err := p.DestroyEnvironment(context.Background(), metadata); err != nil {
		t.Fatalf("DestroyEnvironment failed: %v", err)
	}

	status, err := p.GetVMStatus(context.Background(), result.Workstation.ProviderVMID)
	if err != nil {
		t.Fatalf("GetVMStatus failed: %v", err)
	}
	if status != "destroyed" {
		t.Errorf("expected 'destroyed' status, got %q", status)
	}
}

func TestGetVMStatusRunning(t *testing.T) {
	p := New(0)

	spec := provider.EnvironmentSpec{
		Workstation: provider.VMRequest{Hostname: "ws", Template: "ws"},
		Clusters:    []provider.ClusterRequest{},
	}

	result, err := p.CreateEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("CreateEnvironment failed: %v", err)
	}

	status, err := p.GetVMStatus(context.Background(), result.Workstation.ProviderVMID)
	if err != nil {
		t.Fatalf("GetVMStatus failed: %v", err)
	}
	if status != "running" {
		t.Errorf("expected 'running', got %q", status)
	}
}

func TestGetVMStatusUnknown(t *testing.T) {
	p := New(0)

	status, err := p.GetVMStatus(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetVMStatus failed: %v", err)
	}
	if status != "destroyed" {
		t.Errorf("expected 'destroyed' for unknown VM, got %q", status)
	}
}

func TestCreateEnvironmentWithDelay(t *testing.T) {
	p := New(10 * time.Millisecond)

	spec := provider.EnvironmentSpec{
		Workstation: provider.VMRequest{Hostname: "ws", Template: "ws"},
		Clusters:    []provider.ClusterRequest{},
	}

	start := time.Now()
	_, err := p.CreateEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("CreateEnvironment failed: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < 10*time.Millisecond {
		t.Errorf("expected at least 10ms delay, got %v", elapsed)
	}
}
