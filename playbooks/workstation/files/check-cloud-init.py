#!/usr/bin/env python3
"""Accept completed cloud-init with only Proxmox's known user-schema deprecation."""
import json
import subprocess


def check(result):
    if result.get("status") != "done" or result.get("errors"):
        raise ValueError("cloud-init has not completed successfully")
    for kind, messages in result.get("recoverable_errors", {}).items():
        if kind != "DEPRECATED" or not isinstance(messages, list):
            raise ValueError(f"Unexpected cloud-init warning: {kind}: {messages}")
        for message in messages:
            if not message.startswith("'user' of type string is deprecated in 22.2 "):
                raise ValueError(f"Unexpected cloud-init deprecation: {message}")


if __name__ == "__main__":
    process = subprocess.run(
        ["cloud-init", "status", "--format", "json"], capture_output=True, text=True, check=False,
    )
    if process.returncode not in (0, 2):
        raise SystemExit(process.stderr or process.stdout or "cloud-init failed")
    check(json.loads(process.stdout))
    print("cloud-init: done (only the known Proxmox user-schema deprecation is permitted)")
