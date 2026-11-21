# Student workstation image

The workstation is an independent desktop VM, never a Kubernetes node.
Template **9150** is dedicated to this purpose. Creating the image does not
enable a desktop in the app: interface selection, noVNC and attempt lifecycle
integration are separate work. Existing exams continue to use xterm.js.

## Source and image policy

Use the **latest Ubuntu LTS**, currently **26.04 (Resolute Raccoon)**. The default
source is the user-selected
https://cloud-images.ubuntu.com/resolute/current/resolute-server-cloudimg-amd64.img.
This is a moving **daily build of the LTS**, not a pinned point-release ISO.
The builder verifies `SHA256SUMS.gpg` against a trusted Ubuntu cloud-image keyring,
requires an exact checksum entry, and checks the pristine download both on
management and Proxmox before customization. Each build records the source URL,
SHA256, UTC build time, target configuration and exact installed package versions.
For a reproducible input, set `WORKSTATION_IMAGE_BASE_URL` to the dated Resolute
directory corresponding to that digest; packages still come from current repos.
When the next LTS becomes current, deliberately update the recipe's source,
release assertions and validation together. Do not silently follow a new OS.

This direct cloud-image build is an explicit exception to the `9199` hierarchy
in [vm-images-and-templates.md](vm-images-and-templates.md). Never replace or
upgrade `9199`, `9100`, `9198`, or another exam image as part of this workflow.
Published workstation templates are immutable too: refuse occupied IDs and
select another verified-free `9100–9200` ID for replacements (not `9199`).

## Contents

- XFCE/X11, terminal emulator, Mousepad, Onboard virtual keyboard and fonts.
- Firefox from Canonical's stable snap channel; no VSCodium in this first recipe.
- TigerVNC on **loopback only**, port **5901**, display `:1`, initially 1920×1080.
  It supports client-requested resizing and clipboard exchange. The desktop
  persists across VNC disconnects. Access must be SSH-tunnelled; there is no
  externally reachable unauthenticated VNC listener.
- DHCP and clone hostnames/SSH host keys from Proxmox/cloud-init, QEMU guest
  agent, the operator's management SSH public key, and passwordless lab sudo.
  No baked password, Kubernetes installation, kubeconfig, candidate identity,
  task fixtures, exam state, or browser profile.
- Safe DNS registration, enabled for future clones (not run while baking the image),
  matching the base image's API contract: reject ambiguous
  or non-A records; update a single A record only for this guest's hostname.
  DHCP/NIC settings are never replaced by static lab addresses.

The desktop runs as the configured lab user under a systemd-managed PAM session,
not as root. VNC has no separate password because it is bound to guest loopback
and carried over authenticated SSH. This is a trusted-lab setup, not a multi-user
security boundary. Browser/app authentication is not added by this image.

## Build from management

Read [testing.md](testing.md) and [the image policy](vm-images-and-templates.md).
Inspect the actual Proxmox inventory and ownership before changing anything.
The builder takes the common `data/lab-operation.lock`; no other lab operation
should be running. It refuses existing target IDs and leaves failures intact for
ownership-checked diagnosis; there is no automatic delete or resume.

Management requires Bash, SSH/SCP, curl, `gpgv`, a trusted Ubuntu cloud-image
keyring, `ssh-keygen`, jq, flock, sha256sum and Ansible. On Ubuntu the keyring is
`/usr/share/keyrings/ubuntu-cloudimage-keyring.gpg` (from `ubuntu-keyring`).
Do not substitute the unrelated Ubuntu archive signing key or skip signature
verification. See Canonical's [verification instructions](https://ubuntu.com/docs/public-images/public-images-how-to/verify-image-checksum/).

Proxmox requires `virt-customize`, `qemu-img`, Python 3, the selected DHCP-enabled
bridge and linked-clone-capable storage. The management key must be the public
key corresponding to the identity SSH will use to connect to the guests.
The builder needs 2 GiB local artifact space, 4 GiB remote temporary space,
the full configured VM disk plus 4 GiB datastore headroom, and configured guest
RAM plus 4 GiB available host RAM. Guest package and snap downloads require
working DNS, internet access and time synchronization.

```sh
export LAB_PROXMOX_HOST=root@your-proxmox-host
export LAB_SSH_USER=your-lab-user
export LAB_IP_PREFIX=192.0.2.
export WORKSTATION_SSH_PUBLIC_KEY="$HOME/.ssh/your-management-key.pub"
export WORKSTATION_DNS_API=http://your-dns-api:8888
export WORKSTATION_DNS_ZONE=lab.example.net
export WORKSTATION_DNS_ROUTE=192.0.2.4
bash scripts/build-workstation-template.sh
```

