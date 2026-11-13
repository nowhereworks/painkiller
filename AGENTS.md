# About Project

This is the repository for Painkiller Shell: A Killer.sh clone that aims to mimic killer.sh functionality *as simple as possible*

# Directives

- Always commit and push after work
- Keep things as simple as possible
- When making modifications, change as little as possible in order to achieve goals
- Always persist any useful data for future sessions 
- Display lab operation output in a read-only, auto-scrolling xterm.js box with ANSI colors that fills the remaining viewport height dynamically. The log endpoint returns a rolling 32 KB snapshot; append unchanged-prefix updates and reset on replacement.
- On Proxmox, never alter or delete VMs outside IDs 3000–3999 and 9100–9200. 
- On Proxmox, never alter or delete VMs templates outside IDs 9100–9200. 
- Use only as many lab VMs as the CKA scenario requires; the default is one controller and two workers.
- Provisioning defaults to prebaked template `9100`; explicit `LAB_PROVISION_MODE=legacy` uses `9010`. Both are pre-configured (SSH access for `milkman`, DNS, DHCP). `scripts/create-lab.sh create` must only do linked clones (`qm clone --full 0`), set clone memory, and start them — do NOT manipulate `net0`, `cloud-init`, machine-id, or SSH host keys. Linked clones get unique MACs, hostnames, and SSH host keys, and unique DHCP IPs (machine-id is shared but benign).
- All required playbooks belong in `./playbooks`; never depend on an external playbook checkout. When changing the cluster playbook, update `playbooks/SHA256SUMS` and `playbooks/image-revision` together and build a new compatible template at a free ID. Preserve existing template compatibility when copying playbooks unchanged.
