package scenarios

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadScenario(t *testing.T, path string) *Scenario {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read scenario file: %v", err)
	}
	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		t.Fatalf("failed to parse scenario: %v", err)
	}
	return &s
}

func loadChecks(t *testing.T, path string) *ChecksFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read checks file: %v", err)
	}
	var cf ChecksFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		t.Fatalf("failed to parse checks: %v", err)
	}
	return &cf
}

func TestValidateExampleScenario(t *testing.T) {
	s := loadScenario(t, "../../testdata/scenarios/cka/simulator-001/scenario.yaml")
	if err := Validate(s); err != nil {
		t.Fatalf("expected valid scenario, got error: %v", err)
	}
}

func TestValidateExampleChecks(t *testing.T) {
	s := loadScenario(t, "../../testdata/scenarios/cka/simulator-001/scenario.yaml")
	cf := loadChecks(t, "../../testdata/scenarios/cka/simulator-001/checks/checks.yaml")
	if err := ValidateChecks(cf, s); err != nil {
		t.Fatalf("expected valid checks, got error: %v", err)
	}
}

func TestValidateRejectsDanglingTaskClusterRef(t *testing.T) {
	s := &Scenario{
		ID:                "test",
		Title:             "Test",
		DurationMinutes:   60,
		AccessWindowHours: 24,
		AttemptsAllowed:   1,
		Topology: Topology{
			Clusters: []Cluster{
				{
					ID:          "cluster-a",
					DisplayName: "cluster-a",
					KubeContext:  "cluster-a-admin",
					Nodes: []Node{
						{Name: "cp-1", Role: "control-plane", Template: "kubeadm-cp"},
					},
				},
			},
		},
		Tasks: []Task{
			{ID: "task-01", ClusterID: "nonexistent-cluster", Points: 5, PromptFile: "tasks/task-01.md"},
		},
	}

	err := Validate(s)
	if err == nil {
		t.Fatal("expected error for dangling task cluster reference")
	}
}

func TestValidateRejectsDanglingCheckTaskRef(t *testing.T) {
	s := &Scenario{
		ID:                "test",
		Title:             "Test",
		DurationMinutes:   60,
		AccessWindowHours: 24,
		AttemptsAllowed:   1,
		Topology: Topology{
			Clusters: []Cluster{
				{
					ID:          "cluster-a",
					DisplayName: "cluster-a",
					KubeContext:  "cluster-a-admin",
					Nodes: []Node{
						{Name: "cp-1", Role: "control-plane", Template: "kubeadm-cp"},
					},
				},
			},
		},
		Tasks: []Task{
			{ID: "task-01", ClusterID: "cluster-a", Points: 5, PromptFile: "tasks/task-01.md"},
		},
	}

	cf := &ChecksFile{
		Checks: []Check{
			{ID: "check-01", TaskID: "nonexistent-task", ClusterID: "cluster-a", Type: "kubectl", Command: "kubectl get pods", Points: 5},
		},
	}

	err := ValidateChecks(cf, s)
	if err == nil {
		t.Fatal("expected error for dangling check task reference")
	}
}

func TestValidateRejectsDuplicateClusterID(t *testing.T) {
	s := &Scenario{
		ID:                "test",
		Title:             "Test",
		DurationMinutes:   60,
		AccessWindowHours: 24,
		AttemptsAllowed:   1,
		Topology: Topology{
			Clusters: []Cluster{
				{
					ID: "cluster-a", DisplayName: "a", KubeContext: "a",
					Nodes: []Node{{Name: "cp-1", Role: "control-plane", Template: "t"}},
				},
				{
					ID: "cluster-a", DisplayName: "b", KubeContext: "b",
					Nodes: []Node{{Name: "cp-1", Role: "control-plane", Template: "t"}},
				},
			},
		},
		Tasks: []Task{
			{ID: "task-01", ClusterID: "cluster-a", Points: 5, PromptFile: "tasks/task-01.md"},
		},
	}

	err := Validate(s)
	if err == nil {
		t.Fatal("expected error for duplicate cluster id")
	}
}

func TestValidateRejectsDuplicateNodeName(t *testing.T) {
	s := &Scenario{
		ID:                "test",
		Title:             "Test",
		DurationMinutes:   60,
		AccessWindowHours: 24,
		AttemptsAllowed:   1,
		Topology: Topology{
			Clusters: []Cluster{
				{
					ID: "cluster-a", DisplayName: "a", KubeContext: "a",
					Nodes: []Node{
						{Name: "cp-1", Role: "control-plane", Template: "t"},
						{Name: "cp-1", Role: "worker", Template: "t"},
					},
				},
			},
		},
		Tasks: []Task{
			{ID: "task-01", ClusterID: "cluster-a", Points: 5, PromptFile: "tasks/task-01.md"},
		},
	}

	err := Validate(s)
	if err == nil {
		t.Fatal("expected error for duplicate node name")
	}
}

func TestValidateRejectsEmptyClusters(t *testing.T) {
	s := &Scenario{
		ID:                "test",
		Title:             "Test",
		DurationMinutes:   60,
		AccessWindowHours: 24,
		AttemptsAllowed:   1,
		Topology:          Topology{Clusters: []Cluster{}},
		Tasks: []Task{
			{ID: "task-01", ClusterID: "cluster-a", Points: 5, PromptFile: "tasks/task-01.md"},
		},
	}

	err := Validate(s)
	if err == nil {
		t.Fatal("expected error for empty clusters")
	}
}

func TestValidateRejectsEmptyTasks(t *testing.T) {
	s := &Scenario{
		ID:                "test",
		Title:             "Test",
		DurationMinutes:   60,
		AccessWindowHours: 24,
		AttemptsAllowed:   1,
		Topology: Topology{
			Clusters: []Cluster{
				{
					ID: "cluster-a", DisplayName: "a", KubeContext: "a",
					Nodes: []Node{{Name: "cp-1", Role: "control-plane", Template: "t"}},
				},
			},
		},
		Tasks: []Task{},
	}

	err := Validate(s)
	if err == nil {
		t.Fatal("expected error for empty tasks")
	}
}
