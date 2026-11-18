"""Bounded, read-only grading captures for the Q7/Q8/Q10 host-file subset.

The deployment/Pod state and actual functionality still need separate capture
checks; these results alone must not be used as a complete task grade.
"""
import json
from pathlib import Path
import sys

from host_fixtures import path
from semantic_checks import dockerfile_checks, kubesec_report_valid, seccomp_checks


def read_file(filename):
    try:
        # Candidate files are small. Refuse oversized evidence rather than
        # allowing learning-mode verification to consume unbounded memory.
        with filename.open("r") as stream:
            text = stream.read(1048577)
        if len(text) > 1048576:
            return ""
        return text
    except FileNotFoundError:
        return ""


def json_file(filename):
    try:
        return json.loads(read_file(filename))
    except (ValueError, TypeError):
        return None


def capture(question, root=Path("/")):
    if question == "q7":
        result = dockerfile_checks(read_file(path(root, "/opt/course/ledger/ledger.Dockerfile")))
        messages = {
            "base": "Use the distroless base image.",
            "minimal": "Remove shell-dependent instructions and do not add lines.",
            "binary": "Copy only the supplied ledger binary.",
            "nonroot": "Set USER to nonroot or UID 65532.",
            "exec": "Use an exec-form ENTRYPOINT for the copied binary.",
        }
    elif question == "q8":
        result = {"report": kubesec_report_valid(json_file(path(root, "/root/CKS/invoice-kubesec.json")))}
        messages = {"report": "Save a passing Kubesec JSON report for Pod/invoice-web.default."}
    elif question == "q10":
        result = seccomp_checks(json_file(path(root, "/var/lib/kubelet/seccomp/profiles/no-inspect.json")))
        messages = {"default_allow": "The profile default action must allow.",
                    "debug_blocked": "Deny ptrace and process_vm_readv unconditionally with nonzero errno.",
                    "other_allowed": "Do not deny other syscalls."}
    else:
        raise ValueError("unsupported capture: " + question)
    result["failures"] = [message for key, message in messages.items() if not result[key]]
    return result


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: capture_files.py {q7|q8|q10}")
    print(json.dumps(capture(sys.argv[1])))
