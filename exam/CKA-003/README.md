# CKA-003 — mock 01

120 minutes, 16 questions, 100 points. Adapted from local source
`docs/cka-mock-01.md` (not a runtime dependency);
question order, core skills, and weights are preserved, while resource names,
paths, and selected numeric targets vary from the source. Domain totals are architecture 22, workloads 22,
networking 18, storage 10, troubleshooting 28. These are the mock's weights,
not the official domain percentages.

## Standalone lab

One controller and two workers, linked clones of existing template **9100**,
Flannel, at least 4096 MiB per VM. No new template, multi-cluster lifecycle,
external playbook, reserved LAN address, or changes to other exams are required.
Template ancestry remains unverified; this does not claim image-policy compliance.

Preparation installs NFS packages, Python YAML support, and Helm if absent. The first worker exports
`/srv/cka003` only to the three saved guest addresses. The initial controller
resolves `practice-controller` to its own DHCP address. Guest NIC, DHCP, and
identity configuration remain unchanged.

Each attempt owns its dependencies and fixtures. Pinned assets:

- Metrics Server 0.9.1 (lab-only kubelet insecure TLS).
- Gateway API standard CRDs 1.6.3.
- Traefik 3.7.19, one controller for Gateway API and Ingress, NodePort 30080.
- NFS CSI 4.11.0 and its pinned upstream sidecars. The unused snapshot sidecar
  is removed during preparation.
- Helm 3.19.0, downloaded with archive checksum verification if not installed.

Upstream YAML is vendored from the corresponding release tags; hashes for all
assets, including the broken chart and read-only capture helper, are recorded in
`assets/SHA256SUMS`. Internet access is required for guest packages, Helm when
absent, and uncached container images. No image build is part of validation.

## Explicit source adaptations

- All four source clusters become the one lab cluster. All candidate commands
  and evidence files belong on its initial controller.
- Q3 uses namespace `catalog-repair` and an exhausted Pod quota. Q9 uses
  a Pod-local selector typo. Neither disables shared cluster components.
- Q4 uses an expandable NFS CSI PV initially **64Mi**, not a 150Mi local PV
  already larger than the requested resize. The student patches only the PVC;
  the CSI resizer updates advertised PV/PVC capacity to 96Mi. An active consumer
  verifies the NFS mount. NFS does **not** enforce these sizes as disk quotas;
  this is not a filesystem/disk expansion assessment.
- Q5 supplies `/opt/portal-chart`, a small nginx chart with broken selectors.
  `appVersion: "1.21.6"` determines the nginx image tag.
- Q6 uses namespace `gateway-lab` and Traefik instead of NGINX Gateway
  Fabric. Hostname becomes `practice-controller`.
- Q10 names the Deployment `inventory-api`; preparation establishes a healthy
  nginx:1.27 revision before its broken update.
- Q11 uses a lightweight credential-checking container, not a database server.
  The supplied Secret must remain unchanged.
- Q12 records the Deployment revision immediately after v2 (2 for a fresh
  Deployment), not a count of history entries. History and evidence are checked.
- Q13 uses class name `harbor-http`, using Traefik. The requested nginx
  ssl-redirect annotation remains an assessed metadata field; Traefik does not
  interpret it. The supplied entrypoint is HTTP with no redirect.
- Q14 is an isolated paused rollout, preserving the pending Pod template.
- Q16 assesses StorageClass fields; the expansion flag alone does not make a
  no-provisioner local volume expandable.

## Light question variations

The core asks, fault types, 120-minute duration, and 100-point total are unchanged.
Stable question IDs and internal grading keys are retained for attempt compatibility
(including the historical internal key `pink-replicas` for Q3).

