# Painkiller Shell Implementation Plan

Breaks `docs/architecture.md` into small, ordered implementation units. Each plan lists its dependencies, files to create, deliverables, and acceptance criteria.

---

## Phase 0: Project Scaffolding

### Plan 1: Project Skeleton

**Dependencies:** none

**Files:**
- `cmd/server/main.go`
- `internal/config/config.go`
- `internal/httpx/server.go`
- `internal/httpx/json.go`
- `internal/httpx/errors.go`
- `internal/log/log.go`
- `.env.example`
- `Makefile`

**Deliverables:**
- `cmd/server/main.go`: entrypoint that loads config, connects to Postgres, starts HTTP server with graceful shutdown.
- `internal/config`: typed `Config` struct loaded from env vars / `.env` file. Fields: `DatabaseURL`, `HTTPAddr`, `JWTSecret`, `StripeSecretKey`, `StripeWebhookSecret`, `ProxmoxURL`, `ProxmoxTokenID`, `ProxmoxTokenSecret`, `ProxmoxNode`, `ScenarioRepoPath`, `LogLevel`.
- `internal/httpx/server.go`: HTTP server wrapping `net/http` with a router (use `go-chi/chi`). Mount a health check route `GET /healthz` returning `{"status":"ok"}`.
- `internal/httpx/json.go`: helpers `WriteJSON(w, status, v)` and `ReadJSON(r, v)` using `encoding/json`.
- `internal/httpx/errors.go`: standardized error response format `{"error":"message"}` with helpers for 400, 401, 403, 404, 500.
- `internal/log`: structured logger setup using `log/slog`.
- `.env.example`: all config keys with placeholder values.
- `Makefile`: targets `run`, `test`, `lint`, `migrate-up`, `migrate-down`.

**Acceptance:**
- `go build ./...` succeeds.
- `go run ./cmd/server` starts, `GET /healthz` returns 200 with JSON body.
- `make test` runs (no tests yet, exits clean).

---

## Phase 1: Data Layer

### Plan 2: Scenario Schema

**Dependencies:** Plan 1

**Files:**
- `internal/scenarios/schema.go`
- `internal/scenarios/validate.go`
- `internal/scenarios/validate_test.go`
- `testdata/scenarios/cka/simulator-001/scenario.yaml`
- `testdata/scenarios/cka/simulator-001/tasks/task-01.md`
- `testdata/scenarios/cka/simulator-001/tasks/task-02.md`
- `testdata/scenarios/cka/simulator-001/checks/checks.yaml`

**Deliverables:**
- `schema.go`: Go structs matching the YAML scenario format: `Scenario` (id, title, duration_minutes, access_window_hours, attempts_allowed), `Topology` with `Cluster` entries (id, display_name, kube_context, nodes), `Node` (name, role, template), `Task` (id, cluster_id, kube_context, points, prompt_file), `Check` (id, task_id, cluster_id, type, command, points).
- `validate.go`: `Validate(s *Scenario) error` that checks: cluster IDs are unique, node names unique within cluster, every task references a valid cluster_id, every check references a valid task_id, points are positive, at least one cluster exists, at least one task exists.
- Testdata: one complete example scenario matching the architecture doc example.

**Acceptance:**
- `go test ./internal/scenarios/...` passes.
- Validation rejects a scenario with a dangling task cluster_id reference.
- Validation rejects a scenario with a dangling check task_id reference.
- Validation accepts the testdata example.

---

### Plan 3: Domain Models

**Dependencies:** Plan 1

**Files:**
- `internal/models/user.go`
- `internal/models/product.go`
- `internal/models/test.go`
- `internal/models/purchase.go`
- `internal/models/attempt.go`
- `internal/models/session.go`
- `internal/models/environment.go`
- `internal/models/cluster.go`
- `internal/models/node.go`
- `internal/models/task.go`
- `internal/models/check.go`
- `internal/models/job.go`
- `internal/models/scenario_version.go`
- `internal/models/status.go`

