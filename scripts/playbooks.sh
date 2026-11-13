#!/usr/bin/env bash
# Shared local playbook paths and image compatibility checks. Source only.
playbooks=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/playbooks
revision=$(cat "$playbooks/image-revision" 2>/dev/null) || revision=
check_playbooks() {
  [[ $revision =~ ^[a-f0-9]{40}$ ]] || { echo 'Missing or invalid local playbook image revision' >&2; return 1; }
  (cd "$playbooks" && sha256sum --check --status SHA256SUMS) || { echo 'Missing or modified local playbook; check playbooks/SHA256SUMS' >&2; return 1; }
}
