# CKA-002 execution checkpoint — 2026-11-13 (update 4)

Source plan: `/home/milkman/.opencode/plan/` (CKA-002 finish plan).

**CKA-002 is authored, provisioned from the prebaked HA image `9200`, and
live-validated: baseline 0 → reference 100 → exact per-mutation deductions.**
The full browser submit/reset flow is running via `scripts/e2e/live.js`.

## Completed

- Exam authored and provisioned prebaked (3 controllers, 2 workers, Calico,
  endpoint `.80:8443`, template `9200`); HA balancing relocated to the exam.
- Template `9200` (`painkiller-image-ha-9200`) built and the exam switched to
  `provision_mode: prebaked`.
- Live validation on `prox-k12` (VMs 3000–3004, provisioned in ~120 s):
  - baseline **0/100**;
  - `reference-solutions.sh` applied → **100/100**;
  - `e2e.json` mutations each remove exactly their task's points and restore:
    `rbac` 95/100, `netpol` 94/100, `ingress-nodeport` 95/100.

## Fixes applied during live validation

- `reference-solutions.sh`: idempotent join; null-safe fault discovery; etcdctl
  TLS flags for `snapshot save`; recreate immutable PVC/PV (storageClassName,
  nodeAffinity) instead of patching; idempotent RBAC/quota/HPA; `--cpu=50%`.
- `tasks.json`: jq operator-precedence fix (kubelet Ready), Gateway API
  `parents` (not `routeParents`), `$root` binding for the local-PV node
  cross-reference, and single-escape of nested jq in resource captures.
- `operator.yaml`: `alpine/k8s:1.36.1` (bitnami/kubectl dropped version tags).
- `prepare.sh`: taint the second worker before fixtures so setup Pods never
  land on the worker whose kubelet fault later makes it NotReady; scp assets
  instead of tar-over-ssh; wait for first boot; MetalLB webhook readiness;
  Envoy Gateway server-side apply; literal CoreDNS split/join.

## Still required

1. Confirm the full `live.js` browser flow (marks survive reload, terminal +
   reconnect, immutable submit 100/100, duplicate-submit 409, terminal 409,
   reset to idle) — running now.
2. Record the measured wall times and results in `exam/CKA-002/README.md`.
3. Confirm `go test`/`vet`/shellcheck/ansible still pass after the fixes and
   push.