**Deliverables:**
- One struct per entity matching the MVP schema outline in architecture doc. All use `uuid.UUID` for IDs, `time.Time` for timestamps.
- `status.go`: string-typed enums with `const` blocks:
  - `AttemptStatus`: `purchased`, `available`, `attempt_requested`, `environment_provisioning`, `environment_ready`, `terminal_opened`, `running`, `submitted`, `expired`, `grading`, `scored`, `cleanup_pending`, `destroyed`, `provision_failed`, `expired_before_start`, `expired_running`, `cleanup_failed`.
  - `EnvironmentStatus`: `provisioning`, `ready`, `active`, `destroying`, `destroyed`, `failed`.
  - `JobStatus`: `pending`, `running`, `completed`, `failed`.
  - `NodeRole`: `control-plane`, `worker`.
- Each struct has a `TableName() string` method for use with SQL.

**Acceptance:**
- `go build ./internal/models/...` succeeds.
- All status constants are defined and string-typed.

---

### Plan 4: Database Migrations

**Dependencies:** Plan 3

**Files:**
- `migrations/000001_init.up.sql`
- `migrations/000001_init.down.sql`
- `migrations/000002_scenario_versions.up.sql`
- `migrations/000002_scenario_versions.down.sql`
- `cmd/migrate/main.go`

**Deliverables:**
- `000001_init`: tables `users`, `products`, `tests`, `purchased_tests`, `attempts`, `sessions`, `environments`, `clusters`, `nodes`, `jobs`. All columns from architecture schema outline. Foreign keys. Indexes on `purchased_tests(user_id)`, `attempts(purchased_test_id)`, `sessions(attempt_id)`, `environments(attempt_id)`, `clusters(environment_id)`, `nodes(cluster_id)`, `jobs(status, run_at)`.
- `000002_scenario_versions`: tables `scenario_versions`, `tasks`, `checks`. Foreign keys. Indexes on `tasks(scenario_version_id)`, `checks(task_id)`.
- `cmd/migrate/main.go`: CLI that runs up/down migrations using `golang-migrate/migrate` against `DATABASE_URL`.

**Acceptance:**
- `make migrate-up` creates all tables in a local Postgres.
- `make migrate-down` drops them cleanly.
- All foreign key constraints are enforced.

---

### Plan 5: Repository Layer

**Dependencies:** Plan 4

**Files:**
- `internal/store/store.go`
- `internal/store/user_store.go`
- `internal/store/product_store.go`
- `internal/store/test_store.go`
- `internal/store/purchase_store.go`
- `internal/store/attempt_store.go`
- `internal/store/session_store.go`
- `internal/store/environment_store.go`
- `internal/store/scenario_store.go`
- `internal/store/store_test.go`

**Deliverables:**
- `store.go`: `Store` struct wrapping `*sqlx.DB` (use `jmoiron/sqlx`), constructor `New(db *sqlx.DB) *Store`.
- One `*Store` file per entity with interface + Postgres implementation:
  - `UserStore`: `Create`, `GetByEmail`, `GetByID`.
  - `ProductStore`: `Create`, `GetByID`, `GetByStripePriceID`, `List`.
  - `TestStore`: `Create`, `GetByID`, `List`.
  - `PurchaseStore`: `Create`, `GetByID`, `ListByUserID`, `DecrementAttemptsRemaining`.
  - `AttemptStore`: `Create`, `GetByID`, `UpdateStatus`, `ListByPurchaseID`, `UpdateScore`.
  - `SessionStore`: `Create`, `GetByAttemptID`, `GetByTerminalToken`, `UpdateFirstOpenedAt`.
  - `EnvironmentStore`: `Create`, `GetByID`, `GetByAttemptID`, `UpdateStatus`, `UpdateProviderMetadata`.
  - `ScenarioStore`: `CreateVersion`, `GetVersion`, `ListVersions`, `CreateTask`, `CreateCheck`, `ListChecksByScenarioVersion`.
- `store_test.go`: integration tests using a real Postgres (via `testcontainers-go` or `DATABASE_URL` env var) that exercise Create + Get for each store.

**Acceptance:**
- `go test ./internal/store/...` passes against a local Postgres.
- Every store method has at least one test.

---

## Phase 2: Auth & Billing

### Plan 6: Auth

**Dependencies:** Plan 5

