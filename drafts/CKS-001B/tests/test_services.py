import base64
import io
import json
import subprocess
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "assets"))
import services
import services_ready


def archive(target, image):
    body = json.dumps([{"RepoTags": [image], "Config": "config.json", "Layers": []}]).encode()
    with tarfile.open(target, "w") as out:
        info = tarfile.TarInfo("manifest.json")
        info.size = len(body)
        out.addfile(info, io.BytesIO(body))


class ServicesTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir="/tmp/opencode")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.state = self.root / "var/lib/painkiller/cks001b"
        self.state.mkdir(parents=True)
        self.calls = []
        self.secret = None

    def kubectl(self, *args, data=None, timeout=30):
        self.calls.append(("kubectl", args))
        if args[:3] == ("get", "namespace", "cks001b-system"):
            return SimpleNamespace(stdout=json.dumps({"metadata": {"labels": {"painkiller-shell/exam": "CKS-001B"}}}))
        if args[:2] == ("apply", "-f"):
            self.secret = json.loads(data)
        return SimpleNamespace(stdout="{}")

    def command(self, args, *, data=None, timeout=30):
        self.calls.append((args[0], args))
        if args[0] == "skopeo":
            _, target, image = args[-1].split(":", 2)
            archive(Path(target), image)
        if args[0] == "bom":
            Path(args[args.index("--output") + 1]).write_text(json.dumps({
                "spdxVersion": "SPDX-2.3", "packages": [{"name": "curl"}]}))
        return SimpleNamespace(stdout="{}")

    def authorize(self):
        for name in ("authorized", "host-files"):
            (self.state / name).touch()
        (self.state / "portal-tls.crt").write_text("certificate")
        (self.state / "portal-tls.key").write_text("private key")

    def test_unmarked_cluster_and_guest_refused(self):
        with self.assertRaisesRegex(RuntimeError, "unauthorized"):
            services.setup(self.root)
        self.authorize()
        with patch.object(services, "k", return_value=SimpleNamespace(stdout='{"metadata":{"labels":{}}}')):
            with self.assertRaisesRegex(RuntimeError, "unmarked cluster"):
                services.setup(self.root)

    def test_setup_retry_and_readiness(self):
        self.authorize()
        with patch.object(services, "k", self.kubectl), patch.object(services, "command", self.command):
            services.setup(self.root)
            self.assertTrue((self.state / "services-complete").exists())
            self.assertEqual(len([call for call in self.calls if call[0] == "skopeo"]), 3)
            self.assertEqual(len([call for call in self.calls if call[0] == "istioctl"]), 1)
            self.assertEqual(self.secret["type"], "kubernetes.io/tls")
            before = len(self.calls)
            services.setup(self.root)
            self.assertEqual(len(self.calls), before + 1)  # only the ownership check

        def k(*args, **kwargs):
            if "istiod" in args:
                data = {"metadata": {"generation": 1}, "spec": {"replicas": 1},
                        "status": {"observedGeneration": 1, "updatedReplicas": 1, "readyReplicas": 1}}
            elif "istio-sidecar-injector" in args:
                data = {"webhooks": [{"name": "injector"}]}
            elif "endpointslices" in args:
                data = {"items": [{"endpoints": [{"conditions": {"ready": True}}]}]}
            elif "private-mesh" in args and "namespace" in args:
                data = {"metadata": {"labels": {}}}
            elif "deployments" in args:
                data = {"items": [
                    {"metadata": {"name": name, "generation": 1}, "spec": {"replicas": 1, "template": {
                        "metadata": {}, "spec": {"containers": [{"name": name}]}}},
                     "status": {"observedGeneration": 1, "updatedReplicas": 1, "readyReplicas": 1}}
                    for name in ("north", "south")]}
            elif "pods" in args:
                data = {"items": [{"metadata": {"name": name, "labels": {"app": name}},
                                    "spec": {"containers": [{"name": name}]}}
                                   for name in ("north", "south")]}
            elif "peerauthentications" in args:
                data = {"items": []}
            elif "portal-cert" in args:
                data = self.secret
            else:
                self.fail(f"unexpected kubectl request: {args}")
            return SimpleNamespace(stdout=json.dumps(data))

        with patch.object(services_ready, "k", k):
            self.assertEqual(services_ready.inspect(self.root), [])
            self.secret["data"]["tls.crt"] = base64.b64encode(b"wrong").decode()
            self.assertTrue(any("TLS Secret" in msg for msg in services_ready.inspect(self.root)))
            archive(self.root / "root/ImageTarballs" / services.archive_name(services.IMAGES[0]), "nginx:latest")
            self.assertTrue(any("wrong image" in msg for msg in services_ready.inspect(self.root)))

    def test_failed_export_does_not_publish_archive_or_complete(self):
        self.authorize()

        def fails(args, **kwargs):
            if args[0] == "skopeo":
                raise subprocess.CalledProcessError(1, args)
            return self.command(args, **kwargs)

        with patch.object(services, "k", self.kubectl), patch.object(services, "command", fails):
            with self.assertRaises(subprocess.CalledProcessError):
                services.setup(self.root)
        self.assertTrue((self.state / "istio-installed").exists())
        self.assertFalse((self.state / "services-complete").exists())
        self.assertFalse((self.root / "root/ImageTarballs" / services.archive_name(services.IMAGES[0])).exists())
        with patch.object(services, "k", self.kubectl), patch.object(services, "command", self.command):
            services.setup(self.root)
        self.assertEqual(len([call for call in self.calls if call[0] == "istioctl"]), 1)

    def test_api_error_is_not_reported_as_missing_answer(self):
        with patch.object(services_ready, "k", side_effect=subprocess.CalledProcessError(1, ["kubectl"])):
            with self.assertRaises(subprocess.CalledProcessError):
                services_ready.inspect(self.root)


if __name__ == "__main__":
    unittest.main()
