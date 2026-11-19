"""Run the selected exact candidate solution on a marked disposable exam."""
import json
from pathlib import Path
import subprocess
import sys
import time


def main(n):
    marker = json.loads(subprocess.run(
        ["kubectl", "--request-timeout=5s", "get", "namespace", "cks001b-system", "-o", "json"],
        check=True, text=True, capture_output=True, timeout=8).stdout)
    if marker.get("metadata", {}).get("labels", {}).get("painkiller-shell/exam") != "CKS-001B":
        raise RuntimeError("Refusing reference answers on an unmarked cluster")
    tasks = json.loads(Path("/opt/painkiller/cks001b/tasks.json").read_text())
    if not 1 <= n <= len(tasks):
        raise ValueError("invalid question number")
    print("Solving", tasks[n - 1]["question_id"], flush=True)
    subprocess.run(["bash", "-e", "-o", "pipefail", "-c", tasks[n - 1]["solution"]], check=True, timeout=240)
    # Await asynchronous observers/rollouts without performing any extra repair.
    for _ in range(20):
        result = subprocess.run(["sudo", "-n", "env", "KUBECONFIG=/etc/kubernetes/admin.conf",
            "timeout", "27s", "python3", "-B", "/opt/painkiller/cks001b/capture.py", f"q{n}"],
            text=True, capture_output=True, timeout=29)
        if result.returncode == 0:
            data = json.loads(result.stdout)
            if all(data.get(c["expression"].split()[0][1:]) is True for c in tasks[n - 1]["checks"]):
                return
        time.sleep(1)
    raise RuntimeError(f"Q{n} did not converge: {result.stdout} {result.stderr}")


if __name__ == "__main__":
    main(int(sys.argv[1]))
