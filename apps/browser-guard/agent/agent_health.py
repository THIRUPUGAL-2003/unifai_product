"""Health probes, health.json, and the background health loop."""

from __future__ import annotations

import json
import os
import signal
import socket
import subprocess
import threading
import time

import agent_state
from agent_browser_policy import set_browser_quic
from agent_certs import ca_trusted, install_ca_certificate
from agent_config import (
    AGENT_VERSION,
    HEALTH_SECONDS,
    PROXY_ADDR,
    RAKSHA_BACKEND_URL,
    _FIRST_RUN_FLAG,
    _HEALTH_WHEN_PROXY_DOWN_SECONDS,
    proxy_listen_port,
)
from agent_http import _http_get_text
from agent_logging import get_resource_path
from agent_pac_content import fetch_proxy_pac, local_pac_path, write_local_pac
from agent_pac_orchestration import apply_pac_with_bust, ensure_pac_still_on
from agent_pac_server import pac_http_url
from guard_platform import IS_MAC, data_dir


def health_path() -> str:
    return os.path.join(data_dir(), "health.json")


def first_run_path() -> str:
    return os.path.join(data_dir(), _FIRST_RUN_FLAG)


def port_open(host: str, port: int, timeout: float = 0.6) -> bool:
    """True if TCP connect works. On Windows, also try ::1 when host is 127.0.0.1
    (mitm can briefly bind IPv6-only; PAC still uses 127.0.0.1 after listen-host fix)."""
    candidates = [host]
    if host in ("127.0.0.1", "localhost"):
        candidates.append("::1")
    for h in candidates:
        try:
            with socket.create_connection((h, port), timeout=timeout):
                return True
        except Exception:
            continue
    return False


def free_proxy_port(port: int) -> None:
    """Best-effort free TCP listen port before mitm rebind (avoids macOS Errno 48)."""
    if not IS_MAC:
        return
    try:
        my_pid = os.getpid()

        def _listener_pids() -> list[int]:
            completed = subprocess.run(
                ["lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN", "-t"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=5,
                check=False,
            )
            out: list[int] = []
            for raw in (completed.stdout or "").split():
                pid_s = raw.strip()
                if pid_s.isdigit():
                    pid = int(pid_s)
                    if pid != my_pid:
                        out.append(pid)
            return out

        pids = _listener_pids()
        for pid in pids:
            try:
                os.kill(pid, signal.SIGTERM)
                print(f"[Raksha Guard] Freed stale listener pid={pid} on :{port} (SIGTERM)")
            except Exception:
                pass
        time.sleep(0.5)
        # MitM child can ignore SIGTERM while event-loop is wedged — escalate.
        for pid in _listener_pids():
            try:
                os.kill(pid, signal.SIGKILL)
                print(f"[Raksha Guard] Freed stale listener pid={pid} on :{port} (SIGKILL)")
            except Exception:
                pass
        time.sleep(0.3)
    except Exception as e:
        print(f"[Raksha Guard WARNING] free_proxy_port: {e}")


def _detect_pac_mode() -> str:
    """strict_proxy | fail_open_direct | bypass_chain | unknown — from local PAC file."""
    try:
        with open(local_pac_path(), "r", encoding="utf-8", errors="replace") as f:
            body = f.read()
    except Exception:
        return "unknown"
    if not body or "FindProxyForURL" not in body:
        return "unknown"
    upper = body.upper()
    # Health-loop all-DIRECT while local MitM proxy is down
    if 'RETURN "DIRECT"' in upper.replace(" ", "") and "PROXY " not in upper:
        return "fail_open_direct"
    if "PROXY " in upper and "; DIRECT" in upper:
        return "bypass_chain"  # sticky DIRECT risk — should be stripped
    if "PROXY " in upper:
        return "strict_proxy"
    return "unknown"


def time_iso() -> str:
    import datetime

    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def run_health_check(proxy_port: int | None = None) -> dict:
    """Probe backend, PAC, local proxy, CA — write health.json for support."""
    if proxy_port is None:
        proxy_port = proxy_listen_port()

    checks: dict[str, bool] = {}
    details: list[str] = []

    targets = _http_get_text(f"{RAKSHA_BACKEND_URL}/api/browser-ai/targets?for=agent", "application/json", timeout=45)
    checks["backend_targets"] = bool(targets and "targets" in targets)
    if not checks["backend_targets"]:
        targets = _http_get_text(f"{RAKSHA_BACKEND_URL}/api/browser-ai/targets", "application/json", timeout=45)
        checks["backend_targets"] = bool(targets and "targets" in targets)
    if not checks["backend_targets"]:
        details.append("backend targets unreachable")

    pac_ok = False
    try:
        pac_body = _http_get_text(pac_http_url(), "application/x-ns-proxy-autoconfig,*/*", timeout=3)
        pac_ok = bool(pac_body and "FindProxyForURL" in pac_body)
    except Exception:
        pac_ok = False
    checks["local_pac"] = pac_ok
    if not pac_ok:
        details.append("local PAC HTTP not serving")

    checks["proxy_port"] = port_open("127.0.0.1", proxy_port)
    if not checks["proxy_port"]:
        details.append(f"proxy :{proxy_port} not listening")

    checks["ca_trusted"] = ca_trusted()
    if not checks["ca_trusted"]:
        details.append("CA trust missing or not OK")

    addon = get_resource_path("browser_ai_proxy.py")
    if not os.path.exists(addon):
        addon = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "proxy", "browser_ai_proxy.py"))
    checks["proxy_script"] = os.path.isfile(addon)
    if not checks["proxy_script"]:
        details.append("browser_ai_proxy.py missing")

    pac_mode = _detect_pac_mode()
    checks["pac_strict"] = pac_mode == "strict_proxy" or (
        pac_mode == "fail_open_direct" and not checks.get("proxy_port")
    )
    if pac_mode == "bypass_chain":
        details.append("PAC still has PROXY;DIRECT bypass chain — traffic may skip Guard")
    elif pac_mode == "fail_open_direct" and checks.get("proxy_port"):
        details.append("PAC is all-DIRECT while proxy is up — browsers bypass Guard (Prompt Logs stay 0)")
        checks["pac_strict"] = False

    critical = ("backend_targets", "local_pac", "proxy_script")
    if (
        all(checks.get(k) for k in critical)
        and checks.get("ca_trusted")
        and checks.get("proxy_port")
        and pac_mode == "strict_proxy"
    ):
        status = "ok"
    elif checks.get("backend_targets") and checks.get("proxy_script"):
        status = "degraded"
    else:
        status = "error"

    report = {
        "status": status,
        "agent_version": AGENT_VERSION,
        "backend_url": RAKSHA_BACKEND_URL,
        "proxy_addr": PROXY_ADDR,
        "pac_mode": pac_mode,
        "checks": checks,
        "details": details,
        "updated_at": time_iso(),
    }
    with agent_state._HEALTH_LOCK:
        agent_state._LAST_HEALTH = report
    try:
        with open(health_path(), "w", encoding="utf-8") as f:
            json.dump(report, f, indent=2)
    except Exception as e:
        print(f"[Raksha Guard WARNING] Could not write health.json: {e}")
    print(f"[Raksha Guard] Health={status} checks={checks}")
    return report


