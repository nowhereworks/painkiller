"""Restore exam-owned loopback addresses before starting the fixture service."""
from pathlib import Path
import subprocess

from network_setup import ensure_addresses


def command(args):
    return subprocess.run(args, text=True, capture_output=True, timeout=10, check=True)


if __name__ == "__main__":
    ensure_addresses(Path("/"), command)