**Files:**
- `internal/auth/handler.go`
- `internal/auth/service.go`
- `internal/auth/jwt.go`
- `internal/auth/middleware.go`
- `internal/auth/handler_test.go`

**Deliverables:**
- `service.go`: `Register(ctx, email, password) (*models.User, error)` and `Login(ctx, email, password) (string, error)`. Password hashing with `golang.org/x/crypto/bcrypt`. Token generation returns JWT string.
- `jwt.go`: `GenerateToken(userID uuid.UUID) (string, error)` and `ValidateToken(token string) (uuid.UUID, error)`. Uses `golang-jwt/jwt`. Token expiry: 24 hours.
- `middleware.go`: `AuthMiddleware` that extracts Bearer token or cookie, validates, injects `userID` into request context. Helper `UserIDFromContext(ctx) uuid.UUID`.
- `handler.go`: registers routes on a chi router:
  - `POST /api/v1/auth/register` -> calls service, returns user JSON.
  - `POST /api/v1/auth/login` -> calls service, sets `httpOnly` cookie + returns token JSON.

**Acceptance:**
- `go test ./internal/auth/...` passes.
- Register creates a user, login returns a valid JWT.
- Protected route rejects requests without a valid token.
- Protected route returns user ID from context with a valid token.

---

### Plan 7: Stripe Billing

**Dependencies:** Plan 6

**Files:**
- `internal/billing/handler.go`
- `internal/billing/service.go`
- `internal/billing/webhook.go`
- `internal/billing/handler_test.go`

**Deliverables:**
- `service.go`: `CreateCheckoutSession(ctx, userID, testID) (string, error)` that calls Stripe API to create a Checkout Session with the product's `stripe_price_id`, embedding `user_id` and `test_id` in metadata. Returns the Stripe-hosted checkout URL.
- `webhook.go`: `HandleWebhook(ctx, event stripe.Event) error` that processes `checkout.session.completed` events: reads metadata, creates `PurchasedTest` with `expires_at = now + access_window_hours` and `attempts_remaining = attempts_allowed`. Idempotent on `stripe_session_id`.
- `handler.go`: registers routes:
  - `POST /api/v1/checkout` (authenticated) -> creates checkout session, returns `{url}`.
  - `POST /api/v1/webhooks/stripe` (unauthenticated, signature-verified) -> parses event, calls webhook handler.
- Stripe signature verification using `stripe.ConstructEvent` with the webhook signing secret.

**Acceptance:**
- `go test ./internal/billing/...` passes.
- Checkout endpoint returns a Stripe URL for an authenticated user.
- Webhook handler creates a `PurchasedTest` from a valid signed event payload.
- Webhook handler rejects invalid signatures.
- Duplicate webhook events do not create duplicate purchases.

---

### Plan 8: Test Catalog & Dashboard

**Dependencies:** Plan 7

**Files:**
- `internal/catalog/handler.go`
- `internal/catalog/handler_test.go`
- `internal/entitlements/handler.go`
- `internal/entitlements/handler_test.go`

**Deliverables:**
- `internal/catalog/handler.go`: registers on router:
  - `GET /api/v1/tests` (public) -> lists all tests with product info (title, description, price).
- `internal/entitlements/handler.go`: registers on router:
  - `GET /api/v1/dashboard` (authenticated) -> lists user's purchased tests with status (active/expired), attempts remaining, time remaining.
- Both handlers return JSON arrays using the `httpx` helpers.

**Acceptance:**
- `go test ./internal/catalog/... ./internal/entitlements/...` passes.
- `GET /api/v1/tests` returns all tests without auth.
- `GET /api/v1/dashboard` returns only the authenticated user's purchases.
- Dashboard shows correct `attempts_remaining` and `expires_at`.

---

## Phase 3: Job Queue & Orchestration

### Plan 9: Job Queue

**Dependencies:** Plan 5

**Files:**
- `internal/jobs/queue.go`
- `internal/jobs/worker.go`
- `internal/jobs/types.go`
- `internal/jobs/queue_test.go`