| Question | Variant details |
| --- | --- |
| 1 | `quay-frontend`: CPU 75%, 3–9 replicas |
| 2 | `namespace-observer` account and corresponding role/binding; namespace get only |
| 3 | `catalog-repair/catalog-api`: 3 desired replicas, initially 2; `catalog-quota` |
| 4 | `cedar-pvc`, `cedar-pv`, `cedar-sc`: 64Mi → 96Mi |
| 5 | `harbor-portal`, `portal-lab`, `/opt/portal-chart`: 4 replicas, app 1.21.6 |
| 6 | `gateway-lab/portal-route`, `portal-gateway`, `portal-service`, `practice-controller` |
| 7 | `release-reader` account and corresponding role/binding; Deployment get only |
| 8 | `dns-web`, `dns-web-service`; `/root/CKA/service-dns.txt` and `pod-dns.txt` |
| 9 | `queued-web`; placement label `cka003/placement=controller` |
| 10 | `release-lab/inventory-api`: 6 replicas; `/root/restored-image.txt` |
| 11 | `ledger-db`, `ledger-credentials`; renamed Secret keys, unchanged variable names |
| 12 | `color-stream`: 4 replicas, maxUnavailable 30%, maxSurge 60%; `/opt/rollout-revision.txt` |
| 13 | `portal-ingress`, `harbor-http`, `site-backend`, `site-service` |
| 14 | `reports-api`; pending template annotation `cka003/update: queued` |
| 15 | `status-web`, `status-monitor`, `status-service`: Service 8080 → application 80 |
| 16 | `archive-local`; unchanged provisioner, binding mode, and expansion flag |

## Preparation, readiness, and reset

`prepare.sh` uses the saved attempt environment and ownership-checked guest
discovery. Remote preparation commands fail fast; the NFS exports directory and
configuration are explicitly created, including on minimal images. Phase markers
live under `/var/lib/painkiller/cka003`. Completed
preparation is a no-op, preserving candidate work. Partial preparations reconcile
without reinjecting completed history/fault phases. Reset destroys the owned VMs;
there is no shared storage or Kubernetes-level reset.

`check-ready.sh` is read-only and verifies three Ready nodes, metrics,
routing/Gateway, CSI storage and an actual NFS mount, candidate assets, and
intentional faults. Start remains disabled on failure.

## Grading boundaries

Each question is all-or-nothing using its full source weight. Read-only capture
logic is installed at `/opt/painkiller/cka003/capture.sh`. Missing resources fail
the answer; API errors fail capture. API and HTTP calls are bounded.
Every question also captures specific unmet requirements for Learning
verification, without changing its all-or-nothing weight. Readiness messages
distinguish unsettled workloads from configuration checks; they do not grant
credit before convergence. Already prepared labs retain their installed capture
script, so expanded diagnostics require a newly prepared lab.
For the monitored HTTP question, a completed HTTP failure is an incorrect
answer; inability to exec the probe in `status-monitor` is a capture error.

RBAC roles must contain only the requested rule; effective get permissions and
representative forbidden operations are also tested by impersonation. This is
not an exhaustive audit of all unrelated bindings. File evidence is compared
with live resources. HTTP checks accompany route specification checks. Rollout
history is checked, but final state and student-writable evidence cannot prove
the exact commands performed or prevent fabrication by a cluster admin.

## Validation

Follow `docs/testing.md`. After verifying a completely free three-VM range:

```sh
LAB_VM_START=3100 scripts/test-exam.sh CKA-003
```

Expect baseline zero, reference answers 100/100, and one isolated loss/restore
mutation for each of 16 questions. The PVC mutation disables expansion rather
than attempting an unsupported shrink; this is not exhaustive mutation coverage.
Successful runs destroy their VMs; failures leave them for inspection.
Browser rendering is outside this validation.

The revised variant was validated on 2026-11-14 against disposable clones on prox-k12 using
`scripts/test-exam.sh CKA-003`: baseline 0, reference score 100/100, all 16
loss/restore mutations passed, and automatic VM cleanup passed. The complete
browser-free run took 291.71 seconds. Local Go race tests, vet, JavaScript
syntax, shellcheck, and asset checksum checks also passed.
