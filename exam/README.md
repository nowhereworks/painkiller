# Test organization

Each test lives in `exam/<ID>/`, using an uppercase family and three-digit
number (for example `CKA-001`). The folder name must match `test.json`'s `id`.
Keep all test-specific files, assets, and documentation inside that folder.
Shared server code, lifecycle scripts, and provisioning playbooks stay shared.

Required files:

| File | Purpose |
| --- | --- |
| `test.json` | `id`, `title`, `description`, and `minutes` (1–1440) |
| `tasks.json` | Questions, points, walkthroughs, solutions, capture commands, and checks |
| `setup.yaml` | Kubernetes resources applied after provisioning |
| `reference-solutions.sh` | Reference answers for validation on a disposable lab |

Use `CKA-001` as the starting example. The server discovers and validates
folders at startup; adding a test requires no server or UI code changes.
Restart after adding or editing definitions. Invalid definitions fail startup.
Test files contain trusted code executed on the lab; do not accept untrusted
uploads. Reference scripts should refuse to run on an unmarked cluster.

The first screen lists available tests. Preparing binds the selected test to
the current attempt, including setup, questions, and grading. Only one lab and
attempt are active at a time; reset before choosing another test. Restarts
restore the selected test, and attempts predating test selection belong to
`CKA-001`. Do not remove or edit a test while its attempt is active.

Duration comes from `test.json`; `PAINKILLER_MINUTES`, when set, overrides it.
Without `lab`, tests retain the configured lab topology and provisioning
defaults (one controller and two workers, prebaked template 9100).

## Optional exam-owned lab contract

An exam may declare requirements in `test.json`:

```json
"lab": {
  "controllers": 3,
  "workers": 2,
  "initial_controllers": 2,
  "provision_mode": "prebaked",
  "cni": "calico",
  "endpoint": "192.168.254.80:8443",
  "memory_mb": 4096,
  "template_id": "9200",
  "prepare": "prepare.sh"
}
```

All fields except `endpoint`, `template_id`, and `prepare` are required when
`lab` is present. `memory_mb` is a minimum per VM. Explicit conflicting topology,
mode, CNI, endpoint, range end, or insufficient memory environment settings fail
selection; they never silently replace exam requirements. The VM range end is
derived from `LAB_VM_START` and the exam's total machine count. Proxmox host, SSH
user, VM range start, ownership marker, and template ID remain
operator-configurable; `template_id` (9100–9200) is the exam's default image and
is required for a prebaked multi-controller topology. Legacy mode defaults to
template 9010. Prebaked mode supports one controller with Flannel and no
endpoint, or three controllers with Calico and a stable control-plane endpoint.
The catalog publishes only controller/worker counts, not the trusted preparation
path or endpoint.

New attempts persist their resolved operational environment. Creation, retries,
status, guest access, grading, and reset use that snapshot even after a server
restart or environment change. Older attempts without a snapshot retain legacy
environment-based behavior; do not change their lab environment before reset.
An all-range read-only preflight checks IDs, template, host keys, and memory
before creation is authorized and persisted. Reset after a failed preflight
clears only the attempt; it cannot destroy an existing lab with the same marker.

`prepare` names a regular shell script inside the exam directory. Traversal,
absolute paths, shell metacharacters, and symlink escapes are rejected. The server
invokes it using argument arrays, not shell interpolation, with these phases:

1. `preflight` (180 seconds): read-only prerequisite/allocation checks, before cloning.
2. `infrastructure` (1800 seconds): guest prerequisites and pre-bootstrap services.
3. Shared cluster bootstrap and candidate SSH access to **all** machines.
4. `fixtures` (2400 seconds): exam dependencies, assets, readiness, then faults.

Each invocation is logged through the existing operation log. Preparation scripts
must use explicit readiness checks and exam-specific phase markers, reconcile on
retry, and never repeatedly inject faults or reset a healthy cluster. A trusted
preparation script owns applying its `setup.yaml`; exams without `prepare` retain
the original setup.yaml-only path. `scripts/create-lab.sh prerequisites` prepares
all guests without joining any; `LAB_INITIAL_CONTROLLERS` reserves remaining
controllers without treating them as workers. The API-balancing helper configures
only ownership-checked workers and requires the explicit HA lab configuration.

Every exam must own its dependencies, fixtures, faults, grading, and reference
answers. Do not invoke another exam's setup, assets, or solutions.