**Deliverables:**
- `types.go`: `JobKind` string enum: `provision_environment`, `grade_attempt`, `cleanup_environment`, `expire_attempt`.
- `queue.go`: `Queue` struct wrapping a Postgres-backed job library (use `riverqueue/river`). Methods: `Enqueue(ctx, kind, payload, opts)` and `EnqueueAt(ctx, kind, payload, runAt)`. Payload is `json.RawMessage`.
- `worker.go`: `Worker` struct that registers job handlers per `JobKind`. Method `Start(ctx)` begins processing. Handlers are registered via `RegisterHandler(kind, func(ctx, payload) error)`.
- River migration runs as part of `cmd/migrate`.

**Acceptance:**
- `go test ./internal/jobs/...` passes.
- Enqueueing a job and starting the worker results in the handler being called.
- Failed jobs are retried according to River's default policy.

---

### Plan 10: Attempt State Machine

**Dependencies:** Plan 5, Plan 9

**Files:**
- `internal/attempts/service.go`
- `internal/attempts/handler.go`
- `internal/attempts/statemachine.go`
- `internal/attempts/statemachine_test.go`
- `internal/attempts/handler_test.go`

**Deliverables:**
- `statemachine.go`: `ValidTransition(from, to AttemptStatus) bool` encoding the full lifecycle from architecture doc. `CanTransition(from AttemptStatus, to ...AttemptStatus) AttemptStatus` helper. All valid transitions enumerated as a map.
- `service.go`: `RequestAttempt(ctx, userID, purchasedTestID) (*models.Attempt, error)`:
  - Validates purchase is active and has attempts remaining.
  - Creates `Attempt` in `attempt_requested` status.
  - Decrements `attempts_remaining`.
  - Enqueues `provision_environment` job.
- `service.go`: `TransitionAttempt(ctx, attemptID, toStatus) error`:
  - Validates transition via state machine.
  - Updates status in store.
  - Sets `started_at` on transition to `environment_provisioning`.
  - Sets `ended_at` on transition to `submitted` or `expired`.
- `handler.go`: registers routes:
  - `POST /api/v1/attempts` (authenticated) -> calls `RequestAttempt`.
  - `GET /api/v1/attempts/:id` (authenticated) -> returns attempt status, session info, terminal token if ready.

**Acceptance:**
- `go test ./internal/attempts/...` passes.
- State machine rejects invalid transitions (e.g., `purchased` -> `scored`).
- State machine accepts all valid transitions from architecture doc.
- `POST /api/v1/attempts` creates an attempt and enqueues a provisioning job.
- Starting an attempt with zero remaining attempts is rejected.

---

### Plan 11: Provider Interface & Mock Provider

**Dependencies:** Plan 3

**Files:**
- `internal/provider/provider.go`
- `internal/provider/types.go`
- `internal/provider/mock/mock.go`
- `internal/provider/mock/mock_test.go`

**Deliverables:**
- `types.go`: product-level types that all providers use:
  - `VMRequest`: hostname, role, template, networkProfile, sshPublicKey, cloudInitData, tags.
  - `VMResult`: providerVMID, ipAddress, hostname.
  - `NetworkProfile`: abstract name (e.g., `"student-net"`) that the provider maps to VLAN/SDN internally.
  - `EnvironmentSpec`: workstation VM request + list of cluster VM requests.
  - `EnvironmentResult`: workstation `VMResult` + map of cluster/node `VMResult`s.
- `provider.go`: `Provider` interface:
  - `CreateEnvironment(ctx, EnvironmentSpec) (*EnvironmentResult, error)`
  - `DestroyEnvironment(ctx, providerMetadata json.RawMessage) error`
  - `GetVMStatus(ctx, providerVMID string) (string, error)`
- `mock/mock.go`: in-memory implementation that returns fake IPs (127.0.0.x), fake VMIDs, and simulates a configurable delay. Stores created environments in a map for inspection.

**Acceptance:**
- `go test ./internal/provider/...` passes.
- Mock `CreateEnvironment` returns results matching the input spec shape.
- Mock `DestroyEnvironment` removes the environment from its internal map.
- Provider interface has no Proxmox-specific types.

---

### Plan 12: Proxmox Provider

**Dependencies:** Plan 11

