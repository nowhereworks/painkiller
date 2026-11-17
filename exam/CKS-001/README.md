# CKS-001

Unofficial 120-minute, 15-task, 100-point practical mock based on
`docs/cks-mock-01.md`. Keep the source's task order, weights and core requests.
Names, paths, UID, audit rotation numbers, review fixtures and scenario prose
are adapted; do not expand this mock into a different assessment.

## Lab and ownership

One **schedulable control plane, no workers**, minimum 8192 MB, linked clone of
base `9199`, full Kubernetes install (`legacy` provisioning mode) with Calico.
Controller-manager and scheduler use `--leader-elect=false`: there is only one
controller, and repeated assessed API-server reloads must not cause lease-loss
exits and exponential restart backoff in unrelated tasks.
The disposable API-server Pod has a five-second termination grace period so
these reloads do not exhaust node-heartbeat grace and temporarily mark otherwise
healthy fixtures NotReady. This is lab-only tuning, not production HA guidance.
The controller's node-monitor grace is 120 seconds to tolerate heartbeat/relist
recovery during those planned API reloads without falsely marking fixtures down.
No reserved LAN addresses, external services, HA, Docker daemon or new template.
The shared bootstrap currently defaults to Kubernetes 1.36.1; an incompatible
operator override must fail readiness rather than silently change tasks.
All required assets are exam-owned. Other exams are not imported or changed.

Preparation verifies AppArmor and kernel BTF support before cluster bootstrap.
It installs real Falco, kube-bench and bom, then an in-cluster TLS ImageReview
service and a single Traefik Deployment. The `nginx` IngressClass intentionally
belongs to Traefik, as in the source. Each setup phase has an exam-owned marker;
completed preparation must preserve student work on retry.

All commands belong on the controller. The candidate has passwordless sudo;
root-owned answers explicitly use sudo. `~` refers to the candidate's home,
not `/root`. Supporting probes and captures use the controller's admin
kubeconfig. Q14 and Q15 edit the **same** API-server manifest but neither
depends on the other; their solutions preserve unrelated flags and mounts.
Backups stay outside `/etc/kubernetes/manifests/`.

## Dependencies and sources

Sources checked 2026-11-17:

| Dependency | Version / source | Use |
| --- | --- | --- |
| Falco | 0.45.2, `download.falco.org/packages/bin/x86_64/`; bundled container plugin 0.7.6 | modern eBPF syscall monitoring; no kernel module |
| Falco rules | `falcosecurity/rules`, `falco-rules-5.2.0` | Stable + incubating + sandbox sets; the source's `Write below binary dir` is now in sandbox |
| kube-bench | `aquasecurity/kube-bench` v0.16.0 | Original `cis-1.10` benchmark, only five named checks assessed |
| bom | `kubernetes-sigs/bom` v0.8.0 | SPDX JSON from a real Docker image archive |
| Traefik | `traefik:v3.6.0` | Single TLS ingress controller |
| ImageReview service | Exam-owned `assets/webhook.py`, Python 3.12.12 Alpine 3.22 | Reject explicit and implicit latest across repositories |

Downloaded tools/rules have SHA256 pins in `assets/install.sh`. Exam code has
`assets/SHA256SUMS`. Container fixtures use explicit version tags except the
intentional `latest` violation and Q4's source-required `nginx:alpine`.
Version tags are not immutable supply-chain provenance; image-signature
verification is not assessed here. Host distribution packages use the guest's
configured package repositories. Archive export requires registry access during
preparation; the candidate's SBOM task consumes the local archive.
The AppArmor deny rules cover both the visible `staff/` path and ConfigMap's
timestamped atomic-writer paths. Falco explicitly loads its bundled container
plugin; replacing the upstream config without that plugin prevents rule loading.
Traefik's sole entrypoint defaults to TLS routing. Preparation waits for a
Ready static mirror Pod matching the new spec, including mount-only edits,
and a container started after the manifest write. Candidate solutions likewise
wait for the new command and container, not just an old API server briefly
answering `/readyz`. Identical preparation writes are skipped to preserve retry
idempotence.
API-server changes also wait for controller-manager and scheduler recovery;
mutation checkpoints require newly refreshed witness evidence before grading.
Q7 deliberately uses `alpine/curl:8.12.1`: its APK database includes the curl
package. `curlimages/curl:8.12.1` contains a source-built curl executable but no
curl APK record, making it unsuitable for this package-inventory fixture.

Primary technical references:

- <https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#imagepolicywebhook>
- <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>
- <https://kubernetes.io/docs/tutorials/security/apparmor/>
- <https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/>
- <https://falco.org/docs/concepts/rules/overriding/>
- <https://github.com/kubernetes/kubernetes/blob/v1.36.1/plugin/pkg/admission/imagepolicy/config.go>
- `docs/cks-requirements.md`, `docs/testing.md`, `docs/vm-images-and-templates.md`.

## Source mapping and assessment boundaries

