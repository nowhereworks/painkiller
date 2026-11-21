#!/usr/bin/env bash
# Runs on Proxmox via the management-host builder; never run on an existing VM.
set -Eeuo pipefail
work=$1 id=$2 storage=$3 bridge=$4 user=$5 memory=$6 disk=$7 marker=$8
[[ $work =~ ^/tmp/opencode/painkiller-workstation\.[a-zA-Z0-9]+$ &&
   $id =~ ^[0-9]+$ && $id -ge 9100 && $id -le 9200 && $id != 9199 &&
   $storage =~ ^[a-zA-Z0-9_-]+$ && $bridge =~ ^[a-zA-Z0-9_-]+$ &&
   $user =~ ^[a-z_][a-z0-9_-]*$ && $memory =~ ^[0-9]+$ && $disk =~ ^[0-9]+$ &&
   $marker == painkiller-workstation-image-v1 ]] || { echo 'Invalid import configuration' >&2; exit 1; }
list=$(qm list)
if awk -v id="$id" '$1 == id {found=1} END {exit !found}' <<<"$list"; then echo "VM $id already exists; refusing import" >&2; exit 1; fi
cd "$work"
sha256sum --check image.sha256
# Customize a fresh download, not a published image or another exam's base.
# qemu-guest-agent is required before IP discovery; cloud-init supplies the user.
export LIBGUESTFS_BACKEND=direct
# shellcheck disable=SC2016 # GRUB expands its variable inside the image.
virt-customize -a resolute-server-cloudimg-amd64.img \
  --install qemu-guest-agent \
  --run-command 'systemctl enable qemu-guest-agent.service' \
  --run-command 'printf '\''GRUB_CMDLINE_LINUX="$GRUB_CMDLINE_LINUX net.ifnames=0"\n'\'' > /etc/default/grub.d/90-painkiller-network.cfg; update-grub'
qm create "$id" --name "painkiller-image-workstation-$id" --description "$marker" \
  --numa 0 --ostype l26 --cpu host --cores 2 --memory "$memory" \
  --net0 "virtio,bridge=$bridge" --scsihw virtio-scsi-pci --serial0 socket --vga serial0 --agent enabled=1
qm config "$id" | grep -Fxq "description: $marker"
qm importdisk "$id" "$work/resolute-server-cloudimg-amd64.img" "$storage"
# Read the actual imported volume: storage plugins need not use the same naming.
volume=$(qm config "$id" | sed -n 's/^unused0: //p')
[[ $volume == "$storage":* && $volume != *[[:space:]]* ]] || { echo 'Missing imported volume; inspect builder' >&2; exit 1; }
qm set "$id" --scsi0 "$volume" --ide2 "$storage:cloudinit" --boot order=scsi0 \
  --ipconfig0 ip=dhcp --ciuser "$user" --sshkeys "$work/management.pub" --ciupgrade 0
qm disk resize "$id" scsi0 "${disk}G"
qm start "$id"