| Setting | Default |
| --- | --- |
| `LAB_WORKSTATION_TEMPLATE_ID` | `9150` |
| `WORKSTATION_STORAGE` / `WORKSTATION_BRIDGE` | `local-lvm` / `vmbr0` |
| `WORKSTATION_MEMORY_MB` / `WORKSTATION_DISK_GB` | `4096` / `32`; 2 vCPUs |
| `WORKSTATION_SSH_PUBLIC_KEY` | `$HOME/.ssh/id_rsa.pub` |
| `WORKSTATION_IMAGE_BASE_URL` | `https://cloud-images.ubuntu.com/resolute/current` |
| `WORKSTATION_IMAGE_KEYRING` | `/usr/share/keyrings/ubuntu-cloudimage-keyring.gpg` |
| `WORKSTATION_DNS_API` / `WORKSTATION_DNS_ZONE` / `WORKSTATION_DNS_ROUTE` | Personal-lab defaults `http://192.168.254.4:8888` / `nowhereworks.local` / `192.168.254.4`; explicitly adapt on other networks |

The builder imports a pristine cloud image, customizes only its new builder VM,
installs the repository-owned workstation playbook, performs read-only readiness
checks and actual desktop/browser/VNC smoke checks (not live DNS registration),
clears build-only browser/cache/SSH host-key/cloud-init identity state,
shuts down and converts the builder to a template. Clone creation must still do
only a linked clone, optional memory setting and start; do not change clone NICs,
cloud-init, machine-id or host keys.

Ubuntu 26.04 cloud-init reports Proxmox's legacy string-valued `user` metadata
as a deprecation (exit code 2). The image's read-only boot checker accepts
**only that precise deprecation** when cloud-init is done with no errors; other
warnings and errors fail readiness. At image-build time, GRUB uses `net.ifnames=0`
to match Proxmox's `eth0` metadata from first boot rather than trying to rename an
already active `ens18`. This is baked image configuration, never a clone-time
NIC or identity change.

Artifacts remain under `data/workstation-image-<ID>.*`, including signed checksums,
the pristine image, `source.json`, a pinned guest `known_hosts`, Ansible output
and validation output. Treat them as operationally sensitive. Guest provenance
lives under `/opt/painkiller/workstation/`; no private management key is copied.
Builder failures also retain remote scratch under
`/tmp/opencode/painkiller-workstation.*` and any VM created. Confirm exact
ownership and process state before cleanup or retry.

## Validate before assigning consumers

Local checks and clone-safety procedures are in
[testing.md](testing.md#workstation-image-validation).
Use `WORKSTATION_TEST_START=<verified-free-first-ID> bash scripts/test-workstation-image.sh`
to run the dedicated two-clone smoke test under the shared operation lock.
Require two fresh disposable linked clones to prove distinct DHCP addresses,
hostnames and SSH keys, cloud-init completion, desktop/Firefox operation and
cleanup. Do not use `create-lab.sh provision` for this: it would install/join
Kubernetes. Check no network-accessible VNC listener exists. Test desktop resize,
clipboard and actual browser rendering separately; a service readiness check
does not prove browser UI coverage.

No exam currently consumes `9150`. Record actual build and validation evidence
here before enabling consumers.

The clone wrapper checks live DNS registration by default. For a diagnosed
external DNS API outage only, `WORKSTATION_TEST_SKIP_DNS=1` runs the remaining
image checks, prints a separate **SKIP**, and still cleans up its owned clones
on success. This does not establish DNS readiness; rerun without the override
once that external service is repaired.
The DNS service is a oneshot: successful execution leaves it inactive. Readiness
checks its last invocation timestamp/result/exit status, not `is-active`.
Run `bash scripts/validate-workstation-image.sh --dns-only` on a clone for that
read-only diagnostic alone.

## Published image evidence

Built on **2026-11-21**, on `prox-k12`, as stopped template `9150`:

- Source SHA256:
  `7b90593f5532b49b7e3fe3d2743f0d1f0d822e5b07cebcd8e00c0df8069ecab9`.
  Canonical's UEC signature and the pristine image checksum both verified.
- Ubuntu 26.04 LTS, XFCE, Firefox `160.0.1-1` snap revision `9229`, 2 vCPUs,
  4096 MiB RAM, 32 GiB disk on `local-lvm`.
- Clean builder run passed cloud-init, package/service/image-state checks,
  actual Firefox and terminal rendering, VNC resize/reconnect, and clipboard
  transfer in both directions before conversion to a template.
- Build provenance and logs:
  `data/workstation-image-9150.WDOS6oJD/`; guest package/Firefox manifests and
  recipe checksums are under `/opt/painkiller/workstation/`.
- Two clean linked clones (`3100–3101`) passed distinct DHCP addresses, hostnames
  and SSH host keys, cloud-init, loopback-only VNC, actual Firefox/terminal
  rendering, resize/reconnect and bidirectional clipboard checks. The wrapper
  destroyed both clones on success; final-run artifacts: `data/workstation-test.IVP3js6v/`.
  Live DNS registration was explicitly **skipped**, not passed.
- Local checks passed: 16 Python safety regressions, ShellCheck, workstation asset
  checksums, Ansible syntax, `go test -race ./...` and `go vet ./...`.
- Live DNS registration is not a build check. During validation the separate
  lab DNS API returned HTTP 502, `technitium auth error`. That service was not
  changed. Require successful registration after it is repaired before claiming
  full lab-network readiness.
