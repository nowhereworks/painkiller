#!/usr/bin/env bash
# Set up an ephemeral, lab-only SSH identity for candidate node access.
# Remote shell variables intentionally expand on the lab VM.
# shellcheck disable=SC2016
set -Eeuo pipefail
known_hosts=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}
user=${LAB_SSH_USER:-milkman}
prefix=${LAB_IP_PREFIX:-192.168.254.}
[[ $user =~ ^[a-zA-Z0-9_-]+$ && $prefix =~ ^[0-9.]+$ ]] || { echo 'Invalid SSH user or IP prefix' >&2; exit 1; }
node_ssh() { ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known_hosts" "$@"; }
state=$("$(dirname "$0")/create-lab.sh" status)
controllers=$(jq -r .controller_ips <<<"$state")
workers=$(jq -r .worker_ips <<<"$state")
first=${controllers%%,*}
# Only create a dedicated key within the freshly provisioned controller VM.
node_ssh "$user@$first" 'set -eu; mkdir -p "$HOME/.ssh"; chmod 700 "$HOME/.ssh"; if [ ! -f "$HOME/.ssh/painkiller_lab" ]; then ssh-keygen -q -t ed25519 -N "" -f "$HOME/.ssh/painkiller_lab"; fi'
pub=$(node_ssh "$user@$first" 'cat "$HOME/.ssh/painkiller_lab.pub"')
read -r kind material _ <<<"$pub"
[[ $kind == ssh-ed25519 && $material =~ ^[a-zA-Z0-9+/=]+$ ]] || { echo 'Invalid candidate public key' >&2; exit 1; }
key="$kind $material"
IFS=, read -r -a ips <<<"$controllers${workers:+,$workers}"
for ip in "${ips[@]}"; do
  [[ $ip == "$first" ]] && continue
  node_ssh "$user@$ip" "set -eu; mkdir -p \"\$HOME/.ssh\"; chmod 700 \"\$HOME/.ssh\"; touch \"\$HOME/.ssh/authorized_keys\"; chmod 600 \"\$HOME/.ssh/authorized_keys\"; grep -Fxq '$key' \"\$HOME/.ssh/authorized_keys\" || printf '%s\\n' '$key' >> \"\$HOME/.ssh/authorized_keys\""
done
node_ssh "$user@$first" 'cat > "$HOME/.ssh/painkiller_known_hosts"; chmod 600 "$HOME/.ssh/painkiller_known_hosts"' < "$known_hosts"
# A separate Include file avoids changing unrelated SSH configuration on the VM.
config=$(printf 'Host %s*\n  IdentityFile ~/.ssh/painkiller_lab\n  UserKnownHostsFile ~/.ssh/painkiller_known_hosts\n  StrictHostKeyChecking yes\n' "$prefix")
node_ssh "$user@$first" "printf '%s\\n' '$config' > \"\$HOME/.ssh/painkiller_config\"; chmod 600 \"\$HOME/.ssh/painkiller_config\"; touch \"\$HOME/.ssh/config\"; chmod 600 \"\$HOME/.ssh/config\"; if ! grep -Fxq 'Include ~/.ssh/painkiller_config' \"\$HOME/.ssh/config\"; then tmp=\$(mktemp \"\$HOME/.ssh/painkiller-config.XXXXXX\"); { printf '%s\\n' 'Include ~/.ssh/painkiller_config'; cat \"\$HOME/.ssh/config\"; } > \"\$tmp\"; chmod 600 \"\$tmp\"; mv \"\$tmp\" \"\$HOME/.ssh/config\"; fi"
