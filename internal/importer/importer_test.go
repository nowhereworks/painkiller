package importer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverScenarios(t *testing.T) {
	repoPath := filepath.Join("..", "..", "testdata", "scenarios")

	discovered, err := discoverScenarios(repoPath)
	if err != nil {
		t.Fatalf("discoverScenarios failed: %v", err)
	}

	if len(discovered) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(discovered))
	}

	ps := discovered[0]

	if ps.Scenario.ID != "cka-simulator-001" {
		t.Errorf("expected scenario id 'cka-simulator-001', got %q", ps.Scenario.ID)
	}
	if ps.Scenario.Title != "CKA Simulator 1" {
		t.Errorf("expected title 'CKA Simulator 1', got %q", ps.Scenario.Title)
	}
	if ps.Scenario.DurationMinutes != 120 {
		t.Errorf("expected duration 120, got %d", ps.Scenario.DurationMinutes)
	}
	if len(ps.Scenario.Topology.Clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(ps.Scenario.Topology.Clusters))
	}
	if len(ps.Scenario.Tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(ps.Scenario.Tasks))
	}

	if len(ps.Checks.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(ps.Checks.Checks))
	}
	if ps.Checks.Checks[0].ID != "task-01-check" {
		t.Errorf("expected check id 'task-01-check', got %q", ps.Checks.Checks[0].ID)
	}

	if _, ok := ps.Prompts["task-01"]; !ok {
		t.Error("expected prompt for task-01")
	}
	if _, ok := ps.Prompts["task-02"]; !ok {
		t.Error("expected prompt for task-02")
	}
}

func TestDiscoverScenariosEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	discovered, err := discoverScenarios(tmpDir)
	if err != nil {
		t.Fatalf("discoverScenarios failed: %v", err)
	}
	if len(discovered) != 0 {
		t.Errorf("expected 0 scenarios, got %d", len(discovered))
	}
}

func TestDiscoverScenariosInvalidPath(t *testing.T) {
	_, err := discoverScenarios("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestParseScenarioInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioFile := filepath.Join(tmpDir, "scenario.yaml")
	if err := os.WriteFile(scenarioFile, []byte("invalid: [yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseScenario(scenarioFile)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestParseScenarioMissingChecks(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioFile := filepath.Join(tmpDir, "scenario.yaml")
	scenarioYAML := `id: test
title: Test
duration_minutes: 60
access_window_hours: 24
attempts_allowed: 1
topology:
  clusters:
    - id: cluster-a
      display_name: a
      kube_context: a
      nodes:
        - name: cp-1
          role: control-plane
          template: t
tasks:
  - id: task-01
    cluster_id: cluster-a
    kube_context: a
    points: 5
    prompt_file: tasks/task-01.md
`
	if err := os.WriteFile(scenarioFile, []byte(scenarioYAML), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := parseScenario(scenarioFile)
	if err == nil {
		t.Error("expected error for missing checks.yaml")
	}
}

func TestResolveGitCommitFallback(t *testing.T) {
	tmpDir := t.TempDir()
	commit := resolveGitCommit(tmpDir)
	if commit != "local" {
		t.Errorf("expected 'local' fallback, got %q", commit)
	}
}
