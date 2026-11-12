# Painkiller Shell

An early single-user CKA practice simulator: a timed question panel and SSH terminal, one Proxmox lab, and state-based checks. This is an MVP under development, **not yet a validated full CKA simulator**. The 17 original tasks, their setup, solutions, and checks must be validated against a real lab before relying on scores. This set does not yet cover the full CKA curriculum.

## Safety and prerequisites

- Run only on a trusted management host and network. There is no authentication or multi-user isolation. The server binds to `127.0.0.1:8080` by default; do not expose it to the internet or an untrusted network.
- The terminal is an SSH session as the configured lab user. The management host needs Go with CGO and a C compiler, Ansible, `community.general`, `ssh`, `git`, `jq`, `flock`, and GNU `timeout`. Provisioning configures the first controller with an administrator kubeconfig and installs `jq` if necessary.
- Passwordless SSH and a verified Proxmox host key are required from the management host. Template `9010` is expected to be pre-configured (SSH access, DNS, DHCP). `create` makes **linked clones** (fast, shared base disk) and only starts them; it does not modify the clone's `net0`, machine-id, cloud-init, or SSH host keys. Clone host keys are read through the Proxmox guest agent and pinned in `data/lab_known_hosts`. Provisioning grants the configured lab user passwordless sudo on these disposable VMs. The template is not modified.
- The browser assets, including xterm.js, are bundled in the server. The server must be started from the repository root for scripts and exam data.
- The external `lab-ansible-playbooks` checkout is required and its revision is checked before provisioning. Its kubeadm playbook can reset Kubernetes state on target hosts. The default cluster uses one controller and two workers. For multiple controllers, configure a working stable control-plane endpoint.
- VM operations are restricted to IDs 3000–3004; the default lab uses 3000–3002. Create refuses existing IDs; start/stop/destroy refuse existing VMs without the configured ownership description. If cloning succeeds but marking fails, clean up that VM manually after confirming ownership. Cleanup is destructive: inspect the target host and VM range first. The description marker is a guard, not an isolation boundary.
- A running candidate can start background processes on lab nodes. Closing the browser terminal at submission does not guarantee those processes stop. Resources are captured sequentially, not atomically, so background processes can change answers during capture; after capture, checks and retries use the saved snapshot. Do not treat this as a secure exam environment.

## Run

```sh
# Configure the management host and lab first; review scripts/create-lab.sh.
go run ./cmd/painkiller
# Open http://127.0.0.1:8080
```

Prepare creates three linked clones and invokes `kubeadm-cluster/kubeadm-cluster.yml`, verifies node readiness, then applies `exam/setup.yaml`. Progress is shown in the web interface and stored in `data/operation.log`. If provisioning fails after all VMs are available, use **Retry preparation**; inspect the error and use reset if the lab is incomplete. Start begins the timer. Submit closes app-managed terminal sessions, captures Kubernetes answer objects, and grades the saved data; expiration does the same. Failed captures can be retried without recapturing successful ones. Reset destroys marked lab VMs. A failed operation requires inspection; if interrupted during cloning, manual cleanup may be needed. SQLite stores the current attempt and sensitive answer snapshots in `data/state.db` with restrictive permissions. A default `data/state.json` from the previous release is imported once when a new database is initialized; the original file is not deleted. Back up state if needed; reset clears the current attempt. Kubeconfigs fetched by Ansible are stored in unique directories under `data/kubeconfigs/`.

Preparation also creates a new, lab-only SSH key on the controller and installs its public key on the other lab nodes. From the terminal, use `kubectl get nodes -o wide` to find node addresses and `ssh <node-ip>` to connect. The private key is generated on the disposable controller and is never copied to the management host.

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `PAINKILLER_LISTEN` | `127.0.0.1:8080` | HTTP listen address |
| `PAINKILLER_STATE` | `data/state.db` | Current attempt SQLite database |
| `PAINKILLER_LAB_LOCK_DIR` | `data` | Shared lab lock directory; all servers managing the same lab must use the same directory |
| `PAINKILLER_MINUTES` | `120` | Exam duration in minutes |
| `LAB_PROXMOX_HOST` | `root@prox-k12` | SSH destination for Proxmox |
| `LAB_TEMPLATE_ID` | `9010` | Proxmox VM template |
| `LAB_VM_MEMORY` | `3072` | Memory per clone in MiB; preflight requires an additional 4 GiB of available host memory |
| `LAB_VM_START`, `LAB_VM_END` | `3000`, `3002` | Inclusive VM range, restricted to 3000–3004 |
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
bash -n scripts/create-lab.sh
```

Validated end-to-end on disposable Proxmox VMs: linked-clone creation (~30s), 3-node Ready cluster, baseline `0/100`, reference solutions `100/100`, a deliberately wrong answer `94/100`, browser submit `100/100`, and reset. Still needed: broader exam coverage; atomic answer capture and isolation; stronger grading and recovery verification; and authentication and session isolation if access expands beyond one trusted user. Results are not guaranteed accurate for every answer variant until each check is validated against clean, correct, and incorrect lab states.
