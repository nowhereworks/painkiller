# Painkiller Shell — MVP Completion Plan

> **Status: MVP complete.** All items in §8 are implemented and validated on a
> real 3-node lab. This file remains as the record of scope, architecture,
> validation results, and lessons learned. The restart prompt below is kept for
> context.
>
> **Original restart prompt for a fresh session:** Read this file fully, then continue
> from "Remaining Work". The codebase is already a working, validated MVP; the
> remaining work is exam-content depth, atomic answer capture, and comprehensive
> failure-path testing. Do **not** re-litigate the lab-creation approach — see
> "Lessons Learned / Do Not Repeat".

---

## 1. Project overview

**Painkiller Shell** is a single-user, self-hosted **Killer.sh-style CKA practice
simulator**. A user provisions a local Proxmox Kubernetes lab, takes a timed
exam in the browser (question panel + SSH terminal), submits, and gets
per-task, partial-credit scoring with solutions.

Design directives (from `AGENTS.md`):
- Keep things as simple as possible.
- Change as little as possible to achieve goals.
- Always commit and push after work.
- Always persist useful data for future sessions.

Explicitly **out of scope** for the MVP: authentication, multi-user isolation,
billing/subscriptions, admin dashboards, multiple exam products, exact visual
cloning of Killer.sh, multiple infrastructure providers.

---

## 2. Current status

### Done and validated end-to-end on a real 3-node Proxmox lab
- Linked-clone lab creation: 3 VMs reachable in **~31 s**; provisioning waits for
  cloud-init/apt to settle before Ansible runs.
- kubeadm provisioning: 3 nodes **Ready** (Kubernetes v1.36.6, Flannel, containerd 2.2.1),
  `jq` and `etcd-client` installed on the controller.
- Timed exam UI: question navigation, flags, "mark complete", timer, copy instructions.
- Browser SSH terminal (xterm.js) with input, reconnect, and resize.
- Submit → concurrent snapshot capture → grading with per-check partial credit.
- Retry preparation (reusing VMs) and retry of a failed reset.
- Restart recovery: interrupted lab operations become retryable errors; grading resumes.
- Reset (destroy lab) and re-run.
- **29-task, 100-point exam** covering all five CKA domains (see §5).

### Live validation results (recorded)
| Stage | Result |
|---|---|
| Baseline (fresh lab) | **0 / 100** |
| Reference solutions applied | **100 / 100** |
| One answer deliberately removed | **98 / 100** (configmap = 2 points) |
| Browser submit (graded) | **100 / 100** |
| Reset | complete |

### Automated tests
- `go test -race ./...` — 42 tests: state transitions, grading, partial credit,
  fixture coverage for every packaged task, snapshot immutability/privacy,
  concurrent capture, lab-script guards, retry, restart resume, expiry.
- `go vet ./...`, `shellcheck scripts/*.sh exam/reference-solutions.sh`,
  `node --check web/app.js scripts/e2e/*.js`, YAML/JSON validation.
- `scripts/e2e/` Playwright harness (committed, reproducible).


---

## 3. File map

| Path | Purpose |
|---|---|
| `cmd/painkiller/main.go` | The entire server: state machine, HTTP API, WebSocket terminal, grading, lab orchestration, SQLite persistence. |
| `cmd/painkiller/main_test.go` | 42 tests (state transitions, grading, partial credit, fixtures for every task, snapshot immutability/privacy, concurrent capture, lab-script guards, retry prepare/reset, restart resume, expiry, persistence failure). |
| `web/index.html` | Single-page UI (home / exam / results). |
| `web/app.js` | Frontend logic (status polling, timer, marks, terminal, results). |
| `web/style.css` | Styling. |
| `web/embed.go` | `//go:embed` of the bundled frontend. |
| `web/vendor/` | Bundled xterm.js + fit addon + licenses (no CDN). |
| `exam/tasks.json` | The exam: 29 tasks, 100 points. Schema in §5. |
| `exam/setup.yaml` | Pre-seeded objects: namespaces `exam-workloads`/`exam-repair` (marker label on `exam-repair`), `exam-material` ConfigMap, `exam-backend` Service, `service-target`/`rollout` Deployments, and the ServiceAccounts/Role used by the RBAC tasks. Intentional faults come last: `broken-web` (bad image), `broken-endpoints` (wrong selector), `crashing` (exit 1). |
| `exam/reference-solutions.sh` | Applies all correct answers. **Refuses to run unless the `exam-repair` marker label exists.** Used only for validation. |
| `scripts/create-lab.sh` | Proxmox lab lifecycle: `create`, `status`, `provision`, `start`, `stop`, `destroy`. |
| `scripts/configure-exam-access.sh` | Creates a lab-only SSH key on the controller, installs it on workers, writes `~/.ssh/config` include + known_hosts so the candidate can `ssh <worker-ip>` without prompts. |
| `scripts/e2e/` | Committed Playwright harness: `live.js` (full scoring flow), `terminal.js`, `expiry.js`, `lib.js`, README. |
| `go.mod` / `go.sum` | Go module deps (see §7). |
| `README.md` | User-facing docs (prereqs, safety, env vars, run). |
| `AGENTS.md` | Project directives + Proxmox/template facts. |
| `.opencode/opencode.json` | OpenCode references (includes `lab-ansible-playbooks`). |

