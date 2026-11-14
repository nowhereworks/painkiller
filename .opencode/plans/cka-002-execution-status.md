# CKA-002 execution checkpoint — 2026-11-13 (update 5 — complete)

Source plan: `/home/milkman/.opencode/plan/` (CKA-002 finish plan).

**CKA-002 is authored, provisioned from the prebaked HA image `9200`, and fully
accepted: baseline 0 → reference 100 → exact mutations → browser submit 100 →
submission finality → reset.**

## Acceptance (all green)

- Fresh prepare on `prox-k12` (VMs 3000–3004, prebaked `9200`, ~120 s).
- Baseline **0/100**; reference `reference-solutions.sh` → **100/100**.
- `e2e.json` mutations remove exactly their task's points and restore to 100:
  `rbac` 95, `netpol` 94, `ingress-nodeport` 95.
- Browser submit **100/100**; duplicate submit → 409; terminal after submit → 409.
- Marks, terminal echo, and reconnect survive a reload; reset returns to idle.
- Static checks pass: `go test` (115), `go vet`, shellcheck, `bash -n`, JSON,
  Ansible syntax, and `assets/SHA256SUMS`.

## Final fixes applied (all committed)

- `prepare.sh`: first-boot wait before HAProxy, MetalLB webhook retry, Envoy
  Gateway server-side apply, scp (not tar-over-ssh) for assets, taint the second
  worker before fixtures, literal CoreDNS split/join, preflight skip on retry.
- `reference-solutions.sh`: idempotent join; `</dev/null` on nested ssh (the
  nested ssh was swallowing the script's stdin); etcdctl TLS flags; recreate
  immutable PVC/PV; `--cpu=50%`; apply-based RBAC/quota/HPA.
- `tasks.json`: Gateway API `parents` field, jq precedence and `$root` binding,
  single-escape of nested jq, kubelet-repair graded from node Ready (no nested
  ssh in capture).
- `operator.yaml`: `alpine/k8s:1.36.1` (bitnami/kubectl dropped version tags).
- `scripts/create-lab.sh`: datastore free-space preflight.

## Remaining

None for acceptance. The exam is committed; push is the final step.
