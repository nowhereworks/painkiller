# CKA-002 — HA cluster practice exam

A 120-minute, 20-task, 100-point scenario exam on a five-node, highly available
cluster: three stacked-etcd control-plane nodes behind a redundant API endpoint
and two workers, on Calico. This is readiness evidence, not a guarantee of
passing the CNCF exam.

## Topology and provisioning

CKA-002 declares its own lab contract in `test.json` and provisions, in
prebaked mode, exactly five linked-clone VMs from the HA template `9200`:

- Three control-plane nodes; two are joined initially and the third is prepared
  but left unjoined for the candidate's kubeadm join task.
- Two workers run HAProxy (TCP, `:8443`) and Keepalived (unicast VRRP) so the
  reserved API virtual IP `192.168.254.80` is highly available; the API listens
  on `192.168.254.80:8443`.
- MetalLB serves LoadBalancer Services from the reserved pool
  `192.168.254.81–89`. `192.168.254.90–99` remains unallocated.

Build the HA image before first use with `scripts/build-template-ha.sh`; it
bakes `9200` from base `9199` (Kubernetes packages, `haproxy`/`keepalived`,
Calico, and the exam/dependency images). HAProxy/Keepalived are installed but
not configured at build time — `prepare.sh` configures them per node.

Preparing another test requires resetting this lab; only one active attempt
exists at a time. The reserved range and the standalone-exam rule are documented
in the repository `AGENTS.md`.

## Competency mapping

Every competency in `docs/cka-requirements.md` is addressed by a task or is
explicitly documented below as harness-provided or not independently assessed.

| Domain (weight) | Competency | Task |
| --- | --- | --- |
| Storage (10%) | Storage classes and dynamic provisioning | 13 |
| | Volume types, access modes, reclaim policies | 13, 14 |
| | Manage PVs and PVCs | 14 |
| Troubleshooting (30%) | Troubleshoot clusters and nodes | 15 (kubelet), 2 (drain) |
| | Troubleshoot cluster components | 16 (scheduler) |
| | Monitor cluster/application resource usage | 19 (OOM/metrics/events) |
| | Manage and evaluate container output streams | 20 (current/previous logs) |
| | Troubleshoot services and networking | 18, 17 (CoreDNS) |
| Workloads & Scheduling (15%) | Deployments, rolling update/rollback | 6 |
| | ConfigMaps and Secrets | 7 |
| | Workload autoscaling | 7 (CPU HPA) |
| | Robust self-healing primitives | 6 (probes), 2 (PDB) |
| | Pod admission and scheduling (limits, affinity) | 8, 7 |
| Cluster Architecture (25%) | RBAC | 3 |
| | Prepare underlying infrastructure | harness-provisioned, not candidate-assessed |
| | Create/manage clusters with kubeadm | 1 |
| | Manage cluster lifecycle | 1, 2 |
| | Highly-available control plane | 1 |
| | Helm and Kustomize | 4 |
| | Extension interfaces (CNI, CSI, CRI) | 9 (CNI/Calico), 13–14 (storage provisioner), 15–16 (CRI/containerd) |
| | CRDs, install/configure operators | 5 |
| Services & Networking (20%) | Connectivity between Pods | 9 |
| | Network Policies | 9 |
| | ClusterIP, NodePort, LoadBalancer, endpoints | 11, 12, 18 |
| | Gateway API | 10 |
| | Ingress controllers and Ingress resources | 11 |
| | CoreDNS | 17 |

Domain totals: architecture 25, workloads 15, networking 20, storage 10,
troubleshooting 30.

## Dependencies

All dependencies are vendored under `assets/` with checksums in
`assets/SHA256SUMS`, pinned to versions compatible with Kubernetes `v1.36`:

| Component | Version | Notes |
| --- | --- | --- |
| Calico | 3.32.0 | CNI + NetworkPolicy enforcement, from the shared playbook |
| Metrics Server | v0.9.1 | `--kubelet-insecure-tls` for the self-signed lab kubelets |
| local-path-provisioner | v0.0.37 | dynamic provisioning, `WaitForFirstConsumer` |
| MetalLB | v0.16.2 | native manifests, L2 pool `.81–.89` |
| Envoy Gateway | v1.10.0 | Gateway API CRDs (v1.6.1) + controller |
| Traefik | v3.7.19 | Ingress controller (kubernetesIngress provider) |
| Helm | v3.19.0 | candidate tooling |
| Widget operator | alpine/kubectl:1.36.1 | purpose-built teaching operator |

`ingress-nginx` was retired in March 2026 and is deliberately not used.

## Preparation stages

`prepare.sh` runs three bounded, idempotent phases, all streamed through the
operation log:

1. `preflight` — reserved-address ARP scan (`192.168.254.80–89`) from the
   Proxmox host, before any VM is cloned.
2. `infrastructure` — the redundant API endpoint (HAProxy/Keepalived) on the two
   workers, before `kubeadm init`.
3. `fixtures` — dependency manifests, Helm tooling, supplied chart/kustomize/
   operator assets, `setup.yaml`, node labels/taint, local data, the broken Helm
   release, dependency readiness, then the intentional faults.

Faults are injected last and idempotently so a retry reconciles completed stages
without re-injecting them or resetting healthy state.

## Grading and limitations

- Grading is semantic and read-only: each task captures a JSON snapshot and
  evaluates bounded functional probes (positive and negative), not command
  history.
- Historical actions (drain, rollback, failover, OOM/crash diagnosis) cannot be
  proven by final state alone, so several tasks require saved evidence files;
  this simulator is not tamper-proof.
- Live version upgrades and full etcd disaster restore are **not** independently
  assessed; "manage cluster lifecycle" is exercised through joining, drain, and
  snapshot validation.
- "Prepare underlying infrastructure" is performed by the harness, not the
  candidate. The CRI (containerd) competency is exercised by identifying and
  repairing kubelet/static-Pod faults, not a runtime swap. Storage uses a dynamic
  local-path provisioner rather than a full CSI driver; the CSI extension
  interface is addressed through that provisioner and the static local volume.
- Node/process HA is practiced on a shared Proxmox host, not physical-host fault
  tolerance.

## Validation

Validated on 2026-11-13 on `prox-k12` against the prebaked HA image `9200`
(VMs 3000–3004, three control-plane + two workers, Calico, endpoint
`192.168.254.80:8443`). Provision to all nodes Ready: ~120 s (prebaked). The
full `scripts/e2e/live.js` flow passed end to end:

| Step | Result |
| --- | --- |
| Baseline (untouched lab) | **0 / 100** |
| Reference solutions | **100 / 100** |
| `rbac` answer removed / restored | 95 / 100 → 100 |
| `netpol` answer removed / restored | 94 / 100 → 100 |
| `ingress-nodeport` answer removed / restored | 95 / 100 → 100 |
| Browser submit | **100 / 100** |
| Submission finality | duplicate submit 409, terminal 409 |
| Marks / terminal / reconnect | passed |
| Reset | complete (idle) |

Functional behavior verified on the disposable lab: Calico NetworkPolicy
enforcement (allowed client, denied client, DNS), Envoy Gateway HTTPRoute
Accepted + ResolvedRefs with a cross-namespace ReferenceGrant, Traefik Ingress
and NodePort endpoints, MetalLB allocation from `192.168.254.81–89`, dynamic
local-path provisioning with `WaitForFirstConsumer`, static local-PV binding,
and the operator's Widget→ConfigMap reconciliation.
