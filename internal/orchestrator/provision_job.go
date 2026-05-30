package orchestrator

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"painkiller-shell/internal/jobs"
	"painkiller-shell/internal/models"
	"painkiller-shell/internal/provider"
	"painkiller-shell/internal/provisioner"
	"painkiller-shell/internal/proxy"
)

type provisionPayload struct {
	AttemptID string `json:"attempt_id"`
}

func (o *Orchestrator) handleProvisionEnvironment(ctx context.Context, payload json.RawMessage) error {
	var p provisionPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	attemptID, err := uuid.Parse(p.AttemptID)
	if err != nil {
		return fmt.Errorf("invalid attempt_id: %w", err)
	}

	attempt, err := o.store.Attempts().GetByID(ctx, attemptID)
	if err != nil {
		return fmt.Errorf("attempt not found: %w", err)
	}
	if attempt.Status != models.AttemptStatusAttemptRequested && attempt.Status != models.AttemptStatusEnvironmentProvisioning {
		o.logger.Warn("ignoring stale provision job", "attempt_id", attemptID, "status", attempt.Status)
		return nil
	}

	if attempt.Status == models.AttemptStatusAttemptRequested {
		if err := o.attempts.TransitionAttempt(ctx, attemptID, models.AttemptStatusEnvironmentProvisioning); err != nil {
			return fmt.Errorf("failed to transition attempt: %w", err)
		}
	}

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate SSH key: %w", err)
	}

	sshPubKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return fmt.Errorf("failed to create SSH public key: %w", err)
	}
	sshPubKeyStr := string(ssh.MarshalAuthorizedKey(sshPubKey))

	privKeyBlock, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return fmt.Errorf("failed to marshal SSH private key: %w", err)
	}
	privKeyPEM := pem.EncodeToMemory(privKeyBlock)

	purchase, err := o.store.Purchases().GetByID(ctx, attempt.PurchasedTestID)
	if err != nil {
		return fmt.Errorf("purchase not found: %w", err)
	}

	test, err := o.store.Tests().GetByID(ctx, purchase.TestID)
	if err != nil {
		return fmt.Errorf("test not found: %w", err)
	}

	scenarioVersion, err := o.store.Scenarios().GetVersion(ctx, test.ScenarioVersionID)
	if err != nil {
		return fmt.Errorf("scenario version not found: %w", err)
	}

	var topology struct {
		Clusters []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			KubeContext string `json:"kube_context"`
			Nodes       []struct {
				Name     string `json:"name"`
				Role     string `json:"role"`
				Template string `json:"template"`
			} `json:"nodes"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal([]byte(scenarioVersion.TopologyJSON), &topology); err != nil {
		return fmt.Errorf("failed to unmarshal topology: %w", err)
	}

	envSpec := provider.EnvironmentSpec{
		Workstation: provider.VMRequest{
			Hostname:     fmt.Sprintf("workstation-%s", attemptID.String()[:8]),
			Role:         "workstation",
			Template:     "workstation",
			SSHPublicKey: sshPubKeyStr,
			Tags: map[string]string{
				"attempt_id":     attemptID.String(),
				"environment_id": uuid.New().String(),
			},
		},
		Clusters: make([]provider.ClusterRequest, 0, len(topology.Clusters)),
	}

	for _, cluster := range topology.Clusters {
		cr := provider.ClusterRequest{
			Name:  cluster.ID,
			Nodes: make([]provider.VMRequest, 0, len(cluster.Nodes)),
		}
		for _, node := range cluster.Nodes {
			cr.Nodes = append(cr.Nodes, provider.VMRequest{
				Hostname:     fmt.Sprintf("%s-%s", cluster.ID, node.Name),
				Role:         node.Role,
				Template:     node.Template,
				SSHPublicKey: sshPubKeyStr,
				Tags: map[string]string{
					"attempt_id": attemptID.String(),
					"cluster_id": cluster.ID,
				},
			})
		}
		envSpec.Clusters = append(envSpec.Clusters, cr)
	}

	envResult, err := o.provider.CreateEnvironment(ctx, envSpec)
	if err != nil {
		o.handleProvisionFailure(ctx, attemptID)
		o.logger.Error("failed to create environment", "attempt_id", attemptID, "error", err)
		return nil
	}

	metadata := map[string]interface{}{
		"workstation_vm_id": envResult.Workstation.ProviderVMID,
		"node_vm_ids":       []string{},
	}
	for _, cluster := range envResult.Clusters {
		for _, node := range cluster.Nodes {
			metadata["node_vm_ids"] = append(metadata["node_vm_ids"].([]string), node.ProviderVMID)
		}
	}
	metadataJSON, _ := json.Marshal(metadata)

	env := &models.Environment{
		ID:               uuid.New(),
		AttemptID:        attemptID,
		Status:           models.EnvironmentStatusReady,
		WorkstationIP:    &envResult.Workstation.IPAddress,
		ProviderMetadata: metadataJSON,
		SSHPrivateKey:    privKeyPEM,
		CreatedAt:        time.Now(),
	}

	if err := o.store.Environments().Create(ctx, env); err != nil {
		o.handleProvisionFailure(ctx, attemptID)
		o.logger.Error("failed to create environment record", "attempt_id", attemptID, "error", err)
		return nil
	}

	provisionSpec := provisioner.EnvironmentProvisionSpec{
		WorkstationIP: envResult.Workstation.IPAddress,
		SSHPrivateKey: privKeyPEM,
		PlaybookPath:  fmt.Sprintf("/app/scenarios/%s/provision/playbook.yaml", scenarioVersion.ExternalID),
		Clusters:      make([]provisioner.ClusterSpec, 0, len(envResult.Clusters)),
	}

	if o.proxyConfig != nil && o.proxyConfig.Addr != "" {
		clusterCIDRs := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
		provisionSpec.ProxyAddr = o.proxyConfig.Addr
		provisionSpec.ProxyIPTScript = proxy.GenerateIPTablesScript(*o.proxyConfig, clusterCIDRs)
	}

	for i, cluster := range envResult.Clusters {
		cs := provisioner.ClusterSpec{
			Name:        cluster.Name,
			KubeContext: topology.Clusters[i].KubeContext,
			Nodes:       make([]provisioner.NodeSpec, 0, len(cluster.Nodes)),
		}
		for _, node := range cluster.Nodes {
			cs.Nodes = append(cs.Nodes, provisioner.NodeSpec{
				Hostname: node.Hostname,
				IP:       node.IPAddress,
			})
		}
		provisionSpec.Clusters = append(provisionSpec.Clusters, cs)
	}

	if _, err := o.provisioner.Provision(ctx, provisionSpec); err != nil {
		o.handleProvisionFailure(ctx, attemptID)
		o.logger.Error("failed to provision environment", "attempt_id", attemptID, "error", err)
		return nil
	}

	terminalToken := uuid.New().String()
	session := &models.Session{
		ID:            uuid.New(),
		AttemptID:     attemptID,
		EnvironmentID: env.ID,
		TerminalToken: terminalToken,
		CreatedAt:     time.Now(),
	}

	if err := o.store.Sessions().Create(ctx, session); err != nil {
		o.handleProvisionFailure(ctx, attemptID)
		o.logger.Error("failed to create session", "attempt_id", attemptID, "error", err)
		return nil
	}

	if err := o.attempts.TransitionAttempt(ctx, attemptID, models.AttemptStatusEnvironmentReady); err != nil {
		return fmt.Errorf("failed to transition attempt to ready: %w", err)
	}

	o.logger.Info("environment provisioned", "attempt_id", attemptID, "workstation_ip", envResult.Workstation.IPAddress)
	return nil
}

func (o *Orchestrator) enqueueCleanup(ctx context.Context, attemptID uuid.UUID) error {
	payload, _ := json.Marshal(map[string]string{"attempt_id": attemptID.String()})
	return o.queue.Enqueue(ctx, jobs.JobKindCleanupEnvironment, payload, nil)
}

func (o *Orchestrator) handleProvisionFailure(ctx context.Context, attemptID uuid.UUID) {
	if err := o.attempts.TransitionAttempt(ctx, attemptID, models.AttemptStatusProvisionFailed); err != nil {
		o.logger.Error("failed to mark provisioning failure", "attempt_id", attemptID, "error", err)
		return
	}
	_ = o.enqueueCleanup(ctx, attemptID)
	if err := o.attempts.RestoreAttemptCount(ctx, attemptID); err != nil {
		o.logger.Error("failed to restore attempt count after provision failure", "attempt_id", attemptID, "error", err)
	}
}

func (o *Orchestrator) handleCleanupEnvironment(ctx context.Context, payload json.RawMessage) error {
	var p provisionPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	attemptID, err := uuid.Parse(p.AttemptID)
	if err != nil {
		return fmt.Errorf("invalid attempt_id: %w", err)
	}

	env, err := o.store.Environments().GetByAttemptID(ctx, attemptID)
	if err != nil {
		o.logger.Warn("environment not found for cleanup", "attempt_id", attemptID)
		return nil
	}

	if err := o.store.Environments().UpdateStatus(ctx, env.ID, models.EnvironmentStatusDestroying); err != nil {
		return fmt.Errorf("failed to update environment status: %w", err)
	}

	if err := o.provider.DestroyEnvironment(ctx, env.ProviderMetadata); err != nil {
		_ = o.store.Environments().UpdateStatus(ctx, env.ID, models.EnvironmentStatusFailed)
		return fmt.Errorf("failed to destroy environment: %w", err)
	}

	if err := o.store.Environments().UpdateStatus(ctx, env.ID, models.EnvironmentStatusDestroyed); err != nil {
		return fmt.Errorf("failed to update environment status: %w", err)
	}

	_ = o.attempts.TransitionAttempt(ctx, attemptID, models.AttemptStatusDestroyed)

	o.logger.Info("environment cleaned up", "attempt_id", attemptID)
	return nil
}
