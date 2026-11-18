"""Lab-local TCP/TLS endpoints for egress probes; no external LAN addresses."""
import selectors
import socket
import ssl

ADDRESSES = ("198.18.20.10", "198.18.30.10", "198.18.40.10", "198.18.50.10")
PORTS = (8081, 443, 9443)
BODY = b"cks001b-network-fixture\n"
RESPONSE = b"HTTP/1.1 200 OK\r\nContent-Length: " + str(len(BODY)).encode() + b"\r\nConnection: close\r\n\r\n" + BODY


def serve(addresses, ports, cert, key, *, stop=None, tls_ports=(443, 9443)):
    """A bounded timeout per connection; stop is only supplied by offline tests."""
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(cert, key)
    selector = selectors.DefaultSelector()
    listeners = []
    try:
        for address in addresses:
            for port in ports:
                listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
                listener.bind((address, port))
                listener.listen(16)
                listeners.append(listener)
                selector.register(listener, selectors.EVENT_READ, port in tls_ports)
        while stop is None or not stop.is_set():
            for event, _ in selector.select(timeout=0.2):
                conn, _ = event.fileobj.accept()
                try:
                    conn.settimeout(2)
                    if event.data:
                        conn = context.wrap_socket(conn, server_side=True)
                    conn.recv(4096)
                    conn.sendall(RESPONSE)
                except (OSError, ssl.SSLError):
                    pass
                finally:
                    conn.close()
    finally:
        selector.close()
        for listener in listeners:
            listener.close()


if __name__ == "__main__":
    serve(ADDRESSES, PORTS, "/var/lib/painkiller/cks001b/network-tls.crt",
          "/var/lib/painkiller/cks001b/network-tls.key")
