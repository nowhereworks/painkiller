# Testing runbook

Use the smallest relevant checks first. Run commands from the repository root.
Update this file when the testing workflow or lab requirements change. A
planned or attempted run is not a passing run; report actual results and errors.

## Local checks

These do not create lab VMs:

```sh
go test -race ./...
go vet ./...
node --check web/app.js
shellcheck -x scripts/test-exam.sh scripts/create-lab.sh scripts/build-template.sh scripts/playbooks.sh scripts/validate-image.sh scripts/configure-exam-access.sh exam/*/reference-solutions.sh exam/*/check-ready.sh
shellcheck exam/CKA-003/prepare.sh exam/CKA-003/assets/capture.sh
shellcheck exam/CKAD-001/*.sh exam/CKAD-001/assets/*.sh
shellcheck exam/OPA-001/*.sh exam/OPA-001/assets/*.sh
(cd exam/OPA-001/assets && sha256sum --check --quiet SHA256SUMS)
python3 -B exam/OPA-001/tests/test_capture.py -v
python3 -B exam/CKS-001/build.py --check
python3 -B -m unittest discover -s exam/CKS-001/tests -v
python3 -B -m unittest discover -s drafts/CKS-001B/tests -v
# Optionally set CKS001B_KUBESEC to the pinned kubesec binary for the real-CLI test.
shellcheck drafts/CKS-001B/prepare.sh drafts/CKS-001B/assets/install.sh
shellcheck exam/CKS-001/*.sh exam/CKS-001/assets/*.sh
(cd exam/CKS-001/assets && sha256sum --check --quiet SHA256SUMS)
(cd exam/CKAD-001/assets && sha256sum --check --quiet SHA256SUMS)
(cd exam/CKA-003/assets && sha256sum --check --quiet SHA256SUMS)
ansible-playbook --syntax-check -i localhost, playbooks/kubeadm-cluster/kubeadm-cluster.yml -e controller_ips=192.0.2.1
```

The live Go test is skipped unless explicitly enabled through the wrapper.
Run relevant local checks after changes; run the live exam validation when
exam setup, student access, answers, or grading changes require it.
For capture diagnostics and error classification, the local
`TestVerificationDiagnosticsAndProbeErrors` cases exercise valid/missing answers,
failed application probes, and failed exec transport through the real capture
scripts and grader. Exam-owned missing-answer tests require diagnostic text for
every CKAD-001/CKA-003 question. `TestCKA002RBACCapture` distinguishes authorization
denials from API errors; `TestKubectlLogSubresourceSpelling` optionally uses an
installed kubectl against a local fake API (never the configured cluster).
Supporting probe failures must remain capture errors even when kubectl reports
`(NotFound)`: the generic capture path otherwise treats that text as a missing
candidate answer. Sanitize supporting exec stderr and emit a distinct error.

## Browser-free live exam validation

This provisions real disposable Proxmox VMs, runs candidate commands, modifies
exam answers, and destroys the created lab **on success**. It does not require
a running Painkiller server, Node.js, Playwright, or a browser.

Prerequisites: the normal lab management host setup (Go with CGO and a C
compiler, Ansible and the repository's required collections, SSH, jq, flock,
GNU timeout), verified Proxmox SSH host key, guest SSH access, and the selected
exam's compatible prebuilt template. See the root README and
`exam/<ID>/test.json` for current infrastructure and topology requirements.
Do not build or replace a template as part of this test.

1. Inspect the actual Proxmox host and select a **completely free** contiguous
   range within VM IDs 3000–3999. Its size must fit the exam's controllers and
   workers (`test.json`); the default is three VMs. Do not use an active lab's
   range, even if its ownership marker matches. For CKA-002 inspect active
   allocations for its reserved LoadBalancer address before use; ping alone
   does not establish availability.
2. Run, substituting the verified free first ID:

   ```sh
   LAB_VM_START=3100 scripts/test-exam.sh CKA-001
   LAB_VM_START=3100 scripts/test-exam.sh CKA-002
   LAB_VM_START=3100 scripts/test-exam.sh CKA-003
   LAB_VM_START=3100 scripts/test-exam.sh CKAD-001
    LAB_VM_START=3100 scripts/test-exam.sh OPA-001
    LAB_VM_START=3100 scripts/test-exam.sh CKS-001
   ```

   Exam IDs may have a single uppercase suffix, such as `CKS-001B`.
   Run exams sequentially and verify the range is free before each run. The
   exam contract determines topology; the wrapper derives the last VM ID from
   that topology, and conflicting explicit `LAB_*` settings fail. The script
   uses a unique marker and known-hosts file and the common
   `data/lab-operation.lock`. It does not reuse a running server's attempt.
