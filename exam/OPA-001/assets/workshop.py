#!/usr/bin/env python3
"""Initial files only. Invoked by preparation, never by capture."""
import json
import pathlib
import sys

ROOT = pathlib.Path(sys.argv[1])


def write(name, text):
    path = ROOT / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


for q in range(1, 17):
    (ROOT / f"q{q}").mkdir(parents=True, exist_ok=True)
for q in (1, 2, 3, 5, 8, 16):
    write(f"q{q}/policy.rego", f"package q{q}\n\ndefault allow := true\n")
write("q4/policy.rego", "package q4\n\nviolations := false\n")
write("q6/tests.rego", "package tenant_test\n\nimport data.tenant.allow\n\ntest_placeholder if { true }\n")
write("q7/adapter.py", "import json,sys\njson.load(sys.stdin)\nprint(json.dumps({'allow':True,'reason':'allow'}))\n")
write("q9/src/policy.rego", 'package release\n\ndefault allow := false\n')
write("q9/src/data.json", '{"release":{"readers":["alice","bob"]}}\n')
write("q10/config.yaml", """services:
  workshop:
    url: http://127.0.0.1:18888
bundles:
  broken:
    service: workshop
    resource: bundles/broken.tar.gz
    polling: {min_delay_seconds: 1, max_delay_seconds: 2}
status:
  service: workshop
  partition_name: q10
  console: true
""")
write("q9/config.yaml", """services:
  workshop:
    url: http://127.0.0.1:18888
bundles:
  release:
    service: workshop
    resource: bundles/q9.tar.gz
    polling: {min_delay_seconds: 1, max_delay_seconds: 2}
status:
  service: workshop
  partition_name: q9
  console: true
""")
write("q11/policy.rego", 'package logging\n\ndefault allow := false\nallow if input.action == "read"\n')
write("q11/mask.rego", "package system.log\n\nmask := []\n")
write("q11/config.yaml", """services:
  workshop:
    url: http://127.0.0.1:18888
decision_logs:
  service: workshop
  resource: logs/q11
  reporting:
    min_delay_seconds: 1
    max_delay_seconds: 2
""")
write("q13/audit.json", "{}\n")
write("q15/policy/main.rego", 'package main\n\ndeny contains "all resources forbidden" if { true }\n')
write("q15/public.json", '{"resource_changes":[{"type":"aws_s3_bucket","change":{"after":{"acl":"public-read"}}}]}\n')
write("q15/private.json", '{"resource_changes":[{"type":"aws_s3_bucket","change":{"after":{"acl":"private"}}}]}\n')
write("q15/lint.rego", 'package lint\n\ndefault allow = false\nallow = true if { input.action == "read" }\n')
inventory = {f"doc:{i}": {"tenant": "blue" if i % 2 else "green"} for i in range(4000)}
write("q16/data.json", json.dumps({"inventory": inventory, "records": [
    dict(id=k, **v) for k, v in inventory.items()
]}))
write("q16/input.json", '{"resource":"doc:3999","tenant":"blue","action":"read"}\n')
write("q2/data.json", json.dumps({
    "users": {"alice": {"tenant": "blue", "roles": ["reader"]},
              "bob": {"tenant": "green", "roles": ["reader"]},
              "eve": {"tenant": "blue", "roles": []}},
    "documents": {"doc:1": {"tenant": "blue"}, "doc:2": {"tenant": "green"}}
}))
write("q8/data.json", json.dumps({"relationships": {
    "alice": ["team:blue"], "team:blue": ["doc:7"], "doc:7": [],
    "bob": ["team:green"], "team:green": ["doc:8"], "doc:8": []
}}))
