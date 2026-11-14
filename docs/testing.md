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
ansible-playbook --syntax-check -i localhost, playbooks/kubeadm-cluster/kubeadm-cluster.yml -e controller_ips=192.0.2.1
```

The live Go test is skipped unless explicitly enabled through the wrapper.
Run relevant local checks after changes; run the live exam validation when
exam setup, student access, answers, or grading changes require it.

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
   ```

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

## Browser coverage

The live exam test does not validate UI rendering, the browser terminal,
submission via HTTP, or reconnect. For changes affecting those paths, use the
optional Playwright checks described in `scripts/e2e/README.md` and the Go
server tests. Do not claim browser coverage from the browser-free run.
For mode changes, also check Simulation expiry and Learning overtime, question
verification, navigation, submission, and terminal reconnect in a browser.
