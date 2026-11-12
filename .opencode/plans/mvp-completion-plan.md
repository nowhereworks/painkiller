# Painkiller Shell — MVP Completion Plan

> **Restart prompt for a fresh session:** Read this file fully, then continue
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

## 2. Current status (as of commit `40729e3`, pushed to `origin/main`)

### Done and validated end-to-end on a real 3-node Proxmox lab
- Linked-clone lab creation: 3 VMs reachable in **~31 s**.
- kubeadm provisioning: 3 nodes **Ready** (Kubernetes v1.36.6, Flannel, containerd 2.2.1).
- Timed exam UI: question navigation, flags, "mark complete", timer, copy instructions.
- Browser SSH terminal (xterm.js) with input, reconnect, and resize.
- Submit → snapshot grading → results with per-check partial credit.
- Reset (destroy lab) and re-run.

### Live validation results (recorded)
| Stage | Result |
|---|---|
| Baseline (fresh lab) | **0 / 100** |
| Reference solutions applied | **100 / 100** |
| One answer deliberately removed | **94 / 100** |
| Browser submit (graded) | **100 / 100** |
| Reset | complete |

### Automated tests
- **35 Go tests** pass: `go test -race ./...`
- `go vet ./...`, `shellcheck scripts/*.sh exam/reference-solutions.sh`,
  `node --check web/app.js`, YAML/JSON validation.

### What is **not** done (the remaining work — see §8)
1. Exam content is 17 representative tasks, not genuinely full CKA domain depth.
2. Answer capture is **sequential**, not atomic.
3. Comprehensive failure/recovery test matrix not fully exercised on a real lab.

---

## 3. File map

| Path | Purpose |
|---|---|
| `cmd/painkiller/main.go` | The entire server: state machine, HTTP API, WebSocket terminal, grading, lab orchestration, SQLite persistence. |
| `cmd/painkiller/main_test.go` | 35 tests (state transitions, grading, partial credit, snapshot immutability/privacy, lab-script guards, retry, concurrency, DNS-retry, fixtures). |
| `web/index.html` | Single-page UI (home / exam / results). |
| `web/app.js` | Frontend logic (status polling, timer, marks, terminal, results). |
| `web/style.css` | Styling. |
| `web/embed.go` | `//go:embed` of the bundled frontend. |
| `web/vendor/` | Bundled xterm.js + fit addon + licenses (no CDN). |
| `exam/tasks.json` | The exam: 17 tasks, 100 points. Schema in §5. |
| `exam/setup.yaml` | Pre-seeded cluster objects (`exam-workloads`, `exam-repair/broken-web` fault, `service-target`, `binding-observer`, `binding-reader`). `exam-repair` has label `painkiller-shell/exam: "true"` (marker used by reference solutions). |
| `exam/reference-solutions.sh` | Applies all correct answers. **Refuses to run unless the `exam-repair` marker label exists.** Used only for validation. |
| `scripts/create-lab.sh` | Proxmox lab lifecycle: `create`, `status`, `provision`, `start`, `stop`, `destroy`. |
| `scripts/configure-exam-access.sh` | Creates a lab-only SSH key on the controller, installs it on workers, writes `~/.ssh/config` include + known_hosts so the candidate can `ssh <worker-ip>` without prompts. |
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
- Captures each task's `resource` **once** via `ssh <user>@<host> <resource>`
  (30 s timeout, one retry on transient failure, `(NotFound)` treated as an
  incorrect answer not an infra error).
- Stores raw JSON in `state.Answers[id] = answerSnapshot{Data, Error}` and
  persists after each capture (so retries never recapture successful ones).
- Evaluates each check locally with `jq -e 'try (<expression>) catch false'` on
  the stored JSON.
- `result.Earned` = sum of passing check points; `result.Checks[]` holds per-check
  detail; `result.Error` marks infrastructure/grader failures; totals with errors
  are shown as "Provisional".
- `/api/status` omits `Answers` (may contain secrets).
- **Limitation:** capture is sequential, not an atomic cluster snapshot.