**Files:**
- `internal/provider/proxmox/client.go`
- `internal/provider/proxmox/provider.go`
- `internal/provider/proxmox/cloudinit.go`
- `internal/provider/proxmox/config.go`
- `internal/provider/proxmox/provider_test.go`

**Deliverables:**
- `config.go`: `Config` struct: API URL, token ID, token secret, Proxmox node name, storage pool, template map (template name -> Proxmox template VMID), network bridge, VLAN ID.
- `client.go`: thin HTTP client wrapping Proxmox API (`/api2/json/...`). Methods: `CloneVM`, `ConfigureVM`, `StartVM`, `StopVM`, `DeleteVM`, `GetVMStatus`. Uses `net/http` with token auth header.
- `cloudinit.go`: generates cloud-init config snippets: hostname, SSH authorized key, network config, metadata tags.
- `provider.go`: implements `Provider` interface:
  - `CreateEnvironment`: clones workstation + all cluster nodes from templates, configures cloud-init and network, starts all VMs, waits for IPs, returns `EnvironmentResult`.
  - `DestroyEnvironment`: reads VMIDs from provider metadata JSON, stops and deletes all VMs.
  - `GetVMStatus`: queries Proxmox for VM state.
- VMs are tagged with `attempt_id` and `environment_id` in Proxmox description field.

**Acceptance:**
- `go build ./internal/provider/proxmox/...` succeeds.
- `go test ./internal/provider/proxmox/...` passes (unit tests with mocked HTTP client).
- Provider implements the `Provider` interface (compile-time check: `var _ provider.Provider = (*ProxmoxProvider)(nil)`).
- No Proxmox-specific types leak into `internal/provider/types.go`.

---

### Plan 13: Provisioner Interface & Ansible Provisioner

**Dependencies:** Plan 11

**Files:**
- `internal/provisioner/provisioner.go`
- `internal/provisioner/types.go`
- `internal/provisioner/ansible/ansible.go`
- `internal/provisioner/ansible/inventory.go`
- `internal/provisioner/ansible/vars.go`
- `internal/provisioner/ansible/ansible_test.go`

**Deliverables:**
- `types.go`:
  - `ClusterSpec`: cluster name, kube_context, nodes (with IPs and roles).
  - `EnvironmentProvisionSpec`: list of `ClusterSpec`, workstation IP, SSH private key, scenario provision playbook path.
  - `ProvisionResult`: list of kubeconfig paths written to workstation, readiness status per cluster.
- `provisioner.go`: `Provisioner` interface:
  - `Provision(ctx, EnvironmentProvisionSpec) (*ProvisionResult, error)`
- `ansible/inventory.go`: generates `inventory.ini` content from `EnvironmentProvisionSpec`. Groups: `[control_plane]`, `[workers]`, `[workstation]`. Each host entry includes `ansible_host`, `ansible_user`, `ansible_ssh_private_key_file`.
- `ansible/vars.go`: generates `vars.yaml` with cluster names, kube contexts, pod CIDRs, service CIDRs, scenario-specific variables.
- `ansible/ansible.go`: implements `Provisioner`:
  - Writes inventory and vars to a temp directory.
  - Executes `ansible-playbook -i inventory.ini -e @vars.yaml <playbook_path>` via `os/exec`.
  - Streams stdout/stderr to the structured logger.
  - Returns error if exit code is non-zero.

**Acceptance:**
- `go test ./internal/provisioner/...` passes.
- Generated `inventory.ini` contains correct host groups and IPs.
- Generated `vars.yaml` contains all cluster and node information.
- Provisioner implements the `Provisioner` interface (compile-time check).

---

### Plan 14: Orchestrator

**Dependencies:** Plan 9, Plan 10, Plan 11, Plan 13

**Files:**
- `internal/orchestrator/orchestrator.go`
- `internal/orchestrator/provision_job.go`
- `internal/orchestrator/orchestrator_test.go`