def health_loop(stop_event: threading.Event, proxy_port: int) -> None:
    # First check after proxy has had time to bind
    stop_event.wait(4)
    proxy_was_down = False
    fail_streak = 0
    while not stop_event.is_set():
        try:
            report = run_health_check(proxy_port)
            checks = report.get("checks") if isinstance(report, dict) else {}
            proxy_up = bool(checks.get("proxy_port"))
            # Require several misses before all-DIRECT — one blip must not sticky-bypass forever.
            if not proxy_up:
                fail_streak += 1
                # Faster fail-open: 2 misses (~90s) — PROXY-only PAC causes hard
                # ERR_PROXY_CONNECTION_FAILED while MitM is restarting.
                if fail_streak >= 2:
                    write_local_pac(
                        "// Raksha Guard — local proxy down; fail open until proxy returns.\n"
                        'function FindProxyForURL(url, host) { return "DIRECT"; }\n'
                    )
                    apply_pac_with_bust(silent=True, force_new=True)
                    proxy_was_down = True
                    print("[Raksha Guard] Proxy port down x2 — PAC fail-open DIRECT (browsers stay online).")
            else:
                if fail_streak > 0 or proxy_was_down:
                    pac = fetch_proxy_pac()
                    if pac:
                        write_local_pac(pac)
                    apply_pac_with_bust(silent=True, force_new=True)
                    print("[Raksha Guard] Proxy healthy again — PAC restored (forced browser refetch).")
                    proxy_was_down = False
                else:
                    # Healthy steady-state: do NOT rotate ?v= (avoids mid-session disconnects).
                    ensure_pac_still_on(silent=True)
                fail_streak = 0
                # Keep CA trust healthy without DB — retry install if missing.
                if not ca_trusted():
                    print("[Raksha Guard] CA not trusted — retrying certificate install…")
                    install_ca_certificate()
            set_browser_quic(enable_quic=False)
        except Exception as e:
            print(f"[Raksha Guard WARNING] Health loop: {e}")
        # Probe faster while proxy is down so MitM recovery is not stuck for 45s.
        wait_s = _HEALTH_WHEN_PROXY_DOWN_SECONDS if fail_streak > 0 else HEALTH_SECONDS
        stop_event.wait(wait_s)
