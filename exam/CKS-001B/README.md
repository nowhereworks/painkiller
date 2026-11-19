# CKS-001B

Unofficial **120-minute, 16-question, 100-point** security practice exam based
on `docs/cks-mock-01b.md`. Original scenario wording, new resource names, paths,
CIDRs and one API port; source order, weights and core competencies are retained.
The UID remains nginx-unprivileged's built-in 101 to avoid an unrelated image
configuration exercise. This is not a complete CKS curriculum or an official exam.

## Minimal standalone lab

One **schedulable control plane, zero workers, 8192 MB RAM**, linked clone of
base `9199`. Full-install Kubernetes with Calico (`legacy` provisioning mode;
the explicit template overrides its historical 9010 default). No HA, new
template, Docker daemon, external API service or reserved LAN addresses.
All dependencies and assets belong to this exam; no other exam is imported.

Preparation installs Falco, Kubesec, bom, kube-bench and istioctl. Minimal Istio
has one istiod and no gateway. One Traefik controller uses the **Kubernetes
Ingress NGINX provider**, not the ordinary Ingress provider, to implement the
source's nginx annotations and class. An independent TLS/redirect canary
verifies this behavior before Start.

Four lab-only destinations (`198.18.20.10`, `.30.10`, `.40.10`, `.50.10`) are
bound as `/32` addresses on guest **loopback only**, never on the clone's NIC.
The fixture serves HTTP 8081 and TLS 443/9443. No LAN address allocation is
needed. These support actual Calico egress probes without Internet dependence.

All tasks run on the controller. The candidate has sudo; root paths explicitly
need sudo. `~` is the candidate home, recorded at preparation rather than
assumed to be `/root`. Retries preserve completed phases and candidate work.
Reference answers and fault/mutation helpers refuse unmarked clusters.
Infrastructure preparation waits for cloud-init through the ownership-checked
Proxmox guest agent, then rediscovers DHCP and pins the guest's agent-reported
SSH key. This tolerates first-boot package upgrades restarting networking without
changing the clone's NIC, DHCP client or identity. Exam package installation
lists pending service restarts rather than automatically restarting networking.
Q6 creates nginx's log subdirectory inside its writable `/var/log` volume
before startup. Runtime mount grading resolves `/var/run` to `/run` when the
container's actual symlink requires it; it does not mistake that for a missing
writable volume.

## Source mapping and narrow corrections

| Q | Points | Competency and changed scenario |
| --- | ---: | --- |
| 1 | 5 | Falco local mining-exec rule; compute-watch and MINER_NOTICE |
| 2 | 5 | Egress CIDR exclusion with DNS; outbound-guard, benchmark-only CIDRs |
| 3 | 10 | Non-root nginx, UID/GID, no escalation; workload-identity/web-gate |
| 4 | 7 | Selected ingress sources; storefront/shop-web/shop-api and telemetry |
| 5 | 6 | Remove binding-write escalation, retain other RBAC; delivery/pipeline-agent |
| 6 | 8 | Read-only root with writable mounts; immutable-workloads/report-web |
| 7 | 5 | Distroless/nonroot/exec Dockerfile; ledger binary and paths |
| 8 | 9 | Capability restriction plus Kubesec; invoice-web and new report path |
| 9 | 10 | Restricted PSA/latest/baseline warnings; finance-ledger |
| 10 | 4 | Localhost seccomp deny-debug profile; runtime-shield/no-inspect.json |
| 11 | 7 | Running image identification and archive SPDX; produce/orchard, fruit names |
| 12 | 4 | TLS termination and redirect; secure-portal/portal.training.example |
| 13 | 7 | DNS/HTTPS/CIDR custom-API egress; api-clients, port 9443 |
| 14 | 7 | Injected Istio sidecars and STRICT mTLS; private-mesh/north/south |
| 15 | 2 | Create/mount supplied TLS material; editor/editor-cert |
| 16 | 4 | CIS etcd directory mode/ownership; actual verified data-dir tree |

- Q2 DNS lies outside both denied CIDRs; **all** traffic inside those CIDRs is
  blocked, with no ambiguous port-53 bypass.
- Q8 retains NET_BIND_SERVICE as requested but explicitly explains that 8080
  is not privileged.
- Q12 HTTP is permitted solely for redirection, rather than simultaneously
  requiring a redirect and prohibiting the HTTP endpoint.
- Q16 uses `cis-1.10` checks 1.1.11/1.1.12 and only `/var/lib/etcd`, verified
  against live CRI arguments and the static Pod's host mount. The disposable
  lab runs etcd as root; assigning the tree to the prepared etcd:etcd identity
  does not prevent access. Directories use 0700, data files 0600; no recursive
  execute bit on ordinary database files and no unrelated directory changes.

Categories follow `docs/cks-requirements.md`; source weights are **not**
redistributed to official domain percentages. Unassessed: audit-policy design,
AppArmor, API upgrades, metadata endpoints, binary verification, image signing
and sandbox runtimes.