**Deliverables:**
- `orchestrator.go`: `Orchestrator` struct holding `Provider`, `Provisioner`, `Store`, `Queue`, `Logger`. Method `RegisterJobs()` registers handlers for all job kinds on the worker.
- `provision_job.go`: handles `provision_environment` job:
  1. Loads attempt, purchased test, test, scenario version from store.
  2. Transitions attempt to `environment_provisioning`.
  3. Generates ephemeral ED25519 SSH keypair (`crypto/ed25519`).
  4. Builds `EnvironmentSpec` from scenario topology.
  5. Calls `Provider.CreateEnvironment`.
  6. Stores provider metadata (JSON) on `Environment`.
  7. Builds `EnvironmentProvisionSpec` with VM IPs and SSH key.
  8. Calls `Provisioner.Provision`.
  9. Creates `Session` with a random terminal token.
  10. Transitions attempt to `environment_ready`.
  11. On any failure: transitions attempt to `provision_failed`, enqueues `cleanup_environment` job.
- Also handles `cleanup_environment` job:
  1. Loads environment from store.
  2. Calls `Provider.DestroyEnvironment`.
  3. Transitions environment to `destroyed`.
  4. On failure: transitions to `cleanup_failed`.

**Acceptance:**
- `go test ./internal/orchestrator/...` passes.
- Using mock provider: provisioning job creates environment, session, and transitions attempt to `environment_ready`.
- Failed provisioning transitions attempt to `provision_failed` and enqueues cleanup.
- Cleanup job destroys environment and updates status.

---

## Phase 4: Student Experience & Grading

### Plan 15: Terminal Gateway

**Dependencies:** Plan 14

**Files:**
- `internal/terminal/gateway.go`
- `internal/terminal/ssh.go`
- `internal/terminal/token.go`
- `internal/terminal/gateway_test.go`

**Deliverables:**
- `token.go`: `GenerateToken() (string, error)` produces a cryptographically random 32-byte hex token.
- `ssh.go`: `DialSSH(ctx, host, port, user, privateKey) (*ssh.Client, error)` using `golang.org/x/crypto/ssh`. `NewSSHSession(client, cols, rows) (*ssh.Session, error)` requests a PTY.
- `gateway.go`: `Gateway` struct holding `Store`, `Logger`. Method `HandleWebSocket(w, r)`:
  1. Upgrades HTTP to WebSocket (use `nhooyr.io/websocket` or `gorilla/websocket`).
  2. Extracts terminal token from URL path.
  3. Looks up session by token, validates attempt is in `environment_ready` or `running` state.
  4. Loads environment to get workstation IP and SSH private key.
  5. Dials SSH to workstation.
  6. If `session.first_opened_at` is nil: sets it, transitions attempt to `terminal_opened`, then to `running`. Enqueues `expire_attempt` job at `first_opened_at + duration_minutes`.
  7. Pipes WebSocket binary messages to SSH stdin.
  8. Pipes SSH stdout/stderr to WebSocket binary messages.
  9. Handles WebSocket JSON message `{"type":"resize","cols":N,"rows":N}` by calling `WindowChange` on the SSH session.
  10. On WebSocket close: closes SSH session.
- Registers route: `GET /api/v1/terminal/{token}` (upgraded to WebSocket).

**Acceptance:**
- `go test ./internal/terminal/...` passes.
- Invalid or expired tokens are rejected with 401.
- First connection sets `first_opened_at` and starts the expiry timer.
- Subsequent connections do not reset `first_opened_at`.
- WebSocket close cleanly terminates the SSH session.

---

### Plan 16: Submission & Expiry

**Dependencies:** Plan 10, Plan 9

**Files:**
- `internal/attempts/submit.go`
- `internal/attempts/expire.go`
- `internal/attempts/submit_test.go`
- `internal/attempts/expire_test.go`

**Deliverables:**
- `submit.go`: `SubmitAttempt(ctx, userID, attemptID) error`:
  - Validates attempt belongs to user and is in `running` state.
  - Transitions attempt to `submitted`.
  - Enqueues `grade_attempt` job.
- `expire.go`: handles `expire_attempt` job:
  - Loads attempt. If still in `running` state, transitions to `expired`.
  - Enqueues `grade_attempt` job.
  - If attempt is already `submitted` or beyond, no-op.
- Registers route: `POST /api/v1/attempts/:id/submit` (authenticated).