| Q | Points | Preserved competency | Adaptation |
| --- | ---: | --- | --- |
| 1 | 3 | AppArmor + least-privilege existing identity + unused account cleanup | helix/frontend, staff directory, site-guard; terminal probes replace the unavailable Site button |
| 2 | 3 | Decode Secret, replace env exposure with read-only volume | relay/broker credentials and new output/mount paths |
| 3 | 8 | Manual credential-exposure review | Six original Dockerfile/YAML fixtures, four findings; ignore root execution |
| 4 | 4 | Move and use Localhost seccomp profile | guard-nginx/web-guard.json |
| 5 | 5 | CIS kubelet permissions, etcd ownership, profiling | Scope names 4.1.1, 4.1.9, 1.1.12, 1.3.2, 1.4.1; avoids the source's ambiguous broad FAIL instruction |
| 6 | 14 | Runtime rule identification, local override, text file output | New workload and log path; real syscall monitor and binary writes |
| 7 | 12 | Inspect running containers, image archive SPDX SBOM | toolkit and tree-named containers, new outputs; exactly maple contains curl |
| 8 | 5 | Disable auto-mount, explicitly project service-account token | batch-ops/job-agent/janitor; preserve unrelated Deployment fields |
| 9 | 5 | Repair restricted-admission failure without relaxing policy | locked-down/portal; original unprivileged nginx must still serve HTTP |
| 10 | 8 | Namespace + Pod network selection and negative probes | catalog/storage/billing labels; Calico enforcement |
| 11 | 4 | UID, no privilege escalation, read-only root on both containers | delta/helper, UID 1002 |
| 12 | 5 | TLS ingress, Secret, host mapping, HTTPS | shuttle.lab; supplied CA enables certificate-verified curl |
| 13 | 6 | Namespace-scoped Pod RBAC | alex/team-a/team-b/team-c; retain object names and reject non-Pod access |
| 14 | 10 | Audit policy, rotation, healthy API | security-policy/security-audit; 7 days, 12 MB, 4 backups; explicit change verbs |
| 15 | 8 | Real fail-closed ImagePolicyWebhook and violating Pod replacement | image-gate/harbor, explicit nginx:1.28; preserve audit settings |

Categories follow the six domains in `docs/cks-requirements.md`, but source
weights are preserved, **not** redistributed to official domain percentages.
This is a mock, not complete CKS curriculum coverage: binary verification,
Kubernetes upgrades, metadata endpoints, host attack-surface reduction,
sandbox runtimes, Pod-to-Pod encryption and image signatures are not assessed.

Captures are read-only and bounded. Network/HTTP/exec probes observe existing
resources; authorization reviews ask the authorization API without creating
workloads. Independent `cks001-witness` periodically generates real audit
events and server-side admission dry runs; captures only read its evidence.
It does not submit answers or alter assessed workloads.

Grading checks AppArmor denial plus public availability, real NetworkPolicy
connectivity, runtime UID/no_new_privs/root mount flags, HTTPS certificate
validation, scoped effective RBAC, image/package SBOM identity, and active audit
and admission settings. Default Falco rule files must remain unchanged.
Readiness requires the current Deployment generation and updated replicas;
terminating Pods are never selected as evidence for a completed rollout.
Security-component flags are captured from running CRI process arguments, not
mirror-Pod readiness: node-heartbeat reconciliation after an API reload must not
be mistaken for an incorrect profiling, auditing or admission configuration.
The webhook accepts digests and explicit non-latest tags; signature validity is
outside scope. The independent latest probes exercise multiple repositories,
implicit latest and a positive tagged control. Backend-outage rejection is
supported by `defaultAllow: false` and approval caching disabled; captures do
not take down the webhook, since verification must remain read-only.
ImagePolicyWebhook uses `allowTTL: -1` to disable approval caching; `0` means
the default five-minute TTL, not an uncached configuration.

Audit grading checks final policy semantics and real representative events;
it cannot prove every historical request used that policy. Audit rotation is
checked through live API-server flags, not a destructive multi-megabyte stress
test. Evidence reads bounded tails of the current log and latest rotated log
so ordinary rotation cannot temporarily erase an otherwise correct score.
An on-disk SBOM cannot prove which CLI generated it; grading checks SPDX
structure, selected image identity and curl package inventory instead.

## Authoring and validation

`build.py` owns deterministic `tasks.json`, `setup.yaml` and `e2e.json`.
After editing it, run `python3 -B exam/CKS-001/build.py`; `--check` detects drift.
Refresh `assets/SHA256SUMS` after changing exam-owned runtime assets.

Read `docs/testing.md` before tests. Offline:

```sh
python3 -B exam/CKS-001/build.py --check
python3 -B -m unittest discover -s exam/CKS-001/tests -v
shellcheck exam/CKS-001/*.sh exam/CKS-001/assets/*.sh
(cd exam/CKS-001/assets && sha256sum --check --quiet SHA256SUMS)
go test -race ./...
go vet ./...
```

Live validation uses one **verified-free** VM ID:

```sh
LAB_VM_START=<verified-free-ID> scripts/test-exam.sh CKS-001
```

Require readiness, twice-stable baseline 0, reference 100/100, and all 15
task-level loss/restore mutations with no cross-task score changes. Each
mutation resets that task's main answer; they do not exhaustively test every
partial-credit field. Failures retain the marked disposable VM for diagnosis.
Never build/replace a template as part of validation. Browser coverage is
separate; this exam adds no UI changes.

### Validation result — 2026-11-17

- Clean disposable single-node run: `exam-test.iiDgK3aZ`, 1232 seconds including
  provisioning, answers, grading checkpoints and cleanup.
- Readiness passed; baseline stabilized twice at **0/100**.
- Candidate reference commands scored **100/100** through the production grader.
- All **15** listed mutations removed exactly their task's points, left every
  other score unchanged, and restored **100/100**.
- The wrapper successfully destroyed its marked VM.
- Offline: 21 exam regression tests, generated-definition drift check, asset
  checksums, ShellCheck, `go test -race ./...` (223 tests), and `go vet ./...`
  passed.
- Browser checks were not run; no browser/UI code changed. No template was
  built, replaced or modified.
