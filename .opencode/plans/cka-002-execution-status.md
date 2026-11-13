# CKA-002 execution checkpoint — 2026-11-13 (update 3)

Source plan: `/home/milkman/.opencode/plan/` (CKA-002 finish plan, 3 phases).

**CKA-002 is authored and the provisioning path is now split and prebaked.**
The remaining work is operational: build the HA template `9200` and run the
live 0→100 validation. Do not claim the exam is accepted until that runs.

## Completed (this session)

- Committed the earlier WIP (`prepare.sh` scp + taint-ordering,
  `reference-solutions.sh` fault-repair ordering) and fixed the `join-controller`
  walkthrough (it selected a worker IP; now uses `~/.ssh/painkiller_known_hosts`
  to find the unjoined controller).
- Removed the runtime `awk` surgery from `create-lab.sh`: prebaked provisioning
  now skips package-prep and image-pull via `--skip-tags prerequisites,images`.
- Dropped the unused `cilium` branch from `playbooks/kubeadm-cluster.yml`.
- Relocated CKA-002-only HA balancing into the exam:
  `exam/CKA-002/configure-api-load-balancer.sh` +
  `exam/CKA-002/assets/api-load-balancer.yml`.
- Added a prebaked HA image path: `scripts/build-template-ha.sh` (template
  `9200`) and `scripts/validate-image-ha.sh`; `labSpec.template_id`
  (9100–9200, required for prebaked multi-controller) and relaxed the prebaked
  guard to accept three controllers + Calico + a stable endpoint.
- Switched `exam/CKA-002/test.json` to `provision_mode: prebaked` +
  `template_id: 9200`.

Static checks pass: `go test` (115), `go vet`, shellcheck, `bash -n`, JSON, and
Ansible syntax for both playbooks.

## Still required (operational, needs Proxmox + network)

1. Clear stale `k8s-*` DHCP leases in the LAN Technitium instance before
   provisioning (AGENTS.md).
2. `scripts/build-template-ha.sh` → build template `9200` (needs the 9199 base).
3. Live validate CKA-002 against `9200`: fresh prepare, baseline 0, apply
   `reference-solutions.sh` → 100, per-mutation deductions from `e2e.json`
   (`rbac`, `netpol`, `ingress-nodeport`), browser submit, reset. Envoy
   Gateway/Traefik/MetalLB/operator/HPA jq shapes remain unverified against live
   objects; confirm the total wall time (target < ~120 s).
4. Record measured timings + results in `exam/CKA-002/README.md`; commit/push.

## Notes

- The prebaked HA image bakes Calico + haproxy/keepalived + core/dependency
  images. Envoy's data-plane image (`envoyproxy/envoy`) is not enumerated from
  the install manifest and is pulled on first Gateway use (graceful, documented
  in the builder).
- A previously provisioned `ready` CKA-002 lab (legacy, template 9010) was in
  `data/state.db`; it predates this split and is not valid for the new path —
  reset before re-provisioning.
