"""PAC apply/restore orchestration and background sync loop."""

from __future__ import annotations

import os
import threading
import time

import agent_config
import agent_state
from agent_config import PROXY_ADDR
from agent_pac_content import build_pac_from_targets, fetch_proxy_pac, local_pac_path, write_local_pac
from agent_pac_server import pac_http_url
from guard_platform import clear_autostart


def apply_pac_with_bust(silent: bool = False, force_new: bool = False) -> bool:
    """Enable PAC. Change AutoConfigURL only when PAC content changes or force_new=True.

    Frequent ?v= rotation (old 30s time-slice) forced browsers to rebind proxy and
    dropped live ChatGPT/Claude tunnels — felt like random connection cuts.
    """
    from agent_browser_policy import set_system_proxy_pac_and_browsers

    try:
        content = ""
        path = local_pac_path()
        if os.path.isfile(path):
            with open(path, "r", encoding="utf-8", errors="replace") as f:
                content = f.read()
        content_hash = abs(hash(content or PROXY_ADDR)) % 1000000007
        if force_new:
            bust = (content_hash + int(time.time() * 1000)) % 1000000007
        else:
            bust = content_hash
        pac_url = f"{pac_http_url()}?v={bust}"
        if not force_new and agent_state._LAST_PAC_BUST == pac_url:
            return ensure_pac_still_on(silent=silent)
        agent_state._LAST_PAC_BUST = pac_url
        return set_system_proxy_pac_and_browsers(enable=True, pac_url=pac_url, silent=silent)
    except Exception as e:
        print(f"[UnifAI Guard WARNING] PAC apply failed: {e}")
        return set_system_proxy_pac_and_browsers(enable=True, pac_url=pac_http_url(), silent=silent)


def ensure_pac_still_on(silent: bool = True) -> bool:
    """Keep PAC enabled with the last URL — no cache-bust churn."""
    from agent_browser_policy import set_system_proxy_pac_and_browsers

    url = agent_state._LAST_PAC_BUST or pac_http_url()
    try:
        return set_system_proxy_pac_and_browsers(enable=True, pac_url=url, silent=silent)
    except Exception as e:
        print(f"[UnifAI Guard WARNING] PAC re-assert failed: {e}")
        return False


def pac_fail_open_direct(reason: str = "") -> None:
    """Temporary all-DIRECT so browsers stay online while local proxy restarts."""
    note = reason or "local proxy restarting"
    write_local_pac(
        f"// UnifAI Guard — {note}\n"
        'function FindProxyForURL(url, host) { return "DIRECT"; }\n'
    )
    apply_pac_with_bust(silent=True, force_new=True)
    print(f"[UnifAI Guard] PAC fail-open DIRECT ({note})")


def pac_restore_strict_proxy() -> None:
    pac = fetch_proxy_pac()
    if pac:
        write_local_pac(pac)
    else:
        built = build_pac_from_targets(PROXY_ADDR)
        if built:
            write_local_pac(built)
    apply_pac_with_bust(silent=True, force_new=True)
    print("[UnifAI Guard] PAC restored to strict PROXY (monitored hosts).")


def clear_guard_runtime() -> None:
    from agent_browser_policy import set_browser_quic, set_system_proxy_pac_and_browsers

    set_system_proxy_pac_and_browsers(enable=False)
    set_browser_quic(enable_quic=True)
    clear_autostart()


def sync_pac_loop(stop_event: threading.Event) -> None:
    """Pull Target Website PAC from backend. Only rebind browsers when content actually changes.

    Rebinding AutoConfigURL drops live HTTPS tunnels (ChatGPT/Claude) — never do it on a timer alone.
    """
    last = ""
    while not stop_event.is_set():
        pac = fetch_proxy_pac()
        if pac and pac != last:
            write_local_pac(pac)
            # First load or real domain-list change only.
            apply_pac_with_bust(silent=False, force_new=(last != ""))
            last = pac
            n = pac.count('",') if "aiHosts" in pac else 0
            print(f"[UnifAI Guard] PAC refreshed (~{n} domain entries).")
        stop_event.wait(agent_config.PAC_SYNC_SECONDS)
