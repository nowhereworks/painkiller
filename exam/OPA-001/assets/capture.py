#!/usr/bin/env python3
"""Bounded observational grading. No candidate files or API objects are written."""
import copy
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tarfile
import time
import urllib.error
import urllib.request
import uuid

ROOT = pathlib.Path(os.environ.get("OPA001_ROOT", "/opt/opa-workshop"))
ASSETS = pathlib.Path(os.environ.get("OPA001_ASSETS", "/opt/painkiller/opa001"))


class InfrastructureError(Exception):
    pass


def run(args, value=None, timeout=7):
    try:
        return subprocess.run(
            [str(a) for a in args], input=json.dumps(value) if value is not None else None,
            capture_output=True, text=True, timeout=timeout,
            env=dict(os.environ, PYTHONDONTWRITEBYTECODE="1"),
        )
    except FileNotFoundError as exc:
        raise InfrastructureError(f"required executable unavailable: {args[0]}") from exc


def load(path):
    try:
        return json.loads(path.read_text())
    except (FileNotFoundError, ValueError):
        return None


def evaluate(q, query, value, extra=(), profile=False):
    directory = ROOT / q
    args = ["opa", "eval", "--format=json", "--stdin-input", "--data", directory / "policy.rego"]
    if (directory / "data.json").is_file():
        args += ["--data", directory / "data.json"]
    args += list(extra)
    if profile:
        args += ["--profile", "--profile-limit=100"]
    result = run(args + [query], value)
    if result.returncode:
        return None
    return json.loads(result.stdout)


def decision(document):
    if not document or not document.get("result"):
        return None
    return document["result"][0]["expressions"][0]["value"]


def same(actual, expected):
    return type(actual) is type(expected) and actual == expected


def cases(q, values, query=None):
    failures = []
    for label, value, expected in values:
        actual = decision(evaluate(q, query or f"data.{q}.allow", value))
        if not same(actual, expected):
            failures.append(f"{label}: expected {json.dumps(expected)}, got {json.dumps(actual)}")
    return failures


