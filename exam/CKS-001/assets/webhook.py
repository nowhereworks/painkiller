"""TLS ImageReview endpoint; deliberately not an AdmissionReview webhook."""
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import ssl


def allowed(image):
    if "@sha256:" in image:
        return True
    leaf = image.rsplit("/", 1)[-1]
    return ":" in leaf and leaf.rsplit(":", 1)[-1] != "latest"


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"image-gate\n")

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        ok = all(allowed(c["image"]) for c in request["spec"]["containers"])
        response = {"apiVersion": "imagepolicy.k8s.io/v1alpha1", "kind": "ImageReview",
                    "status": {"allowed": ok, "reason": "" if ok else "image-gate: latest or implicit latest is forbidden"}}
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(response).encode())


if __name__ == "__main__":
    server = HTTPServer(("0.0.0.0", 8443), Handler)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain("/tls/tls.crt", "/tls/tls.key")
    server.socket = context.wrap_socket(server.socket, server_side=True)
    server.serve_forever()
