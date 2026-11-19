# Test organization

Each test lives in `exam/<ID>/`, using an uppercase family and three-digit
number with an optional single uppercase suffix (for example `CKA-001` or
`CKS-001B`). The folder name must match `test.json`'s `id`.
Keep all test-specific files, assets, and documentation inside that folder.
Shared server code, lifecycle scripts, and provisioning playbooks stay shared.

Required files:

| File | Purpose |
| --- | --- |
| `test.json` | `id`, `title`, `description`, `minutes` (1–1440), and allowed `categories` |
| `tasks.json` | Questions, points, walkthroughs, solutions, capture commands, and checks |
| `setup.yaml` | Kubernetes resources applied after provisioning |
| `reference-solutions.sh` | Reference answers for validation on a disposable lab |
| `check-ready.sh` | Fast, read-only verification of the intended initial exam state |

Use `CKA-001` as the starting example. The server discovers and validates
folders at startup; adding a test requires no server or UI code changes.
Restart after adding or editing definitions. Invalid definitions fail startup.
Each question has a stable `question_id` such as `CKA-001-Q1` and a `category`
from its exam's `categories`. IDs must use the owning exam ID followed by `-Q`
and a positive integer, be unique within the exam, and never be renumbered or
reused after removal. The existing `id` remains the internal grading and
progress key. Assign CKA questions their primary official domain from
`docs/cka-requirements.md`; other exam families own their category lists.
Test files contain trusted code executed on the lab; do not accept untrusted
uploads. Reference scripts should refuse to run on an unmarked cluster.

Question instructions must state the required outcome, resource names,
namespaces, file locations, and any prerequisite questions. Identify the host
where local files and commands belong. When a task requires another machine,
provide its connection command in the question and say how to return to the
initial controller; do not rely on the walkthrough for essential information.
For topology-specific questions, `{{controller2}}`, `{{controller3}}`,
`{{worker1}}`, and `{{worker2}}` in instructions or steps render as the
attempt's hostname (when available), IP, and an IP-based SSH command. These
roles follow the ordered lab topology, so only use them when the exam owns that
role assignment. Older attempts without a node snapshot show discovery
instructions instead. Do not use these placeholders in grading commands.

The first screen lists available tests. Preparing binds the selected test to
the current attempt, including setup, questions, and grading. Only one lab and
attempt are active at a time; reset before choosing another test. Restarts
restore the selected test, and attempts predating test selection belong to
`CKA-001`. Do not remove or edit a test while its attempt is active.

The mode is bound to the attempt at preparation. Simulation automatically
submits at the time limit; Learning continues into overtime and can verify an
individual question without submitting or saving scores. Older attempts default
to Simulation. Verification uses the same trusted capture and check commands as
grading, so exam authors must keep these commands read-only and bounded; HTTP and
network probes may observe or contact supporting services.

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
  "endpoint": "cka002-api:8443",
  "memory_mb": 4096,
  "template_id": "9198",
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
Endpoints accept IP addresses or lowercase DNS hostnames. An exam using a
lab-local hostname must make it resolve on every guest before bootstrap.
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
5. `check-ready.sh` (60 seconds): verify the selected exam's initial state before
   allowing Start; a failure is logged and leaves preparation in error.

Each invocation is logged through the existing operation log. Preparation scripts
must use explicit readiness checks and exam-specific phase markers, reconcile on
retry, and never repeatedly inject faults or reset a healthy cluster. A trusted
preparation script owns applying its `setup.yaml`; exams without `prepare` retain
the original setup.yaml-only path. `scripts/create-lab.sh prerequisites` prepares
all guests without joining any; `LAB_INITIAL_CONTROLLERS` reserves remaining
controllers without treating them as workers. CKA-002's API-proxy helper uses
ownership-checked guest addresses and configures one worker plus guest-local name
resolution; its fixture phase preserves completed candidate work.

Every exam must own its dependencies, fixtures, faults, grading, and reference
answers. Do not invoke another exam's setup, assets, or solutions.
Each readiness check is exam-owned, read-only, uses the saved lab environment and
student access, and checks essential infrastructure and intentional faults rather
than grading answers. Keep it fast, with bounded retries for convergence. Update
it whenever the expected starting state changes.

## Fixture and grading pitfalls

- A volume mount hides the image's contents at that path. Initialize required
  subdirectories inside writable volumes before application startup; for example,
  mounting `/var/log` hides nginx's bundled `/var/log/nginx`. Verify application
  readiness and Deployment rollout, not merely a Pod briefly reaching Running.
- Runtime mountpoints can differ from manifest paths because of symlinks.
  Resolve paths inside the actual container before comparing mount flags:
  `/var/run` may appear as `/run` in `/proc/1/mountinfo`. Do not assume aliases
  are equivalent without checking the container's symlink.
- For independently refreshed evidence, read the evidence before sampling the
  current time for freshness validation. Sampling time at capture startup can
  falsely classify an update during API reads as future-dated. Match evidence
  to the observed resource UID/version or generation as well as its age.
- Falco's first matching rule can shadow later local rules. When an exam must
  observe overlapping detections without changing distribution rules, deliberately
  configure and validate matching behavior (CKS-001B uses `rule_matching: all`).
  Check real benign execution events, not just valid YAML.
- An IngressClass name or annotation does not establish controller support.
  Verify the installed controller's actual provider and use an independent
  certificate-verified TLS/redirect canary. Traefik's ordinary Kubernetes Ingress
  provider is not its Kubernetes Ingress NGINX compatibility provider.

See `CKS-001B/README.md` for the concrete fixes and assessment boundaries.
