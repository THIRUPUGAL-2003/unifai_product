"""First-run UX, uninstall flows, and native message wrappers."""

from __future__ import annotations

import os
import subprocess
import sys
import threading

from agent_config import AGENT_VERSION, RAKSHA_BACKEND_URL
from agent_health import first_run_path
from agent_http import _http_json
from agent_identity import get_or_create_agent_id
from agent_pac_orchestration import clear_guard_runtime
from guard_platform import (
    IS_MAC,
    IS_WIN,
    log_hint_path,
    prompt_uninstall_key as platform_prompt_uninstall_key,
    show_message as platform_show_message,
)


def show_message(title: str, text: str, flags: int = 0x40) -> None:
    """Native dialog (Windows MessageBox / macOS AppleScript). flags 0x10 = error."""
    platform_show_message(title, text, error=(flags & 0x10) != 0)


def show_message_async(title: str, text: str, flags: int = 0x40) -> None:
    """Show a dialog without blocking proxy / health startup."""
    threading.Thread(
        target=show_message,
        args=(title, text, flags),
        name="raksha-guard-msg",
        daemon=True,
    ).start()


def maybe_first_run_prompt() -> None:
    """Show first-run tips without blocking mitmproxy start.

    A modal MessageBox on the main thread previously left health stuck on
    "starting" and PAC unset until the user clicked OK — zero predicts.
    """
    path = first_run_path()
    if os.path.isfile(path):
        return
    # Mark seen immediately so a stuck dialog cannot block every restart.
    try:
        with open(path, "w", encoding="utf-8") as f:
            f.write(AGENT_VERSION + "\n")
    except Exception:
        pass
    browsers = (
        "Chrome, Edge, Brave, Opera, Vivaldi, Firefox, Safari"
        if IS_MAC
        else "Chrome, Edge, Brave, Opera, Vivaldi, Firefox"
    )
    show_message_async(
        "Raksha Guard installed",
        "Raksha Guard is running.\n\n"
        "For Browser AI monitoring & predict to work:\n"
        f"1) Fully quit every browser you use ({browsers})\n"
        "2) Reopen the browser and visit a monitored AI site\n"
        "3) Send a test prompt\n\n"
        f"Version {AGENT_VERSION}\n"
        f"Backend: {RAKSHA_BACKEND_URL}\n"
        f"Logs: {log_hint_path()}",
    )


def prompt_uninstall_key() -> str | None:
    return platform_prompt_uninstall_key()


def _is_guard_install_path(path: str) -> bool:
    """True when path looks like a Guard install (avoid deleting unrelated dirs)."""
    n = (path or "").replace("\\", "/").lower()
    if not n:
        return False
    if "raksha" in n and "guard" in n:
        return True
    if n.endswith("/raksha_guard.app") or n.endswith("raksha_guard.app"):
        return True
    if n.endswith("raksha_guard.exe"):
        return True
    return False


def schedule_install_removal() -> None:
    """After PAC/autostart clear: delete Guard EXE / .app once this process exits.

    Windows: detached cmd waits then deletes EXE (+ Guard install folder when safe).
    macOS: detached shell removes known Raksha_Guard.app locations.
    """
    try:
        if IS_WIN:
            exe = os.path.abspath(sys.executable) if getattr(sys, "frozen", False) else ""
            targets: list[str] = []
            if exe and os.path.isfile(exe) and _is_guard_install_path(exe):
                targets.append(exe)
            local = os.environ.get("LOCALAPPDATA", "")
            pf = os.environ.get("ProgramFiles", r"C:\Program Files")
            for cand in (
                os.path.join(local, "Programs", "Raksha", "Guard", "Raksha_Guard.exe") if local else "",
                os.path.join(pf, "Raksha", "Guard", "Raksha_Guard.exe"),
            ):
                if cand and os.path.isfile(cand) and cand not in targets:
                    targets.append(cand)
            if not targets:
                print("[Raksha Guard] Remote uninstall: no Guard EXE path to delete.")
                return
            # `timeout` needs an interactive console and exits instantly when detached, so
            # use ping as the delay; then stop every Guard process (incl. the MitM worker
            # holding the EXE open) before deleting.
            parts: list[str] = [
                "ping -n 5 127.0.0.1 >nul",
                "taskkill /F /IM Raksha_Guard.exe >nul 2>&1",
                "ping -n 2 127.0.0.1 >nul",
            ]
            dirs: set[str] = set()
            for t in targets:
                parts.append(f'del /f /q "{t}"')
                d = os.path.dirname(t)
                if _is_guard_install_path(d):
                    dirs.add(d)
            for d in dirs:
                parts.append(f'rmdir /s /q "{d}" 2>nul')
                parent = os.path.dirname(d)
                if parent and parent.lower().endswith("\\raksha"):
                    parts.append(f'rmdir "{parent}" 2>nul')
            cmd = " & ".join(parts)
            # Both STARTUPINFO(SW_HIDE) and CREATE_NO_WINDOW: completely hides cmd/ping/taskkill
            # on Windows 10 and Windows 11 (prevents Windows Terminal popup).
            si = subprocess.STARTUPINFO()
            si.dwFlags |= subprocess.STARTF_USESHOWWINDOW
            si.wShowWindow = subprocess.SW_HIDE
            flags = getattr(subprocess, "CREATE_NO_WINDOW", 0) | getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
            # String, not list: a list would escape the path quotes as \" for cmd.exe.
            subprocess.Popen(f"cmd.exe /c {cmd}", startupinfo=si, creationflags=flags, close_fds=True)
            print(f"[Raksha Guard] Scheduled EXE removal: {targets}")
            return

        if IS_MAC:
            home = os.path.expanduser("~")
            apps = [
                "/Applications/Raksha_Guard.app",
                os.path.join(home, "Applications", "Raksha_Guard.app"),
            ]
            # If running from a frozen .app, also remove that bundle
            if getattr(sys, "frozen", False):
                exe = os.path.abspath(sys.executable)
                # .../Raksha_Guard.app/Contents/MacOS/Raksha_Guard
                marker = "/Contents/MacOS/"
                if marker in exe.replace("\\", "/"):
                    app = exe[: exe.replace("\\", "/").index(marker)]
                    if app.endswith(".app") and app not in apps:
                        apps.append(app)
            existing = [a for a in apps if os.path.isdir(a)]
            guard_data = os.path.join(home, "Library", "Application Support", "Raksha", "Guard")
            if os.path.isdir(guard_data):
                existing.append(guard_data)
            if not existing:
                print("[Raksha Guard] Remote uninstall: no Guard .app to delete.")
                return
            quoted = " ".join(f'"{a}"' for a in existing)
            script = f'(sleep 3; rm -rf {quoted}) >/dev/null 2>&1 &'
            subprocess.Popen(["/bin/bash", "-c", script], start_new_session=True)
            print(f"[Raksha Guard] Scheduled .app removal: {existing}")
    except Exception as e:
        print(f"[Raksha Guard WARNING] Could not schedule install removal: {e}")


