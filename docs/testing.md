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
python3 -B exam/CKS-001B/build.py --check
python3 -B -m unittest discover -s exam/CKS-001B/tests -v
shellcheck exam/CKS-001B/*.sh exam/CKS-001B/assets/*.sh
(cd exam/CKS-001B/assets && sha256sum --check --quiet SHA256SUMS)
python3 -B -m unittest discover -s drafts/CKS-001B/tests -v
# Optionally set CKS001B_KUBESEC to the pinned kubesec binary and
# CKS001B_BOM_REPORT to an actual bom-generated curl-image report.
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

## Validation discipline

- Generate definitions and refresh asset checksums before starting a live run.
  Freeze runtime assets, setup, solutions and grading for that run; editing them
  during preparation can mix versions and invalidate the result.
- Use a failed disposable guest for ownership-checked diagnosis, but do not call
  a manually repaired guest a clean end-to-end pass. After behavior fixes, rerun
  the complete wrapper on a newly verified-free allocation, including readiness,
  baseline, reference answers, every listed mutation and cleanup.
- First-boot cloud-init/package upgrades can restart networking and change a
  guest's DHCP address after initial SSH readiness. Observe boot completion
  through the ownership-checked guest agent, rediscover the address, and verify
  its SSH key through that agent. Never compensate by changing clone NIC, DHCP,
  machine identity or host keys. Keep exam-specific handling exam-owned.
- Report passed, failed and skipped checks separately. Optional real-tool tests
  need the actual pinned executable and real evidence; confirm temporary paths
  still exist before reusing them after a session resumes.

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
    LAB_VM_START=3100 scripts/test-exam.sh CKS-001B
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

CKS-001B uses **one** schedulable controller from base 9199, no workers,
8192 MB minimum, full-install Kubernetes/Calico and no reserved LAN addresses.
It needs kernel BTF for Falco, package/image downloads, minimal Istio sidecars,
Traefik's actual Kubernetes Ingress NGINX provider, and loopback-only network
fixtures in 198.18.0.0/15. Require all 16 task-level loss/restore mutations.
Its etcd fault and recursive fixes are restricted to the verified data tree
of its authorized disposable guest. See `exam/CKS-001B/README.md`.
Preparation observes cloud-init completion through the owned VM's guest agent
before using its final DHCP address; do not work around boot-time address
changes by replacing guest NIC, DHCP or identity configuration.

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

## Workstation image validation

Local, network-free checks:

```sh
shellcheck scripts/build-workstation-template.sh scripts/workstation/create-image.sh scripts/test-workstation-image.sh scripts/validate-workstation-image.sh playbooks/workstation/files/desktop-session.sh
python3 -B scripts/workstation/test_image.py -v
(cd playbooks/workstation && sha256sum --check --quiet SHA256SUMS)
ansible-playbook --syntax-check -i localhost, playbooks/workstation/workstation.yml
```

The local Python suite stubs network/hypervisor commands. It checks invalid
IDs/accounts, preflight failures, signed-download failures, checksum failures,
occupied imports, the shared operation lock, and DNS conflict preservation.
DNS readiness cases distinguish a completed successful oneshot from an
unstarted, in-progress or failed invocation.
These checks do not create or destroy VMs.

After inspecting the actual host and confirming both IDs are free, run the
dedicated wrapper (substitute the verified first ID):

```sh
WORKSTATION_TEST_START=3100 bash scripts/test-workstation-image.sh
```

It uses the common operation lock, refuses occupied IDs/non-workstation templates,
retains failed guests and artifacts, and destroys its two owned clones on success.
It checks actual Firefox/terminal windows, VNC resize/reconnect and bidirectional
VNC/XFCE clipboard transfer, not the app's not-yet-implemented noVNC UI or cross-browser
clipboard integration. Its cloud-init checker accepts only Proxmox's known
deprecated string-valued `user` metadata; other boot warnings/errors fail.
Image baking enables registration for future clones but does not register the
temporary builder. For a diagnosed external DNS API outage only, explicitly
set `WORKSTATION_TEST_SKIP_DNS=1` to validate the rest of the desktop image.
Report live DNS registration as **skipped**, not passed, and rerun without the
override after repair. The default clone validation still requires registration.

Live validation requires an explicitly authorized workstation image build and
two freshly verified-free IDs within `3000–3999`. Follow the ownership and
locking discipline above and [workstation-image.md](workstation-image.md).
Check the template's provenance/description before making linked clones.
Use the existing `k8s-<ID>` lab hostname prefix, set memory and start; do not
change clone NICs, cloud-init, machine-id or host keys. Wait for cloud-init via
the owned guest agent, discover final DHCP addresses and pin SSH keys through
that agent. Run `scripts/validate-workstation-image.sh` as the lab user in each
clone; compare distinct addresses, hostnames and host keys. Test Firefox and
terminal windows on the actual virtual display, VNC reconnect, resizing and
clipboard before claiming desktop coverage. Preserve failures for diagnosis;
on success destroy only the two verified-owned clones. Never run the Kubernetes
provisioner against the workstation. No exam scoring/UI integration is covered.
