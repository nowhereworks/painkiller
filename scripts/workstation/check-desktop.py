#!/usr/bin/env python3
"""Disposable-clone GUI smoke check; deliberately opens apps, never run on a template."""
import os
import socket
import struct
import subprocess
import tempfile
import time

os.environ["DISPLAY"] = ":1"


def receive(client, length):
    data = b""
    while len(data) < length:
        part = client.recv(length - len(data))
        if not part:
            raise RuntimeError("VNC disconnected during handshake")
        data += part
    return data


def connect():
    client = socket.create_connection(("127.0.0.1", 5901), timeout=10)
    assert receive(client, 12) == b"RFB 003.008\n"
    client.sendall(b"RFB 003.008\n")
    count = receive(client, 1)[0]
    assert receive(client, count) == b"\x01"
    client.sendall(b"\x01")
    assert receive(client, 4) == bytes(4)
    client.sendall(b"\x01")  # Shared persistent session.
    header = receive(client, 24)
    receive(client, struct.unpack(">I", header[20:24])[0])
    return client, struct.unpack(">HH", header[:4])


def wait_for(check, message, attempts=100):
    for _ in range(attempts):
        if check():
            return
        time.sleep(0.2)
    raise AssertionError(message)


client, initial_size = connect()
# Enable ExtendedDesktopSize and resize through RFB, not guest NIC/VM config.
client.sendall(struct.pack(">BBHi", 2, 0, 1, -308))
client.sendall(struct.pack(">BBHHBBIHHHHI", 251, 0, 1600, 900, 1, 0, 0, 0, 0, 1600, 900, 0))
wait_for(
    lambda: "1600x900" in subprocess.check_output(["xdpyinfo"], text=True),
    "VNC client resize did not change the virtual display",
)
client.close()
client, reconnected_size = connect()
assert reconnected_size == (1600, 900), "Reconnect did not preserve the display"
client.close()
print(f"VNC handshake / resize {initial_size} -> {reconnected_size} / reconnect: OK")

client, _ = connect()
clipboard = b"Painkiller VNC clipboard smoke"
client.sendall(struct.pack(">B3xI", 6, len(clipboard)) + clipboard)


def clipboard_matches():
    try:
        text = subprocess.check_output(
            ["xclip", "-selection", "clipboard", "-o"], stderr=subprocess.DEVNULL, timeout=3,
        )
        return text == clipboard
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired):
        return False


wait_for(clipboard_matches, "VNC clipboard did not reach the X11 clipboard")
print("VNC -> XFCE clipboard: OK")
owner = subprocess.Popen(
    ["xclip", "-selection", "clipboard", "-in", "-quiet"],
    stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
)
try:
    return_clipboard = b"Painkiller XFCE clipboard return"
    owner.stdin.write(return_clipboard)
    owner.stdin.close()
    for _ in range(5):
        message = receive(client, 1)[0]
        assert message == 3, f"Unexpected server message while waiting for clipboard: {message}"
        header = receive(client, 7)
        text = receive(client, struct.unpack(">I", header[3:])[0])
        if text == return_clipboard:
            break
    else:
        raise AssertionError("XFCE clipboard did not reach VNC")
    print("XFCE -> VNC clipboard: OK")
finally:
    owner.terminate()
    owner.wait(timeout=5)
    client.close()

with tempfile.TemporaryDirectory(prefix="painkiller-desktop-smoke.") as work:
    with open(work + "/apps.log", "w", encoding="utf-8") as log:
        terminal = subprocess.Popen(
            ["xfce4-terminal", "--disable-server", "--title=Painkiller desktop smoke"],
            stdout=log, stderr=log,
        )
        browser = subprocess.Popen(
            ["/snap/bin/firefox", "--no-remote", "--profile", work + "/firefox", "about:blank"],
            stdout=log, stderr=log,
        )
        try:
            def windows():
                return subprocess.check_output(["xwininfo", "-root", "-tree"], text=True)

            wait_for(
                lambda: "Painkiller desktop smoke" in windows() and "Firefox" in windows(),
                "Firefox and terminal windows did not render on the XFCE display",
                attempts=300,
            )
            print("Actual XFCE terminal and Firefox windows: OK")
        except Exception:
            with open(work + "/apps.log", encoding="utf-8") as handle:
                print(handle.read())
            raise
        finally:
            for app in (browser, terminal):
                app.terminate()
                try:
                    app.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    app.kill()
                    app.wait()
