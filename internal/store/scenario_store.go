package store

import (
	"context"

	"github.com/google/uuid"
	"painkiller-shell/internal/models"
)

type ScenarioStore interface {
	CreateVersion(ctx context.Context, version *models.ScenarioVersion) error
	GetVersion(ctx context.Context, id uuid.UUID) (*models.ScenarioVersion, error)
	GetVersionByExternalID(ctx context.Context, externalID, gitCommit string) (*models.ScenarioVersion, error)
	ListVersions(ctx context.Context) ([]*models.ScenarioVersion, error)
	CreateTask(ctx context.Context, task *models.Task) error
	CreateCheck(ctx context.Context, check *models.Check) error
	ListChecksByScenarioVersion(ctx context.Context, scenarioVersionID uuid.UUID) ([]*models.Check, error)
	ListTasksByScenarioVersion(ctx context.Context, scenarioVersionID uuid.UUID) ([]*models.Task, error)
}

type scenarioStore struct {
	db *Store
}

func (s *Store) Scenarios() ScenarioStore {
	return &scenarioStore{db: s}
}

func (sc *scenarioStore) CreateVersion(ctx context.Context, version *models.ScenarioVersion) error {
	query := `INSERT INTO scenario_versions (id, external_id, title, git_commit, duration_minutes, access_window_hours, attempts_allowed, topology_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := sc.db.db.ExecContext(ctx, query, version.ID, version.ExternalID, version.Title, version.GitCommit, version.DurationMinutes, version.AccessWindowHours, version.AttemptsAllowed, version.TopologyJSON, version.CreatedAt)
	return err
}

func (sc *scenarioStore) GetVersion(ctx context.Context, id uuid.UUID) (*models.ScenarioVersion, error) {
	var version models.ScenarioVersion
	query := `SELECT id, external_id, title, git_commit, duration_minutes, access_window_hours, attempts_allowed, topology_json, created_at FROM scenario_versions WHERE id = $1`
	err := sc.db.db.GetContext(ctx, &version, query, id)
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func (sc *scenarioStore) GetVersionByExternalID(ctx context.Context, externalID, gitCommit string) (*models.ScenarioVersion, error) {
	var version models.ScenarioVersion
	query := `SELECT id, external_id, title, git_commit, duration_minutes, access_window_hours, attempts_allowed, topology_json, created_at FROM scenario_versions WHERE external_id = $1 AND git_commit = $2`
	err := sc.db.db.GetContext(ctx, &version, query, externalID, gitCommit)
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func (sc *scenarioStore) ListVersions(ctx context.Context) ([]*models.ScenarioVersion, error) {
	var versions []*models.ScenarioVersion
	query := `SELECT id, external_id, title, git_commit, duration_minutes, access_window_hours, attempts_allowed, topology_json, created_at FROM scenario_versions ORDER BY created_at DESC`
	err := sc.db.db.SelectContext(ctx, &versions, query)
	return versions, err
}

func (sc *scenarioStore) CreateTask(ctx context.Context, task *models.Task) error {
	query := `INSERT INTO tasks (id, scenario_version_id, external_id, cluster_id, kube_context, points, prompt, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := sc.db.db.ExecContext(ctx, query, task.ID, task.ScenarioVersionID, task.ExternalID, task.ClusterID, task.KubeContext, task.Points, task.Prompt, task.CreatedAt)
	return err
}

func (sc *scenarioStore) CreateCheck(ctx context.Context, check *models.Check) error {
	query := `INSERT INTO checks (id, task_id, external_id, cluster_id, type, command, points, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := sc.db.db.ExecContext(ctx, query, check.ID, check.TaskID, check.ExternalID, check.ClusterID, check.Type, check.Command, check.Points, check.CreatedAt)
	return err
}

func (sc *scenarioStore) ListChecksByScenarioVersion(ctx context.Context, scenarioVersionID uuid.UUID) ([]*models.Check, error) {
	var checks []*models.Check
	query := `SELECT c.id, c.task_id, c.external_id, c.cluster_id, c.type, c.command, c.points, c.created_at
		FROM checks c
		JOIN tasks t ON c.task_id = t.id
		WHERE t.scenario_version_id = $1
		ORDER BY c.created_at`
	err := sc.db.db.SelectContext(ctx, &checks, query, scenarioVersionID)
	return checks, err
}

func (sc *scenarioStore) ListTasksByScenarioVersion(ctx context.Context, scenarioVersionID uuid.UUID) ([]*models.Task, error) {
	var tasks []*models.Task
	query := `SELECT id, scenario_version_id, external_id, cluster_id, kube_context, points, prompt, created_at FROM tasks WHERE scenario_version_id = $1 ORDER BY created_at`
	err := sc.db.db.SelectContext(ctx, &tasks, query, scenarioVersionID)
	return tasks, err
}
