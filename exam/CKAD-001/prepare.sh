#!/usr/bin/env bash
# Exam-owned preparation, using only ownership-checked disposable guests.
# shellcheck disable=SC2016
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
exam=$root/exam/CKAD-001
known=${LAB_KNOWN_HOSTS:-$PWD/data/lab_known_hosts}
user=${LAB_SSH_USER:-milkman}
node() {
  local ip=$1 command
  shift
  printf -v command '%q' "$*"
  ssh -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes \
    -o GlobalKnownHostsFile=/dev/null -o "UserKnownHostsFile=$known" "$user@$ip" \
    "bash -e -o pipefail -c $command"
}
case "${1:-}" in
preflight)
  [[ ${LAB_TEMPLATE_ID:-9199} == 9199 ]] || { echo 'CKAD-001 requires the generic base template 9199' >&2; exit 1; }
  (cd "$exam/assets" && sha256sum --check --quiet SHA256SUMS)
  exit 0 ;;
infrastructure|fixtures) ;;
*) echo "Usage: $0 {preflight|infrastructure|fixtures}" >&2; exit 2 ;;
esac
state=$("$root/scripts/create-lab.sh" status)
cp=$(jq -er '.controller_ips | split(",")[0]' <<<"$state")
marker=/var/lib/painkiller/ckad001
if [[ $1 == infrastructure ]]; then
  mapfile -t ips < <(jq -r '[.controller_ips,.worker_ips] | .[] | split(",")[] | select(length>0)' <<<"$state")
  for ip in "${ips[@]}"; do
    # Fresh base clones can accept SSH before their cloud-init apt work finishes.
    # Exit 2 means recoverable warnings; accept it only after successful completion.
    node "$ip" 'bash -s' < "$exam/assets/wait-cloud-init.sh"
  done
  node "$cp" "if ! sudo -n test -e $marker/packages; then
    sudo -n apt-get update -qq
    sudo -n env DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 install -y -qq python3-yaml curl jq
    sudo -n mkdir -p $marker
    sudo -n touch $marker/packages
  fi
  if ! command -v helm >/dev/null; then
    d=\$(mktemp -d)
    cd \"\$d\"
    curl -fsSLO https://get.helm.sh/helm-v3.19.0-linux-amd64.tar.gz
    curl -fsSLO https://get.helm.sh/helm-v3.19.0-linux-amd64.tar.gz.sha256sum
    sha256sum -c helm-v3.19.0-linux-amd64.tar.gz.sha256sum
    tar -xzf helm-v3.19.0-linux-amd64.tar.gz
    sudo -n install linux-amd64/helm /usr/local/bin/helm
  fi"
  exit 0
fi
if node "$cp" "sudo -n test -e $marker/complete"; then
  echo 'CKAD-001 already prepared; preserving candidate work'
  exit 0
fi
# Install trusted capture code separately from candidate-editable chart/files.
node "$cp" 'sudo -n mkdir -p /opt/painkiller/ckad001 /opt/lantern-chart /root/workshop-env'
node "$cp" 'sudo -n tee /opt/painkiller/ckad001/capture.sh >/dev/null' < "$exam/assets/capture.sh"
node "$cp" 'sudo -n tee /opt/painkiller/ckad001/tasks.json >/dev/null' < "$exam/tasks.json"
node "$cp" 'sudo -n tee /opt/painkiller/ckad001/mutate.sh >/dev/null' < "$exam/assets/mutate.sh"
node "$cp" 'sudo -n tee /opt/painkiller/ckad001/host-web.py >/dev/null' < "$exam/assets/host-web.py"
node "$cp" 'sudo -n tee /etc/systemd/system/ckad001-host-web.service >/dev/null
sudo -n systemctl daemon-reload
sudo -n systemctl enable --now ckad001-host-web' <<'UNIT'
[Unit]
Description=CKAD-001 host HTTP fixture (outside Kubernetes)
After=network.target
[Service]
User=nobody
ExecStart=/usr/bin/python3 /opt/painkiller/ckad001/host-web.py
Restart=on-failure
[Install]
WantedBy=multi-user.target
UNIT
if ! node "$cp" "sudo -n test -e $marker/assets"; then
  tar -C "$exam/assets/chart" -cf - . | node "$cp" 'sudo -n tar -C /opt/lantern-chart -xf -
    sudo -n chown -R "$(id -un):$(id -gn)" /opt/lantern-chart'
  for file in app cache; do
    node "$cp" "sudo -n tee /root/workshop-env/$file.env >/dev/null" < "$exam/assets/$file.env"
  done
  node "$cp" 'sudo -n tee /root/workshop-secret-pod.yaml >/dev/null' < "$exam/assets/secret-pod.yaml"
  node "$cp" 'sudo -n tee /root/workshop-busybox.yaml >/dev/null' < "$exam/assets/busybox.yaml"
  node "$cp" "sudo -n touch $marker/assets"
fi
if ! node "$cp" "sudo -n test -e $marker/fixtures"; then
  node "$cp" 'kubectl apply -f -' < "$exam/setup.yaml"
  node "$cp" "sudo -n touch $marker/fixtures"
fi
node "$cp" 'kubectl wait --for=condition=Established crd/animes.entertainment.ckad001.test --timeout=60s
kubectl -n kube-system rollout status daemonset/calico-node --timeout=300s
kubectl -n kube-system rollout status deployment/coredns --timeout=180s
kubectl -n database-workshop rollout status deployment/sql02 --timeout=300s
for target in "ckad001-system observer" "ckad001-policycheck open-web" "ckad001-policycheck blocked-web" \
  "default parcel-web" "default warmup-web" "qos-workshop baseline-best-effort" "qos-workshop baseline-guaranteed"; do
  read -r ns name <<<"$target"
  kubectl -n "$ns" wait "pod/$name" --for=condition=Ready --timeout=180s
done'
# The read-only check proves network enforcement as well as the intended faults.
bash "$exam/check-ready.sh"
node "$cp" "sudo -n touch $marker/complete"
