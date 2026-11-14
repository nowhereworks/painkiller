#!/usr/bin/env python3
"""Loopback-only bundle distribution and bounded log/status receiver."""
import collections
import gzip
import http.server
import json
import pathlib
import threading

ROOT = pathlib.Path("/opt/opa-workshop")
EVENTS = collections.defaultdict(lambda: collections.deque(maxlen=100))
LOCK = threading.Lock()


class Handler(http.server.BaseHTTPRequestHandler):
    def reply(self, code, body, content_type="application/json"):
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/health":
            self.reply(200, b'{"ok":true}')
        elif self.path.startswith("/events/"):
            key = self.path.removeprefix("/events/")
            with LOCK:
                body = json.dumps(list(EVENTS[key])).encode()
            self.reply(200, body)
        elif self.path in ("/bundles/q9.tar.gz", "/bundles/stable.tar.gz", "/bundles/broken.tar.gz"):
            name = self.path.rsplit("/", 1)[1]
            path = ROOT / "q9/bundle.tar.gz" if name == "q9.tar.gz" else pathlib.Path(
                "/opt/painkiller/opa001/artifacts") / name
            try:
                self.reply(200, path.read_bytes(), "application/gzip")
            except FileNotFoundError:
                self.reply(404, b'{"error":"bundle absent"}')
        else:
            self.reply(404, b"{}")

    def do_POST(self):
        size = int(self.headers.get("Content-Length", "0"))
        if size > 2_000_000:
            self.reply(413, b"{}")
            return
        body = self.rfile.read(size)
        try:
            if self.headers.get("Content-Encoding") == "gzip" or body.startswith(b"\x1f\x8b"):
                body = gzip.decompress(body)
            value = json.loads(body)
            if self.path not in ("/status/q9", "/status/q10", "/logs/q11"):
                self.reply(404, b"{}")
                return
            with LOCK:
                EVENTS[self.path.lstrip("/")].extend(value if isinstance(value, list) else [value])
            self.reply(200, b"{}")
        except (ValueError, OSError):
            self.reply(400, b"{}")

    def log_message(self, *_):
        pass


if __name__ == "__main__":
    http.server.ThreadingHTTPServer(("127.0.0.1", 18888), Handler).serve_forever()