## Dependencies and assessment boundaries

Checked 2026-11-18. Host downloads are SHA256-pinned in `assets/install.sh`:
Falco 0.45.2 (modern eBPF, container plugin), Falco rules 5.2.0, bom 0.8.0,
kube-bench 0.16.0, Kubesec 2.15.0 and istioctl 1.32.0. Traefik 3.7.19 uses a
registry digest. Workload images use version tags except Q3's source-requested
`nginxinc/nginx-unprivileged:alpine`; version tags are not immutable provenance.
Host packages use the guest distribution's repositories. Preparation needs
registry/package access and kernel BTF for Falco.

Read-only captures combine configuration with actual runtime state, network
probes, TLS verification, authorization reviews and SBOM image/package identity.
The suspect Pod periodically executes benign copies of BusyBox under mining
process names; it contains **no miner**. Falco grading observes fresh real
events, without executing commands in the suspect Pod during grading.
Falco uses `rule_matching: all`, so distribution rules cannot shadow a correct
candidate rule for the same exec event.
An independent systemd PSA witness uses server-side dry runs; verification
only reads its fresh evidence matching namespace and Deployment versions.

The network-policy model covers bounded IPv4 CIDR/numeric-port partitions and
additive policies alongside live probes, not every possible IPv6/named-port
configuration. Supporting transport failures are capture errors, not successful
denials. SBOM/Kubesec files establish report content, not which CLI produced it.
The seccomp capture checks file semantics, selected profile and live filter
mode, but cannot prove a running container loaded the latest file contents.
The Dockerfile line limit is final-state evidence, not editing-history proof.
One current-revision Ready Deployment Pod is sampled, with all assessed
containers checked. mTLS probes verify meshed access and plaintext denial, not
a packet-level cryptographic audit.

Technical references:

- `docs/cks-mock-01b.md`, `docs/cks-requirements.md`, `exam/README.md`
- <https://kubernetes.io/docs/concepts/services-networking/network-policies/>
- <https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>
- <https://kubernetes.io/docs/concepts/security/pod-security-admission/>
- <https://falco.org/docs/concepts/rules/>
- <https://doc.traefik.io/traefik/reference/routing-configuration/kubernetes/ingress-nginx/>
- <https://istio.io/latest/docs/reference/config/security/peer_authentication/>

## Authoring and validation

`questions.json` owns candidate scenarios; `solutions.py` owns exact executable
answers. `build.py` generates `tasks.json`, `setup.yaml` and `e2e.json`.

```sh
python3 -B exam/CKS-001B/build.py
python3 -B exam/CKS-001B/build.py --check
python3 -B -m unittest discover -s exam/CKS-001B/tests -v
shellcheck exam/CKS-001B/*.sh exam/CKS-001B/assets/*.sh
(cd exam/CKS-001B/assets && sha256sum --check --quiet SHA256SUMS)
```

Refresh asset checksums after editing runtime assets. Read `docs/testing.md`
before live validation. Use **one verified-free VM ID** within 3000–3999:

```sh
LAB_VM_START=<verified-free-ID> scripts/test-exam.sh CKS-001B
```

The local suite skips two real-tool cases unless `CKS001B_KUBESEC` points to
the pinned Kubesec executable and `CKS001B_BOM_REPORT` points to a real
bom-generated curl-image SPDX JSON report. These can be copied read-only from
the disposable guest; do not substitute mock data when claiming real-tool coverage.

Require initial readiness, twice-stable 0/100 baseline, exact candidate answers
at 100/100 and all **16 task-level loss/restore mutations** without cross-task
score changes. These mutations reset each main answer; local negative cases
cover additional partial-credit semantics but do not establish exhaustive
live mutation coverage. Browser coverage is separate.

### Validation — 2026-11-19

- Clean disposable run `exam-test.FUedxE3m`: **1196.74 seconds**, including
  provisioning and cleanup, on one 8192 MB controller.
- Initial readiness passed; baseline stabilized twice at **0/100**.
- Exact candidate reference solutions scored **100/100** through the production grader.
- All **16** listed mutations removed only their task's points and restored
  **100/100**. The wrapper successfully destroyed its marked VM.
- Local: generated-definition drift check, asset checksums, ShellCheck,
  `go test -race ./...` (226 tests) and `go vet ./...` passed.
  Python: 87 cases; the ordinary run skips two optional real-tool cases.
  Both optional cases also passed with the pinned Kubesec executable and a
  real bom-generated SBOM copied from the earlier disposable development guest.
- Earlier development exposed the hidden nginx log directory, canonical `/run`
  mountpoint, Falco first-match shadowing and boot-time DHCP churn described
  above. The two failed validation guests were ownership-verified and destroyed
  before the clean run; no failed disposable VM was left behind.
- No browser tests were run; no UI code changed. No template was built,
  modified or replaced, and no clone NIC or DHCP configuration was changed.