3. Check the output: the exam-owned readiness check must pass before the lab is
   ready; the exam-owned `e2e.json` baseline must stabilize twice;
   candidate reference commands must score 100/100; every listed mutation must
   remove exactly its task's points without changing other task scores, then
   restore 100/100. Scoring uses the production grader with fresh answer
   snapshots at every checkpoint. Infrastructure/capture errors fail the run.
   Mutation coverage is limited to the mutations actually listed in the fixture.

Commands run over SSH as `LAB_SSH_USER` (default `milkman`) on the first
controller, with the same kubeconfig, node SSH access and passwordless sudo
available to the student. Individual reference commands may use sudo; the
whole solution is not run as root. Reference scripts and mutation fixtures are
trusted code owned by each exam; run them only on the newly created lab.

The script prints its marker and artifact directory under `data/exam-test.*`.
The directory contains `operation.log` and pinned guest host keys; treat these
as sensitive. Successful runs destroy their marked VMs but retain artifacts.
Failures deliberately leave the lab and artifacts for inspection. Check the
operation log, Proxmox host, exact VM IDs, and descriptions before any cleanup.
Never destroy VMs just because a marker matches: a clone can exist without a
marker after an interrupted creation, and requires manual ownership verification.
To use the normal destroy command after verifying ownership, set the same
`LAB_PROXMOX_HOST`, `LAB_VM_START`, resolved `LAB_VM_END`, and printed
`LAB_MARKER` used for the run; review `scripts/create-lab.sh` first. The
destructive command is `scripts/create-lab.sh destroy`. Coordinate with any
other lab operator and preserve the shared operation lock.

CKA-003 uses three VMs and no reserved LAN addresses. Its preparation installs
exam-owned NFS CSI, routing, and metrics dependencies; it needs package/image
downloads and a working NFS server on the first worker. Validate all 16 scoring
mutations, including the PVC expansion-capability check. See its exam-owned
README for source adaptations and evidence limitations.

CKAD-001 uses three linked clones of base 9199 with full-install provisioning
and Calico, not the default prebaked Flannel image. Its readiness check verifies
real ingress/egress isolation with separate canaries. It needs package/image
downloads and its host HTTP fixture on port 9998. Validate all 21 loss/restore
mutations; see `exam/CKAD-001/README.md` for source corrections and boundaries.

OPA-001 uses **one** linked clone of base 9199, 6144 MB minimum, full-install
Kubernetes/Flannel, and a schedulable control plane; there are no workers or
reserved LAN addresses. Preparation installs checksum-pinned OPA, Conftest,
Regal, and Gatekeeper dependencies. Local Python real-CLI cases require the
exam's pinned tools on PATH and explicitly skip without them. Live grading
observes real bundle activation, newly masked decision logs, dry-run admission,
and existing audit violations. Require all 16 loss/restore mutations. See
`exam/OPA-001/README.md` for source mappings and assessed/unassessed boundaries.

CKS-001 uses **one** schedulable control-plane clone of base 9199, no workers,
8192 MB minimum, full-install Kubernetes/Calico, and no reserved LAN addresses.
Preparation requires AppArmor and kernel BTF for real Falco modern-eBPF capture,
plus package/image downloads for exam-owned tools, ingress, image archives and
the TLS ImageReview server. Require all 15 task-level loss/restore mutations.
Its short API-server termination grace and disabled controller leader election
are disposable single-node exam tuning, not shared or production defaults.
Its independent witness generates audit events and admission dry runs; grading
does not mutate workloads or stop the webhook. See `exam/CKS-001/README.md`
for compatibility pins, source adaptations and evidence boundaries.

## Browser coverage

The live exam test does not validate UI rendering, the browser terminal,
submission via HTTP, or reconnect. For changes affecting those paths, use the
optional Playwright checks described in `scripts/e2e/README.md` and the Go
server tests. Do not claim browser coverage from the browser-free run.
For mode changes, also check Simulation expiry and Learning overtime, question
verification, navigation, submission, and terminal reconnect in a browser.
For exam layout changes, run `node scripts/e2e/layout.js` (Playwright and Chrome
required; no server or lab). It checks panel resizing, terminal fitting and
bottom spacing, no exam page overflow, and preserved results-page scrolling.
For Learning verification rendering, run `node scripts/e2e/verification.js`
(same prerequisites; mocked HTTP, no server or lab). It checks separate
green/red/amber result blocks, explicit requirement labels, plain-text
diagnostics, capture errors, and discarded responses after question navigation.
