# CKA-002 execution checkpoint — 2026-11-13

Source plan: `/home/milkman/.opencode/plan/cka-002-execution-plan.md`.

**This is an infrastructure checkpoint, not a completed CKA-002 exam.**
CKA-002 has not been added to the catalog. Do not describe the full plan or its
acceptance criteria as complete.

## Implemented

- Optional validated `test.json` lab contract, with topology-only public catalog.
- Explicit incompatible operator constraints fail selection. Metadata derives
  the complete VM range from its start and specifies minimum VM memory.
- New attempts save the operational environment, including host, range, marker,
  mode, template, controller counts, CNI, endpoint, SSH account, and host-key file.
  Operations, capture, terminal access, retries, and reset use the snapshot.
- Read-only all-range creation preflight. Mutation eligibility is saved before
  cloning; reset after failed preflight cannot delete an older, same-marker lab.
- Trusted, path-validated preparation hooks with bounded preflight,
  infrastructure, and fixture phases and existing operation-log streaming.
- Shared guest-prerequisites-only command; role and initial join eligibility are
  separate. All five guests receive prerequisites and candidate SSH access.
- Worker-only HAProxy/Keepalived playbook and helper: unicast VRRP, discovered
  guest interface, reserved `.80:8443` endpoint, all three API backends.
- Guest discovery excludes the configured `/32` API VIP from the DHCP identity.
- Serial control-plane joins, suppressed bootstrap-secret output, and refusal
  to reset an unhealthy existing cluster during lifecycle reconciliation.
- E2E exam/reference selection and exam-owned baseline/mutation fixtures;
  captures are reused across a task's checks. Existing attempts require explicit
  harness resume and a matching test ID. Fault convergence is bounded.
- Standalone-exam and user-reserved address rules persisted in `AGENTS.md`.

Older saved attempts without an operational snapshot retain legacy
environment-based behavior. This limitation is documented; do not change their
environment before reset.

## Measured validation

- Go race suite: **114 tests passed**; `go vet` passed.
- ShellCheck and JavaScript syntax checks passed.
- Both Ansible playbooks passed syntax checks and checksum verification; the
  generated prebaked bootstrap playbook also passed syntax checks.
- CKA-001 browser regression before HA: **0 → 100 → 98 → 100**; marks, terminal
  reconnect, final submission, and reset passed. An earlier immediate capture
  briefly scored 4 before seeded faults converged; the harness now requires two
  matching baseline captures within 60 seconds.
- HA validation used only newly created VMs **3100–3104**, marked
  `painkiller-cka002-validation`, linked from existing template **9010** with
  4096 MiB each. No template was modified.
- Before allocation, the application attempt was idle, IDs were free, template
  9010 was verified, host memory was sufficient, and a Proxmox-side ARP scan
  found no responders on `.80–.89`. This is not a guarantee against every
  possible external allocation.
- Clone/start: **22 s**; guest readiness: **14 s**. Bootstrap to four Ready
  nodes: **246 s**, excluding the separate prerequisites and balancing phase.
  Legacy packages resolved to **v1.36.6** while kubeadm requested **v1.36.1**;
  the existing legacy minor-repository package behavior is unchanged.
- Initial topology verified: two joined controllers, two Ready workers, third
  controller prepared/unjoined, candidate SSH reaching all five guests.
- Provisioning retry: **65 s**, same kube-system UID, third controller still
  unjoined, no reset or duplicate member.
- Fresh token/certificate upload and candidate-style third-controller join
  produced five Ready nodes and three healthy etcd members through the VIP.
  Credentials were not written to repository assets or printed in tool output.
- Stopping HAProxy on the active balancing worker transferred the VIP to its
  peer while the API remained Ready; service was restored.
- After joining the third controller, stopping API and etcd static Pods on the
  second controller preserved API availability and two-of-three etcd quorum.
  Restoration used both a watchdog timer and unconditional cleanup; all three
  etcd members recovered, balancing services were active, cluster UID unchanged.
- The five HA validation VMs were subsequently removed through ownership-checked
  cleanup. A fresh CKA-001 browser regression after HA again passed
  **0 → 100 → 98 → 100**, marks, terminal reconnect, finality, and reset.

## Still required

1. `exam/CKA-002/` with all required files, 20 complete scenarios, exact points
   and domain totals, independent reference solutions, and coverage limitations.
2. Test-owned preparation implementation, readiness assertions, dependency
   vendoring/version/digest/checksum verification, and per-stage/per-fault retry
   markers. The shared hooks alone do **not** provide fixture idempotence.
3. Automatic reserved-address conflict and storage-capacity preflights before
   allocating a CKA-002 lab.
4. Metrics, CSI, Gateway/Ingress, MetalLB, Helm/Kustomize, and maintained
   CRD/operator dependencies. Upstream compatibility research began, but no
   dependency bundle is shipped or functionally validated.
5. All read-only semantic captures, negative and positive functional probes,
   historical evidence handling, and untouched-fixture baseline assertions.
6. CKA-002 100/100 reference run, individual answer deductions, immutable
   submission/restart checks, partial fixture retry, autoscaling under load,
   persistence across replacement, and complete preparation through the new
   server path (live HA validation above invoked shared scripts directly).
7. Full acceptance review and final CKA-002 documentation. Existing static Go
   tests exercise the new server contract, but this checkpoint does not claim
   the entire exam is implemented or accepted.
