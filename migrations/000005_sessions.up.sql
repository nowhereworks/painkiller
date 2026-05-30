CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    attempt_id UUID NOT NULL REFERENCES attempts(id),
    environment_id UUID NOT NULL REFERENCES environments(id),
    terminal_token VARCHAR(255) UNIQUE NOT NULL,
    first_opened_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sessions_attempt_id ON sessions(attempt_id);
CREATE INDEX idx_sessions_terminal_token ON sessions(terminal_token);
