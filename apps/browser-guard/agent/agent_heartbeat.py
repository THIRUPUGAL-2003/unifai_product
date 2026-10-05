"""Backend heartbeat, fleet config merge, and admin uninstall handling."""

from __future__ import annotations

import json
import os
import random
import threading
import time

import agent_config
from agent_autoupdate import check_and_update_if_needed, nudge_update_check
from agent_browser_policy import set_browser_quic
from agent_config import HEARTBEAT_SECONDS, SERVER_MODE, RAKSHA_BACKEND_URL
from agent_http import _http_json
from agent_identity import collect_agent_info
from agent_lifecycle import launch_windows_uninstaller, schedule_install_removal
from agent_pac_orchestration import clear_guard_runtime, ensure_pac_still_on, pac_restore_strict_proxy
from agent_proxy_bundle import confirm_bundle_healthy, maybe_apply_bundle_async
from agent_proxy_engine import stop_proxy_worker
from guard_platform import IS_MAC, data_dir

_guard_paused = False


def is_guard_paused() -> bool:
    global _guard_paused
    return _guard_paused


def pause_guard(agent_id: str) -> None:
    global _guard_paused
    if _guard_paused:
        return
    _guard_paused = True
    print("[Raksha Guard] PAUSE command: disabling proxy PAC & QUIC bypass (standby mode).")
    clear_guard_runtime()


def resume_guard(agent_id: str) -> None:
    global _guard_paused
    if not _guard_paused:
        return
    _guard_paused = False
    print("[Raksha Guard] RESUME command: re-enabling proxy PAC & monitoring.")
    pac_restore_strict_proxy()
    set_browser_quic(enable_quic=False)


def send_heartbeat(agent_id: str, status: str = "active") -> dict | None:
    info = collect_agent_info(agent_id, status=status)
    code, data = _http_json("POST", f"{RAKSHA_BACKEND_URL}/api/browser-ai/agents/heartbeat", info)
    if code == 200:
        print(f"[Raksha Guard] Heartbeat OK ({info.get('hostname')} / {info.get('ip_address')} / {info.get('mac_address')} / {status})")
        apply_fleet_config_from_heartbeat(data if isinstance(data, dict) else None)
        apply_release_hints_from_heartbeat(data if isinstance(data, dict) else None)
        return data if isinstance(data, dict) else {}
    print(f"[Raksha Guard WARNING] Heartbeat failed status={code} body={data}")
    return None


def apply_fleet_config_from_heartbeat(data: dict | None) -> None:
    """Merge company fleet defaults from Postgres (via heartbeat) into this process."""
    if not isinstance(data, dict):
        return
    fleet = data.get("fleet_config")
    if not isinstance(fleet, dict):
        return
    try:
        sync = int(fleet.get("pac_sync_seconds") or 0)
        if sync >= 2:
            agent_config.PAC_SYNC_SECONDS = min(sync, 600)
            os.environ["RAKSHA_PAC_SYNC_SECONDS"] = str(agent_config.PAC_SYNC_SECONDS)
    except Exception:
        pass
    adv = str(fleet.get("pac_advertise_addr") or "").strip()
    if adv and SERVER_MODE:
        agent_config.PAC_ADVERTISE_ADDR = adv
        os.environ["RAKSHA_PAC_ADVERTISE_ADDR"] = adv
    # Persist a copy for support under data_dir
    try:
        path = os.path.join(data_dir(), "fleet_config_from_db.json")
        with open(path, "w", encoding="utf-8") as f:
            json.dump(fleet, f, indent=2)
    except Exception:
        pass


def apply_release_hints_from_heartbeat(data: dict | None) -> None:
    """Admin Rebuild: new installer → update check now; new Guard code → stage it and restart."""
    if not isinstance(data, dict) or heartbeat_wants_uninstall(data):
        return
    try:
        key = "latest_mac_guard_version" if IS_MAC else "latest_guard_version"
        nudge_update_check(str(data.get(key) or ""))
    except Exception as e:
        print(f"[Raksha Guard WARNING] Update hint ignored: {e}")
    try:
        maybe_apply_bundle_async(data.get("proxy_bundle"))
    except Exception as e:
        print(f"[Raksha Guard WARNING] Guard code hint ignored: {e}")