---

## 4. Architecture (key symbols in `cmd/painkiller/main.go`)

### 4.1 Attempt state machine
Two layer: **lab phase** and **exam phase** share one `phase` string.

```
idle ──prepare──▶ preparing ──ok──▶ ready ──start──▶ running ──submit/expire──▶ grading ──▶ completed
  ▲                   │error                                                                      │
  └──────────reset────┴──▶ resetting ──ok──▶ idle            (completed) ──reset──▶ resetting ──▶ idle
```

- `state` struct: `Phase`, `Deadline`, `Results`, `Flags`, `Done`, `Selected`,
  `Message`, `Host` (controller IP), `FailedOperation`, `Answers`.
- `phase` values: `idle | preparing | ready | running | grading | completed | resetting | error`.
- `FailedOperation` ∈ `prepare | reset | persistence` (drives the retry button).
- `expire()` runs every second; if `running` and past deadline → `grading` (fails
  closed: if persistence fails it stays `running` and retries next tick).

### 4.2 Grading (`grade()`)
- Captures each task's `resource` once via `ssh <user>@<host> <resource>`
  (30 s timeout, one retry on transient failure, `(NotFound)` treated as an
  incorrect answer not an infra error).
- Captures run **concurrently** (`captureConcurrency = 6`), so the live cluster
  is read within a few seconds instead of one task after another. Answers are
  persisted after each capture pass; a failed capture is retried once and
  successful ones are never re-read.
- Stores raw JSON in `state.Answers[id] = answerSnapshot{Data, Error}` (so
  retries never recapture successful ones).
- Evaluates each check locally with
  `jq -e 'try ((<expression>) // false) catch false'` on the stored JSON, so a
  missing path scores zero instead of looking like a grader failure.
- `result.Earned` = sum of passing check points; `result.Checks[]` holds per-check
  detail; `result.Error` marks infrastructure/grader failures; totals with errors
  are shown as "Provisional".
- `/api/status` omits `Answers` (may contain secrets).
- **Limitation:** capture is concurrent but not a transactional cluster snapshot.

### 4.3 HTTP API
- `GET /api/status` — public state (no answers).
- `GET /api/log` — last ~32 KiB of `data/operation.log`.
- `GET /api/tasks` — tasks for `running`/`completed`; solutions/explanations
  only when `completed`.
- `GET /api/terminal` (WebSocket) — only when `running` and before deadline.
- `POST /api/<action>` (same-origin required):
  `prepare | retry | start | submit | regrade | reset | mark | select`.
  - `retry` only when `phase=error` and `failed_operation` is `prepare` or
    `reset`. A prepare retry re-runs `status → provision → configure → setup`
    **without** recreating VMs; a reset retry re-runs the destroy.
  - `mark` updates one task's flag/done (does not clobber others).
  - `select` persists the current question id.

### 4.4 Persistence & concurrency
- SQLite (`mattn/go-sqlite3`, **CGO required**), single-row `attempt(id=1,state)`.
- `data/state.db` (0600). Legacy `data/state.json` imported once if present.
- `flock` on `data/state.db.lock` → single server per state.
- `flock` on `<lockdir>/lab-server.lock` → single server per shared lab
  (`PAINKILLER_LAB_LOCK_DIR`, default `data`).
