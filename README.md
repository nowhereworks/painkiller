# Painkiller Shell

An early single-user CKA practice simulator: a timed question panel and SSH terminal, one Proxmox lab, and state-based checks. This is an MVP under development, **not yet a validated full CKA simulator**. The 17 original tasks are a first question set; their setup, solutions, and checks must be validated against a real lab before relying on scores. Some tasks depend on earlier tasks, and this set does not yet cover the full CKA curriculum.

## Safety and prerequisites

- Run only on a trusted management host and network. There is no authentication or multi-user isolation. The server binds to `127.0.0.1:8080` by default; do not expose it to the internet or an untrusted network.
- The terminal is an SSH session as the configured lab user. This user needs `kubectl`, an administrator kubeconfig, `jq`, and `base64` on the first controller. The management host needs Go, Ansible, `community.general`, `ssh`, `jq`, and GNU `timeout`. Both the management host and Proxmox host need `jq` only if your own tooling requires it (the provided script runs `jq` locally).
- Passwordless SSH and verified host keys are required from the management host to Proxmox and the lab nodes. Nodes must be Debian-family hosts with unique machine IDs, DHCP identities, and reachable addresses. Prepare the template accordingly: the script does **not** reset machine IDs or DHCP leases after cloning.
- The browser loads xterm.js and its fit addon from jsDelivr, requiring network access. The server must be started from the repository root.
- The external `lab-ansible-playbooks` checkout is required. Review and pin it to a known revision yourself before running; the application does not pin or verify it. Its kubeadm playbook can reset Kubernetes state on target hosts. For three controllers, configure a working stable control-plane endpoint; the fallback to the first controller is not HA.
- IDs 3000–3004 are reserved by default. Create refuses existing IDs; start/stop/destroy refuse existing VMs without the configured ownership description. If cloning succeeds but marking fails, clean up that VM manually after confirming ownership. Cleanup is destructive: inspect the target host and VM range first. The description marker is a guard, not an isolation boundary.
- A running candidate can start background processes on lab nodes. Closing the browser terminal at submission does not guarantee those processes stop; scores are not tamper-proof. Do not treat this as a secure exam environment.

## Run

```sh
# Configure the management host and lab first; review scripts/create-lab.sh.
go run ./cmd/painkiller
# Open http://127.0.0.1:8080
```

Prepare creates five linked clones and invokes `kubeadm-cluster/kubeadm-cluster.yml`, then applies `exam/setup.yaml`. Start begins the 120-minute timer. Submit closes app-managed terminal sessions and runs checks; expiration does the same. Reset destroys marked lab VMs. A failed operation requires inspection and reset; if interrupted during creation, an unmarked VM may need manual cleanup. State is stored in `data/state.json` with restrictive permissions. Back it up if attempt history matters; reset clears the current attempt.

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `PAINKILLER_LISTEN` | `127.0.0.1:8080` | HTTP listen address |
| `PAINKILLER_STATE` | `data/state.json` | Current attempt state file |
| `LAB_PROXMOX_HOST` | `root@prox-k12` | SSH destination for Proxmox |
| `LAB_TEMPLATE_ID` | `9010` | Proxmox VM template |
| `LAB_VM_START`, `LAB_VM_END` | `3000`, `3004` | Inclusive reserved VM range |
| `LAB_CONTROLLERS` | `3` | Number of first VMs assigned control-plane role |
| `LAB_MARKER` | `painkiller-mvp` | Required Proxmox VM description |
| `LAB_IP_PREFIX` | `192.168.254.` | Prefix used to select exactly one guest IPv4 address |
| `LAB_READY_TIMEOUT` | `600` | Guest-agent/SSH readiness timeout per VM in seconds |
| `LAB_SSH_USER` | `milkman` | Node SSH user |
| `LAB_PLAYBOOKS_DIR` | `$HOME/repo/_nowhereworks/lab-ansible-playbooks` | External playbook checkout |
| `LAB_KUBERNETES_VERSION` | `1.36.1` | kubeadm playbook version |
| `LAB_CNI_PLUGIN` | `flannel` | kubeadm playbook CNI |
| `LAB_CONTROL_PLANE_ENDPOINT` | unset | Stable API endpoint, e.g. `192.0.2.10:6443` |

`scripts/create-lab.sh help` lists the direct lifecycle commands. `status` emits JSON; `provision` requires an existing lab. Inspect the actual environment before using any command. The script does not install dependencies or a load balancer.

## Development

```sh
go test -race ./...
bash -n scripts/create-lab.sh
```

Still needed before considering this production-ready: end-to-end verification on disposable Proxmox VMs; stronger grading error detection and coverage; isolated and independent exam scenarios; reliable interruption recovery; local/vendor terminal assets; and secure authentication and session isolation if access expands beyond one trusted user. Results are not guaranteed accurate until each check is validated against clean, correct, and incorrect lab states.
