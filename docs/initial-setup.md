# Initial setup: empty Proxmox host to working Painkiller

This runbook assumes a fresh repository clone and a Proxmox host with **no
Painkiller templates**. It covers the infrastructure that cloning the repository
does not provide. Start with CKA-001; enable the other exams afterward.

Commands are examples, not authorization to change an existing environment.
Replace placeholders before running them. **Agents must stop when an input,
ownership, address allocation, or expected result is uncertain.** Do not silently
substitute the project's personal-lab defaults.

## 1. Understand the machines and safety boundaries

| Machine | Responsibilities | Command context in this guide |
| --- | --- | --- |
| Management host | Repository, Go server, Ansible, SSH keys, persistent application data | **Management**: ordinary user, repository root unless stated otherwise |
| Proxmox host | VM storage, bridge, QEMU, base-template creation | **Proxmox**: root shell on the selected host |
| DNS/API host | DNS service and guest DNS-registration API | **DNS/API**: a trusted service host, possibly the management host |
| Disposable guests | Kubernetes and exam workloads | **Guest**: console or explicitly verified SSH connection |
| Browser workstation | Web UI; optional SSH tunnel to the management host | **Workstation** |

- Painkiller is single-user and has **no authentication or multi-user isolation**.
  Keep the server on `127.0.0.1:8080`; never publish it to an untrusted network.
- Use VM IDs **3000–3999** for disposable labs and **9100–9200** for templates
  and image builders. Base template **9199** is reserved. Do not alter or delete
  VMs outside those ranges, including legacy template `9010`.
- Inspect the actual host before selecting IDs. An SSH failure does not mean an
  ID is free. Never overwrite an existing template.
- Templates are immutable once published. Linked clones depend on their disks;
  never remove or replace a template while dependent clones exist.
- Read [the image policy](vm-images-and-templates.md) before image operations
  and [the testing runbook](testing.md) before tests. This guide does not
  authorize builds, live tests, or cleanup on someone else's infrastructure.

## 2. Choose infrastructure and record the inputs

Use a supported Proxmox VE installation with working hardware virtualization.
The repository's base image and several exam downloads are **amd64/x86-64**;
this is not an ARM setup recipe. Provision a separate Debian/Ubuntu management
host or VM; do not install the application stack on the hypervisor by default.

Record these inputs before continuing:

| Input | Requirement |
| --- | --- |
| Proxmox SSH destination | For example `root@pve.example.net`; root is the simplest supported setup because scripts use `qm`, `pvesm`, and guest execution directly, without remote `sudo` |
| Proxmox storage | Active VM-image storage supporting full and linked clones; the base builder defaults to `local-lvm` |
| Guest bridge | A bridge connected to the intended DHCP network; base builder hardcodes `vmbr0` |
| Guest IPv4 network | Routed from management, with enough free DHCP leases; set `LAB_IP_PREFIX` to the textual IPv4 prefix used to select guest addresses |
| Guest user | A non-root Linux account, matching the template and `LAB_SSH_USER` |
| Management SSH public key | Must be authorized in every guest; possession of a Proxmox login key alone is insufficient |
| DNS zone and resolver | A writable lab zone and a resolver guests can reach |
| Registration API | Guest-reachable API URL and IPv4 route target; see section 4 |
| VM allocations | Free `9199`, `9100`, optionally `9198`, and a contiguous lab range beginning at `LAB_VM_START` |

### Capacity

Only one attempt is active at a time. Capacity below is **available memory at
preflight**, not total installed RAM; allow additional space for Proxmox and
other workloads.

| Exam | Lab VMs | Memory per VM | Required available host memory | Template |
| --- | --- | --- | --- | --- |
| CKA-001 | 1 controller + 2 workers | 3072 MiB default | 13 GiB | `9100`, prebaked Flannel |
| CKA-002 | 3 controllers + 2 workers | at least 4096 MiB | 24 GiB | `9198`, prebaked Calico/HA |
| CKA-003 | 1 controller + 2 workers | at least 4096 MiB | 16 GiB | `9100`, prebaked Flannel |
| CKAD-001 | 1 controller + 2 workers | at least 4096 MiB | 16 GiB | `9199`, full install with Calico |

