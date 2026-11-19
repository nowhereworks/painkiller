"""Seed Q16's single-directory fault only on a marked disposable cluster."""
import grp
import os
from pathlib import Path, PurePosixPath
import pwd
import subprocess

import yaml

from capture_files import read_file
from etcd_capture import DATA_DIR, live_runtime, validate_config
from host_fixtures import create_once, path
from workload_capture import Kubernetes


def ensure_identity():
    try:
        group = grp.getgrnam("etcd")
    except KeyError:
        subprocess.run(["groupadd", "--system", "etcd"], check=True, timeout=10)
        group = grp.getgrnam("etcd")
    try:
        user = pwd.getpwnam("etcd")
    except KeyError:
        subprocess.run(["useradd", "--system", "--gid", "etcd", "--no-create-home",
                        "--shell", "/usr/sbin/nologin", "etcd"], check=True, timeout=10)
        user = pwd.getpwnam("etcd")
    if user.pw_gid != group.gr_gid or user.pw_uid == 0 or group.gr_gid == 0:
        raise RuntimeError("existing etcd user has an unexpected primary group")
    return user.pw_uid, group.gr_gid


def inject(kube, root, *, runtime=live_runtime, ids=ensure_identity, euid=os.geteuid,
           change_owner=os.fchown, change_mode=os.fchmod):
    state = path(root, "/var/lib/painkiller/cks001b")
    if euid() != 0 or not (state / "authorized").is_file() or not (state / "services-complete").is_file():
        raise RuntimeError("Refusing etcd fault on unauthorized or unprepared guest")
    marker = kube.get("namespace", "cks001b-system")
    if marker.get("metadata", {}).get("labels", {}).get("painkiller-shell/exam") != "CKS-001B":
        raise RuntimeError("Refusing etcd fault on unmarked cluster")
    nodes = kube.get("nodes").get("items", [])
    if len(nodes) != 1:
        raise RuntimeError("Refusing etcd fault outside a single-node cluster")
    if (state / "etcd-fault").exists():
        return
    try:
        manifest = yaml.safe_load(read_file(path(root, "/etc/kubernetes/manifests/etcd.yaml")))
    except yaml.YAMLError:
        raise RuntimeError("supporting etcd manifest is invalid") from None
    if validate_config(manifest, runtime()) != 0:
        raise RuntimeError("Refusing etcd fault when the live etcd process is not root")
    target = path(root, DATA_DIR)
    current = root
    for component in PurePosixPath(DATA_DIR).parts[1:]:
        current = current / component
        if current.is_symlink():
            raise RuntimeError("Refusing etcd fault on a symlinked data path")
    if not target.is_dir():
        raise RuntimeError("Refusing etcd fault without the expected data directory")
    uid, gid = ids()
    if uid == 0 or gid == 0:
        raise RuntimeError("Refusing etcd fault without a non-root etcd identity")
    # Only the single top-level data directory is deliberately misconfigured.
    # Files and subdirectories remain untouched until the candidate works.
    create_once(state / "etcd-fault-started", "complete\n", 0o600)
    fd = os.open(target, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        change_owner(fd, 0, 0)
        change_mode(fd, 0o750)
    finally:
        os.close(fd)
    create_once(state / "etcd-fault", "complete\n", 0o600)


if __name__ == "__main__":
    inject(Kubernetes(), Path("/"))
