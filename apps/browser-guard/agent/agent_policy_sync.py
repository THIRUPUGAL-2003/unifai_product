"""Live policy version shared with the browser extension and the MitM worker.

The server wakes this Guard with a "policy" event when Target Websites, rules, or
governance controls change. refresh_now() re-pulls them and bumps the version only
when something really changed. The extension background and the proxy worker
long-poll /extension/wait on the local port, so an admin edit on the website reaches
the browser in about a second. A slow refresh loop is the fallback for servers
that do not send events.
"""

from __future__ import annotations

import json
import threading

REFRESH_FALLBACK_SECONDS = 5.0
WAIT_MAX_SECONDS = 20.0  # under the 30s idle limit of an MV3 extension service worker

_cond = threading.Condition()
_version = 0
_fingerprint = ""
_uninstalling = False
_refresh_nudge = threading.Event()
pac_nudge = threading.Event()


def current_version() -> int:
    with _cond:
        return _version


def is_uninstalling() -> bool:
    with _cond:
        return _uninstalling


def note_policy(snapshot) -> bool:
    """Bump the version when the policy snapshot differs from the last one."""
    global _version, _fingerprint
    fp = json.dumps(snapshot, sort_keys=True, default=str)
    with _cond:
        if fp == _fingerprint:
            return False
        _fingerprint = fp
        _version += 1
        _cond.notify_all()
        return True


def refresh_now() -> int:
    """Re-pull Target Websites, rules and controls from the backend now."""
    import agent_extension as ext

    ext.invalidate_policy_caches()
    hosts = ext.monitored_hosts()
    blocked = ext.blocked_hosts()
    rules = ext.dynamic_rules()
    note_policy([hosts, blocked, rules, ext.get_governance_controls()])
    return current_version()


def wait_for_change(since: int, timeout: float) -> tuple[int, bool]:
    """Block until the version differs from `since` (or uninstall starts)."""
    timeout = max(0.0, min(float(timeout), WAIT_MAX_SECONDS))
    with _cond:
        _cond.wait_for(lambda: _version != since or _uninstalling, timeout)
        return _version, _uninstalling


def signal_uninstalling() -> None:
    """Release every waiting extension with uninstalling=true before Guard exits."""
    global _uninstalling
    with _cond:
        _uninstalling = True
        _cond.notify_all()


def on_server_policy_event() -> None:
    """Server said the policy changed: refresh now and let PAC follow."""
    _refresh_nudge.set()
    pac_nudge.set()


def refresh_loop(stop_event: threading.Event) -> None:
    while not stop_event.is_set():
        try:
            refresh_now()
        except Exception as e:
            print(f"[Gateway Guard WARNING] Policy refresh failed: {e}")
        _refresh_nudge.wait(REFRESH_FALLBACK_SECONDS)
        _refresh_nudge.clear()
