"""Agent identity: persistent ID, local IP, and heartbeat payload assembly."""

from __future__ import annotations

import os
import platform
import socket
import uuid

import agent_state
from agent_config import AGENT_TYPE, AGENT_VERSION
from agent_proxy_bundle import running_bundle_sha
from guard_platform import data_dir, detect_mac_and_transport


def agent_id_path() -> str:
    return os.path.join(data_dir(), "agent_id.txt")


def get_or_create_agent_id() -> str:
    path = agent_id_path()
    try:
        if os.path.isfile(path):
            with open(path, "r", encoding="utf-8") as f:
                existing = f.read().strip()
            if existing:
                return existing
    except Exception:
        pass
    new_id = str(uuid.uuid4())
    try:
        with open(path, "w", encoding="utf-8") as f:
            f.write(new_id)
    except Exception as e:
        print(f"[UnifAI Guard WARNING] Could not persist agent_id: {e}")
    return new_id


def detect_local_ip() -> str:
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.settimeout(1)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        return ip
    except Exception:
        try:
            return socket.gethostbyname(socket.gethostname())
        except Exception:
            return ""


def collect_agent_info(agent_id: str, status: str = "active") -> dict:
    hostname = socket.gethostname()
    username = os.environ.get("USERNAME") or os.environ.get("USER") or ""
    mac, transport = detect_mac_and_transport()
    health = agent_state._LAST_HEALTH if isinstance(agent_state._LAST_HEALTH, dict) else {}
    hs = str(health.get("status") or "").strip()
    details = health.get("details") if isinstance(health.get("details"), list) else []
    detail_s = "; ".join(str(x) for x in details[:6])
    pac_mode = str(health.get("pac_mode") or "")
    if pac_mode and pac_mode not in detail_s:
        detail_s = f"pac={pac_mode}" + (f"; {detail_s}" if detail_s else "")
    return {
        "id": agent_id,
        "hostname": hostname,
        "username": username,
        "ip_address": detect_local_ip(),
        "mac_address": mac,
        "transport_name": transport,
        "os_version": platform.platform(),
        "agent_version": AGENT_VERSION,
        "proxy_bundle_sha": running_bundle_sha(),
        "agent_type": AGENT_TYPE,
        "health_status": hs or "unknown",
        "health_detail": detail_s,
        "pac_mode": pac_mode or "unknown",
        "status": status or "active",
    }
