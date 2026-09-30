"""MITM CA certificate helpers for UnifAI Guard."""

from __future__ import annotations

import os

from guard_platform import (
    ca_trusted as platform_ca_trusted,
    data_dir,
    install_ca_certificate as platform_install_ca,
)


def ca_trusted() -> bool:
    return platform_ca_trusted(os.path.join(data_dir(), "ca_install_status.txt"))


def ensure_mitm_certs() -> None:
    """Create mitmproxy CA in ~/.mitmproxy if missing."""
    try:
        from pathlib import Path
        from mitmproxy.certs import CertStore

        mitm_dir = Path(os.path.expanduser("~/.mitmproxy"))
        mitm_dir.mkdir(parents=True, exist_ok=True)
        CertStore.from_store(path=mitm_dir, basename="mitmproxy", key_size=2048)
        print(f"[UnifAI Guard] mitmproxy cert store ready: {mitm_dir}")
    except Exception as e:
        print(f"[UnifAI Guard WARNING] Could not ensure mitm certs: {e}")


def install_ca_certificate() -> bool:
    ensure_mitm_certs()
    status_path = os.path.join(data_dir(), "ca_install_status.txt")
    # macOS `security add-trusted-cert` can block on an admin password dialog forever.
    # If we already trust the CA, never re-prompt on every launch.
    if platform_ca_trusted(status_path):
        print("[UnifAI Guard] CA already trusted — skip reinstall.")
        return True
    return platform_install_ca(status_path)
