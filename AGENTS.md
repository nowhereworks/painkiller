# About Project

This is the repository for Painkiller Shell: A Killer.sh clone that aims to mimic killer.sh functionality *as simple as possible*

# Directives

- Always commit and push after work
- Keep things as simple as possible
- When making modifications, change as little as possible in order to achieve goals
- Always persist any useful data for future sessions 
- On Proxmox, never alter or delete VMs outside IDs 3000–3004. Use only as many lab VMs as the CKA scenario requires; the default is one controller and two workers.
- Proxmox template `9010` is already pre-configured (SSH access for `milkman`, DNS, DHCP). `scripts/create-lab.sh create` must only do linked clones (`qm clone --full 0`) and start them — do NOT manipulate `net0`, `cloud-init`, machine-id, or SSH host keys. Linked clones get unique MACs, hostnames, and SSH host keys, and unique DHCP IPs (machine-id is shared but benign). Boot takes ~30–60s.
