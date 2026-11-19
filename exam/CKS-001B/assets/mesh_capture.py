"""Read-only Q14 PeerAuthentication, sidecar and traffic capture."""
import json
import sys

from workload_capture import Kubernetes, deployment_pods, ready_deployment, ready_pod

PLAIN_PROBE = r"""
import json, socket, sys
def request(address):
  try:
    with socket.create_connection((address,80), timeout=2) as conn:
      conn.settimeout(2)
      conn.sendall(b'GET / HTTP/1.1\r\nHost: fixture\r\nConnection: close\r\n\r\n')
      return b'HTTP/1.' in conn.recv(1024)
  except OSError:
    return False
print(json.dumps({'control':request(sys.argv[1]),'plaintext':request(sys.argv[2])}))
"""


def strict_policy(policy):
    spec = policy.get("spec", {})
    selector = spec.get("selector") or {}
    if not isinstance(selector, dict) or selector.get("matchLabels") or set(selector) - {"matchLabels"}:
        return False
    overrides = spec.get("portLevelMtls") or {}
    if not isinstance(overrides, dict):
        return False
    mtls = spec.get("mtls") or {}
    return (isinstance(mtls, dict) and mtls.get("mode") == "STRICT" and all(
        isinstance(value, dict) and value.get("mode", "UNSET") in ("STRICT", "UNSET")
        for value in overrides.values()))


def proxy_ready(pod):
    if not ready_pod(pod):
        return False
    spec = pod.get("spec", {})
    normal = spec.get("containers", [])
    native = [c for c in spec.get("initContainers", []) if c.get("restartPolicy") == "Always"]
    has_container = any(c.get("name") == "istio-proxy" for c in normal + native)
    statuses = pod.get("status", {}).get("containerStatuses", []) + pod.get("status", {}).get("initContainerStatuses", [])
    return has_container and any(c.get("name") == "istio-proxy" and c.get("ready") is True and
                                 "running" in c.get("state", {})
                                 for c in statuses)


def capture(kube=None):
    kube = kube or Kubernetes()
    policy = kube.get("peerauthentication", "default", "private-mesh", optional=True)
    strict = bool(policy) and strict_policy(policy)
    result = {"failures": []}

    def check(key, condition, message):
        result[key] = bool(condition)
        if not condition:
            result["failures"].append(message)

    check("strict", strict, "Configure namespace-wide STRICT mTLS with no permissive port overrides.")
    deployments = {name: kube.get("deployment", name, "private-mesh", optional=True)
                   for name in ("north", "south")}
    pods = {name: deployment_pods(kube, dep, "private-mesh") for name, dep in deployments.items()}
    proxies = {name: bool(pods[name]) and proxy_ready(pods[name][0]) for name in ("north", "south")}
    check("north_proxy", proxies["north"], "Leave north running with a Ready istio-proxy.")
    check("south_proxy", proxies["south"], "Leave south running with a Ready istio-proxy.")
    communicate = plaintext_blocked = False
    if strict and all(proxies.values()):
        istiod = kube.get("deployment", "istiod", "istio-system")
        control = kube.get("pod", "net-control", "cks001b-system")
        portal = kube.get("service", "portal", "secure-portal")
        control_ip = portal.get("spec", {}).get("clusterIP")
        south_ip = pods["south"][0].get("status", {}).get("podIP")
        if not ready_deployment(istiod) or not ready_pod(control) or not control_ip or not south_ip:
            raise RuntimeError("supporting Istio control plane, probe Pod or network endpoints unavailable")
        north_name = pods["north"][0]["metadata"]["name"]
        containers = [c.get("name") for c in pods["north"][0].get("spec", {}).get("containers", [])
                      if c.get("name") and c["name"] != "istio-proxy"]
        if not containers:
            raise RuntimeError("north workload has no application container for its connectivity probe")
        container = "north" if "north" in containers else containers[0]
        # Probe the named Service via the application's own sidecar.
        request = kube.exec("private-mesh", north_name,
                            ["wget", "-T", "3", "-q", "-O", "/dev/null",
                             "http://south.private-mesh.svc.cluster.local/"], container)
        communicate = request.returncode == 0
        if communicate:
            raw = kube.exec("cks001b-system", "net-control",
                            ["python3", "-c", PLAIN_PROBE, control_ip, south_ip], "net-control")
            if raw.returncode:
                raise RuntimeError("supporting plaintext control probe failed")
            try:
                observed = json.loads(raw.stdout)
            except (TypeError, ValueError):
                raise RuntimeError("supporting plaintext control probe returned invalid data") from None
            if not isinstance(observed, dict) or observed.get("control") is not True or not isinstance(
                    observed.get("plaintext"), bool):
                raise RuntimeError("independent plaintext control cannot reach its known-good endpoint")
            plaintext_blocked = not observed["plaintext"]
    check("communication", strict and all(proxies.values()) and communicate,
          "Leave both meshed workloads able to communicate.")
    check("plaintext_blocked", strict and all(proxies.values()) and communicate and plaintext_blocked,
          "Reject an unmeshed plaintext request while meshed traffic succeeds.")
    return result


if __name__ == "__main__":
    print(json.dumps(capture()))
