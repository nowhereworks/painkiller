"""Bounded, read-only Q16 etcd data-directory and benchmark capture."""
import grp
import json
import os
from pathlib import Path, PurePosixPath
import pwd
import stat
import sys

import yaml

from capture_files import read_file
from host_fixtures import path
from workload_capture import command

DATA_DIR = "/var/lib/etcd"


def flag_value(args, key):
    for index, arg in enumerate(args):
        if arg.startswith(key + "="):
            return arg.split("=", 1)[1]
        if arg == key and index + 1 < len(args):
            return args[index + 1]
    return None


def validate_config(manifest, runtime):
    """Refuse unrelated paths and mismatched live/static component settings."""
    try:
        spec = manifest["spec"]
        container = next(c for c in spec["containers"] if c.get("name") == "etcd")
        desired = flag_value(container.get("command", []) + container.get("args", []), "--data-dir")
        live = flag_value(runtime["info"]["runtimeSpec"]["process"]["args"], "--data-dir")
        mount = next(m for m in container["volumeMounts"] if m.get("mountPath") == DATA_DIR)
        volume = next(v for v in spec["volumes"] if v["name"] == mount["name"])
        host = volume["hostPath"]["path"]
        if desired != DATA_DIR or live != DATA_DIR or host != DATA_DIR or mount.get("subPath") or mount.get("subPathExpr"):
            raise ValueError("unexpected etcd data path")
        uid = runtime["info"]["runtimeSpec"]["process"].get("user", {}).get("uid", 0)
        if not isinstance(uid, int) or isinstance(uid, bool):
            raise ValueError("invalid etcd process UID")
        return uid
    except (KeyError, TypeError, StopIteration, AttributeError, ValueError):
        raise RuntimeError("live etcd configuration does not match the expected data-directory mount") from None


def live_runtime(run=command):
    containers = json.loads(run(["crictl", "ps", "--name", "etcd", "-o", "json"], timeout=8).stdout)
    matches = [c for c in containers.get("containers", []) if c.get("metadata", {}).get("name") == "etcd"]
    if len(matches) != 1:
        raise RuntimeError("expected one running etcd container")
    return json.loads(run(["crictl", "inspect", matches[0]["id"]], timeout=8).stdout)


def identity():
    try:
        user = pwd.getpwnam("etcd")
        group = grp.getgrnam("etcd")
        if user.pw_uid == 0 or group.gr_gid == 0:
            raise RuntimeError("supporting etcd user or group must not be root")
        return user.pw_uid, group.gr_gid
    except KeyError:
        raise RuntimeError("supporting etcd user or group is missing") from None


def tree_checks(root, uid, gid, limit=10000):
    """No symlink following, filesystem traversal or modification outside the data tree."""
    directory = path(root, DATA_DIR)
    if not directory.is_dir() or directory.is_symlink():
        raise RuntimeError("supporting etcd data directory is missing or unsafe")
    # Check each ancestor so a symlink cannot redirect the scan elsewhere.
    current = root
    for component in PurePosixPath(DATA_DIR).parts[1:]:
        current = current / component
        if current.is_symlink():
            raise RuntimeError("supporting etcd data path contains a symlink")
    pending, seen = [directory], 0
    mode_ok = owner_ok = True
    root_device = directory.stat(follow_symlinks=False).st_dev
    while pending:
        item = pending.pop()
        info = item.stat(follow_symlinks=False)
        seen += 1
        if seen > limit:
            raise RuntimeError("etcd data tree exceeds bounded capture budget")
        if info.st_dev != root_device:
            raise RuntimeError("etcd data tree crosses an unexpected filesystem")
        mode = stat.S_IMODE(info.st_mode)
        owner_ok = owner_ok and info.st_uid == uid and info.st_gid == gid
        if stat.S_ISDIR(info.st_mode):
            mode_ok = mode_ok and mode & ~0o700 == 0
            with os.scandir(item) as entries:
                for entry in entries:
                    pending.append(Path(entry.path))
                    if len(pending) + seen > limit:
                        raise RuntimeError("etcd data tree exceeds bounded capture budget")
        elif stat.S_ISREG(info.st_mode):
            mode_ok = mode_ok and mode & 0o077 == 0 and mode & 0o600 == 0o600
        else:
            # Do not follow candidate-created symlinks or special files.
            mode_ok = owner_ok = False
    return bool(mode_ok), bool(owner_ok)


def benchmark(run=command):
    result = run(["kube-bench", "--config-dir", "/opt/kube-bench/cfg", "--benchmark", "cis-1.10",
                  "--check", "1.1.11,1.1.12", "--json", "--nototals", "run", "--targets", "master"], timeout=20)
    try:
        reports = json.loads(result.stdout)
        statuses = {check["test_number"]: check["status"] for report in reports
                    for group in report.get("tests", []) for check in group.get("results", [])}
        if not {"1.1.11", "1.1.12"} <= statuses.keys():
            raise ValueError("missing benchmark checks")
        return statuses["1.1.11"] == "PASS" and statuses["1.1.12"] == "PASS"
    except (ValueError, TypeError, KeyError, AttributeError):
        raise RuntimeError("supporting kube-bench returned invalid or incomplete results") from None


def capture(root=Path("/"), *, run=command, runtime=live_runtime, ids=identity, bench=benchmark):
    text = read_file(path(root, "/etc/kubernetes/manifests/etcd.yaml"))
    try:
        manifest = yaml.safe_load(text)
    except yaml.YAMLError:
        raise RuntimeError("supporting etcd manifest is invalid") from None
    process = runtime()
    live_uid = validate_config(manifest, process)
    uid, gid = ids()
    if live_uid not in (0, uid):
        raise RuntimeError("live etcd process does not run as root or the etcd owner")
    permissions, ownership = tree_checks(root, uid, gid)
    passing = bench()
    healthy = run(["kubectl", "--request-timeout=5s", "get", "--raw=/readyz"], timeout=8).stdout.strip() == "ok"
    result = {"permissions": permissions, "ownership": ownership,
              "benchmark": permissions and ownership and passing,
              "healthy": permissions and ownership and healthy}
    messages = {"permissions": "Restrict directories and files while retaining owner read/write on files.",
                "ownership": "Set ownership of the data tree to etcd:etcd.",
                "benchmark": "Ensure CIS checks 1.1.11 and 1.1.12 pass.",
                "healthy": "Keep etcd and the API server healthy after remediation."}
    result["failures"] = [message for key, message in messages.items() if not result[key]]
    return result


if __name__ == "__main__":
    print(json.dumps(capture()))
