# Painkiller Shell Architecture Plan
Painkiller Shell is a Killer.sh-style Kubernetes training platform. Students buy tests, receive time-limited access and a fixed number of attempts, then train in live kubeadm-based environments running on Proxmox VMs.

## Current Decisions
- **Backend:** Go first, kept as a modular monolith for the MVP.
- **Frontend:** Next.js.
- **Database:** PostgreSQL.
- **Job Queue:** Postgres-backed (e.g., `river` or `goqite`) to avoid adding Redis for MVP.
- **Runtime provider:** Proxmox first.
- **Provider strategy:** adapter layer so future providers do not require core product changes.
- **Kubernetes:** kubeadm.
- **Environment isolation:** per-student hidden environment.
- **Test topology:** configurable per test; one test can contain multiple kubeadm clusters with variable node counts.
- **Student UX:** mimic official Kubernetes exam UX where practical.
- **Frontend Terminal:** `xterm.js` with `WebSocket`.
- **Billing:** Stripe.
- **Auth:** built-in auth for MVP; keep a boundary for possible Keycloak/OIDC later.
- **Scenario authoring:** Git initially.
- **Grading:** final submission or expiry only, with weighted checks and scoring.
- **Restricted internet:** use a Squid-like proxy with allowlisted official documentation sites.
- **Progress preservation:** optional paid snapshot feature for exams and labs, deferred until after MVP.
- **Secrets:** Environment variables (`.env`) for MVP, migrating to a vault/KMS later.

## Product & Data Model
Students should see tests, not infrastructure.

### Core Concepts
- `Product`: Stripe sellable item.
- `Test`: user-facing training or exam product.
- `PurchasedTest`: access a user owns for a fixed window and number of tries.
- `Attempt`: one try against a purchased test.
- `Session`: runtime state for an attempt.
- `Environment`: hidden infrastructure created for an attempt.
- `Cluster`: one kubeadm cluster inside an environment.
- `Node`: one VM inside a cluster.
- `Task`: one question in a test.
- `Check`: final validation logic for scoring.

### MVP Database Schema Outline
- `User`: id, email, password_hash, created_at
- `Product`: id, stripe_price_id, title, description
- `Test`: id, product_id, scenario_version_id, duration_minutes, access_window_hours, attempts_allowed
- `PurchasedTest`: id, user_id, test_id, stripe_session_id, expires_at, attempts_remaining
- `Attempt`: id, purchased_test_id, status (state machine), started_at, ended_at, score
- `Session`: id, attempt_id, environment_id, terminal_token, first_opened_at
- `Environment`: id, attempt_id, provider_metadata (JSONB for Proxmox VMIDs), status, workstation_ip
- `Cluster`: id, environment_id, name, kube_context
- `Node`: id, cluster_id, name, role, provider_vm_id
- `Task`: id, scenario_version_id, cluster_id, points, prompt
- `Check`: id, task_id, type, command/script, points
- `Job`: id, queue, payload, status, attempts, run_at (for async provisioning/grading)

### Example flow
Student pays $8
  -> PurchasedTest is valid for 36 hours
  -> PurchasedTest grants 2 attempts
  -> Student starts a test attempt
  -> Backend provisions a hidden multi-cluster environment
  -> Exam timer starts on first successful terminal open
  -> Final grading runs only on submit or expiry

## High-Level Architecture
Next.js app
  -> Go API
      -> auth (JWT/Session cookies)
      -> Stripe billing (Webhooks)
      -> test catalog
      -> purchased test access
      -> attempts and sessions
      -> terminal gateway (WebSocket <-> SSH bridge)
      -> grading engine
      -> orchestration jobs (Postgres-backed job queue)
          -> Proxmox provider
          -> kubeadm/Ansible provisioner
      -> PostgreSQL

Git scenario repo
  -> scenario importer
  -> immutable scenario versions

Proxmox
  -> VM templates
  -> kubeadm clusters
  -> VLAN/SDN isolation

## API Contract (MVP)
- `POST /api/v1/auth/register`, `POST /api/v1/auth/login`
- `GET /api/v1/tests`
- `POST /api/v1/checkout` (Returns Stripe session URL)
- `POST /api/v1/webhooks/stripe`
- `GET /api/v1/dashboard` (Purchased tests)
- `POST /api/v1/attempts` (Starts provisioning job)
- `GET /api/v1/attempts/:id` (Status, terminal token)
- `WS /api/v1/terminal/:token` (WebSocket connection)
- `POST /api/v1/attempts/:id/submit` (Triggers grading job)

## Suggested Go Boundaries
- `/internal/auth`
- `/internal/billing`
- `/internal/entitlements`
- `/internal/scenarios`
- `/internal/attempts`
- `/internal/sessions`
- `/internal/orchestrator`
- `/internal/provider`
- `/internal/provider/proxmox`
- `/internal/provisioner`
- `/internal/provisioner/ansible`
- `/internal/grading`
- `/internal/terminal`
- `/internal/jobs`
- `/internal/audit`

