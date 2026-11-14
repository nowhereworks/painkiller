# Painkiller

<p align="center">
  <img src="docs/painkiller.png" alt="Painkiller — Preparing for certifications shouldn’t be a pain" width="600">
</p>

Painkiller is a self-hosted practice environment inspired by killer.sh. It
creates disposable Kubernetes labs so you can practice certification-style
tasks on real clusters, using a browser-based terminal and automatically graded
answers.

Choose an exam, prepare its lab, work through the questions, and submit your
answers for a score and breakdown. When you are finished, reset the lab to
remove its VMs. The interface shows provisioning progress and supports retrying
failed operations.

## Current state

Painkiller is an **early, single-user MVP**, not an official certification exam
simulator. It currently requires a Proxmox host and includes:

- **CKA-001:** 29 tasks across cluster architecture, workloads, networking,
  storage, troubleshooting, and RBAC; one controller and two workers.
- **CKA-002:** 20 scenario-based tasks on a high-availability cluster; three
  controllers and two workers, with two controllers initially joined.
- **CKA-003:** 16 mock-exam tasks covering autoscaling, RBAC, Helm, routing,
  DNS, CSI expansion, rollouts, and troubleshooting; one controller and two workers.
- **CKAD-001:** 21 application-development tasks covering Pods, CronJobs, Helm,
  Services, NetworkPolicies, configuration, Secrets, and probes; one controller
  and two workers with Calico.

All exams default to 120 minutes. Exam-specific requirements and lab settings
live in `exam/<ID>/`. Only one attempt and lab can be active at a time.

**Security:** Run only on a trusted management host and network. There is no
authentication or multi-user isolation, and the environment is not tamper-proof.
The server listens on `127.0.0.1:8080` by default; do not expose it to the
internet or an untrusted network. Reset destroys the marked lab VMs. Review
the [operational safety notes](docs/operations.md#safety-and-prerequisites)
before provisioning.

## Quick start

### 1. Prepare the management host and Proxmox

You need:

- Go **1.26 or later**, CGO and a C compiler, Ansible, SSH, `sha256sum`, `jq`,
  `flock`, and GNU `timeout` on the management host.
- A Proxmox host reachable by passwordless SSH, with its host key verified on
  the management host. The SSH account must be able to manage VMs and use the
  guest agent.
- A compatible template: prebaked `9100` for CKA-001/CKA-003, prebaked `9198`
  for CKA-002, or generic base `9199` for CKAD-001's full-install provisioning.
  The template must provide DHCP, DNS, a running QEMU guest agent, and
  passwordless SSH access for the configured lab user. Clones must be reachable
  from the management host and have network access for provisioning.
- A free contiguous range of VM IDs in `3000–3999`, sufficient Proxmox memory
  and storage, and DHCP capacity. Preflight requires the VMs' configured memory
  plus 4 GiB of available host memory.

CKA-002 is currently tailored to the project's lab network: it uses
`192.168.254.81` for a LoadBalancer service and `cka002-api:8443` for its API.
Check active address allocations before use; a failed ping alone does not prove
an address is free. See its [exam assets](exam/CKA-002/) before using it on
another network.

If you do not already have compatible images, review the
[image and template policy](docs/vm-images-and-templates.md) and the
[builder instructions](docs/operations.md#prebaked-image-default). Building
requires a suitable base template `9199` and network access; do not overwrite
an existing template. These templates and their guest access configuration are
not supplied by cloning this repository.

### 2. Clone and install dependencies

```sh
git clone https://github.com/nowhereworks/painkiller.git
cd painkiller
ansible-galaxy collection install -r playbooks/requirements.yml
```

### 3. Configure your lab

Set these values for your environment in the shell that starts Painkiller.
The examples are placeholders, **not** the project's personal-lab defaults:

```sh
export LAB_PROXMOX_HOST='root@your-proxmox-host'
export LAB_SSH_USER='your-lab-user'
export LAB_IP_PREFIX='192.0.2.'
export LAB_VM_START=3000
```

Use the IPv4 prefix of the guests' DHCP network, and verify the selected range
is free on the actual Proxmox host. The selected exam determines how many
consecutive IDs it needs. For CKA-001, the default prebaked template is `9100`;
`LAB_TEMPLATE_ID` can select another compatible image. CKA-002 selects its own
template and topology. Other settings and defaults are listed in the
[configuration reference](docs/operations.md#prebaked-image-default).

If you need to build an image, configure the same host, user, and IP prefix
first; inspect the builder instructions and check that the destination ID is
free before running either builder.

### 4. Run and practice

From the repository root:

```sh
go run ./cmd/painkiller
```

Open <http://127.0.0.1:8080>, select a test and mode, and choose **Prepare lab**.
Once the lab is ready, choose **Start exam**, complete tasks in the terminal,
and submit for grading. Use **Destroy lab** when finished or before choosing
another test. If preparation fails, inspect the error before retrying or using
**Reset failed attempt**; interrupted cloning may require manual investigation.
Resetting a failed attempt can destroy any VMs already created for it. Lab state
and answer snapshots are stored under `data/` and should be treated as sensitive.

## Documentation

- [Operations, configuration, image builds, and limitations](docs/operations.md)
- [VM images and templates](docs/vm-images-and-templates.md)
- [Exam structure and authoring](exam/README.md)
- [Testing and lab safety](docs/testing.md)
- [Project-owned playbooks](playbooks/README.md)
