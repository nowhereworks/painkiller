"""Pure, bounded grading helpers for selected CKS-001B requirements."""
import json
import shlex

from host_fixtures import DOCKERFILE


def dockerfile_checks(text):
    """Assess Q7 final state; line count cannot establish editing history."""
    lines = text.splitlines()
    instructions = []
    for line in lines:
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        fields = stripped.split(None, 1)
        instructions.append((fields[0].upper(), fields[1].strip() if len(fields) == 2 else ""))
    by_name = {}
    for name, value in instructions:
        by_name.setdefault(name, []).append(value)
    bases = by_name.get("FROM", [])
    base = False
    if len(bases) == 1:
        parts = bases[0].split()
        parts = [part for part in parts if not part.startswith("--platform=")]
        base = len(parts) in (1, 3) and (len(parts) == 1 or parts[1].upper() == "AS") and (
            parts[0] == "gcr.io/distroless/base" or
            parts[0].startswith(("gcr.io/distroless/base:", "gcr.io/distroless/base@sha256:")))
    copy_target = None
    copies = by_name.get("COPY", [])
    if len(copies) == 1 and not by_name.get("ADD"):
        text_copy = copies[0]
        try:
            tokens = shlex.split(text_copy)
            while tokens and tokens[0].startswith("--"):
                tokens.pop(0)
            if text_copy.lstrip().startswith("["):
                tokens = json.loads(text_copy)
            if (isinstance(tokens, list) and len(tokens) == 2 and all(isinstance(v, str) for v in tokens)
                    and tokens[0] in ("ledger", "./ledger") and tokens[1].startswith("/")):
                copy_target = tokens[1].rstrip("/")
                if tokens[1].endswith("/"):
                    copy_target += "/ledger"
        except (ValueError, TypeError, json.JSONDecodeError):
            pass
    user = by_name.get("USER", [])
    nonroot = len(user) == 1 and user[0].split(":", 1)[0] in ("nonroot", "65532")
    entrypoint = by_name.get("ENTRYPOINT", [])
    exec_form = False
    if len(entrypoint) == 1:
        try:
            argv = json.loads(entrypoint[0])
            exec_form = (isinstance(argv, list) and bool(argv) and
                         all(isinstance(arg, str) for arg in argv) and argv[0] == copy_target)
        except (ValueError, TypeError):
            pass
    no_shell = not any(name in by_name for name in ("RUN", "SHELL"))
    cmd = by_name.get("CMD", [])
    if cmd:
        try:
            parsed = json.loads(cmd[0])
            no_shell = no_shell and len(cmd) == 1 and isinstance(parsed, list) and all(
                isinstance(item, str) for item in parsed)
            if (isinstance(parsed, list) and parsed and isinstance(parsed[0], str) and
                    parsed[0].split("/")[-1] in ("sh", "bash", "ash", "dash")):
                no_shell = False
        except (ValueError, TypeError):
            no_shell = False
    return {
        "base": base,
        "minimal": bool(instructions) and no_shell and len(lines) <= len(DOCKERFILE.splitlines()),
        "binary": copy_target is not None,
        "nonroot": nonroot,
        "exec": exec_form,
    }


def seccomp_checks(profile):
    """Q10: exact default-allow behavior with unconditional errno denials."""
    if not isinstance(profile, dict):
        return {"default_allow": False, "debug_blocked": False, "other_allowed": False}
    syscalls = profile.get("syscalls", [])
    if not isinstance(syscalls, list):
        return {"default_allow": profile.get("defaultAction") == "SCMP_ACT_ALLOW",
                "debug_blocked": False, "other_allowed": False}
    desired = {"ptrace", "process_vm_readv"}
    blocked = set()
    conflicting = False
    others_allowed = True
    for rule in syscalls:
        if not isinstance(rule, dict) or not isinstance(rule.get("names"), list) or not all(
                isinstance(name, str) for name in rule["names"]):
            others_allowed = False
            continue
        names = set(rule["names"])
        action = rule.get("action")
        if action == "SCMP_ACT_ALLOW":
            conflicting = conflicting or bool(names & desired)
            continue
        errno = rule.get("errnoRet", 1)
        if (action == "SCMP_ACT_ERRNO" and not rule.get("args") and
                isinstance(errno, int) and not isinstance(errno, bool) and 1 <= errno <= 4095):
            blocked.update(names & desired)
            if names - desired:
                others_allowed = False
        else:
            others_allowed = False
    default_allow = profile.get("defaultAction") == "SCMP_ACT_ALLOW"
    return {"default_allow": default_allow,
            "debug_blocked": desired <= blocked and not conflicting,
            "other_allowed": default_allow and others_allowed}


def kubesec_report_valid(report):
    """A final report proves content, not which command generated it."""
    if not isinstance(report, list) or len(report) != 1 or not isinstance(report[0], dict):
        return False
    result = report[0]
    score = result.get("score")
    return (result.get("object") == "Pod/invoice-web.default" and result.get("valid") is True
            and isinstance(score, (int, float)) and not isinstance(score, bool) and score > 0
            and isinstance(result.get("message"), str) and result["message"].startswith("Passed"))