The base builder sets **2 vCPUs, 8192 MiB RAM, and adds 30 GiB to the
downloaded disk**. Specialized-image builders inherit that memory allocation,
so require at least 12 GiB available with the default base configuration.
They reserve the full base virtual disk size plus 4 GiB free storage.
Lab preflight requires at least 10 GiB free on the template datastore, but
that is only a guard: allocate substantially more for templates, full image
builds, container images, and growing clone deltas.

### Networking

- Provide DHCP, gateway, DNS, and correct time. Ensure clones receive distinct
  IPv4 addresses, hostnames, and SSH host keys without per-clone NIC changes.
  The current identity contract permits a shared machine-id; verify DHCP
  uniqueness rather than changing it in `create-lab.sh`.
- Keep guest LAN/VPN routes separate from the default Pod CIDR
  `10.244.0.0/16` and Service CIDR `10.96.0.0/12`.
- Management needs SSH TCP 22 to Proxmox and all guests. Guests need DNS
  UDP/TCP 53, time synchronization, and package/image download access.
  The management host also needs Go module and Ansible collection downloads.
- Permit Kubernetes node-to-node/API/kubelet and CNI traffic on the isolated
  lab network. Examples include API TCP 6443, etcd TCP 2379–2380,
  kubelet TCP 10250, Flannel VXLAN UDP 8472, and Calico's configured
  BGP/encapsulation traffic. Do not blindly disable the hypervisor firewall
  or expose these ports publicly.
- CKA-003 creates its own NFS service on a worker; permit guest NFS traffic.
  CKAD-001 creates a controller-host HTTP fixture on TCP 9998.
- Prebaked does **not** mean offline. Allow required Ubuntu/Kubernetes package
  repositories, GitHub/raw GitHub, Helm downloads, and image registries such as
  `registry.k8s.io`, Docker Hub, and `quay.io`. Exam dependencies can require
  additional image sources; inspect failures rather than bypassing TLS checks.

## 3. Prepare the management host and SSH

**Management**, Debian/Ubuntu example:

```sh
sudo apt-get update
sudo apt-get install -y git build-essential python3 python3-venv \
  openssh-client jq util-linux coreutils ca-certificates curl
python3 -m venv "$HOME/.venvs/painkiller"
. "$HOME/.venvs/painkiller/bin/activate"
python -m pip install --upgrade pip ansible
```