- Each script invocation is wrapped in `flock -n -E 75 <lockdir>/lab-operation.lock`
  so an orphaned provisioning run blocks a new one (exit 75 → "Another lab
  operation is still running").

### 4.5 Terminal
- WebSocket → `pty.Start(ssh -tt ...)` per session; PTY resize via
  `{cols,rows}` text frames; killed on submit/expire via `closeTerminals()`.

---

## 5. Data schemas

### `exam/tasks.json` entry
```json
{
  "id": "configmap",
  "title": "Create a ConfigMap",
  "points": 6,
  "instructions": "…",
  "resource": "kubectl -n exam-workloads get configmap settings --ignore-not-found -o json",
  "checks": [
    {"description": "ConfigMap data", "points": 6, "expression": ".data.mode == \"production\""}
  ],
  "solution": "kubectl -n exam-workloads create configmap settings --from-literal=mode=production",
  "explanation": "…"
}
```
- `points` must equal the sum of `checks[].points`.
- `resource` is captured once; each `checks[].expression` is a jq boolean.
- The Go `task` type also supports a legacy `check`/`checks[].command` (ssh) mode,
  but **all current tasks use `resource` + `expression`**.

### Current 29 tasks (100 points)
`namespace (2), node-label (2), node-placement (4), static-pod (4),
etcd-snapshot (4), pod (2), deployment (4), rollout (4), multicontainer (5),
resources (4), limits (4), priorityclass (3), job (2), cronjob (3),
configmap (2), secret (3), serviceaccount (2), pvc-binding (6),
mount-config (3), service (3), nodeport (3), ingress (5), networkpolicy (3),
coredns (3), repair (4), svc-selector (4), crashloop (4), rbac (4),
cluster-rbac (4)`

Multi-object tasks use one capture over a resource list
(`kubectl get pv,pvc -o json`, `kubectl get role,rolebinding -o json`, ...) and
select items by `kind`/`metadata.name`, so a single captured document grades
several objects without another round trip.

---

## 6. Environment / infrastructure facts

- **Management host** runs the Go server; needs Go (CGO + C compiler), Ansible,
  `community.general`, `ssh`, `git`, `jq`, `flock`, GNU `timeout`, `sqlite3` optional.
- **Proxmox host:** `root@prox-k12` (SSH config resolves to
  `prox-k12.nowhereworks.local` = `192.168.254.31`). The `.nowhereworks.local`
  DNS server is intermittently unavailable → transient "Could not resolve
  hostname" errors. `remote()` in `create-lab.sh` now retries **only** such
  connection errors (never command errors, so `qm clone` is never repeated).
- **Template `9010`** is pre-configured (Ubuntu 24.04, cloud-init, ciuser
  `milkman`, authorized SSH key, DNS, DHCP). Linked clones get unique MACs,
  hostnames, SSH host keys, and DHCP IPs; the machine-id is shared but benign.
- **Lab VM range: IDs 3000–3004 only.** Default `3000–3002` = 1 controller +
  2 workers. Never touch the template or any VM outside this range.
- **Per-VM memory:** `LAB_VM_MEMORY=3072` MiB; create requires +4 GiB host headroom.
- **Ansible playbooks:** `/home/milkman/repo/_nowhereworks/lab-ansible-playbooks`,
  pinned rev `8ec676e93017ce3e110c27485e25d1f32d541056`,
  `kubeadm-cluster/kubeadm-cluster.yml` (k8s 1.36.x from `pkgs.k8s.io`, Flannel).
- **Provision step** also: sets NOPASSWD sudo for `milkman` on lab VMs, installs
  admin kubeconfig to `~/.kube/config` on the first controller and `jq` if
  missing, waits for all nodes Ready.
- **Candidate SSH:** `configure-exam-access.sh` creates a dedicated key on the
  controller, installs its public key on workers, and writes an SSH `Include`
  config + lab known_hosts so `ssh <worker-ip>` works without prompts.

---

## 7. Run / verify

### Run
```sh
# from repo root
go run ./cmd/painkiller
# open http://127.0.0.1:8080
```
Flow: **Prepare lab** (create → provision → configure access → apply setup) →
**Start exam** → work in terminal → **Submit** → results → **Reset**.
On a transient prepare failure, use **Retry operation**.

### Checks
```sh
go test -race ./...
go vet ./...
shellcheck scripts/create-lab.sh scripts/configure-exam-access.sh exam/reference-solutions.sh
node --check web/app.js
```

### Env vars
| Var | Default | Purpose |
|---|---|---|
| `PAINKILLER_LISTEN` | `127.0.0.1:8080` | HTTP listen |
| `PAINKILLER_STATE` | `data/state.db` | SQLite attempt DB |
| `PAINKILLER_MINUTES` | `120` | Exam duration |
| `PAINKILLER_LAB_LOCK_DIR` | `data` | Shared lab lock dir |
| `LAB_PROXMOX_HOST` | `root@prox-k12` | Proxmox SSH target |
| `LAB_TEMPLATE_ID` | `9010` | VM template |
| `LAB_VM_MEMORY` | `3072` | MiB per clone |
| `LAB_VM_START`,`LAB_VM_END` | `3000`,`3002` | Range (must be within 3000–3004) |
| `LAB_CONTROLLERS` | `1` | Control-plane count |
| `LAB_MARKER` | `painkiller-mvp` | Required VM description |
| `LAB_IP_PREFIX` | `192.168.254.` | DHCP subnet prefix |
| `LAB_READY_TIMEOUT` | `600` | Per-VM readiness timeout (s) |
| `LAB_SSH_USER` | `milkman` | Node SSH user |
| `LAB_KNOWN_HOSTS` | `data/lab_known_hosts` | Managed guest host keys |
| `LAB_PLAYBOOKS_DIR` | `/home/milkman/repo/_nowhereworks/lab-ansible-playbooks` | Playbooks |
| `LAB_PLAYBOOKS_REV` | `8ec676…` | Pinned revision |
| `LAB_KUBERNETES_VERSION` | `1.36.1` | kubeadm version (apt resolves to latest patch) |
| `LAB_CNI_PLUGIN` | `flannel` | CNI |
| `LAB_CONTROL_PLANE_ENDPOINT` | unset | HA endpoint |

### E2E validation harness (committed in `scripts/e2e/`)
Uses Playwright-core + local Chrome; `npm --prefix scripts/e2e install` once.
- `live.js`: prepare → start exam → flag → reload → baseline 0 → apply
  `exam/reference-solutions.sh` → 100 → delete one answer → 98 → restore →
  browser submit → expect "100 / 100" → duplicate submit 409 → terminal 409 →
  reset.
- `terminal.js`: interactive echo + reconnect (timer preserved).
- `expiry.js`: auto-grade at the deadline with `PAINKILLER_MINUTES=1`.

Expected values: baseline `0`, reference `100`, one-wrong `98`, submit `100`.
Failure paths are covered by Go tests (see `scripts/e2e/README.md` for the map).

### Remaining work

All MVP items from the original plan are complete:
- **8.0** E2E harness committed to `scripts/e2e/`.
- **8.1** Exam expanded to 29 tasks / 100 points across the five CKA domains,
  with setup objects, faults injected last, reference solutions gated by the
  `exam-repair` marker, and a correct fixture for every task
  (`TestFixturesScoreEveryTask`, plus partial-credit cases).
- **8.2** Capture runs concurrently (6 at a time) with snapshot-once semantics;
  remaining non-atomicity documented in README and §10.
- **8.3** Failure/recovery paths automated: retry preparation, retry reset,
  duplicate start/submit, restart resume for preparing/resetting/running/grading,
  expiry, grading persistence failure, terminal reconnect (e2e).
- **8.4** Submit captures immediately after closing terminals, so later changes
  to the cluster do not alter a submitted attempt; checks are documented in
  README. No local CI script was added (plain `go test`/`shellcheck` commands).

Optional future work: Calico for enforced NetworkPolicy, a second exam variant,
authentication if access expands beyond one trusted user.

---

## 9. Lessons learned / do not repeat

- **Do not** manipulate clone `net0`, `cloud-init`, machine-id, or SSH host keys.
  The template is prepared; `create` must only do linked clones + start.
- **Do not** do offline-boot / hard-`qm reset` identity gymnastics — it fights
  cloud-init and produces zero-byte SSH keys and duplicate IDs. (Caused a long
  debug loop; the shared machine-id is benign.)
- **Use linked clones** (`qm clone … --full 0`). ~30 s to ready.
- **Wait for cloud-init and the guest package manager before provisioning.**
  Fresh clones run `apt-get` from cloud-init and hold the dpkg lock, so a
  playbook started immediately fails with "Could not get lock
  /var/lib/dpkg/lock-frontend". `provision` now runs `cloud-init status --wait`
  and polls `pgrep apt-get|dpkg` on every VM first.
- Proxmox hostname resolves via `.nowhereworks.local`, which blips; `remote()`
  retries connection-only failures. Do not add blanket retries (would re-run
  `qm clone`).
- `etcd-client` is installed by provisioning on the first controller, so the etcd
  snapshot task is solvable (etcdctl 3.4.30 reads etcd 3.6 snapshots).
- `data/` is git-ignored; state/kubeconfigs/host-keys live there.
- CGO is required (sqlite3). `jq` is used on the management host for grading.

---

## 10. Known limitations (be honest)

- Answer capture is concurrent (six at a time) but not a transactional snapshot.
- Not tamper-proof: a candidate can leave background processes on nodes.
- Flannel does not enforce NetworkPolicy (policy task is resource-only).
- Transient DNS blips on prox-k12 (mitigated by connection retry + Retry button).
- Single-user, no auth — trusted host/network only.
- Exam content is 29 tasks; it covers the domains but is not an official CKA
  exam format or a full question bank.

---

## 11. Decisions (resolved)

1. **CNI for the exam:** Flannel, NetworkPolicy graded as configuration only
   (documented in the task instructions).
2. **Exam size/target:** one 100-point exam with 29 tasks.
3. **Answer-capture atomicity:** concurrent capture with snapshot-once
   semantics; no freeze. Remaining non-atomicity documented.
4. **E2E harness location:** `scripts/e2e/`.
