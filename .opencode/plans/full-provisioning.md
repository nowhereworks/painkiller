# Full Provisioning: Missing Pieces

## Context

The Proxmox provider can now clone/start/stop/delete VMs and retrieve IPs. The no-op provisioner validates the VM lifecycle. But the full provisioning pipeline — kubeadm cluster init, worker join, CNI install, workstation kubeconfig setup, scenario-specific setup — requires Ansible playbooks, Docker image changes, code fixes, and Proxmox VM templates. This plan covers everything needed to go from "VMs exist" to "student can kubectl into clusters."

---

## 1. Fix SSH Key Encoding Bug

**File:** `internal/orchestrator/provision_job.go:56-59`

The current code uses `pem.EncodeToMemory` with raw `ed25519.PrivateKey` bytes, which produces an invalid OpenSSH key file. Ansible (OpenSSH) cannot parse this format. The terminal gateway's `ssh.ParsePrivateKey` likely also fails with this encoding.

**Fix:** Replace with `ssh.MarshalPrivateKey` which produces correct OpenSSH format:

```go
// Before (broken):
privKeyPEM := pem.EncodeToMemory(&pem.Block{
    Type:  "OPENSSH PRIVATE KEY",
    Bytes: privKey,
})

// After (correct):
privKeyPEM, err := ssh.MarshalPrivateKey(privKey, "")
if err != nil {
    return fmt.Errorf("failed to marshal SSH private key: %w", err)
}
privKeyBytes := pem.EncodeToMemory(privKeyPEM)
```

Then use `privKeyBytes` everywhere `privKeyPEM` was used (environment record, provisioner spec).