def remote_uninstall_authorized(agent_id: str) -> bool:
    """True when an admin already approved removing this Guard from the dashboard."""
    status, data = _http_json(
        "POST",
        f"{RAKSHA_BACKEND_URL}/api/browser-ai/agents/uninstall-status",
        {"agent_id": agent_id},
    )
    return status == 200 and isinstance(data, dict) and bool(data.get("authorized"))


def launch_windows_uninstaller() -> bool:
    """Run the Inno Setup uninstaller silently (removes files, autostart, Apps entry)."""
    if not IS_WIN or not getattr(sys, "frozen", False):
        return False
    app_dir = os.path.dirname(os.path.abspath(sys.executable))
    try:
        names = sorted(n for n in os.listdir(app_dir) if n.lower().startswith("unins") and n.lower().endswith(".exe"))
    except OSError:
        return False
    if not names:
        return False
    uninstaller = os.path.join(app_dir, names[-1])
    try:
        subprocess.Popen(
            [uninstaller, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"],
            cwd=os.environ.get("TEMP") or None,
            env={**os.environ, "PYINSTALLER_RESET_ENVIRONMENT": "1"},
            creationflags=getattr(subprocess, "DETACHED_PROCESS", 0) | getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0),
            close_fds=True,
        )
        print(f"[Raksha Guard] Launched silent uninstaller: {uninstaller}")
        return True
    except Exception as e:
        print(f"[Raksha Guard WARNING] Could not launch uninstaller: {e}")
        return False


def run_uninstall(key: str, schedule_cleanup: bool = True) -> int:
    """Verify company uninstall key, mark agent uninstalled, clear local proxy.

    Exit codes: 0=ok, 2=bad key, 3=cancelled (prompt), 1=other.
    Backend unreachable / 5xx does not clear PAC — company key must be verified.
    """
    agent_id = get_or_create_agent_id()
    clean_key = (key or "").strip()
    status, data = _http_json(
        "POST",
        f"{RAKSHA_BACKEND_URL}/api/browser-ai/agents/uninstall",
        {"agent_id": agent_id, "key": clean_key},
    )
    if status == 200:
        print("[Raksha Guard] Uninstall authorized by backend.")
        clear_guard_runtime()
        if schedule_cleanup:
            schedule_install_removal()
        return 0
    if status == 403:
        print("[Raksha Guard ERROR] Invalid uninstall key.")
        return 2
    if status == 0:
        print("[Raksha Guard ERROR] Backend unreachable — uninstall key not verified. PAC left on.")
        return 1
    print(f"[Raksha Guard ERROR] Uninstall rejected status={status} body={data}")
    return 1


def run_uninstall_prompt() -> int:
    if remote_uninstall_authorized(get_or_create_agent_id()):
        print("[Raksha Guard] Uninstall already approved by admin — no key needed.")
        clear_guard_runtime()
        return 0
    key = prompt_uninstall_key()
    if key is None:
        print("[Raksha Guard] Uninstall cancelled by user.")
        return 3
    # When called from Inno Setup uninstaller, Inno Setup itself deletes the files/folder.
    # schedule_cleanup=False avoids launching a separate background cmd process that fights Inno.
    return run_uninstall(key, schedule_cleanup=False)