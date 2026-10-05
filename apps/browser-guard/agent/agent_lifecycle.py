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
    """After PAC/autostart clear: delete Guard EXE / .app, registry entries, and shortcuts.

    Windows: detached cmd waits, terminates Guard, deletes registry uninstall & run keys,
    shortcuts, EXE, and install/data directories completely.
    macOS: detached shell removes LaunchAgent, .app locations, and data dir.
    """
    try:
        if IS_WIN:
            exe = os.path.abspath(sys.executable) if getattr(sys, "frozen", False) else ""
            targets: list[str] = []
            if exe and os.path.isfile(exe) and _is_guard_install_path(exe):
                targets.append(exe)
            local = os.environ.get("LOCALAPPDATA", "")
            pf = os.environ.get("ProgramFiles", r"C:\Program Files")
            appdata = os.environ.get("APPDATA", "")
            userprofile = os.environ.get("USERPROFILE", "")
            allusersprofile = os.environ.get("ALLUSERSPROFILE", "")

            for cand in (
                os.path.join(local, "Programs", "Raksha", "Guard", "Raksha_Guard.exe") if local else "",
                os.path.join(pf, "Raksha", "Guard", "Raksha_Guard.exe"),
            ):
                if cand and os.path.isfile(cand) and cand not in targets:
                    targets.append(cand)

            parts: list[str] = [
                "ping -n 4 127.0.0.1 >nul",
                "taskkill /F /IM Raksha_Guard.exe >nul 2>&1",
                "ping -n 2 127.0.0.1 >nul",
                'reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\{8F3C2A91-6B4E-4D2F-9A71-A1B2C3D4E5F6}" /f >nul 2>&1',
                'reg delete "HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run" /v "Raksha_Guard" /f >nul 2>&1',
            ]

            if userprofile:
                parts.append(f'del /f /q "{userprofile}\\Desktop\\Raksha Guard.lnk" >nul 2>&1')
            if allusersprofile:
                parts.append(f'del /f /q "{allusersprofile}\\Desktop\\Raksha Guard.lnk" >nul 2>&1')

            if appdata:
                sm = os.path.join(appdata, "Microsoft", "Windows", "Start Menu", "Programs", "Raksha Guard")
                parts.append(f'del /f /q "{sm}\\*.*" >nul 2>&1')
                parts.append(f'rmdir /s /q "{sm}" >nul 2>&1')
            if allusersprofile:
                sm_all = os.path.join(allusersprofile, "Microsoft", "Windows", "Start Menu", "Programs", "Raksha Guard")
                parts.append(f'del /f /q "{sm_all}\\*.*" >nul 2>&1')
                parts.append(f'rmdir /s /q "{sm_all}" >nul 2>&1')

            dirs: set[str] = set()
            for t in targets:
                parts.append(f'del /f /q "{t}" >nul 2>&1')
                d = os.path.dirname(t)
                if _is_guard_install_path(d):
                    dirs.add(d)
            if local:
                dirs.add(os.path.join(local, "Programs", "Raksha", "Guard"))

            for d in dirs:
                parts.append(f'del /f /q "{d}\\*.*" >nul 2>&1')
                parts.append(f'rmdir /s /q "{d}" >nul 2>&1')
                parent = os.path.dirname(d)
                if parent and parent.lower().endswith("\\raksha"):
                    parts.append(f'rmdir "{parent}" >nul 2>&1')

            if local:
                parts.append(f'del /f /q "{os.path.join(local, "Raksha", "Guard")}\\*.*" >nul 2>&1')
                parts.append(f'rmdir /s /q "{os.path.join(local, "Raksha", "Guard")}" >nul 2>&1')
                parts.append(f'rmdir "{os.path.join(local, "Raksha")}" >nul 2>&1')

            cmd = " & ".join(parts)
            si = subprocess.STARTUPINFO()
            si.dwFlags |= subprocess.STARTF_USESHOWWINDOW
            si.wShowWindow = subprocess.SW_HIDE
            flags = getattr(subprocess, "CREATE_NO_WINDOW", 0) | getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
            subprocess.Popen(f'cmd.exe /c "{cmd}"', startupinfo=si, creationflags=flags, close_fds=True)
            print("[Raksha Guard] Scheduled complete Windows uninstall cleanup.")
            return

        if IS_MAC:
            home = os.path.expanduser("~")
            apps = [
                "/Applications/Raksha_Guard.app",
                os.path.join(home, "Applications", "Raksha_Guard.app"),
            ]
            if getattr(sys, "frozen", False):
                exe = os.path.abspath(sys.executable)
                marker = "/Contents/MacOS/"
                if marker in exe.replace("\\", "/"):
                    app = exe[: exe.replace("\\", "/").index(marker)]
                    if app.endswith(".app") and app not in apps:
                        apps.append(app)
            plist = os.path.join(home, "Library", "LaunchAgents", "com.raksha.guard.plist")
            guard_data = os.path.join(home, "Library", "Application Support", "Raksha")
            app_paths = " ".join(f'"{a}"' for a in apps)
            script = (
                f'(sleep 3; '
                f'launchctl unload "{plist}" 2>/dev/null; '
                f'rm -f "{plist}"; '
                f'rm -rf {app_paths} "{guard_data}"; '
                f'pkill -f "Raksha_Guard.app/Contents/MacOS/Raksha_Guard" 2>/dev/null; '
                f'pkill -f "/MacOS/Raksha_Guard" 2>/dev/null) >/dev/null 2>&1 &'
            )
            subprocess.Popen(["/bin/bash", "-c", script], start_new_session=True)
            print("[Raksha Guard] Scheduled complete macOS uninstall cleanup.")
            return
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
    """Run the Inno Setup uninstaller silently if present (removes files, autostart, Apps entry)."""
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
        clear_guard_runtime(clear_startup=True)
        try:
            from agent_identity import agent_id_path
            p = agent_id_path()
            if os.path.isfile(p):
                os.remove(p)
        except Exception:
            pass
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
    agent_id = get_or_create_agent_id()
    if remote_uninstall_authorized(agent_id):
        print("[Raksha Guard] Uninstall already approved by admin — no key needed.")
        clear_guard_runtime(clear_startup=True)
        try:
            from agent_identity import agent_id_path
            p = agent_id_path()
            if os.path.isfile(p):
                os.remove(p)
        except Exception:
            pass
        schedule_install_removal()
        if IS_WIN or not os.environ.get("RAKSHA_WRAPPER_UI"):
            show_message(
                "Raksha Guard Uninstalled",
                "Raksha Guard has been successfully uninstalled.\n\nBrowser protection has been disabled and files have been removed.\nPlease restart your browsers.",
            )
        return 0

    key = prompt_uninstall_key()
    if key is None:
        print("[Raksha Guard] Uninstall cancelled by user.")
        return 3
    if not key.strip():
        if IS_WIN or not os.environ.get("RAKSHA_WRAPPER_UI"):
            show_message(
                "Uninstall Key Required",
                "Uninstall key is required to uninstall Raksha Guard.\n\nRaksha Guard remains installed and active.",
                flags=0x10,
            )
        return 2

    code = run_uninstall(key, schedule_cleanup=True)
    if IS_WIN or not os.environ.get("RAKSHA_WRAPPER_UI"):
        if code == 0:
            show_message(
                "Raksha Guard Uninstalled",
                "Raksha Guard has been successfully uninstalled.\n\nBrowser protection has been disabled and files have been removed.\nPlease restart your browsers.",
            )
        elif code == 2:
            show_message(
                "Invalid Uninstall Key",
                "The uninstall key entered is incorrect.\n\nRaksha Guard remains installed and active.\nContact your IT administrator for the uninstall key.",
                flags=0x10,
            )
        else:
            show_message(
                "Uninstall Failed",
                "Unable to verify uninstall key with the server.\n\nCheck your network connection and try again.\nRaksha Guard remains active.",
                flags=0x10,
            )
    return code