package ansible

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"painkiller-shell/internal/provisioner"
)

type AnsibleProvisioner struct {
	logger *slog.Logger
}

func New(logger *slog.Logger) *AnsibleProvisioner {
	return &AnsibleProvisioner{logger: logger}
}

func (a *AnsibleProvisioner) Provision(ctx context.Context, spec provisioner.EnvironmentProvisionSpec) (*provisioner.ProvisionResult, error) {
	tmpDir, err := os.MkdirTemp("", "painkiller-ansible-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	keyPath := filepath.Join(tmpDir, "ssh-key")
	if err := os.WriteFile(keyPath, spec.SSHPrivateKey, 0600); err != nil {
		return nil, fmt.Errorf("failed to write SSH key: %w", err)
	}

	inventory := GenerateInventory(spec)
	inventoryPath := filepath.Join(tmpDir, "inventory.ini")
	if err := os.WriteFile(inventoryPath, []byte(inventory), 0644); err != nil {
		return nil, fmt.Errorf("failed to write inventory: %w", err)
	}

	vars, err := GenerateVars(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to generate vars: %w", err)
	}
	varsPath := filepath.Join(tmpDir, "vars.yaml")
	if err := os.WriteFile(varsPath, []byte(vars), 0644); err != nil {
		return nil, fmt.Errorf("failed to write vars: %w", err)
	}

	cmd := exec.CommandContext(ctx, "ansible-playbook",
		"-i", inventoryPath,
		"-e", "@"+varsPath,
		"--private-key", keyPath,
		spec.PlaybookPath,
	)

	cmd.Env = append(os.Environ(), "ANSIBLE_ROLES_PATH=/app/ansible/roles")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	a.logger.Info("running ansible-playbook", "playbook", spec.PlaybookPath)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ansible-playbook failed: %w", err)
	}

	return &provisioner.ProvisionResult{Ready: true}, nil
}

var _ provisioner.Provisioner = (*AnsibleProvisioner)(nil)
