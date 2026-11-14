# CKA-002 — practical CKA exam

120 minutes, 20 tasks, 100 points. Domain weights follow
`docs/cka-requirements.md`: architecture 25, workloads 15, networking 20,
storage 10, troubleshooting 30.

## Minimal supporting lab

- Five linked clones of template **9198**: three controllers and two workers.
  Two controllers start joined; the candidate prepares forwarding and joins the
  third. Three stacked-etcd members provide real control-plane redundancy.
- One HAProxy on the west worker serves `cka002-api:8443`. Each guest resolves
  that name through `/etc/hosts` to the worker's DHCP address. The proxy itself
  is intentionally not redundant. No API VIP or Keepalived.
- Calico enforces NetworkPolicy.
- One Traefik controller serves **both Ingress and Gateway API**.
- Metrics Server supports resource inspection and HPA.
- Local-path provisioner provides functional dynamic storage.
- MetalLB allocates only **192.168.254.81**, for the LoadBalancer repair task.
  Other reserved addresses remain unused.

Build a new image at a free permitted VM ID with `scripts/build-template-ha.sh`
(`LAB_IMAGE_HA_TEMPLATE_ID` overrides the default 9198). The builder refuses to
overwrite an existing template. Tools and required images are cached at build
time; cluster identity and per-attempt addresses are configured after cloning.
The older image 9200 is retained, but does not satisfy this exam's tooling cache.

## Preparation and reset

1. Check the LoadBalancer address using a local-network ARP scan.
2. Linked-clone and start the VMs; configure the single TCP API proxy.
3. Bootstrap Kubernetes using the existing shared prebaked path.
4. Apply independent dependencies before waiting for readiness, copy candidate
   assets, and apply fixtures. Required readiness failures stop preparation.
5. Establish a good Deployment revision, introduce its failed update, then inject
   node/component faults once.

Exam-owned phase markers live on the first controller under
`/var/lib/painkiller/cka002`. Re-running the fixture phase after completion is a
no-op, preserving candidate answers. Reset destroys the owned VMs and clears the
attempt; no Kubernetes-level resource cleanup is needed.

The east worker is deliberately broken. Repair its kubelet before drain or
spread-scheduling tasks. Drain **only the east worker**; the west worker hosts
shared fixtures and the proxy. The drain workload tolerates the east worker's
taint. Scheduling/quota tasks use their own namespace.

## Competency coverage

| Domain | Requirement | Tasks |
| --- | --- | --- |
| Storage | StorageClasses and dynamic provisioning | 13 |
| | Volume types, access modes, reclaim policies; PV/PVC management | 13–14 |
| Troubleshooting | Clusters and nodes | 15; maintenance in 2 |
| | Cluster components | 16 |
| | Cluster/application resource usage | 19 (`top` for nodes and Pods) |
| | Container output streams | 20 |
| | Services and networking | 17–18 |
| Workloads | Rolling updates and rollback | 6 |
| | ConfigMaps, Secrets and autoscaling | 7 |
| | Self-healing workloads | 6 probes; 2 PDB |
| | Admission and scheduling | 8 quota, affinity, toleration, spread |
| Architecture | RBAC | 3 |
| | Prepare infrastructure | 1 persistent kernel forwarding configuration |
| | kubeadm cluster management and HA | 1 control-plane join and etcd health |
| | Cluster lifecycle | 1 join; 2 drain and snapshot |
| | Helm and Kustomize | 4 |
| | CNI, CSI, CRI interfaces | 13 inspection; 9 policy; 15–16 node/runtime diagnosis |
| | CRDs and operators | 5 CRD installation and reconciliation |
| Networking | Pod connectivity and NetworkPolicy | 9 |
| | ClusterIP, NodePort, LoadBalancer and endpoints | 9, 11, 12, 18 |
| | Gateway API | 10 functional HTTP routing |
| | Ingress controller/resources | 11 functional HTTP routing |
| | CoreDNS | 17 |

### Assessment boundaries

- The harness performs initial `kubeadm init` and package installation; the
  candidate prepares and joins an additional controller. Live version upgrades
  and full etcd disaster restore are not independently assessed.
- Local-path is **not CSI**. Task 13 explicitly distinguishes the installed
  non-CSI provisioner, CNI and CRI runtime; it does not assess CSI installation.
- The small supplied Widget operator teaches CRD installation/reconciliation
  without adding a heavyweight application platform.
- Drain, rollback, and diagnostic history use evidence files in addition to
  final-state checks. Evidence is not tamper-proof.
- HA is node/process redundancy on one physical host, not host fault tolerance.

## Pinned dependencies

Calico 3.32.0; Metrics Server 0.9.1; local-path-provisioner 0.0.37;
MetalLB 0.16.2; Traefik 3.7.19; Gateway API standard CRDs 1.6.3;
Helm 3.19.0; teaching operator image alpine/kubectl:1.36.1.

Gateway CRDs are vendored from the upstream v1.6.3 `standard-install.yaml`.
Traefik's pinned version documents support for that version. Assets are
checksummed in `assets/SHA256SUMS`.

## Validation

Run `scripts/e2e/live.js` with `E2E_EXAM=CKA-002` against a local server.
The harness checks baseline scoring, reference answers, answer-removal penalties,
browser terminal/reconnect, submission finality, and reset. It prints full
preparation and reset durations.

Measured original setup on prox-k12: **504 seconds prepare, 20 seconds reset**.
This includes fixtures, unlike the previous nodes-Ready-only timing.
