package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"painkiller-shell/internal/models"
	"painkiller-shell/internal/scenarios"
	"painkiller-shell/internal/store"
)

type parsedScenario struct {
	Scenario  *scenarios.Scenario
	Checks    *scenarios.ChecksFile
	Prompts   map[string]string
	Dir       string
}

func ImportAll(ctx context.Context, st *store.Store, repoPath string, logger *slog.Logger) error {
	if repoPath == "" {
		return fmt.Errorf("scenario repo path is empty")
	}

	gitCommit := resolveGitCommit(repoPath)

	discovered, err := discoverScenarios(repoPath)
	if err != nil {
		return err
	}

	if len(discovered) == 0 {
		logger.Info("no scenarios found", "path", repoPath)
		return nil
	}

	for _, ps := range discovered {
		if err := importParsedScenario(ctx, st, ps, gitCommit, logger); err != nil {
			logger.Error("failed to import scenario", "id", ps.Scenario.ID, "error", err)
			continue
		}
	}

	return nil
}

func discoverScenarios(repoPath string) ([]*parsedScenario, error) {
	var scenarioFiles []string
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == "scenario.yaml" {
			scenarioFiles = append(scenarioFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking repo path: %w", err)
	}

	var result []*parsedScenario
	for _, sf := range scenarioFiles {
		ps, err := parseScenario(sf)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", sf, err)
		}
		result = append(result, ps)
	}
	return result, nil
}

func parseScenario(scenarioFile string) (*parsedScenario, error) {
	scenarioDir := filepath.Dir(scenarioFile)

	data, err := os.ReadFile(scenarioFile)
	if err != nil {
		return nil, fmt.Errorf("reading scenario.yaml: %w", err)
	}

	var s scenarios.Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing scenario.yaml: %w", err)
	}

	if err := scenarios.Validate(&s); err != nil {
		return nil, fmt.Errorf("validating scenario %s: %w", s.ID, err)
	}

	checksFile := filepath.Join(scenarioDir, "checks", "checks.yaml")
	var cf scenarios.ChecksFile
	checksData, err := os.ReadFile(checksFile)
	if err != nil {
		return nil, fmt.Errorf("reading checks.yaml: %w", err)
	}
	if err := yaml.Unmarshal(checksData, &cf); err != nil {
		return nil, fmt.Errorf("parsing checks.yaml: %w", err)
	}
	if err := scenarios.ValidateChecks(&cf, &s); err != nil {
		return nil, fmt.Errorf("validating checks for %s: %w", s.ID, err)
	}

	prompts := make(map[string]string)
	for _, t := range s.Tasks {
		promptPath := filepath.Join(scenarioDir, t.PromptFile)
		content, err := os.ReadFile(promptPath)
		if err != nil {
			return nil, fmt.Errorf("reading prompt file %s: %w", t.PromptFile, err)
		}
		prompts[t.ID] = string(content)
	}

	return &parsedScenario{
		Scenario: &s,
		Checks:   &cf,
		Prompts:  prompts,
		Dir:      scenarioDir,
	}, nil
}

func importParsedScenario(ctx context.Context, st *store.Store, ps *parsedScenario, gitCommit string, logger *slog.Logger) error {
	s := ps.Scenario
	cf := ps.Checks
	prompts := ps.Prompts

	existing, err := st.Scenarios().GetVersionByExternalID(ctx, s.ID, gitCommit)
	if err == nil && existing != nil {
		logger.Info("scenario version already exists, skipping", "id", s.ID, "git_commit", gitCommit)
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("checking existing scenario version: %w", err)
	}

	topologyJSON, err := json.Marshal(s.Topology)
	if err != nil {
		return fmt.Errorf("serializing topology: %w", err)
	}

	now := time.Now()
	svID := uuid.New()
	sv := &models.ScenarioVersion{
		ID:                svID,
		ExternalID:        s.ID,
		Title:             s.Title,
		GitCommit:         gitCommit,
		DurationMinutes:   s.DurationMinutes,
		AccessWindowHours: s.AccessWindowHours,
		AttemptsAllowed:   s.AttemptsAllowed,
		TopologyJSON:      string(topologyJSON),
		CreatedAt:         now,
	}
	if err := st.Scenarios().CreateVersion(ctx, sv); err != nil {
		return fmt.Errorf("creating scenario version: %w", err)
	}

	taskUUIDs := make(map[string]uuid.UUID)
	for _, t := range s.Tasks {
		tID := uuid.New()
		taskUUIDs[t.ID] = tID
		task := &models.Task{
			ID:                tID,
			ScenarioVersionID: svID,
			ExternalID:        t.ID,
			ClusterID:         t.ClusterID,
			KubeContext:       t.KubeContext,
			Points:            t.Points,
			Prompt:            prompts[t.ID],
			CreatedAt:         now,
		}
		if err := st.Scenarios().CreateTask(ctx, task); err != nil {
			return fmt.Errorf("creating task %s: %w", t.ID, err)
		}
	}

	for _, c := range cf.Checks {
		taskUUID, ok := taskUUIDs[c.TaskID]
		if !ok {
			return fmt.Errorf("check %s references unknown task %s", c.ID, c.TaskID)
		}
		check := &models.Check{
			ID:         uuid.New(),
			TaskID:     taskUUID,
			ExternalID: c.ID,
			ClusterID:  c.ClusterID,
			Type:       models.CheckType(c.Type),
			Command:    c.Command,
			Points:     c.Points,
			CreatedAt:  now,
		}
		if err := st.Scenarios().CreateCheck(ctx, check); err != nil {
			return fmt.Errorf("creating check %s: %w", c.ID, err)
		}
	}

	product, err := st.Products().GetByTitle(ctx, s.Title)
	if err == sql.ErrNoRows || product == nil {
		product = &models.Product{
			ID:          uuid.New(),
			Title:       s.Title,
			Description: s.Title,
			CreatedAt:   now,
		}
		if err := st.Products().Create(ctx, product); err != nil {
			return fmt.Errorf("creating product: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("looking up product: %w", err)
	}

	test := &models.Test{
		ID:                uuid.New(),
		ProductID:         product.ID,
		ScenarioVersionID: svID,
		DurationMinutes:   s.DurationMinutes,
		AccessWindowHours: s.AccessWindowHours,
		AttemptsAllowed:   s.AttemptsAllowed,
		CreatedAt:         now,
	}
	if err := st.Tests().Create(ctx, test); err != nil {
		return fmt.Errorf("creating test: %w", err)
	}

	logger.Info("imported scenario", "id", s.ID, "title", s.Title, "git_commit", gitCommit)
	return nil
}

func resolveGitCommit(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "local"
	}
	return strings.TrimSpace(string(out))
}