**Acceptance:**
- `go test ./internal/attempts/...` passes (new tests added).
- Submit transitions a `running` attempt to `submitted` and enqueues grading.
- Submit rejects attempts not in `running` state.
- Expiry job transitions a `running` attempt to `expired` and enqueues grading.
- Expiry job is a no-op for already-submitted attempts.

---

### Plan 17: Grading Engine

**Dependencies:** Plan 14, Plan 9

**Files:**
- `internal/grading/engine.go`
- `internal/grading/runner.go`
- `internal/grading/checks.go`
- `internal/grading/engine_test.go`

**Deliverables:**
- `checks.go`: `CheckResult` struct: check_id, passed (bool), stdout, stderr, exit_code, points_awarded, points_possible, ran_at.
- `runner.go`: `RunCheck(ctx, check models.Check, clusterIP, sshKey string) (*CheckResult, error)`:
  - For `type: kubectl`: SSHes to workstation, runs the check command with the correct `--context` flag, captures stdout/stderr/exit code.
  - For `type: script`: SSHes to workstation, runs the script, captures output.
  - Awards full points if exit code is 0 and output matches expected pattern (or just exit code 0 for MVP).
  - Awards 0 points otherwise.
- `engine.go`: `GradeAttempt(ctx, attemptID) error`:
  - Loads attempt, scenario version, all tasks, all checks.
  - Loads environment and SSH key.
  - Runs all checks via runner.
  - Sums points_awarded / points_possible per task.
  - Updates attempt with final score.
  - Transitions attempt to `scored`.
  - Enqueues `cleanup_environment` job.
- Registered as handler for `grade_attempt` job kind in orchestrator.

**Acceptance:**
- `go test ./internal/grading/...` passes.
- Grading a mock attempt with passing checks produces a correct score.
- Grading a mock attempt with failing checks produces partial or zero score.
- Check results include stdout, stderr, exit code for debugging.
- Attempt transitions to `scored` after grading.

---

### Plan 18: Score Report API

**Dependencies:** Plan 17

**Files:**
- `internal/scoring/handler.go`
- `internal/scoring/handler_test.go`

**Deliverables:**
- `handler.go`: registers route:
  - `GET /api/v1/attempts/:id/score` (authenticated) -> returns:
    - `total_score`, `max_score`, `percentage`.
    - `tasks[]`: each with `task_id`, `prompt`, `points_awarded`, `points_possible`.
    - `status`: `scored` or current status if not yet graded.
  - Only accessible to the attempt owner.
  - Returns 404 if attempt is not in `scored` state (no partial results during exam).

**Acceptance:**
- `go test ./internal/scoring/...` passes.
- Score endpoint returns correct totals for a graded attempt.
- Score endpoint returns 404 for an ungraded attempt.
- Score endpoint rejects access from non-owner users.

---

## Phase 5: Operations

### Plan 19: Cleanup Reconciler

**Dependencies:** Plan 14

**Files:**
- `internal/orchestrator/reconciler.go`
- `internal/orchestrator/reconciler_test.go`

**Deliverables:**
- `reconciler.go`: `Reconciler` struct with `Store`, `Queue`, `Logger`. Method `Run(ctx)` starts a ticker (every 60s) that:
  1. Finds environments in `destroying` status that have been stuck for > 10 minutes -> enqueues `cleanup_environment` job.
  2. Finds attempts in `cleanup_pending` status -> enqueues `cleanup_environment` job.
  3. Finds environments for attempts in terminal states (`scored`, `expired_before_start`) with no cleanup job pending -> enqueues `cleanup_environment` job.
  4. Finds attempts stuck in `environment_provisioning` for > 30 minutes -> transitions to `provision_failed`, enqueues cleanup.

**Acceptance:**
- `go test ./internal/orchestrator/...` passes (new tests added).
- Reconciler enqueues cleanup for stale environments.
- Reconciler does not enqueue duplicate cleanup jobs.

---

### Plan 20: Admin Controls

**Dependencies:** Plan 6, Plan 10

**Files:**
- `internal/admin/handler.go`
- `internal/admin/middleware.go`
- `internal/admin/handler_test.go`

