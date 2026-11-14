#!/usr/bin/env bash
# Run only against a disposable, explicitly selected VM range.
set -Eeuo pipefail
cd "$(dirname "$0")/.."
if [[ $# != 1 || ! $1 =~ ^[A-Z][A-Z0-9]*-[0-9]{3}$ ]]; then
  echo "Usage: LAB_VM_START=<free VM ID> scripts/test-exam.sh <exam ID>" >&2
  exit 2
fi
if [[ ! ${LAB_VM_START:-} =~ ^[0-9]+$ ]] || (( 10#$LAB_VM_START < 3000 || 10#$LAB_VM_START > 3999 )); then
  echo "Set LAB_VM_START to the first ID of a verified free disposable range (3000–3999)." >&2
  exit 2
fi
test_file="exam/$1/test.json"
[[ -f $test_file ]] || { echo "Unknown exam: $1" >&2; exit 2; }
nodes=$(jq -er 'if .lab then .lab.controllers + .lab.workers else 3 end | select(type == "number" and . >= 1 and . <= 20 and floor == .)' "$test_file")
export LAB_VM_START=$((10#$LAB_VM_START))
export LAB_VM_END=${LAB_VM_END:-$((LAB_VM_START + nodes - 1))}
if [[ ! $LAB_VM_END =~ ^[0-9]+$ ]] || (( 10#$LAB_VM_END != LAB_VM_START + nodes - 1 || 10#$LAB_VM_END > 3999 )); then
  echo "LAB_VM_END must cover exactly $nodes VMs within 3000–3999." >&2
  exit 2
fi
mkdir -p data
run_dir=$(mktemp -d "$PWD/data/exam-test.XXXXXXXX")
export LAB_KNOWN_HOSTS="$run_dir/known_hosts"
export LAB_MARKER="painkiller-test-${run_dir##*.}"
export PAINKILLER_LIVE_EXAM="$1" PAINKILLER_LIVE_DIR="$run_dir"
echo "Test artifacts: $run_dir"
echo "Ownership marker: $LAB_MARKER"
go test -v -count=1 -timeout=120m ./cmd/painkiller -run '^TestLiveExam$'
