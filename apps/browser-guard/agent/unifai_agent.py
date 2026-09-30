#!/usr/bin/env python3
"""
UnifAI Enterprise Security Guard Agent (Desktop)
================================================
Employee laptop agent for company deployments (Windows + macOS).

- Talks only to the UnifAI backend HTTPS API (no direct DB).
- Enables system PAC proxy for monitored Target Websites only.
- Registers heartbeat + agent identity in Browser AI.
- Logs to per-user UnifAI/Guard data dir.
- Autostart: Windows Run key / macOS LaunchAgent.
- Uninstall: UnifAI_Guard --uninstall "KEY"
"""

from __future__ import annotations

import os
import signal
import sys
import threading
import time

import agent_config
from agent_certs import install_ca_certificate
from agent_config import (
    AGENT_TYPE,
    AGENT_VERSION,
    LISTEN_HOST,
    PROXY_ADDR,
    SERVER_MODE,
    UNIFAI_BACKEND_URL,
    proxy_listen_port,
)
from agent_health import free_proxy_port, health_loop, port_open
from agent_heartbeat import (
    apply_admin_uninstall,
    heartbeat_loop,
    heartbeat_wants_uninstall,
    send_heartbeat,
)
from agent_identity import collect_agent_info, get_or_create_agent_id
from agent_lifecycle import (
    maybe_first_run_prompt,
    run_uninstall,
    run_uninstall_prompt,
    show_message_async,
)
from agent_autoupdate import check_and_update_if_needed
from agent_logging import ensure_single_instance, get_resource_path, setup_file_logging
from agent_pac_content import (
    check_backend,
    fetch_proxy_pac,
    write_local_pac,
)
from agent_pac_orchestration import (
    apply_pac_with_bust,
    clear_guard_runtime,
    pac_fail_open_direct,
    pac_restore_strict_proxy,
    sync_pac_loop,
)
from agent_pac_server import start_local_pac_http_server
from agent_proxy_bundle import active_addon, mark_bundle_bad, relaunch_guard, running_bundle_sha
from agent_proxy_engine import run_proxy_server
from agent_browser_policy import set_browser_quic
from guard_platform import (
    IS_MAC,
    IS_WIN,
    data_dir,
    log_hint_path,
    register_autostart,
)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> None:
    # CLI: UnifAI_Guard.exe --uninstall "KEY"  |  --uninstall-prompt
    if len(sys.argv) >= 2 and sys.argv[1] in ("--uninstall", "/uninstall", "--uninstall-prompt"):
        setup_file_logging()
        if sys.argv[1] == "--uninstall-prompt":
            code = run_uninstall_prompt()
        else:
            key = sys.argv[2] if len(sys.argv) >= 3 else ""
            code = run_uninstall(key)
        sys.exit(code)

    # MitM worker child — must NOT take single-instance lock (parent holds it).
    if len(sys.argv) >= 2 and sys.argv[1] == "--mitm-worker":
        setup_file_logging()
        from agent_proxy_engine import run_mitm_worker_main

        sys.exit(run_mitm_worker_main(sys.argv[1:]))

    log_path = setup_file_logging()
    if not ensure_single_instance():
        print("[UnifAI Guard] Already running. Exit.")
        return

    agent_id = get_or_create_agent_id()
    info = collect_agent_info(agent_id)
    os.environ["UNIFAI_AGENT_ID"] = agent_id
    os.environ["UNIFAI_AGENT_HOSTNAME"] = info["hostname"]

    print("==========================================================")
    print(f"   UnifAI Enterprise Desktop Security Guard Agent v{AGENT_VERSION}")
    print("==========================================================")
    print(f"[UnifAI Guard] Log file: {log_path}")
    print(f"[UnifAI Guard] Data dir: {data_dir()}")
    print(f"[UnifAI Guard] Agent ID: {agent_id}")
    print(f"[UnifAI Guard] Hostname: {info['hostname']} / User: {info['username']}")
    print(f"[UnifAI Guard] MAC: {info.get('mac_address') or '—'} / Transport: {info.get('transport_name') or '—'}")

    addon_script = get_resource_path("browser_ai_proxy.py")
    if not os.path.exists(addon_script):
        addon_script = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "proxy", "browser_ai_proxy.py"))
    addon = active_addon(addon_script)
    code_sha = running_bundle_sha()

    print(f"[UnifAI Guard] Guard code: {'server bundle ' + code_sha[:8] if code_sha else 'built-in (installer)'}")
    print(f"[UnifAI Guard] Proxy Addon: {addon}")
    print(f"[UnifAI Guard] Backend URL: {UNIFAI_BACKEND_URL}")
    print(f"[UnifAI Guard] Mode: {'network/server' if SERVER_MODE else 'endpoint'} (agent_type={AGENT_TYPE})")
    print(f"[UnifAI Guard] Listen: {LISTEN_HOST} / advertise PAC proxy: {agent_config.PAC_ADVERTISE_ADDR or PROXY_ADDR}")
    print(f"[UnifAI Guard] Local proxy: {PROXY_ADDR}")

    check_backend()
    # Check for Guard update on startup (background — does not block)
    check_and_update_if_needed()
    hb = send_heartbeat(agent_id)
    if heartbeat_wants_uninstall(hb):
        apply_admin_uninstall(agent_id)
        return

    pac = fetch_proxy_pac()
    if pac:
        write_local_pac(pac)
    else:
        write_local_pac(
            "// UnifAI — waiting for backend Target Websites\n"
            'function FindProxyForURL(url, host) { return "DIRECT"; }\n'
        )

    if SERVER_MODE:
        # Network proxy: do NOT rewrite this machine's browser PAC. IT points
        # employee browsers at the company PAC URL advertising this host.
        corp_pac = f"{UNIFAI_BACKEND_URL}/api/browser-ai/pac?proxy={agent_config.PAC_ADVERTISE_ADDR or PROXY_ADDR}"
        print("[UnifAI Guard] SERVER MODE — shared network proxy (same UnifAI dashboard as laptop Guard).")
        print(f"[UnifAI Guard] Point office browsers / GPO PAC to: {corp_pac}")
        print("[UnifAI Guard] Skipping local OS PAC + browser policy (endpoint-only).")
        if not install_ca_certificate():
            print(f"[UnifAI Guard ERROR] CA trust failed — open {log_hint_path()}/ca_install_status.txt")
            print("[UnifAI Guard ERROR] Distribute this CA to employee machines for HTTPS MITM.")
    else:
        start_local_pac_http_server()
        # Do NOT apply PAC until local mitmproxy is listening — otherwise browsers
        # hit PROXY;DIRECT, fail once, and stick on DIRECT (Claude works, logs stay 0).
        set_browser_quic(enable_quic=False)
        if IS_WIN:
            print("[UnifAI Guard] Browser HTTP/3 (QUIC) disabled for Chromium browsers so Target Websites use the proxy.")
            print("[UnifAI Guard] PAC policies: Chrome, Edge, Brave, Opera, Vivaldi + Firefox.")
        elif IS_MAC:
            print("[UnifAI Guard] macOS system Auto Proxy URL set; Safari/Chrome/Firefox follow system proxy.")
        if not install_ca_certificate():
            print(f"[UnifAI Guard ERROR] CA trust failed — open {log_hint_path()}/ca_install_status.txt")
            print("[UnifAI Guard ERROR] Without CA trust, browsers will not accept MITM HTTPS. Fix cert then restart Guard.")
            show_message_async(
                "UnifAI Guard — CA trust failed",
                "Certificate install failed.\nHTTPS intercept / predict may not work until CA is trusted.\n\n"
                f"See {log_hint_path()}/ca_install_status.txt",
                0x10,
            )

        try:
            if getattr(sys, "frozen", False):
                register_autostart(sys.executable)
        except Exception as e:
            print(f"[UnifAI Guard WARNING] Autostart: {e}")

        maybe_first_run_prompt()

    stop_event = threading.Event()
    port = proxy_listen_port()

    def apply_pac_when_proxy_ready() -> None:
        """Wait for MitM listen port on a helper thread — mitmdump must own the main thread on macOS."""
        for _ in range(75):  # ~15s
            if stop_event.is_set():
                return
            if port_open("127.0.0.1", port):
                print(
                    f"[UnifAI Guard] Proxy listening on {LISTEN_HOST}:{port} "
                    f"(advertise {agent_config.PAC_ADVERTISE_ADDR or PROXY_ADDR})."
                )
                if not SERVER_MODE:
                    print(f"[UnifAI Guard] Local proxy listening on {PROXY_ADDR} — applying PAC now.")
                    apply_pac_with_bust(silent=False, force_new=True)
                return
            time.sleep(0.2)
        print("[UnifAI Guard WARNING] Proxy port not open yet — PAC deferred; health loop will apply when ready.")

    def proxy_ready_watch() -> None:
        """After a supervise restart, restore strict PROXY once MitM is listening again."""
        if SERVER_MODE:
            return
        was_up = False
        while not stop_event.is_set():
            up = port_open("127.0.0.1", port)
            if up and not was_up:
                pac_restore_strict_proxy()
            was_up = up
            stop_event.wait(1)

    threading.Thread(target=apply_pac_when_proxy_ready, daemon=True).start()
    if not SERVER_MODE:
        threading.Thread(target=proxy_ready_watch, daemon=True).start()
        threading.Thread(target=sync_pac_loop, args=(stop_event,), daemon=True).start()
    # Cap AI Guard Bot hold so browsers do not drop the request (felt as connection cut).
    os.environ.setdefault("UNIFAI_EVAL_TIMEOUT", "18")
    threading.Thread(target=heartbeat_loop, args=(agent_id, stop_event), daemon=True).start()
    threading.Thread(target=health_loop, args=(stop_event, port), daemon=True).start()

    def cleanup_and_exit(signum=None, frame=None):
        print("\n[UnifAI Guard] Shutting down agent...")
        stop_event.set()
        if not SERVER_MODE:
            clear_guard_runtime()
        sys.exit(0)

    signal.signal(signal.SIGINT, cleanup_and_exit)
    signal.signal(signal.SIGTERM, cleanup_and_exit)

    print("[UnifAI Guard] Agent is active. Sleep/shutdown keep monitoring; only uninstall stops Guard.")
    print("[UnifAI Guard] IMPORTANT: Fully quit Chrome/Edge/Safari (all windows) then reopen for PAC to stick.")
    # mitmdump/asyncio requires the main thread (macOS: set_wakeup_fd). Do NOT run proxy in a worker thread.
    bundle_quick_failures = 0
    try:
        while not stop_event.is_set():
            # Ensure previous bind released before restart (avoids Errno 48 on macOS).
            free_proxy_port(port)
            for _ in range(60):
                if stop_event.is_set():
                    break
                if not port_open("127.0.0.1", port):
                    break
                time.sleep(0.25)
            started = time.time()
            try:
                run_proxy_server(addon, port=port)
            except Exception as e:
                print(f"[UnifAI Guard WARNING] Proxy crashed: {e}")
            if stop_event.is_set():
                break
            if running_bundle_sha() and time.time() - started < 60:
                bundle_quick_failures += 1
                if bundle_quick_failures >= 2:
                    mark_bundle_bad(running_bundle_sha(), "MitM worker failed twice within 60s")
                    relaunch_guard("server code rejected — back to built-in code")
            else:
                bundle_quick_failures = 0
            if not SERVER_MODE:
                # Known restart window: fail-open FIRST so browsers do not get ERR_PROXY.
                pac_fail_open_direct("proxy engine restarting")
            print("[UnifAI Guard] Proxy stopped — staying Active, restarting in 2s (sleep/wake safe).")
            free_proxy_port(port)
            stop_event.wait(2)
    finally:
        stop_event.set()
        if not SERVER_MODE:
            clear_guard_runtime()


if __name__ == "__main__":
    main()