**Deliverables:**
- `middleware.go`: `AdminMiddleware` checks that the authenticated user has an `is_admin` flag (add `is_admin bool` column to `users` table via new migration `000003`).
- `handler.go`: registers routes under `/api/v1/admin` (authenticated + admin):
  - `POST /api/v1/admin/attempts/:id/retry-provision` -> resets attempt to `attempt_requested`, enqueues `provision_environment`.
  - `POST /api/v1/admin/attempts/:id/retry-grade` -> enqueues `grade_attempt`.
  - `POST /api/v1/admin/environments/:id/destroy` -> enqueues `cleanup_environment`.
  - `GET /api/v1/admin/attempts` -> list all attempts with status, filter by status.
  - `GET /api/v1/admin/environments` -> list all environments with status.

**Acceptance:**
- `go test ./internal/admin/...` passes.
- Non-admin users receive 403 on all admin routes.
- Admin can retry provisioning for a failed attempt.
- Admin can force-destroy an environment.

---

### Plan 21: Documentation Proxy

**Dependencies:** Plan 14

**Files:**
- `infra/proxy/squid.conf`
- `infra/proxy/allowlist.txt`
- `infra/workstation/iptables.sh`
- `docs/proxy.md`

**Deliverables:**
- `squid.conf`: Squid proxy configuration with ACL rules reading from `allowlist.txt`. Denies all requests not matching the allowlist. Listens on port 3128.
- `allowlist.txt`: one domain per line. Initial entries: `kubernetes.io`, `*.kubernetes.io`, `helm.sh`, `*.helm.sh`.
- `iptables.sh`: script baked into workstation VM template. Sets `http_proxy`/`https_proxy` env vars in `/etc/environment`. Adds iptables rules: allow outbound 80/443 only to proxy IP, drop all other outbound 80/443.
- `docs/proxy.md`: documents how to deploy the proxy, update the allowlist, and configure the proxy IP in the workstation template.

**Acceptance:**
- Squid config is valid (can be validated with `squid -k parse`).
- Allowlist file is a plain text file, one domain per line.
- iptables script blocks direct outbound HTTP/HTTPS except to the proxy.

---

## Deferred (Post-MVP)

- Snapshot and progress preservation (paid add-on).
- Keycloak or external OIDC auth.
- Subscriptions and recurring billing.
- Instructor and team management.
- Live grading during labs.
- Advanced typed check DSL.
- Additional runtime providers beyond Proxmox.

---

## Dependency Graph

```
Plan 1 (Skeleton)
  -> Plan 2 (Scenario Schema)
  -> Plan 3 (Domain Models)
       -> Plan 4 (Migrations)
            -> Plan 5 (Repositories)
                 -> Plan 6 (Auth)
                      -> Plan 7 (Stripe)
                           -> Plan 8 (Catalog & Dashboard)
                 -> Plan 9 (Job Queue)
                      -> Plan 10 (Attempt State Machine)
       -> Plan 11 (Provider Interface + Mock)
            -> Plan 12 (Proxmox Provider)
            -> Plan 13 (Ansible Provisioner)
  Plan 9 + Plan 10 + Plan 11 + Plan 13
       -> Plan 14 (Orchestrator)
            -> Plan 15 (Terminal Gateway)
            -> Plan 16 (Submission & Expiry)
            -> Plan 17 (Grading Engine)
                 -> Plan 18 (Score Report)
            -> Plan 19 (Cleanup Reconciler)
            -> Plan 21 (Documentation Proxy)
  Plan 6 + Plan 10
       -> Plan 20 (Admin Controls)
```

## Execution Rules

1. Execute plans in phase order. Within a phase, plans with satisfied dependencies can be worked in parallel.
2. Each plan must pass its acceptance criteria before dependent plans begin.
3. Keep infrastructure details (Proxmox, Ansible, Squid) behind their interface boundaries.
   Squid is external infrastructure: Painkiller may configure workstations with a proxy address, but Squid ACLs and allowlists are managed in Squid/infra config.
4. Do not grant purchased test access from frontend redirects. Trust Stripe webhooks only.
5. Use the mock provider for all testing until Proxmox integration begins.
6. Every plan should end with `go build ./...` and `go test ./...` passing.
