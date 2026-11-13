#!/bin/bash
set -euo pipefail

export URL=https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
export IMG=/tmp/noble-server-cloudimg-amd64.img
export LABEL=ubuntu-server-cloud-init
export VM_ID=9199
export LOCAL_DISK=${LOCAL_DISK:-local-lvm}

if qm status "${VM_ID}" >/dev/null 2>&1 || [ -e "/etc/pve/qemu-server/${VM_ID}.conf" ]; then
    echo "VM ${VM_ID} already exists; refusing to overwrite it" >&2
    exit 1
fi
for cmd in wget virt-customize qm; do
    command -v "$cmd" >/dev/null || { echo "Missing required command: $cmd" >&2; exit 1; }
done
workdir=$(mktemp -d /tmp/ubuntu-template.XXXXXX)
trap 'rm -rf -- "$workdir"' EXIT
cat > "$workdir/register-dns" <<'REGISTER'
#!/bin/bash
set -euo pipefail
api=http://192.168.254.4:8888
zone=nowhereworks.local
hostname=$(hostname -s | tr '[:upper:]' '[:lower:]')
if [[ ! "$hostname" =~ ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$ ]] || [[ "$hostname" == ubuntu-server-cloud-init || "$hostname" == localhost ]]; then
    echo "DNS registration requires a unique clone hostname; got: $hostname" >&2
    exit 1
fi
ip=$(ip -4 route get 192.168.254.4 | awk '{for(i=1;i<=NF;i++) if($i=="src") {print $(i+1); exit}}')
[[ -n "$ip" ]] || { echo 'No IPv4 address for API route' >&2; exit 1; }
existing=$(curl --fail --silent --show-error --connect-timeout 5 --max-time 15 "$api/hosts/$zone/$hostname")
payload=$(python3 - "$hostname" "$zone" "$ip" "$existing" <<'PYJSON'
import ipaddress, json, sys
host, zone, ip, existing = sys.argv[1:]
ipaddress.IPv4Address(ip)
records = json.loads(existing)
if not isinstance(records, list):
    sys.exit('Unexpected DNS API response')
if records:
    if len(records) != 1 or records[0].get('record_type', '').lower() != 'a':
        sys.exit('Conflicting or ambiguous DNS records; refusing to overwrite them')
    if records[0].get('value') == ip:
        sys.exit(0)
print(json.dumps({'hostname': host, 'zone': zone, 'record_type': ['a'], 'value': ip}))
PYJSON
)
if [[ -z "$payload" ]]; then
    echo "DNS record already exists: $hostname.$zone -> $ip"
    exit 0
fi
curl --fail --silent --show-error --connect-timeout 5 --max-time 15 -X POST "$api/hosts" -H 'Content-Type: application/json' --data "$payload"
echo
REGISTER
cat > "$workdir/register-dns.service" <<'SERVICE'
[Unit]
Description=Register this VM in nowhereworks.local
Wants=network-online.target
After=network-online.target cloud-final.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/register-dns
Restart=on-failure
RestartSec=60

[Install]
WantedBy=cloud-final.service
SERVICE
cd /tmp
wget -c "${URL}"
virt-customize --add "${IMG}" --install qemu-guest-agent,curl \
    --upload "$workdir/register-dns:/usr/local/sbin/register-dns" \
    --upload "$workdir/register-dns.service:/etc/systemd/system/register-dns.service" \
    --run-command 'chmod 0755 /usr/local/sbin/register-dns && rm -f /etc/systemd/system/multi-user.target.wants/register-dns.service && systemctl enable register-dns.service'

qm create ${VM_ID} \
	--name ${LABEL} \
	--numa 0 \
	--ostype l26 \
	--cpu cputype=host \
	--cores 2 \
	--memory 8192 \
	--net0 virtio,bridge=vmbr0
	
qm importdisk ${VM_ID} ${IMG} ${LOCAL_DISK}

qm set ${VM_ID} --scsihw virtio-scsi-pci --scsi0 ${LOCAL_DISK}:vm-${VM_ID}-disk-0

qm set ${VM_ID} --ide2 ${LOCAL_DISK}:cloudinit

qm set ${VM_ID} --boot c --bootdisk scsi0

qm set ${VM_ID} --serial0 socket --vga serial0

qm set ${VM_ID} --agent enabled=1

qm disk resize ${VM_ID} scsi0 +30G

qm set ${VM_ID} --ipconfig0 ip=dhcp
qm set ${VM_ID} --ciuser milkman --cipassword 'password'
cat /root/id_rsa.pub > "$workdir/authorized-keys"
printf '\n%s\n' 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIND6yMxjBd6LFLQGUKv74S4VSce/AAP4LjbD8je35epL semaphore-lab-ansible' >> "$workdir/authorized-keys"
qm set ${VM_ID} --sshkey "$workdir/authorized-keys"

qm template ${VM_ID}

