# CKA-002 execution checkpoint — 2026-11-13 (update 2)

Source plan: `/home/milkman/.opencode/plan/cka-002-execution-plan.md`.

**This is an infrastructure + content checkpoint, not a fully validated exam.**
The `exam/CKA-002/` exam is now authored and passes all static checks; the live
100/100 validation was blocked by a DHCPv4 outage on the lab network and has not
completed. Do not claim the exam is accepted.

## Completed in this session

- `exam/CKA-002/` authored: `test.json` (lab contract: 3 controllers, 2 workers,
  initial 2, legacy, calico, endpoint `.80:8443`, memory 4096, `prepare.sh`),
  `tasks.json` (20 scenarios, exactly 100 points; domain totals architecture 25,
  workloads 15, networking 20, storage 10, troubleshooting 30), `setup.yaml`,
  `reference-solutions.sh`, `prepare.sh`, `e2e.json`, `README.md`.
- All 20 tasks have read-only semantic captures with positive **and** negative
  functional probes; historical actions (drain, OOM/crash logs) require saved
  evidence files; baseline is 0/100 by construction (positive checks are gated on
  the candidate-created object where default state would otherwise grant credit).
- Dependency bundle vendored under `assets/` with `SHA256SUMS`: Metrics Server
  v0.9.1, local-path-provisioner v0.0.37, MetalLB v0.16.2, Envoy Gateway v1.10.0
  (Gateway API v1.6.1), Traefik v3.7.19, Helm v3.19.0, Widget operator
  (bitnami/kubectl:1.36.1). Calico 3.32.0 from the shared playbook. `ingress-nginx`
  deliberately avoided (retired March 2026).
- `prepare.sh` implements `preflight` (reserved-address nmap scan, skipped on
  retry), `infrastructure` (guest boot wait + redundant API endpoint), and
  `fixtures` (deps, helm/chart/kustomize/operator assets, `setup.yaml`, node
  labels/taint/local data, broken helm release, readiness, idempotent faults).
- Added storage-capacity preflight to `scripts/create-lab.sh` (datastore free
  space via `pvesm status`, 10 GiB headroom).
- README maps all 27 competencies in `docs/cka-requirements.md` to tasks, with
  harness-provided / not-independently-assessed items stated explicitly.

## Static validation (passing)

- Go race suite: **114 tests passed**; `go vet` passed.
- ShellCheck, `bash -n`, JSON validity, jq expression compile checks passed.
- Ansible syntax checks passed; `assets/SHA256SUMS` verifies.
- `loadExams` loads CKA-002 with 20 tasks / 100 points alongside CKA-001.

## Live validation (blocked)

Provisioning reached the dependency-install stage over several attempts. Fixes
made during those attempts: infrastructure phase now waits for first-boot
cloud-init/apt before HAProxy; MetalLB pool applied only after its webhook is
available; Envoy Gateway applied server-side (its CRDs exceed kubectl's 256 KiB
last-applied annotation limit); CoreDNS fault uses a literal split/join; the
reserved-address preflight is skipped when the lab already owns the addresses.

**Blocking issue:** the lab network's DHCPv4 service stopped responding
(confirmed by `tcpdump` on `vmbr0`: DHCPREQUEST broadcasts with zero OFFER/ACK
replies). VM 3000 obtained `.243` early; VMs 3001–3004 received IPv6 but no
IPv4, so `create-lab.sh` could not reach them. This is external infrastructure
(DHCP server), not exam code; the router was not modified. The broken VMs
3000–3004 were destroyed.

## Still required

1. Live validation once DHCPv4 is restored: fresh prepare through the new server
   path, `0 → 100`, per-answer mutation deductions, immutable submission/restart,
   partial-fixture retry, autoscaling-under-load, persistence-across-replacement,
   and full reset. Reference solutions, Envoy Gateway/Traefik/MetalLB/operator
   behavior, HPA, and exact jq shapes against live API objects remain unverified.
2. Record measured results in `exam/CKA-002/README.md` (nothing is assumed).
3. Full acceptance review and commit/push of the new exam.
