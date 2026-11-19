"""Read-only readiness for Q16's deliberately faulty data directory."""
import json
from pathlib import Path
import stat
import sys

import yaml

from capture_files import read_file
from etcd_capture import DATA_DIR, identity, live_runtime, validate_config
from host_fixtures import path
from workload_capture import command


def inspect(root=Path("/"), *, runtime=live_runtime, ids=identity, run=command, stat_path=None):
    state = path(root, "/var/lib/painkiller/cks001b")
    failures = []
    if not (state / "etcd-fault").is_file():
        failures.append("etcd fault marker missing")
    try:
        manifest = yaml.safe_load(read_file(path(root, "/etc/kubernetes/manifests/etcd.yaml")))
    except yaml.YAMLError:
        raise RuntimeError("supporting etcd manifest is invalid") from None
    live_uid = validate_config(manifest, runtime())
    uid, gid = ids()
    target = path(root, DATA_DIR)
    if target.is_symlink() or not target.is_dir():
        raise RuntimeError("supporting etcd data directory is missing or unsafe")
    info = stat_path(target) if stat_path else target.stat(follow_symlinks=False)
    if (live_uid != 0 or uid == 0 or gid == 0 or info.st_uid != 0 or info.st_gid != 0 or
            stat.S_IMODE(info.st_mode) != 0o750):
        failures.append("etcd initial ownership or directory-mode fault missing")
    if run(["kubectl", "--request-timeout=5s", "get", "--raw=/readyz"], timeout=8).stdout.strip() != "ok":
        failures.append("API server not ready after etcd fault injection")
    return failures


if __name__ == "__main__":
    problems = inspect()
    if problems:
        print("\n".join(problems), file=sys.stderr)
        sys.exit(1)
    print("CKS-001B etcd subset ready (not full exam readiness)")
