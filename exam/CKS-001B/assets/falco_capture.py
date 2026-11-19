"""Observe local rules and fresh real exec alerts; never generate grading traffic."""
import hashlib
import json
from pathlib import Path
import subprocess

import yaml

from capture_files import read_file
from host_fixtures import path

OUTPUT = "MINER_NOTICE: %evt.time,%container.name,%proc.name"
TAGS = {"container", "crypto_mining", "mitre_execution"}
MINERS = {"xmrig", "minerd", "cpuminer"}


def rule_fields(rule, detected):
    if not isinstance(rule, dict) or not isinstance(rule.get("condition"), str):
        return {"processes": False, "priority": False, "output": False}
    # Real Falco alerts provide behavioral evidence instead of matching one
    # particular expression spelling or requiring a particular rule title.
    processes = MINERS <= detected
    return {"processes": processes,
            "priority": processes and str(rule.get("priority", "")).upper() == "CRITICAL",
            "output": processes and rule.get("output") == OUTPUT and TAGS <= set(rule.get("tags") or [])}


def capture(root=Path("/")):
    state = path(root, "/var/lib/painkiller/cks001b")
    try:
        rules = yaml.safe_load(read_file(path(root, "/etc/falco/falco_rules.local.yaml"))) or []
    except yaml.YAMLError:
        rules = []
    if not isinstance(rules, list):
        rules = []
    service = subprocess.run(["systemctl", "is-active", "--quiet", "falco"], capture_output=True, timeout=3)
    logs = subprocess.run(["journalctl", "-u", "falco", "--since=-20s", "-n", "150", "-o", "cat", "--no-pager"],
                          text=True, capture_output=True, check=True, timeout=4).stdout
    # Falco runs with json_output=true so the selected rule and process cannot
    # be confused with unrelated service messages or older rule executions.
    events = []
    for line in logs.splitlines():
        try:
            item = json.loads(line)
            if isinstance(item, dict):
                events.append(item)
        except ValueError:
            pass
    best = {"processes": False, "priority": False, "output": False}
    for rule in rules:
        if not isinstance(rule, dict) or not rule.get("rule") or rule.get("enabled") is False:
            continue
        detected = {e.get("output_fields", {}).get("proc.name") for e in events
                    if e.get("rule") == rule["rule"] and
                    e.get("output_fields", {}).get("container.name") not in (None, "", "host", "<NA>")}
        fields = rule_fields(rule, detected)
        if sum(fields.values()) > sum(best.values()):
            best = fields
    defaults = [path(root, "/etc/falco/" + name) for name in
                ("falco_rules.yaml", "falco-incubating_rules.yaml", "falco-sandbox_rules.yaml")]
    expected = read_file(state / "falco-default.sha256").strip()
    unchanged = all(p.is_file() for p in defaults) and hashlib.sha256(
        b"".join(p.read_bytes() for p in defaults)).hexdigest() == expected
    best["active"] = best["processes"] and service.returncode == 0 and unchanged
    messages = {"processes": "Detect real execution of xmrig, minerd and cpuminer in the suspect container.",
                "priority": "Set the detection rule priority to CRITICAL.",
                "output": "Set the requested output format and tags.",
                "active": "Keep Falco active with the local rule loaded and default rules unchanged."}
    best["failures"] = [message for key, message in messages.items() if not best[key]]
    return best
