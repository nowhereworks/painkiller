#!/usr/bin/env python3
"""Read-only initial-state assertions; never grades completed candidate work."""
import json
import pathlib
import subprocess
import time
import traceback
import urllib.request

import yaml


def run(*args):
    return subprocess.check_output(args, text=True, timeout=5)


def get(kind, name=None, namespace=None):
    args = ["kubectl", "--request-timeout=3s", "get", kind]
    if name:
        args.append(name)
    if namespace:
        args += ["-n", namespace]
    return json.loads(run(*args, "-o", "json"))


def http(path):
    with urllib.request.urlopen("http://127.0.0.1:18888" + path, timeout=2) as response:
        return json.load(response)


def check():
    nodes = get("nodes")["items"]
    assert len(nodes) == 1
    assert any(c["type"] == "Ready" and c["status"] == "True" for c in nodes[0]["status"]["conditions"])
    assert not any(t["key"] == "node-role.kubernetes.io/control-plane" for t in nodes[0]["spec"].get("taints", []))
    assert get("namespace", "opa001-system")["metadata"]["labels"]["painkiller-shell/exam"] == "OPA-001"
    for name in ("gatekeeper-controller-manager", "gatekeeper-audit"):
        dep = get("deployment", name, "gatekeeper-system")
        assert dep["spec"]["replicas"] == dep["status"].get("readyReplicas") == 1
    assert http("/health") == {"ok": True}
    assert "Version: 1.22.2" in run("opa", "version")
    assert "0.71.1" in run("conftest", "--version")
    assert "0.43.0" in run("regal", "version")
    for name in ("receiver", "q9", "q10", "q11"):
        assert run("systemctl", "is-active", f"opa001-{name}").strip() == "active"
    for number in range(1, 17):
        assert pathlib.Path(f"/opt/opa-workshop/q{number}").is_dir()
    for number in (1, 2, 3, 5, 8, 16):
        assert "default allow := true" in pathlib.Path(f"/opt/opa-workshop/q{number}/policy.rego").read_text()
    assert json.loads(pathlib.Path("/opt/opa-workshop/q2/data.json").read_text())["users"]["alice"]["tenant"] == "blue"
    assert json.loads(pathlib.Path("/opt/opa-workshop/q8/data.json").read_text())["relationships"]["alice"] == ["team:blue"]
    assert len(json.loads(pathlib.Path("/opt/opa-workshop/q16/data.json").read_text())["inventory"]) == 4000
    for path, expected in {
        "q4/policy.rego": "violations := false",
        "q6/tests.rego": "test_placeholder",
        "q7/adapter.py": "'allow':True",
        "q11/mask.rego": "mask := []",
        "q15/policy/main.rego": "all resources forbidden",
    }.items():
        assert expected in pathlib.Path("/opt/opa-workshop", path).read_text()
    assert json.loads(pathlib.Path("/opt/opa-workshop/q13/audit.json").read_text()) == {}
    assert not pathlib.Path("/opt/opa-workshop/q9/bundle.tar.gz").exists()
    assert not get("config.config.gatekeeper.sh", "config", "gatekeeper-system").get("spec", {}).get("sync", {}).get("syncOnly")
    assert get("opaauditlabels", "audit-env")["spec"]["match"]["excludedNamespaces"] == ["opa-audit"]
    assert not get("configmap", "legacy", "opa-audit")["metadata"].get("labels", {}).get("env")
    for team in ("blue", "green"):
        assert get("namespace", "opa-inventory-" + team)["metadata"]["labels"]["team"] == team
    templates = get("constrainttemplates")["items"]
    assert {t["metadata"]["name"] for t in templates} == {"opaauditlabels", "opanamespaceteam"}
    for template in templates:
        assert template.get("status", {}).get("created") is True
        assert not any(p.get("errors") for p in template.get("status", {}).get("byPod", []))
    config = yaml.safe_load(pathlib.Path("/opt/opa-workshop/q10/config.yaml").read_text())
    assert config["bundles"]["broken"]["resource"] == "bundles/broken.tar.gz"
    events = http("/events/status/q10")
    assert events and events[-1].get("bundles", {}).get("broken", {}).get("code")


if __name__ == "__main__":
    for attempt in range(3):
        try:
            check()
            print("OPA-001 single-node topology, services, fixtures, and intentional faults verified")
            break
        except (AssertionError, KeyError, OSError, ValueError, subprocess.SubprocessError) as exc:
            if attempt == 2:
                location = traceback.extract_tb(exc.__traceback__)[-1]
                raise SystemExit(f"OPA-001 readiness failed at line {location.lineno}: {type(exc).__name__}: {exc}") from exc
            time.sleep(2)
