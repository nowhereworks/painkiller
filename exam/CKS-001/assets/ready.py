"""Read-only starting-state contract. No kubectl writes or answer grading."""
import hashlib
import json
from pathlib import Path
import stat

from common import STATE, default_rules_digest, exec_probe, flags, flagmap, get, k, ready, run, yaml_file


def main():
    nodes = get("nodes")["items"]
    assert len(nodes) == 1
    assert all(any(c["type"] == "Ready" and c["status"] == "True" for c in n["status"]["conditions"]) for n in nodes)
    assert get("namespace", "cks001-system")["metadata"]["labels"]["painkiller-shell/exam"] == "CKS-001"
    assert get("daemonset", "calico-node", "kube-system")["status"]["numberReady"] == 1
    for ns, name in [("helix", "frontend"), ("relay", "connector"), ("runtime-watch", "binary-writer"),
                     ("storage", "catalog-db"), ("storage", "other-client"), ("billing", "billing-client"),
                     ("default", "outsider"), ("cks001-system", "observer")]:
        assert ready(get("pod", name, ns)), f"{ns}/{name} is not Ready"
    for ns, name in [("inventory", "toolkit"), ("batch-ops", "janitor"), ("observatory", "delta"),
                     ("catalog", "catalog-web"), ("launch", "shuttle"), ("traefik", "traefik"),
                     ("cks001-system", "image-gate")]:
        assert ready(get("deployment", name, ns)), f"{ns}/{name} is not Ready"
    assert not ready(get("deployment", "portal", "locked-down"))
    assert get("namespace", "locked-down")["metadata"]["labels"]["pod-security.kubernetes.io/enforce"] == "restricted"
    assert get("pod", "frontend", "helix")["spec"]["serviceAccountName"] == "site-admin"
    assert len(get("serviceaccounts", ns="helix")["items"]) == 4
    assert get("pod", "connector", "relay")["spec"]["containers"][0]["env"][0]["name"] == "BROKER_PASSWORD"
    assert get("pod", "harbor-web", "harbor")["spec"]["containers"][0]["image"] == "nginx:latest"
    assert get("deployment", "janitor", "batch-ops")["spec"]["template"]["spec"].get("serviceAccountName", "default") == "default"
    for ns in ["team-a", "team-b", "team-c"]:
        assert get("role", "alex-pods", ns)["rules"] == [{"apiGroups": [""], "resources": ["*"], "verbs": ["*"]}]
    assert Path("/root/CKS/web-guard.json").is_file()
    assert Path("/etc/apparmor.d/site-guard").is_file()
    assert Path("/root/ImageTarballs/alpine_curl_8.12.1.tar").stat().st_size > 100000
    assert Path("/root/backup/kube-apiserver.yaml").is_file()
    assert Path("/root/CKS/ImageGate/admission-kubeconfig.yaml").is_file()
    assert len(list(Path("/opt/security-review").glob("Dockerfile.*"))) == 3
    assert stat.S_IMODE(Path("/var/lib/kubelet/config.yaml").stat().st_mode) == 0o666
    assert stat.S_IMODE(Path("/usr/lib/systemd/system/kubelet.service").stat().st_mode) == 0o666
    assert Path("/var/lib/etcd").stat().st_uid == 0
    for name in ["kube-controller-manager", "kube-scheduler"]:
        assert flagmap(flags(name))["profiling"] == "true"
        assert flagmap(flags(name))["leader-elect"] == "false"
    assert flagmap(flags("kube-controller-manager"))["node-monitor-grace-period"] == "120s"
    assert "audit-policy-file" not in flagmap(flags("kube-apiserver"))
    assert yaml_file("/etc/kubernetes/manifests/kube-apiserver.yaml")["spec"]["terminationGracePeriodSeconds"] == 5
    assert "ImagePolicyWebhook" not in flagmap(flags("kube-apiserver")).get("enable-admission-plugins", "")
    assert yaml_file("/etc/falco/falco_rules.local.yaml") == []
    run(["systemctl", "is-active", "--quiet", "falco", "cks001-witness"])
    journal = run(["journalctl", "-u", "falco", "-n", "100", "--no-pager"]).stdout
    assert "File below a known binary directory opened for writing" in journal, "Falco is not detecting real binary writes"
    for tool in ["bom", "kube-bench", "apparmor_parser"]:
        run(["which", tool])
    assert default_rules_digest() == (STATE / "falco-default.sha256").read_text()
    assert "helix-private" in exec_probe("helix", "frontend", ["wget", "-qO-", "http://127.0.0.1/staff/"]).stdout
    toolkit = next(p for p in get("pods", ns="inventory")["items"] if ready(p))
    for container in ["cedar", "maple", "birch"]:
        probe = exec_probe("inventory", toolkit["metadata"]["name"], ["sh", "-c", "command -v curl"], container=container)
        assert (probe.returncode == 0) == (container == "maple")
    assert exec_probe("inventory", toolkit["metadata"]["name"], ["apk", "info", "-e", "curl"], container="maple").returncode == 0
    # Initial candidate policy is absent. Check CNI enforcement using separate canaries.
    open_ip = get("pod", "open-web", "cks001-canary")["status"]["podIP"]
    blocked_ip = get("pod", "blocked-web", "cks001-canary")["status"]["podIP"]
    assert exec_probe("cks001-system", "observer", ["wget", "-T", "2", "-qO-", f"http://{open_ip}"]).returncode == 0
    assert exec_probe("cks001-canary", "blocked-web", ["wget", "-T", "2", "-qO-", "http://127.0.0.1/"]).returncode == 0
    assert exec_probe("cks001-system", "observer", ["wget", "-T", "2", "-qO-", f"http://{blocked_ip}"]).returncode != 0
    evidence = json.loads((STATE / "admission-evidence.json").read_text())
    assert evidence["tests"]["latest"]["code"] == 0
    print("CKS-001 initial topology, supporting services, network enforcement, and faults verified")


if __name__ == "__main__":
    main()