def http(path, value=None, supporting=False):
    url = ("http://127.0.0.1:18888" + path) if path.startswith("/") else path
    request = urllib.request.Request(url, data=json.dumps(value).encode() if value is not None else None,
                                     headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(request, timeout=2) as response:
            return json.load(response)
    except (urllib.error.URLError, TimeoutError, ValueError) as exc:
        if supporting:
            raise InfrastructureError("workshop receiver unavailable or invalid") from exc
        return None


def kget(kind, name, namespace=None):
    args = ["kubectl", "--request-timeout=4s", "get", kind, name, "-o", "json"]
    if namespace:
        args += ["-n", namespace]
    result = run(args)
    if result.returncode:
        if result.stderr.startswith("Error from server (NotFound):"):
            return None
        # Do not expose raw NotFound from supporting transports to the generic grader.
        raise InfrastructureError("Kubernetes observation failed (API/access/transport)")
    return json.loads(result.stdout)


def constraint(kind, name, namespaces, labels=None):
    obj = kget(kind, name)
    if not obj:
        return None, ["required Constraint is missing"]
    spec = obj.get("spec", {})
    match = spec.get("match", {})
    ok = (
        spec.get("enforcementAction", "deny") == "deny"
        and sorted(match.get("namespaces", [])) == sorted(namespaces)
        and match.get("kinds") == [{"apiGroups": [""], "kinds": ["ConfigMap"]}]
        and not match.get("excludedNamespaces")
        and not any(match.get(k) for k in ("labelSelector", "namespaceSelector", "name"))
        and match.get("scope", "*") in ("*", "Namespaced")
    )
    if labels is not None:
        ok = ok and spec.get("parameters", {}).get("labels") == labels
    return obj, [] if ok else ["Constraint must retain exact namespace/kind scope, parameters, and deny enforcement"]


def admission(namespace, labels, expected, constraint_name):
    obj = {"apiVersion": "v1", "kind": "ConfigMap",
           "metadata": {"name": "opa001-probe-" + uuid.uuid4().hex[:12],
                        "namespace": namespace, "labels": labels}}
    result = run(["kubectl", "--request-timeout=5s", "create", "--dry-run=server", "-f", "-", "-o", "json"], obj)
    if result.returncode and not (
        'admission webhook "validation.gatekeeper.sh" denied' in result.stderr
        and f"[{constraint_name}]" in result.stderr
    ):
        raise InfrastructureError("admission probe failed without the expected policy denial")
    return [] if (result.returncode == 0) == expected else [
        f"{namespace} labels={labels}: expected {'accept' if expected else 'policy rejection'}"
    ]


def grade(q):
    if q == "q1":
        return cases(q, [
            ("finance read", {"user": {"department": "finance"}, "action": "read"}, True),
            ("other department", {"user": {"department": "legal"}, "action": "read"}, False),
            ("write", {"user": {"department": "finance"}, "action": "write"}, False),
            ("missing user", {"action": "read"}, False), ("empty input", {}, False),
            ("wrong type", {"user": {"department": ["finance"]}, "action": "read"}, False),
        ])
    if q == "q2":
        rows = [
            ("alice own", "alice", "doc:1", "read", True),
            ("alice cross-tenant", "alice", "doc:2", "read", False),
            ("bob own", "bob", "doc:2", "read", True),
            ("bob cross-tenant", "bob", "doc:1", "read", False),
            ("eve no role", "eve", "doc:1", "read", False),
            ("unknown principal", "mallory", "doc:1", "read", False),
            ("unknown document", "alice", "doc:404", "read", False),
            ("write", "alice", "doc:1", "write", False),
        ]
        values = [(label, {"user": u, "resource": r, "action": a}, expected)
                  for label, u, r, a, expected in rows]
        values += [("forged claims", {"user": "alice", "resource": "doc:2", "action": "read",
                                     "tenant": "green", "resource_tenant": "green", "roles": ["reader"]}, False),
                   ("forged resource owner", {"user": "alice", "resource": "doc:2", "action": "read",
                                              "tenant": "blue", "resource_tenant": "blue"}, False),
                   ("forged role grant", {"user": "eve", "resource": "doc:1", "action": "read",
                                         "roles": ["reader"]}, False),
                   ("ignore conflicting caller claims", {"user": "alice", "resource": "doc:1", "action": "read",
                                                         "tenant": "green", "resource_tenant": "green", "roles": []}, True),
                   ("empty input", {}, False)]
        return cases(q, values)
    if q == "q3":
        base = {"action": "read", "subject": {"department": "finance", "roles": ["reader"], "suspended": False},
                "resource": {"department": "finance"}}
        values = [("valid reader", base, True), ("empty input", {}, False)]
        for label, key, val in [
            ("suspended", "suspended", True), ("untyped suspension", "suspended", "false"),
            ("no role", "roles", []), ("wrong role type", "roles", "reader"),
            ("other department", "department", "legal"), ("empty department", "department", ""),
        ]:
            item = copy.deepcopy(base)
            item["subject"][key] = val
            values.append((label, item, False))
        missing = copy.deepcopy(base)
        del missing["subject"]["suspended"]
        values.append(("missing suspension", missing, False))
        item = copy.deepcopy(base)
        item["action"] = "write"
        values.append(("write", item, False))
        item = copy.deepcopy(base)
        item["subject"]["department"] = item["resource"]["department"] = 5
        values.append(("non-string departments", item, False))
        return cases(q, values)
    if q == "q4":
        values = [
            ("no violations", {"owner": "alice", "public": False}, []),
            ("both violations", {"public": True}, ["missing-owner", "public-access"]),
            ("public only", {"owner": "alice", "public": True}, ["public-access"]),
            ("empty owner", {"owner": ""}, ["missing-owner"]),
            ("wrong owner type", {"owner": 9}, ["missing-owner"]),
            ("wrong public type", {"owner": "alice", "public": "true"}, []),
        ]
        failures = []
        for label, value, expected in values:
            actual = decision(evaluate(q, "data.q4.violations", value))
            if not isinstance(actual, list) or sorted(actual) != sorted(expected):
                failures.append(f"{label}: expected set {expected}, got {actual}")
        return failures
    if q == "q5":
        return cases(q, [
            ("all accepted", {"user": {"admin": False}, "approvals": [{"accepted": True}, {"accepted": True}]}, True),
            ("mixed approvals", {"user": {"admin": False}, "approvals": [{"accepted": True}, {"accepted": False}]}, False),
            ("empty approvals", {"user": {"admin": False}, "approvals": []}, False),
            ("missing admin", {"user": {}, "approvals": [{"accepted": True}]}, False),
            ("admin true", {"user": {"admin": True}, "approvals": [{"accepted": True}]}, False),
            ("wrong admin type", {"user": {"admin": "false"}, "approvals": [{"accepted": True}]}, False),
            ("missing acceptance", {"user": {"admin": False}, "approvals": [{}]}, False),
            ("wrong approvals type", {"user": {"admin": False}, "approvals": {}}, False),
            ("empty input", {}, False),
        ])
    if q == "q6":
        test = ROOT / q / "tests.rego"
        if not test.is_file():
            return ["tests.rego is missing"]
        result = run(["opa", "test", "--format=json", ASSETS / "testing/good.rego", test])
        if result.returncode:
            return ["candidate tests must compile and pass the correct policy"]
        tests = json.loads(result.stdout)
        if len(tests) < 4 or any(t.get("skip") or t.get("fail") or t.get("error") for t in tests):
            return ["at least four passing, non-skipped test rules are required"]
        failures = []
        for variant in ("role-only", "tenant-only", "allow-all", "deny-all", "undefined"):
            result = run(["opa", "test", "--format=json", ASSETS / f"testing/{variant}.rego", test])
            try:
                results = json.loads(result.stdout)
                caught = result.returncode != 0 and isinstance(results, list) and any(t.get("fail") for t in results)
            except ValueError:
                caught = False
            if not caught:
                failures.append(f"tests did not detect {variant} regression with a failing assertion")
        return failures
    if q == "q7":
        path = ROOT / q / "adapter.py"
        if not path.is_file():
            return ["adapter.py is missing"]
        observations = [
            ({"status": 200, "body": {"result": True}}, True, "allow"),
            ({"status": 200, "body": {"result": False}}, False, "deny"),
            ({"status": 200, "body": {}}, False, "undefined"),
            ({"status": 200, "body": {"result": "true"}}, False, "invalid"),
            ({"status": 200, "body": {"result": 1}}, False, "invalid"),
            ({"status": 200, "body": {"result": None}}, False, "invalid"),
            ({"status": 200, "body": []}, False, "invalid"),
            ({"status": 500, "body": {"result": True}}, False, "error"),
            ({"error": "timeout", "status": 200, "body": {"result": True}}, False, "timeout"),
            ({"error": "connection"}, False, "error"), ({}, False, "error"),
        ]
        failures = []
        for observation, allow, reason in observations:
            result = run(["python3", "-B", path], observation, timeout=2)
            try:
                actual = json.loads(result.stdout) if not result.returncode else None
            except ValueError:
                actual = None
            if not isinstance(actual, dict) or not same(actual.get("allow"), allow) or actual.get("reason") != reason:
                failures.append(f"observation {observation}: expected allow={allow}, reason={reason}")
        return failures
    if q == "q8":
        return cases(q, [
            ("alice path", {"user": "alice", "resource": "doc:7", "action": "read"}, True),
            ("bob path", {"user": "bob", "resource": "doc:8", "action": "read"}, True),
            ("cross-team", {"user": "alice", "resource": "doc:8", "action": "read"}, False),
            ("team not document", {"user": "alice", "resource": "team:blue", "action": "read"}, False),
            ("untrusted principal", {"user": "team:blue", "resource": "doc:7", "action": "read"}, False),
            ("unknown", {"user": "mallory", "resource": "doc:7", "action": "read"}, False),
            ("write", {"user": "alice", "resource": "doc:7", "action": "write"}, False),
            ("empty input", {}, False),
        ])
    if q in ("q9", "q10"):
        import yaml
        http("/health", supporting=True)
        config_path = ROOT / q / "config.yaml"
        if not config_path.is_file():
            return ["OPA config.yaml is missing"]
        try:
            config = yaml.safe_load(config_path.read_text()) or {}
        except yaml.YAMLError:
            return ["OPA config.yaml must be valid YAML"]
        name, revision, package, port = ("release", "release-1", "release", 18189) if q == "q9" else (
            "stable", "known-good", "recovery", 18190)
        bundles = config.get("bundles", {})
        if set(bundles) != {name} or bundles[name].get("resource") != (
            "bundles/q9.tar.gz" if q == "q9" else "bundles/stable.tar.gz"
        ):
            return ["configure exactly the requested bundle name and resource"]
        if q == "q9":
            try:
                with tarfile.open(ROOT / q / "bundle.tar.gz") as archive:
                    manifests = [m for m in archive.getmembers() if m.name.lstrip("/") == ".manifest"]
                    manifest = json.load(archive.extractfile(manifests[0]))
                if manifest.get("revision") != revision or manifest.get("roots") != ["release"] or manifest.get("rego_version") != 1:
                    return ["bundle manifest must declare release-1, root release, and rego_version 1"]
            except (OSError, ValueError, IndexError, tarfile.TarError):
                return ["bundle.tar.gz is absent or not a readable OPA bundle"]
        events = http(f"/events/status/{q}", supporting=True)
        active = events[-1].get("bundles", {}).get(name, {}) if events else {}
        if active.get("active_revision") != revision or active.get("code") or active.get("errors"):
            return ["latest status must report the intended active revision without activation errors"]
        values = [
            ({"action": "read", "user": "alice"}, True),
            ({"action": "write", "user": "alice"}, False),
            ({}, False),
        ]
        if q == "q9":
            values += [({"action": "read", "user": "bob"}, True),
                       ({"action": "read", "user": "eve"}, False)]
        failures = []
        for value, expected in values:
            response = http(f"http://127.0.0.1:{port}/v1/data/{package}/allow", {"input": value})
            if not isinstance(response, dict) or not same(response.get("result"), expected):
                failures.append(f"active {package}.allow must return {expected} for {value}")
        return failures
    if q == "q11":
        http("/health", supporting=True)
        token = "request-" + uuid.uuid4().hex
        secret = "fake-credential-" + uuid.uuid4().hex
        value = {"action": "read", "request_id": token, "authorization": "Bearer " + secret, "password": secret}
        response = http("http://127.0.0.1:18191/v1/data/logging/allow", {"input": value})
        if not response or response.get("result") is not True:
            return ["logging.allow must still allow read on the running service"]
        for _ in range(12):
            records = http("/events/logs/q11", supporting=True)
            matching = [r for r in records if r.get("input", {}).get("request_id") == token]
            if matching:
                record = matching[-1]
                inp = record.get("input", {})
                return [] if (
                    "authorization" not in inp and "password" not in inp
                    and inp.get("action") == "read" and record.get("result") is True
                    and record.get("decision_id") and record.get("path") == "logging/allow"
                    and secret not in json.dumps(record)
                ) else ["exported event must remove both secrets but retain action, request_id, result, path, and decision_id"]
            time.sleep(0.25)
        return ["no fresh correlated decision log arrived; retain enabled export to the workshop receiver"]
    if q == "q12":
        template = kget("constrainttemplate", "opalabels")
        if not template:
            return ["ConstraintTemplate opalabels is missing"]
        spec = template.get("spec", {})
        schema = spec.get("crd", {}).get("spec", {}).get("validation", {}).get("openAPIV3Schema", {})
        target = spec.get("targets", [{}])[0]
        code = target.get("code", [{}])[0]
        if (schema.get("type") != "object" or
            schema.get("properties", {}).get("labels", {}) != {"type": "array", "items": {"type": "string"}} or
            code.get("engine") != "Rego" or code.get("source", {}).get("version") != "v1" or target.get("rego")):
            return ["template must use the structural labels schema and explicit Rego v1 code engine"]
        _, failures = constraint("opalabels", "require-team-owner", ["opa-labels"], ["team", "owner"])
        if failures:
            return failures
        for labels, accepted in [({"team": "blue", "owner": "alice"}, True),
                                 ({"team": "blue"}, False), ({"owner": "alice"}, False),
                                 ({"team": "", "owner": "alice"}, False),
                                 ({"team": "blue", "owner": ""}, False)]:
            failures += admission("opa-labels", labels, accepted, "require-team-owner")
        return failures
    if q == "q13":
        obj, failures = constraint("opaauditlabels", "audit-env", ["opa-audit"], ["env"])
        if failures:
            return failures
        if load(ROOT / q / "audit.json") != {"namespace": "opa-audit", "name": "legacy"}:
            failures.append("audit.json must identify the legacy ConfigMap")
        legacy = kget("configmap", "legacy", "opa-audit")
        if not legacy or legacy.get("metadata", {}).get("labels", {}).get("env"):
            failures.append("preserve the existing unlabeled legacy ConfigMap")
        violations = obj.get("status", {}).get("violations", [])
        if not any(v.get("name") == "legacy" and v.get("namespace") == "opa-audit" and v.get("kind") == "ConfigMap"
                   for v in violations):
            failures.append("live audit status must report legacy; wait for the next audit")
        failures += admission("opa-audit", {"env": "dev"}, True, "audit-env")
        failures += admission("opa-audit", {}, False, "audit-env")
        return failures
    if q == "q14":
        config = kget("config.config.gatekeeper.sh", "config", "gatekeeper-system")
        sync = (config or {}).get("spec", {}).get("sync", {}).get("syncOnly", [])
        if {"group": "", "version": "v1", "kind": "Namespace"} not in sync:
            return ["Gatekeeper Config must synchronize core/v1 Namespace"]
        _, failures = constraint("opanamespaceteam", "namespace-team", ["opa-inventory-blue", "opa-inventory-green"])
        if failures:
            return failures
        import yaml
        supplied = list(yaml.safe_load_all((ASSETS / "gatekeeper-fixtures.yaml").read_text()))[-1]
        # Kubernetes defaults this field when admitting a v1 ConstraintTemplate.
        supplied["spec"]["crd"]["spec"]["validation"].setdefault("legacySchema", False)
        actual = kget("constrainttemplate", "opanamespaceteam")
        if not actual or actual.get("spec") != supplied["spec"]:
            return ["preserve the supplied inventory-based template"]
        for team in ("blue", "green"):
            ns = "opa-inventory-" + team
            namespace = kget("namespace", ns)
            if not namespace or namespace["metadata"].get("labels", {}).get("team") != team:
                failures.append(f"preserve {ns} team label")
            for label, accepted in [(team, True), ("other", False), ("", False)]:
                failures += admission(ns, {"team": label}, accepted, "namespace-team")
        return failures
    if q == "q15":
        directory = ROOT / q
        policy = directory / "policy/main.rego"
        if not policy.is_file() or not (directory / "lint.rego").is_file():
            return ["Conftest policy/main.rego and lint.rego are required"]
        failures = []
        for typ, acl, denied in [
            ("aws_s3_bucket", "public-read", True), ("aws_s3_bucket", "public-read-write", True),
            ("aws_s3_bucket", "private", False), ("aws_instance", "public-read", False),
        ]:
            value = {"resource_changes": [{"type": typ, "change": {"after": {"acl": acl}}}]}
            result = run(["conftest", "test", "--policy", directory / "policy", "--output", "json", "-"], value)
            try:
                reports = json.loads(result.stdout)
                findings = sum(len(r.get("failures", [])) + len(r.get("exceptions", [])) for r in reports)
                correct = (findings > 0) == denied and result.returncode == (1 if denied else 0)
            except (ValueError, TypeError):
                correct = False
            if not correct:
                failures.append(f"Conftest must {'reject' if denied else 'accept'} {typ} ACL {acl}")
        empty = run(["conftest", "test", "--policy", directory / "policy", "-"], {"resource_changes": []})
        if empty.returncode:
            failures.append("empty plan must pass Conftest")
        result = run(["regal", "lint", "--disable-all", "--enable", "use-assignment-operator", directory / "lint.rego"])
        if result.returncode:
            failures.append("selected Regal assignment-operator rule must pass")
        for value, expected in [({"action": "read"}, True), ({"action": "write"}, False), ({}, False)]:
            result = run(["opa", "eval", "--format=json", "--stdin-input", "--data", directory / "lint.rego", "data.lint.allow"], value)
            actual = decision(json.loads(result.stdout)) if result.returncode == 0 else None
            if not same(actual, expected):
                failures.append(f"lint policy decision changed for {value}")
        return failures
    if q == "q16":
        if not (ROOT / q / "data.json").is_file():
            return ["preserve the supplied representative data.json"]
        values = [
            ("representative allow", {"resource": "doc:3999", "tenant": "blue", "action": "read"}, True),
            ("other tenant", {"resource": "doc:3999", "tenant": "green", "action": "read"}, False),
            ("green allow", {"resource": "doc:2", "tenant": "green", "action": "read"}, True),
            ("write", {"resource": "doc:3999", "tenant": "blue", "action": "write"}, False),
            ("missing resource", {"resource": "doc:4040", "tenant": "blue", "action": "read"}, False),
            ("missing tenant", {"resource": "doc:3999", "action": "read"}, False),
            ("empty input", {}, False),
        ]
        failures = cases(q, values)
        profile = load(ROOT / q / "profile.json")
        bench = load(ROOT / q / "bench.json")
        if not profile or decision(profile) is not True or not profile.get("profile"):
            failures.append("save a successful representative opa eval --profile JSON artifact")
        if not isinstance(bench, dict) or not all(
            isinstance(bench.get(key), (int, float)) and bench[key] > 0 for key in ("N", "T", "MemAllocs")
        ):
            failures.append("save an opa bench JSON artifact")
        candidate = evaluate(q, "data.q16.allow", values[0][1], profile=True)
        result = run(["opa", "eval", "--format=json", "--profile", "--profile-limit=100", "--stdin-input",
                      "--data", ASSETS / "testing/slow.rego", "--data", ROOT / q / "data.json",
                      "data.q16.allow"], values[0][1])
        if result.returncode:
            raise InfrastructureError("trusted baseline profile failed")
        baseline = json.loads(result.stdout)
        work = lambda doc: sum(row.get("num_eval", 0) for row in (doc or {}).get("profile", []))
        if not candidate or not work(candidate) or work(candidate) >= work(baseline):
            failures.append("fresh representative profile must reduce expression evaluations versus the scanning baseline")
        return failures
    raise ValueError("unknown question")


def main():
    if len(sys.argv) != 2 or sys.argv[1] not in {f"q{i}" for i in range(1, 17)}:
        sys.exit("Usage: capture.py q1..q16")
    if not shutil.which("opa"):
        sys.exit("capture infrastructure error: pinned opa executable is unavailable")
    try:
        failures = grade(sys.argv[1])
    except InfrastructureError as exc:
        sys.exit(f"capture infrastructure error: {exc}")
    except (subprocess.TimeoutExpired, FileNotFoundError, ValueError, TypeError, KeyError, AttributeError, IndexError) as exc:
        failures = [f"answer is missing, malformed, or did not complete: {type(exc).__name__}"]
    print(json.dumps({"ok": not failures, "failures": failures}))


if __name__ == "__main__":
    main()
