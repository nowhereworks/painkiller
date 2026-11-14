#!/usr/bin/env python3
"""Root-only guest setup, called once on a fresh marked exam."""
import io
import json
import pathlib
import subprocess
import sys
import tarfile

import yaml

assets = pathlib.Path("/opt/painkiller/opa001")
user = sys.argv[1]
artifacts = assets / "artifacts"
artifacts.mkdir(exist_ok=True)
for name, module in [
    ("stable", 'package recovery\n\ndefault allow := false\nallow if input.action == "read"\n'),
    ("broken", "package recovery\n\nallow if {\n"),
]:
    with tarfile.open(artifacts / f"{name}.tar.gz", "w:gz") as archive:
        for path, text in {
            ".manifest": json.dumps({"revision": "known-good" if name == "stable" else "broken",
                                     "roots": ["recovery"], "rego_version": 1}),
            "policy.rego": module,
        }.items():
            content = text.encode()
            item = tarfile.TarInfo(path)
            item.size = len(content)
            item.mode = 0o644
            archive.addfile(item, io.BytesIO(content))
manifest = list(yaml.safe_load_all((assets / "gatekeeper-upstream.yaml").read_text()))
for item in manifest:
    if item.get("kind") != "Deployment":
        continue
    item["spec"]["replicas"] = 1
    if item["metadata"]["name"] == "gatekeeper-audit":
        args = item["spec"]["template"]["spec"]["containers"][0]["args"]
        args[:] = [a for a in args if not a.startswith("--audit-interval=")]
        args.append("--audit-interval=10")
(assets / "gatekeeper-single.yaml").write_text(yaml.safe_dump_all(manifest))
units = pathlib.Path("/etc/systemd/system")
commands = {"receiver": "/usr/bin/python3 -B /opt/painkiller/opa001/service.py"}
for number in (9, 10, 11):
    directory = f"/opt/opa-workshop/q{number}"
    command = f"/usr/local/bin/opa run --server --addr=127.0.0.1:{18180 + number} --config-file={directory}/config.yaml"
    if number == 11:
        command += f" {directory}/policy.rego {directory}/mask.rego"
    commands[f"q{number}"] = command
for name, command in commands.items():
    (units / f"opa001-{name}.service").write_text(f"""[Unit]
Description=OPA-001 isolated {name} fixture
After=network.target
[Service]
User={user}
ExecStart={command}
Restart=on-failure
RestartSec=2
NoNewPrivileges=true
[Install]
WantedBy=multi-user.target
""")
subprocess.run(["systemctl", "daemon-reload"], check=True)
subprocess.run(["systemctl", "enable", "--now", *[f"opa001-{name}" for name in commands]], check=True)