### 4.3 HTTP API
- `GET /api/status` — public state (no answers).
- `GET /api/log` — last ~32 KiB of `data/operation.log`.
- `GET /api/tasks` — tasks for `running`/`completed`; solutions/explanations
  only when `completed`.
- `GET /api/terminal` (WebSocket) — only when `running` and before deadline.
- `POST /api/<action>` (same-origin required):
  `prepare | retry | start | submit | regrade | reset | mark | select`.
  - `retry` only when `phase=error && failed_operation=prepare`; re-runs
    `status → provision → configure → setup` **without** recreating VMs.
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

### Current 17 tasks (all 6 pts except `cronjob` = 4; total 100)
`namespace, pod, deployment, service, configmap, secret, serviceaccount, quota,
job, label, pv, pvc, networkpolicy, role, rolebinding, repair, cronjob`

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
- **Ansible playbooks:** `$HOME/repo/_nowhereworks/lab-ansible-playbooks`,
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
On a transient prepare failure, use **Retry preparation**.

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
| `LAB_PLAYBOOKS_DIR` | `$HOME/repo/_nowhereworks/lab-ansible-playbooks` | Playbooks |
| `LAB_PLAYBOOKS_REV` | `8ec676…` | Pinned revision |
| `LAB_KUBERNETES_VERSION` | `1.36.1` | kubeadm version (apt resolves to latest patch) |
| `LAB_CNI_PLUGIN` | `flannel` | CNI |
| `LAB_CONTROL_PLANE_ENDPOINT` | unset | HA endpoint |

### E2E validation harness (currently **ephemeral**, in `/tmp/opencode/painkiller-test/`)
Uses Playwright-core + local Chrome. Recreate/commit these **first** (see §8.0):
- `live.js`: ready → start exam → flag → reload → baseline 0 → apply
  `exam/reference-solutions.sh` → 100 → delete one answer → 94 → restore →
  browser submit → expect "100 / 100" → reset.
- `terminal.js`: interactive echo + reconnect.
- `expiry.js`: past-deadline attempt auto-grades after restart.
- `copy.js`, `browser.js`, `retry.js`, `verify-access.js`, `identity.js`.

Expected values: baseline `0`, reference `100`, one-wrong `94`, submit `100`.

---

## 8. Remaining work (ordered)

### 8.0 Commit the E2E harness (prerequisite)
The Playwright harness in `/tmp/opencode/painkiller-test/` is outside the repo
and will be lost. Move it into e.g. `scripts/e2e/` (or `testdata/e2e/`) with a
short README, or inline the checks as runnable commands. Goal: reproducible
end-to-end validation from a clean context.
- **Accept:** `scripts/e2e/` exists, documented, and re-runs the §7 E2E flow.

### 8.1 Exam content depth & coverage (largest gap)
Current 17 tasks are representative but shallow. Expand to genuinely cover the
CKA domains, keeping **100 points** total and the `resource`+`expression` model.
Domains to strengthen/add (each with setup, instructions, partial-credit checks,
solution, explanation):
- **Cluster architecture/install/config:** kubeadm upgrade (plan/apply), etcd
  snapshot + restore, static Pod, kubelet config, node drain/cordon/uncordon,
  taint/toleration, `kubectl` context/namespace switching.
- **Workloads & scheduling:** Deployment rollback/strategy, init containers,
  sidecars, multi-container Pod, resource requests/limits, LimitRange,
  PriorityClass, HPA (metrics-server), Job/CronJob completion semantics.
- **Services & networking:** ClusterIP/NodePort, Endpoints/EndpointSlice,
  Ingress, NetworkPolicy **enforcement** (requires Calico — or document the
  Flannel limitation and keep policy tasks resource-only), CoreDNS.
- **Storage:** PV/PVC **binding** (volumeName, storageClassName), PV reclaim
  policy, access modes, mounting ConfigMap/Secret as volumes, subPath.
- **Troubleshooting:** broken kubelet/manifest, CrashLoopBackOff with real fix,
  Service selector mismatch, image pull failure, node NotReady recovery.
- **RBAC:** ClusterRole + ClusterRoleBinding, `kubectl auth can-i` verification.

