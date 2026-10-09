"""Agent identity: persistent ID, local IP, and heartbeat payload assembly."""

from __future__ import annotations

import os
import platform
import socket
import subprocess
import sys
import uuid

import agent_state
from agent_config import AGENT_TYPE, AGENT_VERSION
from agent_proxy_bundle import running_bundle_sha
from guard_platform import data_dir, detect_mac_and_transport


def agent_id_path() -> str:
    return os.path.join(data_dir(), "agent_id.txt")


def _read_durable_agent_id() -> str:
    """ID that survives uninstall of the data folder.

    Windows: HKCU\\Software\\Gateway\\Guard AgentId (cleanup does not delete this key).
    macOS: ~/Library/Preferences/com.gateway.guard.plist AgentId (cleanup deletes
    Application Support, not Preferences).
    """
    if os.name == "nt":
        try:
            import winreg
            with winreg.OpenKey(winreg.HKEY_CURRENT_USER, r"Software\Gateway\Guard", 0, winreg.KEY_READ) as k:
                val, _ = winreg.QueryValueEx(k, "AgentId")
                if val and str(val).strip():
                    return str(val).strip()
        except Exception:
            return ""
        return ""
    if sys.platform == "darwin":
        try:
            completed = subprocess.run(
                ["defaults", "read", "com.gateway.guard", "AgentId"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=5,
                check=False,
            )
            val = (completed.stdout or "").strip().strip('"')
            if completed.returncode == 0 and val:
                return val
        except Exception:
            return ""
    return ""


def _write_durable_agent_id(agent_id: str) -> None:
    agent_id = (agent_id or "").strip()
    if not agent_id:
        return
    if os.name == "nt":
        try:
            import winreg
            with winreg.CreateKey(winreg.HKEY_CURRENT_USER, r"Software\Gateway\Guard") as k:
                winreg.SetValueEx(k, "AgentId", 0, winreg.REG_SZ, agent_id)
        except Exception:
            pass
        return
    if sys.platform == "darwin":
        try:
            subprocess.run(
                ["defaults", "write", "com.gateway.guard", "AgentId", "-string", agent_id],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                timeout=5,
                check=False,
            )
        except Exception:
            pass


def _write_agent_id_file(path: str, agent_id: str) -> None:
    try:
        with open(path, "w", encoding="utf-8") as f:
            f.write(agent_id)
    except Exception as e:
        print(f"[Gateway Guard WARNING] Could not persist agent_id: {e}")


def get_or_create_agent_id() -> str:
    path = agent_id_path()
    try:
        if os.path.isfile(path):
            with open(path, "r", encoding="utf-8") as f:
                existing = f.read().strip()
            if existing:
                _write_durable_agent_id(existing)
                return existing
    except Exception:
        pass

    saved = _read_durable_agent_id()
    if saved:
        _write_agent_id_file(path, saved)
        return saved

    new_id = str(uuid.uuid4())
    _write_agent_id_file(path, new_id)
    _write_durable_agent_id(new_id)
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


_AD_IDENTITY_CACHE: dict | None = None


def detect_ad_identity() -> dict:
    """Detects corporate Microsoft Active Directory identity when joined to a domain.
    Seamlessly falls back to local user & standalone mode for non-domain machines.
    """
    global _AD_IDENTITY_CACHE
    if _AD_IDENTITY_CACHE is not None:
        return _AD_IDENTITY_CACHE

    result = {
        "is_domain_joined": False,
        "ad_domain": "",
        "ad_upn": "",
        "ad_groups": "",
        "domain_user": "",
    }

    if os.name != "nt":
        _AD_IDENTITY_CACHE = result
        return result

    try:
        user_domain = os.environ.get("USERDOMAIN", "").strip()
        user_dns = os.environ.get("USERDNSDOMAIN", "").strip()
        username = os.environ.get("USERNAME", "").strip()
        computer_name = os.environ.get("COMPUTERNAME", "").strip()

        is_domain = bool(user_dns) or (bool(user_domain) and user_domain.upper() != computer_name.upper())

        upn = ""
        try:
            p = subprocess.run(
                ["whoami", "/upn"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=3,
                check=False,
            )
            if p.returncode == 0 and "@" in p.stdout:
                upn = p.stdout.strip()
                is_domain = True
        except Exception:
            pass

        domain_name = user_dns or user_domain if is_domain else ""
        domain_user = f"{user_domain}\\{username}" if (is_domain and user_domain) else username

        # Fetch AD Security Groups
        groups_list = []
        if is_domain:
            try:
                p_grp = subprocess.run(
                    ["whoami", "/groups", "/fo", "csv", "/nh"],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=3,
                    check=False,
                )
                if p_grp.returncode == 0:
                    for line in p_grp.stdout.splitlines():
                        line = line.strip()
                        if line:
                            parts = [x.strip(' "') for x in line.split(",")]
                            if parts:
                                gname = parts[0]
                                if not gname.startswith("NT AUTHORITY") and not gname.startswith("Mandatory Label") and not gname.lower().startswith("builtin"):
                                    groups_list.append(gname)
            except Exception:
                pass

        result = {
            "is_domain_joined": is_domain,
            "ad_domain": domain_name,
            "ad_upn": upn,
            "ad_groups": "; ".join(groups_list[:15]),
            "domain_user": domain_user,
        }
    except Exception:
        pass

    _AD_IDENTITY_CACHE = result
    return result


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
    ad = detect_ad_identity()
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
        "is_domain_joined": ad.get("is_domain_joined", False),
        "ad_domain": ad.get("ad_domain", ""),
        "ad_upn": ad.get("ad_upn", ""),
        "ad_groups": ad.get("ad_groups", ""),
        "domain_user": ad.get("domain_user", username),
        "contact_email": ad.get("ad_upn") or "",
    }
