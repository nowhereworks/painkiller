CREATE TABLE scenario_versions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    external_id VARCHAR(255) NOT NULL,
    title VARCHAR(255) NOT NULL,
    git_commit VARCHAR(40) NOT NULL DEFAULT '',
    duration_minutes INTEGER NOT NULL,
    access_window_hours INTEGER NOT NULL,
    attempts_allowed INTEGER NOT NULL,
    topology_json JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(external_id, git_commit)
);

CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scenario_version_id UUID NOT NULL REFERENCES scenario_versions(id),
    external_id VARCHAR(255) NOT NULL,
    cluster_id VARCHAR(255) NOT NULL,
    kube_context VARCHAR(255) NOT NULL DEFAULT '',
    points INTEGER NOT NULL,
    prompt TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_tasks_scenario_version_id ON tasks(scenario_version_id);

CREATE TABLE checks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    task_id UUID NOT NULL REFERENCES tasks(id),
    external_id VARCHAR(255) NOT NULL,
    cluster_id VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    command TEXT NOT NULL,
    points INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_checks_task_id ON checks(task_id);

ALTER TABLE tests
    ADD CONSTRAINT fk_tests_scenario_version
    FOREIGN KEY (scenario_version_id) REFERENCES scenario_versions(id);