Keep Proxmox, Ansible, and proxy details behind infrastructure boundaries. Product code should speak in terms of tests, attempts, clusters, tasks, scores, and retakes.

## Multi-Cluster Test Model
A test can define several clusters. Each task tells the student which cluster/context to use.
Example shape:
```yaml
id: cka-simulator-001
title: CKA Simulator 1
duration_minutes: 120
access_window_hours: 36
attempts_allowed: 2

topology:
  clusters:
    - id: cluster-a
      display_name: cluster-a
      kube_context: cluster-a-admin
      nodes:
        - name: cp-1
          role: control-plane
          template: kubeadm-control-plane
        - name: worker-1
          role: worker
          template: kubeadm-worker
    - id: cluster-b
      display_name: cluster-b
      kube_context: cluster-b-admin
      nodes:
        - name: cp-1
          role: control-plane
          template: kubeadm-control-plane
        - name: worker-1
          role: worker
          template: kubeadm-worker
        - name: worker-2
          role: worker
          template: kubeadm-worker

tasks:
  - id: task-01
    cluster_id: cluster-a
    kube_context: cluster-a-admin
    points: 8
    prompt_file: tasks/task-01.md
  - id: task-02
    cluster_id: cluster-b
    kube_context: cluster-b-admin
    points: 12
    prompt_file: tasks/task-02.md
```
The terminal environment should provide kubeconfig contexts for all required clusters:
```bash
kubectl config use-context cluster-a-admin
kubectl config use-context cluster-b-admin
```

## Student Environment Layout
Each attempt should provision one hidden environment:
```text
Environment
  Student workstation VM
  Cluster A
    control-plane VM
    worker VM(s)
  Cluster B
    control-plane VM
    worker VM(s)
  Restricted docs proxy path
```
Use a dedicated student workstation VM as the browser terminal target. It gives students one stable shell, stores kubeconfigs for all clusters, centralizes proxy configuration, and keeps cluster nodes cleaner.

## Proxmox Provider
The Go backend calls the Proxmox API directly through a provider adapter.
Provider responsibilities:
- Clone selected pre-baked VM templates.
- Create all VMs needed by a test topology.
- Attach Proxmox VLAN/SDN/network config through abstract network profiles.
- Inject cloud-init data and per-session SSH keys.
- Start, stop, inspect, and destroy VMs.
- Tag VMs with attempt/environment metadata for cleanup.
- Later: snapshot and restore VMs for paid progress preservation.

Core product code should not know VMIDs, storage pools, bridge names, VLAN IDs, Proxmox node names, or template IDs.

## Kubeadm Provisioning
Use pre-baked templates to reduce startup time.
Templates should include:
- kubeadm, kubelet, kubectl
- container runtime
- cloud-init, SSH
- base troubleshooting tools
- common packages needed during tests

Provisioning flow:
1. Proxmox clones pre-baked templates.
2. cloud-init sets hostname, SSH key, network, and base metadata.
3. Go backend dynamically generates `inventory.ini` and `vars.yaml`.
4. Go backend executes `ansible-playbook` via `os/exec` to initialize control planes.
5. Ansible joins worker nodes.
6. Ansible installs CNI.
7. Ansible writes kubeconfigs to the student workstation.
8. Ansible applies scenario-specific setup and intentional misconfigurations.
9. Readiness checks confirm all clusters are usable.

Use Ansible for the MVP provisioner because it is simple, Git-friendly, and replaceable behind a provisioner interface.

## Timers And Attempts
Use independent timers:
- **Access window:** starts when Stripe payment is confirmed.
- **Exam timer:** starts on first successful terminal open.
- **Infrastructure TTL:** limits cost for prepared but unopened or abandoned environments.

Attempt lifecycle:
`purchased` -> `available` -> `attempt_requested` -> `environment_provisioning` -> `environment_ready` -> `terminal_opened` -> `running` -> `submitted | expired` -> `grading` -> `scored` -> `cleanup_pending` -> `destroyed`

Also support failure states:
`provision_failed`, `expired_before_start`, `expired_running`, `cleanup_failed`

Retakes consume allowed attempts and normally create a fresh environment.

## Terminal Gateway
The browser connects to the Go terminal gateway over WebSocket (`xterm.js`). The gateway connects to the student workstation over SSH (`golang.org/x/crypto/ssh`).
Rules:
- The browser never receives Proxmox credentials.
- The browser never receives long-lived SSH private keys.
- Terminal tokens are short-lived and scoped to one attempt/session.
- The first successful terminal connection records `first_terminal_opened_at` and starts the exam timer.
- **Implementation Detail:** The gateway maintains a mapping of active WebSocket connections to SSH sessions. It pipes stdin/stdout/stderr between them and handles window resize events.

## Restricted Documentation Access
Students should only reach approved documentation sites.
Implementation: Deploy a single shared Squid proxy in the management network.
- Student workstations are provisioned with `http_proxy` and `https_proxy` environment variables pointing to the shared proxy.
- `iptables` rules on the student workstation block all outbound traffic on ports 80 and 443 except to the shared proxy's IP.
- The Squid proxy uses an allowlist ACL for domains.

