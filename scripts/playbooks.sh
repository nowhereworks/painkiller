#!/usr/bin/env bash
# Shared local playbook paths and image compatibility checks. Source only.
playbooks=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/playbooks
check_playbooks() {
  (cd "$playbooks" && sha256sum --check --status SHA256SUMS) || { echo 'Missing or modified local playbook; check playbooks/SHA256SUMS' >&2; return 1; }
}
