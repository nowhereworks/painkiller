# VM images and templates

Follow these rules when creating or changing lab images, templates, or
provisioning. Keep this document and the registry below current.

## Template hierarchy

`9199` is the sole base template for all lab images. Its purpose is to provide
the minimal common VM foundation: automatic DNS registration, baseline VM
shape, SSH access and keys, DHCP, the guest agent, and other shared boot and
access configuration. Keep exam-specific dependencies, fixtures, faults, and
cluster state out of it.

There are at most two template levels:

```text
9199 base template
├── disposable lab VMs
└── optional specialized template
    └── disposable lab VMs
```

Disposable lab VMs are not template levels. There may be multiple specialized
templates, but each must be built directly from `9199`, never from another
specialized template, even via a full clone. Do not convert a lab VM into a
template. Record the source and build procedure so ancestry is verifiable.

## Creating and using images

- Prefer the base and provisioning-time setup where practical. Create a
  specialized image only when needed for dependencies, compatibility, or
  meaningful provisioning-time savings. `9199` itself is not a drop-in
  prebaked Kubernetes image: its consumers must install their requirements.
- Full-clone `9199` to build a specialized image; use a free ID in
  `9100–9200`, excluding the reserved base ID `9199`. Check the actual host
  and refuse occupied IDs. The existing builders are
  `scripts/build-template.sh` and `scripts/build-template-ha.sh`.
- Keep specialized images uninitialized and reusable. Do not publish joined
  clusters, Kubernetes credentials or PKI, etcd data, lab attempt state, or
  injected faults. Apply node-specific and exam-specific configuration during
  provisioning. Each exam owns its unique assets and requirements.
- Document the image's purpose, source template, build command, relevant pinned
  versions, consumers, and validation results. Record its ID and usage in the
  registry. Do not infer ancestry from an image's ID or name.
- Treat published templates as immutable. When contents need to change, build
  a replacement at a free ID, validate disposable clones, and explicitly update
  the intended consumers. Never silently change another exam's requirements.
- Create lab VMs as linked clones (`qm clone --full 0`); creation only sets
  clone memory and starts them. Do not manipulate clone NICs, cloud-init,
  machine-id, or SSH host keys. Preserve the existing DHCP, DNS, and guest
  identity contract.
- Before retirement, verify ownership, consumers, active attempts, and
  linked-clone dependencies. Never delete or replace a template just because
  it appears unused. Respect Proxmox's permitted VM and template ID ranges in
  `AGENTS.md`.
- Before planning or running tests, read `docs/testing.md` and follow its
  safety procedures. Validate changes using the smallest relevant checks and,
  when appropriate, disposable clone and exam validation. Documentation changes
  alone do not authorize image builds or live lab creation.

## Existing images and transition

The current prebaked default remains `9100`, and CKA-002 selects `9198`.
Legacy mode currently defaults to `9010`, outside the permitted template
management range and outside this policy's sole-base hierarchy. Migrate that
consumer separately to a verified compatible image derived from `9199`; do not
modify or delete `9010`. Existing templates are not automatically compliant:
verify their build history and compatibility before claiming ancestry or
migrating consumers. Do not disrupt active attempts.

## Template registry

Update this table when a template is added, replaced, retired, or reassigned.
The entries describe observed usage, not proof of ancestry or permission to
alter an image. Verify current allocations and consumers before acting.

| ID | Usage | Owning or consuming exam(s) |
| --- | --- | --- |
| `9010` | Current legacy-mode default; transitional, ancestry unverified | Legacy-mode consumers; specific exams not established |
| `9100` | Current default prebaked Kubernetes image; ancestry unverified | Default for exams without an explicit lab template, including CKA-001 |
| `9198` | Prebaked HA Kubernetes/Calico image; ancestry unverified | CKA-002 |
| `9199` | Generic VM foundation and sole approved base | Shared—all exams |
| `9200` | Existing HA image; current consumer and ancestry unverified | Not established |