Allow examples:
- Official Kubernetes docs.
- Official docs for the technology being tested.
- Package mirrors only when required by provisioning.

Block examples:
- General internet.
- Search engines.
- AI tools.
- Paste sites.
- Other student environments.
- Proxmox, database, backend, and other platform internals.

Network intent:
- student workstation -> docs proxy -> allowlisted docs
- student workstation -> assigned cluster nodes
- grader/orchestrator -> environment over SSH/Kubernetes API
- environment -> no direct internet except proxy
- environment -> no platform internals

## Grading
Exam grading runs only after final submission or expiry.
Rules:
- No grading button during the exam.
- No partial feedback during the attempt.
- Checks validate final state, not command history.
- Checks run from trusted backend-controlled infrastructure.
- Results are weighted by task/check points.
- Store stdout, stderr, exit code, points, and timestamps for internal debugging.

Example checks:
```yaml
checks:
  - id: task-01-check
    task_id: task-01
    cluster_id: cluster-a
    type: kubectl
    command: "kubectl get networkpolicy ..."
    points: 8
```

## Scenario Authoring In Git
Initial source of truth:
```text
scenarios/
  cka/
    simulator-001/
      scenario.yaml
      tasks/
        task-01.md
        task-02.md
      provision/
        playbook.yaml
        group_vars/
        files/
      checks/
        checks.yaml
        scripts/
```
Importer behavior:
- Read the scenario Git repo.
- Validate schema and topology references.
- Validate task-to-cluster and check-to-task references.
- Create immutable scenario versions.
- Store the Git commit SHA with each imported version.
- Reject invalid scenarios before they can be sold or attempted.

## Billing
Stripe creates purchased tests through webhooks.
Flow:
User selects test
  -> Stripe Checkout
  -> Stripe webhook confirms payment
  -> backend creates PurchasedTest
  -> student can start attempts while access remains valid

Do not grant access from frontend success redirects. Trust Stripe webhooks only.

## Security & Secrets
- **Proxmox API:** Token/Secret stored in Go backend env vars.
- **SSH Keys:** Backend generates an ephemeral ED25519 keypair per environment. The public key is injected into the VM via Proxmox cloud-init. The private key is kept in memory (or encrypted in DB) for the duration of the attempt and used by the Terminal Gateway and Ansible provisioner.
- **Stripe:** Webhook signing secret and API keys in env vars.

## Snapshots
Snapshot-based progress preservation is deferred until after MVP.
Keep these concepts in mind but do not build them first:
- `PurchasedTest.preserve_progress_enabled`
- `EnvironmentSnapshot`
- `SnapshotRetentionPolicy`
- `Provider.SnapshotEnvironment`
- `Provider.RestoreEnvironment`

When implemented, snapshots should be a paid add-on, available for exams and labs, with hard retention TTLs and storage quotas.

## MVP Scope
Include:
- Built-in auth.
- Stripe one-time purchase.
- Purchased test dashboard.
- Git scenario import.
- Configurable multi-cluster topology.
- Proxmox VM clone/start/destroy.
- kubeadm provisioning via Ansible (`os/exec`).
- Student workstation VM.
- Browser terminal (`xterm.js` + WebSocket gateway).
- First-terminal-open timer.
- Final-only grading.
- Score report.
- Retakes.
- Cleanup reconciler.
- Shared Squid-like docs proxy allowlist & `iptables` enforcement.
- Basic admin tools for retry/destroy/debug.

Defer:
- Snapshots/progress preservation.
- Keycloak/OIDC.
- Subscriptions.
- Instructor/team management.
- Live grading during labs.
- Advanced typed check DSL.
- Additional runtime providers beyond a clean provider interface.

## Build Order
1. Define scenario schema with multi-cluster topology and task-to-cluster mapping.
2. Build Go domain model, PostgreSQL schema, and migrations for tests, purchases, attempts, sessions, environments, clusters, nodes, checks, and scores.
3. Build Git scenario importer and validator.
4. Add built-in auth (JWT/cookies).
5. Add Stripe checkout and webhook entitlement creation.
6. Build Postgres-backed job queue for asynchronous tasks.
7. Build attempt lifecycle state machine.
8. Build mock provider for local development.
9. Build Proxmox provider for clone/start/destroy and ephemeral SSH key injection.
10. Build kubeadm templates and Ansible provisioner (triggered via `os/exec`).
11. Add student workstation VM and generated kubeconfig contexts.
12. Add terminal gateway (WebSocket to SSH bridge) and start timer on first terminal open.
13. Add final submission/expiry job.
14. Add grading engine with cluster-aware checks.
15. Add score report UI.
16. Add cleanup reconciler and admin destroy/retry controls.
17. Deploy shared Squid proxy and configure workstation network enforcement.
18. Later add snapshot preservation as a paid add-on.