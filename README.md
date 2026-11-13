# Painkiller Shell

An early single-user CKA practice simulator: a timed question panel and SSH terminal, one Proxmox lab, and state-based checks. This is an MVP under development, **not an official CKA exam simulator**. The packaged exam has 29 tasks worth 100 points covering cluster architecture, workloads and scheduling, services and networking, storage, troubleshooting, and RBAC; every task was validated against a real 3-node lab (see [Development](#development)).

## Safety and prerequisites

- Run only on a trusted management host and network. There is no authentication or multi-user isolation. The server binds to `127.0.0.1:8080` by default; do not expose it to the internet or an untrusted network.
- The terminal is an SSH session as the configured lab user. The management host needs Go with CGO and a C compiler, Ansible, `community.general`, `ssh`, `git`, `jq`, `flock`, and GNU `timeout`. Provisioning configures the first controller with an administrator kubeconfig and installs `jq` and `etcd-client` if necessary.
- Passwordless SSH and a verified Proxmox host key are required from the management host. Template `9010` is expected to be pre-configured (SSH access, DNS, DHCP). `create` makes **linked clones** (fast, shared base disk) and only starts them; it does not modify the clone's `net0`, machine-id, cloud-init, or SSH host keys. Clone host keys are read through the Proxmox guest agent and pinned in `data/lab_known_hosts`. Provisioning grants the configured lab user passwordless sudo on these disposable VMs. The template is not modified.
- The browser assets, including xterm.js, are bundled in the server. The server must be started from the repository root for scripts and exam data.
- The external `lab-ansible-playbooks` checkout is required and its revision is checked before provisioning. Its kubeadm playbook can reset Kubernetes state on target hosts. The default cluster uses one controller and two workers. For multiple controllers, configure a working stable control-plane endpoint.
- VM operations are restricted to IDs 3000–3999; the default lab uses 3000–3002. Create refuses existing IDs; start/stop/destroy refuse existing VMs without the configured ownership description. If cloning succeeds but marking fails, clean up that VM manually after confirming ownership. Cleanup is destructive: inspect the target host and VM range first. The description marker is a guard, not an isolation boundary.
- A running candidate can start background processes on lab nodes. Closing the browser terminal at submission does not guarantee those processes stop. Answers are captured after submission with bounded concurrency (six captures at a time), so the whole cluster is read within a few seconds; that is a much smaller window than a sequential capture but still not a transactional cluster snapshot. Once captured, checks and retries use the saved snapshot and never re-read live state. Do not treat this as a secure exam environment.

## Run

```sh
# Configure the management host and lab first; review scripts/create-lab.sh.
go run ./cmd/painkiller
# Open http://127.0.0.1:8080
```

Prepare creates three linked clones and invokes `kubeadm-cluster/kubeadm-cluster.yml`, verifies node readiness, installs `jq` and the `etcd-client` tools on the controller, then applies `exam/setup.yaml`. Progress is shown in the web interface and stored in `data/operation.log`. If provisioning fails after all VMs are available, use **Retry operation**; inspect the error and use reset if the lab is incomplete. Start begins the timer. Submit closes app-managed terminal sessions, captures Kubernetes answer objects, and grades the saved data; expiration does the same. Failed captures can be retried without recapturing successful ones. Reset destroys marked lab VMs; a failed reset can be retried as well. A failed operation requires inspection; if interrupted during cloning, manual cleanup may be needed. SQLite stores the current attempt and sensitive answer snapshots in `data/state.db` with restrictive permissions. A default `data/state.json` from the previous release is imported once when a new database is initialized; the original file is not deleted. Back up state if needed; reset clears the current attempt. Kubeconfigs fetched by Ansible are stored in unique directories under `data/kubeconfigs/`.

Preparation also creates a new, lab-only SSH key on the controller and installs its public key on the other lab nodes. From the terminal, use `kubectl get nodes -o wide` to find node addresses and `ssh <node-ip>` to connect. The private key is generated on the disposable controller and is never copied to the management host.

### Prebaked image (opt-in)

To avoid repeating Kubernetes package installation and image downloads on every lab, build a shared controller/worker template from `9010`:

```sh
scripts/build-template.sh
LAB_PROVISION_MODE=prebaked go run ./cmd/painkiller
```

The builder full-clones `9010` to the unused template ID `9100`, prepares packages and cached images, then shuts down and converts the VM into a template. It requires the same pinned playbook checkout as the legacy path and network access during the build. It takes the lab-operation lock, refuses an occupied destination, and never modifies the base. A failed build leaves its marked VM for inspection; after investigating, `scripts/build-template.sh --resume` can resume an unfinished VM with the exact builder marker. It refuses templates and unmarked VMs. Do not blindly delete an unverified VM. Only IDs `9100–9200` are accepted as builder destinations.

Prebaked mode verifies the template marker, package versions, guest build metadata, and cached images, then makes linked clones and bootstraps a new cluster without the playbook's package-preparation phase. It currently supports one controller, Flannel, and the default control-plane endpoint only. Incompatible images or settings fail rather than silently falling back. The original provisioning remains available with `LAB_PROVISION_MODE=legacy` (the default). Build updated images under a new free ID; do not replace a template that has dependent linked clones. Cached images do not make the entire lab offline: for example, first-boot services may still use the network.

The image contains no initialized Kubernetes cluster, certificates, admin kubeconfig, etcd data, or candidate SSH key. `/opt/painkiller/` records the build identity, exact installed package versions, cached image metadata, the runtime's sandbox image, and the cached Flannel manifest checksum. Kubernetes packages are held at the requested patch version. Both kubeadm's pause image and containerd's sandbox default are cached, since these can differ. A retry preserves a healthy initialized cluster; an unhealthy control plane or changed controller address requires inspection and an explicit lab reset, not an automatic `kubeadm reset`.

Live validation on 2026-11-13, one controller and two workers on `prox-k12`:

| Phase | Legacy | Prebaked run 1 | Prebaked run 2 |
| --- | ---: | ---: | ---: |
| Linked cloning and start | 16 s | 14 s | 14 s |
| Guest readiness | 13 s | 12 s | 11 s |
| Provision to all nodes Ready | 207 s | 46 s | 41 s |
| Sum of logged phases | 236 s | 72 s | 66 s |

These are phase timings, excluding inter-command status overhead, candidate access setup, and exam object setup; run 1's external create/provision wall time was 76 s. They are measurements, not a startup-time guarantee. Legacy retains its existing package behavior (the minor repository's current patch); the prebaked nodes were verified at exactly `1.36.1`. Run 1 verified distinct node identities, no Kubernetes bootstrap image-pull events, and retry preserving the same cluster. Run 2 passed the browser/scoring flow `0 → 100 → 98 → 100`, terminal reconnect, final submission, and reset. Tests used the separate marked range `3100–3102`, then removed it; the existing lab was not reset.

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `PAINKILLER_LISTEN` | `127.0.0.1:8080` | HTTP listen address |
| `PAINKILLER_STATE` | `data/state.db` | Current attempt SQLite database |
| `PAINKILLER_LAB_LOCK_DIR` | `data` | Shared lab lock directory; all servers managing the same lab must use the same directory |
| `PAINKILLER_MINUTES` | `120` | Exam duration in minutes |
| `LAB_PROXMOX_HOST` | `root@prox-k12` | SSH destination for Proxmox |
| `LAB_PROVISION_MODE` | `legacy` | `legacy` or opt-in `prebaked` |
| `LAB_TEMPLATE_ID` | `9010` (legacy), `9100` (prebaked) | Proxmox VM template |
| `LAB_IMAGE_TEMPLATE_ID` | `9100` | Destination for `scripts/build-template.sh` (9100–9200) |
| `LAB_BASE_TEMPLATE_ID` | `9010` | Builder source; currently must be 9010 |
| `LAB_VM_MEMORY` | `3072` | Memory per clone in MiB; preflight requires an additional 4 GiB of available host memory |
| `LAB_VM_START`, `LAB_VM_END` | `3000`, `3002` | Inclusive VM range, restricted to 3000–3999 |
| `LAB_CONTROLLERS` | `1` | Number of first VMs assigned control-plane role |
| `LAB_MARKER` | `painkiller-mvp` | Required Proxmox VM description |
| `LAB_IP_PREFIX` | `192.168.254.` | Prefix used to select exactly one guest IPv4 address |
| `LAB_READY_TIMEOUT` | `600` | Guest-agent/SSH readiness timeout per VM in seconds |
| `LAB_SSH_USER` | `milkman` | Node SSH user |
| `LAB_KNOWN_HOSTS` | `data/lab_known_hosts` | Lab-managed guest host-key file; do not point at a general SSH known-hosts file |
| `LAB_PLAYBOOKS_DIR` | `$HOME/repo/_nowhereworks/lab-ansible-playbooks` | External playbook checkout |
| `LAB_PLAYBOOKS_REV` | `8ec676e93017ce3e110c27485e25d1f32d541056` | Required playbook Git revision |
| `LAB_KUBERNETES_VERSION` | `1.36.1` | kubeadm playbook version |
| `LAB_CNI_PLUGIN` | `flannel` | kubeadm playbook CNI |
| `LAB_CONTROL_PLANE_ENDPOINT` | unset | Stable API endpoint, e.g. `192.0.2.10:6443` |

`scripts/create-lab.sh help` lists the direct lifecycle commands. `status` emits JSON; `provision` requires an existing lab. Inspect the actual environment before using any command. The script does not install dependencies or a load balancer.

The default Flannel CNI does not enforce NetworkPolicy. The NetworkPolicy exercise checks the Kubernetes resource; use a policy-capable CNI such as Calico if network enforcement is required.

Checks award the points shown in the result breakdown. Capture commands have a timeout and one automatic retry. Infrastructure or capture failures are reported separately from incorrect answers, and totals with grading errors are provisional. A manual retry recaptures only failed resources; previously captured answers are not changed. `exam/reference-solutions.sh` is intended only for validation against a fresh disposable exam lab.

## Development

```sh
go test -race ./...
go vet ./...
shellcheck scripts/create-lab.sh scripts/build-template.sh scripts/validate-image.sh scripts/configure-exam-access.sh exam/reference-solutions.sh
node --check web/app.js
```

End-to-end checks on a real lab live in `scripts/e2e/` (see its README):
`node scripts/e2e/live.js` prepares the lab, scores 0, applies the reference
solutions (100), removes one answer (100 − that task), submits through the
browser, verifies submission is final, and resets.

Live validation on disposable Proxmox VMs: three linked clones created and
provisioned in a few minutes, 29-task exam, baseline `0/100`, reference
solutions `100/100`, a deliberately wrong answer `98/100`, browser submit
`100/100`, and reset. Known limitations: not tamper-proof (a candidate can
leave background processes running), answers are captured concurrently rather
than as a transaction, NetworkPolicy is graded as configuration only under
Flannel, and there is no authentication or multi-user isolation.