Remove the unused `encoding/pem` import if no longer needed (it's still needed for `pem.EncodeToMemory`).

---

## 2. Fix Cloud-Init Type for Linux Guests

**File:** `internal/provider/proxmox/provider.go` — `cloudInitConfig` method

The current code sets `citype=configdrive2`, which is for OpenStack/Windows guests. Linux guests (Ubuntu) need `nocloud`.

**Fix:** Change `citype` from `configdrive2` to `nocloud`.

---

## 3. Ansible Provisioner: Set ANSIBLE_ROLES_PATH

**File:** `internal/provisioner/ansible/ansible.go`

The provisioner runs `ansible-playbook` but doesn't set `ANSIBLE_ROLES_PATH`, so shared roles won't be found.

**Fix:** Add environment variable to the exec command:

```go
cmd.Env = append(os.Environ(), "ANSIBLE_ROLES_PATH=/app/ansible/roles")
```

---

## 4. Shared Ansible Roles

Create shared roles under `ansible/roles/` (new top-level directory). These roles are reused by all scenarios.

### 4a. Role: `kubeadm-init`

**Path:** `ansible/roles/kubeadm-init/tasks/main.yaml`

Initializes a kubeadm control plane on the first node of each cluster group.

Tasks:
1. Run `kubeadm init --apiserver-advertise-address={{ ansible_host }} --pod-network-cidr={{ pod_cidr | default('10.244.0.0/16') }}`
2. Register the init output
3. Extract the `kubeadm join` command from the output using regex
4. Store the join command as a host fact for worker nodes to consume
5. Install Calico CNI (`kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/v3.28.0/manifests/calico.yaml`)
6. Wait for control plane to be ready

### 4b. Role: `kubeadm-join`

**Path:** `ansible/roles/kubeadm-join/tasks/main.yaml`

Joins worker nodes to the cluster using the join command from the control plane.

Tasks:
1. Retrieve the join command from the control plane's host facts via `hostvars`
2. Run the join command on the worker node
3. Wait for the node to appear in `kubectl get nodes` from the control plane

### 4c. Role: `workstation-setup`

**Path:** `ansible/roles/workstation-setup/tasks/main.yaml`

Sets up the student workstation with kubeconfigs for all clusters.

Tasks:
1. Create `/root/.kube` directory
2. For each cluster:
   a. Fetch `/etc/kubernetes/admin.conf` from the control plane
   b. Replace the server address (typically `https://<control-plane-ip>:6443`)
   c. Set the context name to match the cluster's `kube_context`
3. Merge all cluster kubeconfigs into `/root/.kube/config`
4. Install useful student tools (bash-completion, tmux, jq)
5. Set up shell prompt and kubectl completion

---

## 5. Scenario Playbooks

### 5a. CKA Simulator 001

**Path:** `testdata/scenarios/cka/simulator-001/provision/playbook.yaml`

Main playbook that orchestrates the full environment setup:

```yaml
- name: Initialize control planes
  hosts: "*_control_plane"
  become: yes
  tasks:
    - name: Compute per-cluster pod CIDR
      set_fact:
        pod_cidr: "10.{{ 244 + cluster_idx }}.0.0/16"
      vars:
        cluster_idx: "{{ groups.keys() | select('match', '.*_control_plane') | list | index(group_names[0]) }}"
    - include_role:
        name: kubeadm-init

- name: Join workers
  hosts: "*_workers"
  become: yes
  tasks:
    - include_role:
        name: kubeadm-join

- name: Setup workstation
  hosts: workstation
  become: yes
  tasks:
    - include_role:
        name: workstation-setup
      vars:
        clusters: "{{ clusters }}"

- name: Scenario-specific setup
  hosts: workstation
  become: yes
  tasks:
    - name: Create production namespace on cluster-b
      command: kubectl --context=cluster-b-admin create namespace production

    - name: Create web-app deployment with 1 replica
      command: >
        kubectl --context=cluster-b-admin -n production create deployment web-app
        --image=nginx --replicas=1
```

This sets up both clusters, configures the workstation, and creates the intentional starting state (web-app with 1 replica that the student must scale to 3).

---

## 6. Dockerfile Updates

**File:** `Dockerfile`

The runtime stage needs Ansible and SSH client to run playbooks against VMs.

Changes:
1. Add `python3`, `py3-pip`, `openssh-client` via apk
2. Install Ansible via pip (`pip3 install ansible`)
3. Copy shared Ansible roles: `COPY ./ansible /app/ansible`
4. The existing `COPY ./testdata/scenarios /app/scenarios` already copies scenario playbooks

```dockerfile
FROM alpine:3.22

RUN apk --no-cache add ca-certificates python3 py3-pip openssh-client \
    && pip3 install --break-system-packages ansible

WORKDIR /app

COPY --from=builder /out/server ./server
COPY --from=builder /out/migrate ./migrate
COPY --from=builder /app/migrations ./migrations
COPY ./ansible /app/ansible
COPY ./testdata/scenarios /app/scenarios

EXPOSE 8080

CMD ["./server"]
```

---

## 7. Proxmox VM Templates (Infrastructure / Manual)

This is not code — it's infrastructure that must be set up in Proxmox before provisioning works.

### Requirements for all templates:

| Component | Purpose |
|-----------|---------|
| Ubuntu 22.04/24.04 | Base OS |
| kubeadm, kubelet, kubectl | Kubernetes components |
| containerd | Container runtime |
| cloud-init | First-boot customization |
| Cloud-init drive (`ide0: cloudinit`) | Required for Proxmox built-in cloud-init params |
| SSH server | Remote management |
| qemu-guest-agent | IP address reporting to Proxmox |
| Swap disabled | Kubernetes requirement |

### Template creation steps:

1. Create a VM with 2 vCPUs, 4GB RAM, 20GB disk
2. Install Ubuntu 22.04
3. Run setup script:
   ```bash
   # Disable swap
   swapoff -a && sed -i '/swap/d' /etc/fstab

   # Install container runtime
   apt-get update && apt-get install -y containerd
   mkdir -p /etc/containerd
   containerd config default > /etc/containerd/config.toml
   sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
   systemctl restart containerd

   # Install kubeadm/kubelet/kubectl
   curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.31/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes.gpg
   echo 'deb [signed-by=/etc/apt/keyrings/kubernetes.gpg] https://pkgs.k8s.io/core:/stable:/v1.31/deb/ /' > /etc/apt/sources.list.d/kubernetes.list
   apt-get update && apt-get install -y kubelet kubeadm kubectl
   apt-mark hold kubelet kubeadm kubectl
   systemctl enable kubelet

   # Install base tools
   apt-get install -y curl wget vim jq openssh-server cloud-init qemu-guest-agent
   systemctl enable qemu-guest-agent ssh cloud-init

   # Load required kernel modules
   cat <<EOF > /etc/modules-load.d/k8s.conf
   overlay
   br_netfilter
   EOF
   modprobe overlay br_netfilter

   # Set required sysctl params
   cat <<EOF > /etc/sysctl.d/k8s.conf
   net.bridge.bridge-nf-call-iptables = 1
   net.bridge.bridge-nf-call-ip6tables = 1
   net.ipv4.ip_forward = 1
   EOF
   sysctl --system

   # Clean up
   cloud-init clean --logs
   apt-get clean
   rm -rf /var/lib/apt/lists/*
   ```
4. Shut down the VM
5. Add cloud-init drive: `qm set <vmid> --ide0 local-lvm:cloudinit`
6. Convert to template: `qm template <vmid>`
7. Clone for workstation template (add extra tools): `qm clone <vmid> <workstation-id> --full` then install bash-completion, tmux, etc., then convert back to template
8. Record VMIDs in the file referenced by `PROXMOX_PROFILES_FILE`

### Template VMIDs to configure:

```yaml
profiles:
  workstation:
    template_vmid: <WS_VMID>
  kubeadm-control-plane:
    template_vmid: <CP_VMID>
  kubeadm-worker:
    template_vmid: <WORKER_VMID>
```

---

## 8. End-to-End Flow Validation

After all changes, the full flow should work:

1. Student purchases a test → `PurchasedTest` created
2. Student starts attempt → `POST /api/v1/attempts` → provisioning job enqueued
3. Job runs:
   a. Generates ephemeral ED25519 SSH keypair
   b. Proxmox clones workstation + cluster VMs from templates
   c. Proxmox configures cloud-init (hostname, SSH key, DHCP networking)
   d. Proxmox starts all VMs
   e. Waits for IPs via guest agent
   f. Ansible playbook runs:
      - `kubeadm init` on control planes (with per-cluster pod CIDRs)
      - Calico CNI installed
      - Workers join clusters
      - Workstation gets merged kubeconfig with all contexts
      - Scenario-specific setup (namespaces, deployments)
   g. Session created with terminal token
   h. Attempt transitions to `environment_ready`
4. Student opens terminal → WebSocket → SSH to workstation
5. Student uses `kubectl config use-context cluster-a-admin` etc.
6. Student submits → grading runs checks via SSH → scored
7. Cleanup destroys VMs

---

## Summary of Changes

| # | Type | Path | Description |
|---|------|------|-------------|
| 1 | Bug fix | `internal/orchestrator/provision_job.go` | Fix SSH key encoding with `ssh.MarshalPrivateKey` |
| 2 | Bug fix | `internal/provider/proxmox/provider.go` | Change `citype` from `configdrive2` to `nocloud` |
| 3 | Code | `internal/provisioner/ansible/ansible.go` | Set `ANSIBLE_ROLES_PATH` env var |
| 4a | Ansible | `ansible/roles/kubeadm-init/tasks/main.yaml` | Control plane init role |
| 4b | Ansible | `ansible/roles/kubeadm-join/tasks/main.yaml` | Worker join role |
| 4c | Ansible | `ansible/roles/workstation-setup/tasks/main.yaml` | Workstation kubeconfig role |
| 5 | Ansible | `testdata/scenarios/cka/simulator-001/provision/playbook.yaml` | CKA scenario playbook |
| 6 | Docker | `Dockerfile` | Add Ansible, openssh-client, copy roles |
| 7 | Infra | Proxmox (manual) | Build VM templates with cloud-init drive |
