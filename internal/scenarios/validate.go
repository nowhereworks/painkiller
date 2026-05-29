package scenarios

import (
	"fmt"
	"strings"
)

func Validate(s *Scenario) error {
	if s.ID == "" {
		return fmt.Errorf("scenario id is required")
	}
	if s.Title == "" {
		return fmt.Errorf("scenario title is required")
	}
	if s.DurationMinutes <= 0 {
		return fmt.Errorf("duration_minutes must be positive")
	}
	if s.AccessWindowHours <= 0 {
		return fmt.Errorf("access_window_hours must be positive")
	}
	if s.AttemptsAllowed <= 0 {
		return fmt.Errorf("attempts_allowed must be positive")
	}

	if len(s.Topology.Clusters) == 0 {
		return fmt.Errorf("at least one cluster is required")
	}

	clusterIDs := make(map[string]bool)
	for _, c := range s.Topology.Clusters {
		if c.ID == "" {
			return fmt.Errorf("cluster id is required")
		}
		if clusterIDs[c.ID] {
			return fmt.Errorf("duplicate cluster id: %s", c.ID)
		}
		clusterIDs[c.ID] = true

		if len(c.Nodes) == 0 {
			return fmt.Errorf("cluster %s must have at least one node", c.ID)
		}

		nodeNames := make(map[string]bool)
		for _, n := range c.Nodes {
			if n.Name == "" {
				return fmt.Errorf("node name is required in cluster %s", c.ID)
			}
			if nodeNames[n.Name] {
				return fmt.Errorf("duplicate node name %s in cluster %s", n.Name, c.ID)
			}
			nodeNames[n.Name] = true

			role := strings.ToLower(n.Role)
			if role != "control-plane" && role != "worker" {
				return fmt.Errorf("node %s in cluster %s has invalid role: %s", n.Name, c.ID, n.Role)
			}
			if n.Template == "" {
				return fmt.Errorf("node %s in cluster %s requires a template", n.Name, c.ID)
			}
		}
	}

	if len(s.Tasks) == 0 {
		return fmt.Errorf("at least one task is required")
	}

	taskIDs := make(map[string]bool)
	for _, t := range s.Tasks {
		if t.ID == "" {
			return fmt.Errorf("task id is required")
		}
		if taskIDs[t.ID] {
			return fmt.Errorf("duplicate task id: %s", t.ID)
		}
		taskIDs[t.ID] = true

		if !clusterIDs[t.ClusterID] {
			return fmt.Errorf("task %s references unknown cluster: %s", t.ID, t.ClusterID)
		}
		if t.Points <= 0 {
			return fmt.Errorf("task %s points must be positive", t.ID)
		}
		if t.PromptFile == "" {
			return fmt.Errorf("task %s requires a prompt_file", t.ID)
		}
	}

	return nil
}

func ValidateChecks(cf *ChecksFile, s *Scenario) error {
	if len(cf.Checks) == 0 {
		return fmt.Errorf("at least one check is required")
	}

	taskIDs := make(map[string]bool)
	for _, t := range s.Tasks {
		taskIDs[t.ID] = true
	}

	clusterIDs := make(map[string]bool)
	for _, c := range s.Topology.Clusters {
		clusterIDs[c.ID] = true
	}

	checkIDs := make(map[string]bool)
	for _, ch := range cf.Checks {
		if ch.ID == "" {
			return fmt.Errorf("check id is required")
		}
		if checkIDs[ch.ID] {
			return fmt.Errorf("duplicate check id: %s", ch.ID)
		}
		checkIDs[ch.ID] = true

		if !taskIDs[ch.TaskID] {
			return fmt.Errorf("check %s references unknown task: %s", ch.ID, ch.TaskID)
		}
		if !clusterIDs[ch.ClusterID] {
			return fmt.Errorf("check %s references unknown cluster: %s", ch.ID, ch.ClusterID)
		}
		if ch.Type == "" {
			return fmt.Errorf("check %s requires a type", ch.ID)
		}
		if ch.Command == "" {
			return fmt.Errorf("check %s requires a command", ch.ID)
		}
		if ch.Points <= 0 {
			return fmt.Errorf("check %s points must be positive", ch.ID)
		}
	}

	return nil
}