def heartbeat_wants_uninstall(data: dict | None) -> bool:
    if not isinstance(data, dict):
        return False
    if str(data.get("command") or "").strip().lower() == "uninstall":
        return True
    agent = data.get("agent") if isinstance(data.get("agent"), dict) else {}
    status = str(agent.get("status") or "").strip().lower()
    return bool(agent.get("uninstall_requested")) or status in ("uninstall_pending", "uninstalled")


def apply_admin_uninstall(agent_id: str) -> None:
    """Remote uninstall after admin verified company key on the API request.

    Stops Guard, clears PAC/autostart, then deletes the installed EXE / .app.
    """
    print("[Raksha Guard] Remote uninstall authorized — stopping Guard + removing install.")
    _http_json(
        "POST",
        f"{RAKSHA_BACKEND_URL}/api/browser-ai/agents/uninstall-ack",
        {"agent_id": agent_id},
    )
    clear_guard_runtime()
    stop_proxy_worker()
    # Remove agent_id.txt so a future install starts with a fresh agent ID and avoids stale uninstalled lock
    try:
        from agent_identity import agent_id_path
        p = agent_id_path()
        if os.path.isfile(p):
            os.remove(p)
    except Exception:
        pass
    if not launch_windows_uninstaller():
        schedule_install_removal()
    os._exit(0)


def heartbeat_loop(agent_id: str, stop_event: threading.Event) -> None:
    ticks = 0
    while not stop_event.is_set():
        current_status = "paused" if is_guard_paused() else "active"
        data = send_heartbeat(agent_id, status=current_status)
        if heartbeat_wants_uninstall(data):
            apply_admin_uninstall(agent_id)
            return

        cmd = str((data or {}).get("command") or "").strip().lower()
        agent_obj = (data or {}).get("agent") if isinstance((data or {}).get("agent"), dict) else {}
        server_status = str(agent_obj.get("status") or "").strip().lower()

        if cmd == "pause" or server_status == "paused":
            pause_guard(agent_id)
        elif cmd in ("resume", "active") or server_status == "active":
            if is_guard_paused():
                resume_guard(agent_id)

        ticks += 1
        if not is_guard_paused():
            if data is not None and ticks >= 2:
                confirm_bundle_healthy()
            # Re-assert PAC only every ~2 min — every-30s registry poke can drop tunnels on Windows.
            if ticks % 4 == 1:
                ensure_pac_still_on(silent=True)
            set_browser_quic(enable_quic=False)

        # Auto-update check every hour (rate-limited inside check_and_update_if_needed)
        check_and_update_if_needed()
        stop_event.wait(HEARTBEAT_SECONDS)


COMMAND_WAIT_TIMEOUT = 40  # server holds the request ~25s


def command_wait_loop(agent_id: str, stop_event: threading.Event) -> None:
    """Long-poll the server so remote uninstall / Rebuild & Publish land in ~1s, not on the next heartbeat."""
    url = f"{RAKSHA_BACKEND_URL}/api/browser-ai/agents/wait-command"
    failures = 0
    while not stop_event.is_set():
        started = time.monotonic()
        code, data = _http_json("POST", url, {"agent_id": agent_id}, timeout=COMMAND_WAIT_TIMEOUT)
        if code == 200:
            failures = 0
            event = str((data or {}).get("event") or "").strip().lower() if isinstance(data, dict) else ""
            if event:
                print(f"[Raksha Guard] Server event: {event}")
                if event == "rebuild":
                    # Spread fleet-wide bundle downloads so the server is not hit by every Guard at once.
                    stop_event.wait(random.uniform(0.0, 3.0))
                elif event == "pause":
                    pause_guard(agent_id)
                elif event == "resume":
                    resume_guard(agent_id)
                hb = send_heartbeat(agent_id, status="paused" if is_guard_paused() else "active")
                if heartbeat_wants_uninstall(hb) or (event == "uninstall" and hb is None):
                    apply_admin_uninstall(agent_id)
                    return
            if time.monotonic() - started < 1.0:
                stop_event.wait(2)
            continue
        if code == 404:
            # Older server without wait-command — heartbeat still delivers commands.
            stop_event.wait(600)
            continue
        failures += 1
        stop_event.wait(min(60, 2 * failures))
