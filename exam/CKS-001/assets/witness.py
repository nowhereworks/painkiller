"""Generate independent evidence. Grading never creates/deletes/patches resources."""
import json
import time
from common import STATE, NGINX, k, pod, write


def sample():
    # A delete of a nonexistent Secret still produces an audit event.
    k("-n", "kube-system", "delete", "secret", "cks001-absent-audit-secret", "--ignore-not-found")
    k("-n", "default", "patch", "deployment", "cks001-audit-canary", "--type=merge", "-p",
      json.dumps({"metadata": {"annotations": {"cks001-witness": str(time.time_ns())}}}))
    k("-n", "cks001-system", "get", "service", "image-gate")
    tests = {}
    for key, image in [("latest", "nginx:latest"), ("other", "registry.example/security/demo:latest"),
                       ("implicit", "nginx"), ("tagged", NGINX)]:
        result = k("create", "--dry-run=server", "-f", "-", data=json.dumps(
            pod("cks001-admission-probe", "cks001-system", image)), check=False)
        tests[key] = {"code": result.returncode, "error": result.stderr}
    write(STATE / "admission-evidence.tmp", json.dumps({"time": time.time(), "tests": tests}))
    (STATE / "admission-evidence.tmp").replace(STATE / "admission-evidence.json")


if __name__ == "__main__":
    while True:
        try:
            sample()
        except Exception as error:
            print(error, flush=True)
        time.sleep(5)
