# OPA-001 — unofficial practical exam

150 minutes, 16 independent tasks, 100 points. This is an original hands-on
adaptation of the 62-question bank in `sim-opa/data/questions.json` version
1.0.1, not an official certification or an assertion of an official syllabus.
The source repository is an authoring reference only: runtime, preparation,
grading, and reference answers never depend on that checkout.

## Topology and dependencies

One control-plane VM, **zero workers**, 6144 MB minimum. All workloads schedule
on that node; Gatekeeper has one webhook controller and one audit replica.
No question needs another machine, reserved LAN addresses, HA, an ingress
controller, an external database, or production-grade redundancy.

Use linked clones of base `9199` with full-install (`legacy`) Kubernetes/Flannel
provisioning. No specialized image is built or modified. Existing exams and
shared cluster playbooks are unchanged. Exam preparation removes the controller
scheduling taint only on this exam's disposable guest.

The guest downloads the following Linux/amd64 releases, with pinned SHA256
verification in `assets/install.sh` and `prepare.sh`:

| Component | Version |
| --- | --- |
| OPA | 1.22.2 (static CLI/server binary) |
| Gatekeeper | 3.23.1 (upstream release manifest, single-node overlay) |
| Conftest | 0.71.1 |
| Regal | 0.43.0 |

Gatekeeper embeds its own OPA version; it does not use the installed CLI.
Templates explicitly opt into Rego v1 through the Rego code engine source.
Provisioning needs package, GitHub release, Kubernetes, Flannel, and Gatekeeper
image access. Image tags are those in the checksum-pinned upstream manifest;
they are not claimed to be independently digest-pinned.

Student files are under `/opt/opa-workshop/q1` through `q16`. Trusted captures,
fixtures, immutable recovery artifacts, and mutation policies are under
`/opt/painkiller/opa001`. Supporting services listen only on guest loopback:

| Port | Service |
| --- | --- |
| 18888 | Bundle server and bounded in-memory status/decision-log receiver |
| 18189 | Q9 release OPA |
| 18190 | Q10 recovery OPA |
| 18191 | Q11 logging OPA |

Systemd units are `opa001-receiver`, `opa001-q9`, `opa001-q10`, `opa001-q11`.
Status routing uses `status.partition_name: q9`/`q10` (OPA does not support a
`status.resource` field); decision logs use `decision_logs.resource: logs/q11`.
Each task owns separate files/services or namespace-scoped constraints. Q13
and Q14 have preinstalled templates independent of Q12. Preparation phase
markers preserve completed candidate work on retries. Reset destroys the VM;
it does not rebuild or mutate any template.

## Coverage and source mapping

IDs below identify source concepts, not verbatim question copies.

| Task | Points | Source question IDs |
| --- | ---: | --- |
| Q1 Boolean decision contract | 5 | REG-START-01/02, REG-BANK-02, TST-BANK-30 |
| Q2 Tenant-scoped RBAC/trust boundaries | 8 | AUT-START-02, AUT-BANK-26/TENANT |
| Q3 ABAC and suspension precedence | 7 | AUT-START-01, AUT-BANK-28, REG-BANK-23/24 |
| Q4 Partial violation sets | 5 | REG-BANK-06/08 |
| Q5 Negation and nonempty universal approvals | 5 | REG-BANK-09/24/25 |
| Q6 Mutation-sensitive Rego tests | 8 | TST-BANK-INTRO/10/11/29/31 |
| Q7 Enforcement result adapter | 7 | FND-START-01/02, FND-BANK-05, INT-BANK-12/13 |
| Q8 Relationship graph authorization | 5 | AUT-BANK-27 |
| Q9 Bundles, roots, revision, activation | 7 | OPS-BANK-03/15/34 |
| Q10 Failed activation and recovery | 6 | OPS-BANK-15/35, FND-BANK-22 |
| Q11 Actual decision-log masking | 6 | OPS-BANK-16/36 |
| Q12 Parameterized Gatekeeper template | 6 | GTK-START-02, GTK-DD-001 |
| Q13 Matching, admission, existing audit violations | 5 | GTK-START-01, GTK-BANK-17 |
| Q14 Inventory synchronization | 6 | GTK-BANK-18/37, GTK-DD-002 |
| Q15 Conftest and Regal | 4 | ECO-BANK-04/21 |
| Q16 Representative profiling and safe optimization | 10 | SEC-BANK-20/41, SEC-DD-003 |

Q7 deliberately isolates a CLI result-interpretation adapter; it is not a live
HTTP enforcement application or authentication exercise. Q10 uses a real
compile-invalid artifact, not an overlapping-root/fleet rollout simulation.
Q16 compares freshly measured expression evaluation counts, not timing; saved
profile/benchmark artifacts provide supporting evidence, not a proof of all
workload equivalence. Q6 detects five specified mutants, not every possible
policy bug. Broad Rego-v0 migration, OPA API authentication/authorization, signed
bundles, discovery, partial evaluation, Wasm host integration, Envoy, cache
invalidation, Gatekeeper mutation, and multi-instance canaries are not assessed
by this first practical exam. Explanations link to official tool documentation.

## Grading and validation

Captures are bounded and observational: local policy evaluations, candidate
test/adapter execution, GETs, decision POSTs, and server-side dry-run admission.
Admission probes do not persist API objects. Log queries may produce new
decision events in the bounded receiver, but never rewrite candidate answers.
Candidate policy syntax/missing-answer errors produce failed checks with
diagnostics; supporting API/receiver/transport errors are capture errors.
Learning verification and submission share the same capture commands.
Template CRDs are awaited for creation before establishment. The Q14 template
comparison normalizes the API's `legacySchema: false` default, without allowing
changes to the supplied referential policy.

Read `docs/testing.md` before validation. Local checks:

```sh
shellcheck exam/OPA-001/*.sh exam/OPA-001/assets/*.sh
(cd exam/OPA-001/assets && sha256sum --check --quiet SHA256SUMS)
python3 -B exam/OPA-001/tests/test_capture.py -v
go test ./cmd/painkiller -run '^TestOPA001' -v
```

Real CLI cases require the pinned `opa`, `conftest`, and `regal` on PATH;
otherwise Python explicitly skips those cases. The tests never contact the
configured Kubernetes cluster. Live validation requires a freshly verified free
single VM ID (substitute that ID for the example `3100`):

```sh
LAB_VM_START=3100 scripts/test-exam.sh OPA-001
```

Require a stable 0/100 baseline, references at 100/100, all 16 independent
loss/restore mutations, and successful cleanup. Reference and mutation scripts
refuse an unmarked cluster. This does not provide browser/UI coverage.

Validated on 2026-11-14 with a fresh linked clone of `9199`: readiness passed,
baseline stabilized at 0/100, references scored 100/100 through the production
grader, all 16 mutations independently lost/restored their exact task points,
and the disposable VM was destroyed successfully. The local real-CLI regression
suite also passed, including status delivery to the actual receiver. No browser
coverage is claimed.