Install **Go 1.26 or later** using the
[official installation instructions](https://go.dev/doc/install), selecting
the management host's architecture and verifying the published archive checksum.
Do not assume the distribution's `golang` package is new enough.
CGO must be enabled and a C compiler available for SQLite.

```sh
go version
go env CGO_ENABLED
cc --version
ansible-playbook --version
ssh -V
jq --version
flock --version
timeout --version
sha256sum --version
```

Expected: Go meets the minimum, `CGO_ENABLED` is `1`, and every command exists.
Node.js, npm, Playwright, Helm, and local kubectl are not required to run the
application; browser assets are embedded and guest tooling is provisioned.

```sh
git clone https://github.com/nowhereworks/painkiller.git
cd painkiller
ansible-galaxy collection install -r playbooks/requirements.yml
```

Choose or generate a dedicated SSH key as the same user who will start the
server. Do not overwrite an existing key. For example:

```sh
ssh-keygen -t ed25519 -f "$HOME/.ssh/painkiller_ed25519"
```

Use an unlocked SSH agent for a passphrase-protected key. If your session does
not already have an agent, start one, then load the key:

```sh
eval "$(ssh-agent -s)"
ssh-add "$HOME/.ssh/painkiller_ed25519"
```

Configure the chosen
identity in `~/.ssh/config` for both the Proxmox hostname and guest IP network;
scripts do not take a general SSH private-key environment variable. An example
to adapt and merge into your existing configuration:

```sshconfig
Host pve.example.net
    IdentityFile ~/.ssh/painkiller_ed25519
    IdentitiesOnly yes

# Replace with the actual guest address pattern.
Host 192.168.50.*
    IdentityFile ~/.ssh/painkiller_ed25519
    IdentitiesOnly yes
```

Through the Proxmox console or another trusted provisioning channel, authorize
this public key for root on Proxmox. From that console, obtain the SSH server
fingerprint, for example:

```sh
# Proxmox
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

**Management**: connect once, compare the presented fingerprint out of band,
and only then accept it. Do not use `StrictHostKeyChecking=no` or trust an
unverified `ssh-keyscan`.

```sh
ssh root@pve.example.net
# After verifying the fingerprint and exiting the interactive session:
ssh -o BatchMode=yes root@pve.example.net 'qm list'
ssh -o BatchMode=yes root@pve.example.net 'pvesm status'
```

Expected: no password prompts, the correct host's VM inventory, and active
storage. Guest public-key authorization is configured in section 5.

## 4. Supply DNS and the registration API

Ordinary DNS resolution and automatic guest DNS registration are separate
requirements. DHCP should supply a working resolver. The current base builder
also installs `/usr/local/sbin/register-dns` and its systemd service, which
register each clone's hostname at boot.

**Do not run the base builder unchanged on a new network.** It embeds:

- Registration API `http://192.168.254.4:8888`.
- DNS zone `nowhereworks.local`.
- Route target `192.168.254.4`, used to choose the guest's source IP.

The compatible service is
[`api-technitium-auto`](https://github.com/nowhereworks/api-technitium-auto).
It is external infrastructure, not bundled or started by Painkiller.

For a new installation:

1. Install [Technitium DNS Server](https://technitium.com/dns/) on a trusted
   service host using its installation documentation. Create a writable primary
   lab zone, configure recursive/forwarded resolution as appropriate, and allow
   the guest network to query it. DHCP can be supplied by Technitium or an
   existing router; do not enable a second competing DHCP server.
2. Create a Technitium API token authorized to read and update that zone.
   Keep it on the DNS/API host, **not in the VM image or this repository**.
3. Deploy the registration API using that project's README. For example, on a
   **DNS/API host** with Go 1.26+:

   ```sh
   git clone https://github.com/nowhereworks/api-technitium-auto.git
   cd api-technitium-auto
   ```

   Create a mode-0600 `.env` containing the real values:

   ```dotenv
   TECHNITIUM_BASE_URL=http://your-dns-host:5380
   TECHNITIUM_API_TOKEN=replace-with-your-zone-token
   HTTP_ADDR=your-trusted-api-interface-ip:8888
   ```

   ```sh
   chmod 600 .env
   go run ./cmd/server
   ```

   Keep it running under a service supervisor for ongoing use. The API has no
   client authentication or TLS: restrict its listener/firewall to trusted lab
   clients. Its default localhost binding example is not guest-reachable.
4. From the guest network, check `GET /healthz` and
   `GET /hosts/<zone>/<unused-probe-hostname>`. The latter must return a JSON
   array (normally `[]`); health alone does not prove Technitium connectivity.
   Verify that a controlled test A record can be created and resolved, then
   remove only that test record.

The base registration script expects:

```text
GET  /hosts/<zone>/<hostname>  -> JSON array of records
POST /hosts                  -> create/update A record
```

POST body:

```json
{"hostname":"k8s-3000","zone":"lab.example.net","record_type":["a"],"value":"192.168.50.100"}
```

Names and addresses above are examples, not records to create. The script
refuses ambiguous/conflicting records, but can update a single A record for the
same name. Reserve the `k8s-<ID>` and `painkiller-image-<ID>` hostname space for
this installation. Existing DNS/DHCP services are acceptable only if they
preserve the same required guest identity/registration behavior; a different
registration mechanism needs an explicitly adapted base image, not an assumed
environment variable.

## 5. Create base template 9199

There is a repository-owned starting recipe:
[`scripts/build-base-template.sh`](../scripts/build-base-template.sh).
It runs **on Proxmox as root**, not on management. Unlike the specialized image
builders, it is **not environment-neutral**. Review it in full before executing.

**Proxmox**:

```sh
qm list
pvesm status
ip link show vmbr0
apt-get update
apt-get install -y wget libguestfs-tools
```

Verify `9199` is unused, the chosen bridge is correct, and storage can hold VM
images, cloud-init disks, and linked clones. Package installation and image
customization need working repository/network access.

**Management**: copy the recipe and your public key to new, non-conflicting
paths on Proxmox (check those paths first):

```sh
scp scripts/build-base-template.sh root@pve.example.net:/root/painkiller-build-base.sh
scp "$HOME/.ssh/painkiller_ed25519.pub" root@pve.example.net:/root/painkiller-management.pub
```

**Proxmox**: edit the private working copy, leaving the upstream recipe intact.
Apply **all** of these adaptations:

| Recipe location | Required action |
| --- | --- |
| `URL` / `IMG` | Ubuntu 24.04 Noble amd64 cloud image; download and verify against Ubuntu's [published checksums/signatures](https://cloud-images.ubuntu.com/noble/current/) before customization. The `current` URL changes over time; record the exact image digest |
| `VM_ID` | Keep `9199`; refuse occupied IDs |
| `LOCAL_DISK` | Select your active linked-clone-capable storage; export it before running if not `local-lvm` |
| `--net0 ... bridge=vmbr0` | Select the actual guest bridge; retain DHCP |
| `api=`, `zone=`, `ip -4 route get ...` | Replace all three with the API URL, lab zone, and reachable IPv4 route target selected in section 4 |
| `Description=` in the DNS unit | Update its zone description to avoid misleading operators |
| `--ciuser milkman --cipassword 'password'` | Use your selected lab account and **remove the `--cipassword` option**; do not bake the example password into a new image |
| `/root/id_rsa.pub` read | Replace with `/root/painkiller-management.pub`, containing the management user's authorized public key |
| Appended `semaphore-lab-ansible` public key | Remove this append; do not authorize a third party's hardcoded key |

Keep the guest agent enabled, cloud-init DHCP, serial console, `scsi0` disk,
and guest SSH capability. The specialized builders parse `scsi0`, its storage,
and its integer `M`/`G` disk size. Do not preinstall or initialize an exam cluster.
The guest needs Python 3 and sudo for Ansible/provisioning; Noble cloud images
normally supply them. Ensure the guest agent is enabled/running at boot;
package installation without a running agent is insufficient.

The recipe downloads into `/tmp` and customizes that image **in place**.
For a checksum-verified build, predownload/verify the pristine image, remove the
recipe's subsequent `wget -c` step in your working copy, and use that verified
file. On failure, inspect both VM/storage artifacts and the customized download
before retrying; do not treat an already modified file as a pristine image.

After reviewing the edited script and confirming prerequisites:

```sh
# Proxmox; substitute your storage if different.
bash -n /root/painkiller-build-base.sh
LOCAL_DISK=local-lvm bash /root/painkiller-build-base.sh
qm config 9199
```

Expected: stopped template `9199` with `template: 1`, the intended bridge,
DHCP cloud-init, chosen user/public key, `agent: enabled=1` (or equivalent),
and a `scsi0` disk on the intended storage. Record the image checksum, adapted
recipe, creation date, and source in your infrastructure records.

**A template config alone does not prove clone readiness.** During the first
specialized build/first lab below, verify unique hostname/IP/SSH keys, guest
agent, SSH, Python/sudo, DNS resolution, and successful registration. If they
fail, inspect cloud-init and boot services before publishing more images.
Do not fix clone identity by modifying `create-lab.sh` or rewriting clone NICs,
machine-ids, cloud-init, or host keys. Failed builders are retained for inspection.

## 6. Save application configuration and build the default image

**Management**: create a private shell configuration outside the checkout,
for example `~/.config/painkiller/environment.sh`:

```sh
mkdir -p "$HOME/.config/painkiller"
```

Create that file with these values adapted:

```sh
export LAB_PROXMOX_HOST='root@pve.example.net'
export LAB_SSH_USER='your-lab-user'
export LAB_IP_PREFIX='192.168.50.'
export LAB_VM_START=3000
export PAINKILLER_LISTEN='127.0.0.1:8080'
```

`LAB_IP_PREFIX` is a textual address selector, **not a CIDR or IP allocator**.
Each guest must expose exactly one matching primary IPv4 address. Do not use
the documentation-only `192.0.2.` prefix on a real LAN.

```sh
chmod 600 "$HOME/.config/painkiller/environment.sh"
. "$HOME/.config/painkiller/environment.sh"
. "$HOME/.venvs/painkiller/bin/activate"
```

Painkiller does **not** automatically load `.env` or this shell file. Source it
in every shell that builds images or starts the application; configure the same
environment, working directory, SSH agent, and PATH if using a service manager.

Leave `LAB_TEMPLATE_ID`, `LAB_PROVISION_MODE`, `LAB_CNI_PLUGIN`,
`LAB_CONTROLLERS`, `LAB_INITIAL_CONTROLLERS`, `LAB_CONTROL_PLANE_ENDPOINT`,
`LAB_VM_END`, and `LAB_VM_MEMORY` **unset** for the normal multi-exam setup.
Exam definitions select their topology and requirements. Explicit incompatible
settings can reject preparation; a globally pinned template can select the
wrong image for another exam. Do not set global legacy mode to bootstrap a new
installation: its historical default is `9010`, which is not supplied here.

Confirm destination `9100` is free on the actual host:

```sh
ssh -o BatchMode=yes "$LAB_PROXMOX_HOST" 'qm list'
ssh -o BatchMode=yes "$LAB_PROXMOX_HOST" 'qm config 9199'
scripts/build-template.sh
ssh -o BatchMode=yes "$LAB_PROXMOX_HOST" 'qm config 9100'
```

The builder full-clones `9199`, installs pinned Kubernetes packages and cached
images, then shuts down and converts the result to template `9100`. It does
not initialize Kubernetes. Build dependencies include the required Ansible
collection, verified Proxmox SSH trust, guest key access, working DNS/API,
package repositories, and image registries.

Expected: `Built template 9100; validate clones before use.` and a stopped
template. The default Kubernetes version is `1.36.1`; do not substitute versions
without reviewing exam requirements and image compatibility.

On failure, inspect the marked builder VM and guest boot/cloud-init logs.
After fixing the cause, `scripts/build-template.sh --resume` resumes only its
unfinished owned VM; it refuses existing templates and unmarked VMs.
Do not delete/recreate an uncertain VM or overwrite a published image.
For future image replacements, use a free permitted destination ID and explicitly
update intended consumers; changing a builder destination alone does not change
an exam's selected template.

## 7. Start Painkiller and complete the first lab

**Management**, repository root, with the virtualenv, shell environment, and
SSH identity available:

```sh
# Read-only infrastructure preflight for default CKA-001; creates no VMs.
scripts/create-lab.sh preflight
go run ./cmd/painkiller
```

Preflight checks the default three-ID range, template, playbooks, memory, and
storage. It does not prove all guest networking or exam readiness.
The application must run from the repository root to find scripts and exams.
No external playbook checkout, frontend build, or database service is needed.

**Workstation**: open <http://127.0.0.1:8080> if local. If management is remote,
keep its listener on localhost and tunnel instead of exposing the service:

```sh
ssh -N -L 8080:127.0.0.1:8080 your-user@your-management-host
```

Then open the same local URL. Choose a different local tunnel port if occupied.

1. Select **CKA-001** and a practice mode.
2. Select **Prepare lab**. Watch the operation log. Creation should use only
   three linked clones in the verified range (`3000–3002` by default).
3. Wait for preparation, including the exam-owned readiness check, to pass.
   Do not bypass a failed readiness check or manually enable Start.
4. Select **Start exam**. In the browser terminal:

   ```sh
   whoami
   sudo -n true
   kubectl get nodes -o wide
   ```

   Expected: configured lab user, working passwordless sudo, and three Ready
   nodes with distinct addresses. Check node SSH access using one listed worker
   IP. Guest SSH trust is pinned through the Proxmox guest agent in
   `data/lab_known_hosts`; do not replace this with disabled host-key checking.
5. Confirm guest DNS resolution and registration through the configured resolver.
   Investigate registration service failures using the guest commands below.
6. Attempt a task and submit. The result should distinguish wrong answers from
   infrastructure/capture errors. A new unanswered lab need not score well.
7. Select **Destroy lab** when finished. Verify the three lab VMs were removed
   and templates remain. Stopping the Go server does **not** destroy the lab.

Do not apply reference solutions to a candidate's active attempt. Optional
automated validation is described in [testing.md](testing.md); it creates real
VMs and requires a separately verified free range.

## 8. Enable the remaining exams

Reset/destroy the current lab before switching exams. Inspect
`exam/<ID>/test.json` and the exam-owned assets for current requirements.

### CKA-003

Uses `9100`, three VMs, and at least 4096 MiB per VM, selected by its exam
contract. Preparation installs its NFS CSI, routing, and metrics dependencies.
Allow package/image downloads and NFS between guests. No reserved LAN address
or extra external NFS VM is needed.

### CKAD-001

Uses three linked clones of **base `9199`**, at least 4096 MiB each, with
full-install Kubernetes/Calico provisioning. This exam selects legacy mode
and `9199` itself; do not set global legacy mode or build a Flannel image for it.
Allow package/image downloads, policy-capable Calico traffic, and the
exam-provisioned controller HTTP fixture on TCP 9998. Its readiness check
verifies real NetworkPolicy enforcement.

### CKA-002

This exam is **not network-portable through `LAB_IP_PREFIX` alone**:

- It allocates only **192.168.254.81** as a LoadBalancer address, outside DHCP.
  The project's lab reserves `.80–.99`; leave the other addresses unallocated.
- Its API is `cka002-api:8443`, mapped in each guest's `/etc/hosts` to the west
  worker's DHCP address. It does not require an additional API VIP.
- Before use, check actual DHCP/static allocations and network ownership.
  A failed ping or ARP probe alone does not prove `.81` is free.
- The exam preflight runs **`nmap` on Proxmox**. Install it there if enabling
  this exam: `apt-get install -y nmap`.
- It needs five free consecutive lab IDs and at least 24 GiB available memory.
  Only two controllers initially join; intentional faults are part of its
  starting state, not necessarily a provisioning failure.

On another subnet, stop before selecting this exam. Adapting it requires an
explicit exam-owned configuration/assets change and corresponding readiness,
grading, and reference-answer review; do not assign the reserved address to
clone NICs or assume this guide authorizes a network migration.

If the network prerequisites are satisfied, **Management**:

```sh
ssh -o BatchMode=yes "$LAB_PROXMOX_HOST" 'qm list'
# Confirm 9198 is free before building:
scripts/build-template-ha.sh
ssh -o BatchMode=yes "$LAB_PROXMOX_HOST" 'qm config 9198'
```

This builds directly from `9199`, not `9100`. The exam selects template
`9198`. The HA builder also supports `--resume` for its own unfinished VM.
Do not build or alter an image merely because an exam preparation failed.

## 9. Diagnostics, recovery, and ongoing operation

| Symptom | Check before retrying |
| --- | --- |
| Go build fails on SQLite | Go version, `CGO_ENABLED=1`, C compiler, module-download connectivity |
| Missing `ansible-playbook`/collection | Activate the same virtualenv used during setup; install `playbooks/requirements.yml` as the server user |
| Proxmox SSH prompts or fails | Correct destination/user/key, unlocked agent, verified known-hosts entry; never disable verification |
| Guest-agent/SSH timeout | VM console, bridge/DHCP capacity, unique lease, running guest agent, cloud-init status, authorized management key |
| Multiple or duplicate guest IPs | Actual interfaces and DHCP allocations, textual `LAB_IP_PREFIX`; do not rewrite clone NICs to hide the error |
| DNS registration repeatedly fails | API route/URL, zone/token permissions, unique hostname, record conflicts, service logs |
| Image validation fails | Correct exam template, pinned packages/cache metadata, compatible image build; no automatic legacy fallback |
| Explicit setting is incompatible | Remove stale global topology/mode/template overrides; restart server after intentionally changing configuration |
| Not enough memory/storage | Actual `MemAvailable`, active datastore, full-builder disk requirement, clone deltas and reserve |
| UI works but terminal fails | Guest SSH access as server user, pinned keys, websocket/tunnel path; avoid putting an unconfigured proxy in front |
| Prepare fails after some VMs exist | Inspect `data/operation.log`, actual IDs/descriptions, and current attempt before retry/reset |

Read-only diagnostics:

```sh
# Management, repository root
tail -n 100 data/operation.log
ssh -o BatchMode=yes "$LAB_PROXMOX_HOST" 'qm list'
# Only for an existing default lab with matching environment:
scripts/create-lab.sh status
```

For nondefault exams, direct scripts do not automatically read the application's
saved lab contract. Use the exact resolved range, template, topology, and marker
when diagnosing them; the UI preserves those settings for its active attempt.

```sh
# Proxmox: replace 3000 with the verified owned guest/builder ID
qm config 3000
qm agent 3000 network-get-interfaces
qm guest exec 3000 -- systemctl status qemu-guest-agent

# Guest console or verified SSH
cloud-init status --long
systemctl status qemu-guest-agent
systemctl status register-dns.service
journalctl -u register-dns.service -b --no-pager
ip -4 address
ip -4 route
```

Use **Retry operation** only after inspecting and correcting the failure.
An interrupted clone can exist without its expected ownership marker; manual
ownership investigation is required. UI reset/destroy is destructive and may
remove partially created VMs. Never issue broad `qm destroy` loops or remove
VMs only because their names resemble a lab.

DHCP leases and DNS records may outlive destroyed VMs. Before deleting stale
leases, compare lease MACs against **all existing VM NICs**, preserving leases
for existing VMs. Recognize both `k8s-` and `painkiller-image` hostname prefixes;
the hostname alone is not proof a lease is stale. Apply the same ownership care
to stale DNS records.

Application state and answer snapshots are stored in `data/state.db`;
operation logs, fetched kubeconfigs, and guest trust data also live under
`data/`. Treat this directory as sensitive and preserve it across server
restarts. Do not delete state as a substitute for destroying a lab. Stop the
server before a simple filesystem backup so SQLite files are consistent.
All server instances managing the same lab must share the same operation-lock
directory; the simplest supported arrangement is one server.

### Setup is complete when

- [ ] Proxmox identity, VM ranges, datastore, bridge, DHCP, and address ownership
      are explicitly verified.
- [ ] Management dependencies, Proxmox SSH, and guest SSH work without prompts.
- [ ] Base `9199` has only intended access keys and working boot-time DNS
      registration; image provenance is recorded.
- [ ] Default image `9100` builds without an initialized cluster.
- [ ] CKA-001 passes preparation/readiness and provides a usable terminal.
- [ ] Submission completes without infrastructure/capture errors.
- [ ] Destroy removes only the attempt's VMs and leaves templates intact.
- [ ] Each additional exam's requirements are checked before enabling it.

This document describes the setup procedure; it is not evidence that a fresh-host
installation has been live-tested. See [operations.md](operations.md) for the
full configuration reference and [testing.md](testing.md) for validation.