Design rules:
- Tasks must be **independent** where possible; intentional faults injected
  **last** in `exam/setup.yaml`.
- Graders check resulting **state**, not command history; no mutation.
- Keep `exam/reference-solutions.sh` in sync and gated by the `exam-repair` marker.
- **Accept:** `validateTasks` passes (100 pts), reference solutions score 100,
  empty state scores 0, and a per-task wrong-answer sweep loses exactly the
  expected points. Extend `TestExamChecksAcceptCorrectFixtures` with fixtures
  for every new task.
- **Caveat:** Flannel does not enforce NetworkPolicy. Either switch `LAB_CNI_PLUGIN`
  to `calico` for the exam or keep policy checks configuration-only (current
  approach) and label the task accordingly.

### 8.2 Atomic answer capture
Replace the sequential per-task capture with a capture that minimizes the
window for background processes to change answers.
- Option A (preferred): capture all relevant objects in **one pass** (e.g., a
  single `kubectl get` over the exam namespaces/kinds → one JSON document),
  then evaluate parts with jq.
- Option B: freeze first (close terminals, optionally `systemctl stop kubelet`
  is too invasive) then capture quickly; document residual risk.
- Keep the existing "capture once, retry only failures, never recapture
  successes" behavior and privacy (never expose `Answers`).
- **Accept:** a background process racing capture cannot change an answer that
  was already captured; document any remaining non-atomicity.

### 8.3 Comprehensive failure/recovery testing (real lab)
Add e2e coverage for:
- Provisioning fails mid-run → `phase=error`, **Retry preparation** reuses VMs.
- Terminal disconnect mid-exam → reconnect resumes a shell (attempt time preserved).
- Duplicate start/submit clicks → 409, no double grading.
- Failed cleanup (destroy partially fails) → `error` + retry reset.
- Service restart during `running` and during `grading` → state preserved;
  grading resumes on boot.
- Expiry during an active session → auto-grade, terminals closed.
- **Accept:** each path has an automated test (Go where possible, e2e otherwise)
  and passes on the real 3-node lab.

### 8.4 Optional hardening (only if cheap)
- Prevent answer changes after submit (capture immediately; consider snapshot
  of lab state / stop candidate-accessible processes — document trade-offs).
- Local CI script that runs all checks.

---

## 9. Lessons learned / do not repeat

- **Do not** manipulate clone `net0`, `cloud-init`, machine-id, or SSH host keys.
  The template is prepared; `create` must only do linked clones + start.
- **Do not** do offline-boot / hard-`qm reset` identity gymnastics — it fights
  cloud-init and produces zero-byte SSH keys and duplicate IDs. (Caused a long
  debug loop; the shared machine-id is benign.)
- **Use linked clones** (`qm clone … --full 0`). ~30 s to ready.
- Proxmox hostname resolves via `.nowhereworks.local`, which blips; `remote()`
  retries connection-only failures. Do not add blanket retries (would re-run
  `qm clone`).
- `data/` is git-ignored; state/kubeconfigs/host-keys live there.
- CGO is required (sqlite3). `jq` is used on the management host for grading.

---

## 10. Known limitations (be honest)

- Answer capture is sequential (not atomic).
- Not tamper-proof: a candidate can leave background processes on nodes.
- Flannel does not enforce NetworkPolicy (policy task is resource-only).
- Transient DNS blips on prox-k12 (mitigated by connection retry + Retry button).
- Single-user, no auth — trusted host/network only.
- Exam content is 17 tasks, not full CKA depth.
- No official CKA exam format claim.

---

## 11. Open questions / decisions

1. **CNI for the exam:** switch to Calico to make NetworkPolicy tasks enforceable,
   or keep Flannel and keep policy checks configuration-only? (Current: Flannel,
   configuration-only.)
2. **Exam size/target:** keep one 100-point exam and deepen it, or add a second
   variant? (MVP plan: one exam.)
3. **Answer-capture atomicity:** Option A single-pass capture vs. Option B
   freeze-then-capture — pick before implementing §8.2.
4. **E2E harness location:** `scripts/e2e/` vs `testdata/e2e/`.
