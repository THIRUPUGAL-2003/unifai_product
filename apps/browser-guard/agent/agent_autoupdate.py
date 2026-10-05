"""
Raksha Guard — Silent Auto-Updater
===================================
Guard startup + every heartbeat cycle-ல server version check பண்ணும்.
New version இருந்தா → background-ல download → silent install → Guard restart.
Employee laptop-ல manual action தேவையில்லை.

Windows + macOS both supported with the same Guard-Key auth path.
"""

from __future__ import annotations

import os
import platform
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request
import urllib.error
from typing import Optional

from agent_config import AGENT_VERSION, RAKSHA_BACKEND_URL, RAKSHA_GUARD_SECRET

# ── Constants ──────────────────────────────────────────────────────────────────
_WIN_DL_URL = "{backend}/api/browser-ai/setup/download-windows.zip"
_MAC_DL_URL = "{backend}/api/browser-ai/setup/download-mac.zip"
_CHECK_INTERVAL = 3600  # 1 hour
_DOWNLOAD_TIMEOUT = 120
_IS_WIN = platform.system().lower() == "windows"
_IS_MAC = platform.system().lower() == "darwin"

_NUDGE_MIN_INTERVAL = 300
_update_lock = threading.Lock()
_last_check: float = 0.0
_last_nudge: float = 0.0


def _auth_headers() -> dict[str, str]:
    headers = {"User-Agent": f"Raksha-Guard/{AGENT_VERSION}"}
    secret = (RAKSHA_GUARD_SECRET or "").strip()
    if secret:
        headers["X-Raksha-Guard-Key"] = secret
    return headers


def _download_url() -> str:
    if _IS_WIN:
        return _WIN_DL_URL.format(backend=RAKSHA_BACKEND_URL)
    return _MAC_DL_URL.format(backend=RAKSHA_BACKEND_URL)


def _get_server_version() -> Optional[str]:
    """Server-ல இருக்க latest version-ஐ HTTP header-ல இருந்து படிக்கும்."""
    url = _download_url()
    try:
        req = urllib.request.Request(url, method="HEAD")
        for k, v in _auth_headers().items():
            req.add_header(k, v)
        with urllib.request.urlopen(req, timeout=10) as resp:
            version = resp.headers.get("X-Raksha-Guard-Version", "").strip()
            if version:
                return version
    except Exception as e:
        print(f"[Raksha Guard AutoUpdate] HEAD check failed ({e}), trying GET fallback...")

    try:
        req = urllib.request.Request(url, method="GET")
        for k, v in _auth_headers().items():
            req.add_header(k, v)
        req.add_header("Range", "bytes=0-0")
        with urllib.request.urlopen(req, timeout=10) as resp:
            version = resp.headers.get("X-Raksha-Guard-Version", "").strip()
            return version if version else None
    except Exception as e:
        print(f"[Raksha Guard AutoUpdate] GET fallback failed: {e}")
        return None


def _versions_differ(current: str, server: str) -> bool:
    def parse(v: str):
        try:
            return tuple(int(x) for x in v.strip().lstrip("v").split("."))
        except Exception:
            return (0,)

    return parse(server) > parse(current)


def _download_file(url: str, dest: str) -> None:
    req = urllib.request.Request(url, method="GET")
    for k, v in _auth_headers().items():
        req.add_header(k, v)
    with urllib.request.urlopen(req, timeout=_DOWNLOAD_TIMEOUT) as resp, open(dest, "wb") as out:
        while True:
            chunk = resp.read(1024 * 256)
            if not chunk:
                break
            out.write(chunk)


def _download_and_install_windows(server_version: str) -> bool:
    """Download latest Windows ZIP, extract Setup EXE, silent install, restart Guard."""
    import zipfile

    dl_url = _WIN_DL_URL.format(backend=RAKSHA_BACKEND_URL)
    tmp_zip = os.path.join(tempfile.gettempdir(), "raksha_guard_autoupdate.zip")
    tmp_exe = os.path.join(tempfile.gettempdir(), "raksha_guard_autoupdate_setup.exe")

    print(f"[Raksha Guard AutoUpdate] Downloading {dl_url}")
    try:
        _download_file(dl_url, tmp_zip)
    except Exception as e:
        print(f"[Raksha Guard AutoUpdate] Download failed: {e}")
        return False

    try:
        with zipfile.ZipFile(tmp_zip, "r") as z:
            names = z.namelist()
            setup = next((n for n in names if n.lower().endswith("setup.exe")), None)
            if not setup:
                setup = next((n for n in names if n.lower().endswith(".exe")), None)
            if not setup:
                print("[Raksha Guard AutoUpdate] No EXE found in ZIP")
                return False
            with z.open(setup) as src, open(tmp_exe, "wb") as dst:
                dst.write(src.read())
    except Exception as e:
        print(f"[Raksha Guard AutoUpdate] Extract failed: {e}")
        return False
    finally:
        try:
            os.remove(tmp_zip)
        except Exception:
            pass

    print(f"[Raksha Guard AutoUpdate] Installing v{server_version} silently...")
    try:
        local = os.environ.get("LOCALAPPDATA", "")
        pf = os.environ.get("ProgramFiles", r"C:\Program Files")
        candidates = [
            os.path.join(local, "Programs", "Raksha", "Guard", "Raksha_Guard.exe") if local else "",
            os.path.join(pf, "Raksha", "Guard", "Raksha_Guard.exe"),
        ]
        guard_exe = next((p for p in candidates if p and os.path.isfile(p)), candidates[0] or "")
        # The installer relaunches Guard too; both must not inherit this onefile's _MEI dir.
        fresh_env = {**os.environ, "PYINSTALLER_RESET_ENVIRONMENT": "1"}
        if guard_exe:
            # Safety-net relaunch if the installer's own restart fails. `timeout` exits
            # instantly without a console, so wait with ping instead. Passed as a string so
            # the quotes reach cmd.exe unescaped.
            subprocess.Popen(
                f'cmd.exe /c ping -n 46 127.0.0.1 >nul & start "" "{guard_exe}"',
                env=fresh_env,
                creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0)
                | getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0),
                close_fds=True,
            )

        creation_flags = (
            getattr(subprocess, "CREATE_NO_WINDOW", 0)
            | (getattr(subprocess, "DETACHED_PROCESS", 0))
            | (getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0))
        )
        subprocess.Popen(
            [tmp_exe, "/VERYSILENT", "/NORESTART", "/SUPPRESSMSGBOXES"],
            env=fresh_env,
            creationflags=creation_flags,
        )
        print("[Raksha Guard AutoUpdate] Install launched. Guard will restart.")
        time.sleep(5)
        os._exit(0)
    except Exception as e:
        print(f"[Raksha Guard AutoUpdate] Install launch failed: {e}")
        return False


def _mac_install_dest() -> str:
    """Prefer replacing the already-installed location."""
    home_app = os.path.expanduser("~/Applications/Raksha_Guard.app")
    sys_app = "/Applications/Raksha_Guard.app"
    if os.path.isdir(home_app):
        return home_app
    if os.path.isdir(sys_app):
        return sys_app
    if not os.path.isdir(os.path.expanduser("~/Applications")):
        try:
            os.makedirs(os.path.expanduser("~/Applications"), exist_ok=True)
        except Exception:
            return sys_app
    return home_app


def _mac_stop_guard() -> None:
    try:
        subprocess.run(
            ["pkill", "-f", "Raksha_Guard"],
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        time.sleep(2)
    except Exception:
        pass


def _download_and_install_mac(server_version: str) -> bool:
    """Download latest macOS ZIP, extract .app, replace current install, restart."""
    import zipfile
    import shutil

    dl_url = _MAC_DL_URL.format(backend=RAKSHA_BACKEND_URL)
    tmp_zip = os.path.join(tempfile.gettempdir(), "raksha_guard_autoupdate_mac.zip")

    print(f"[Raksha Guard AutoUpdate] Downloading macOS package {dl_url}")
    try:
        _download_file(dl_url, tmp_zip)
    except Exception as e:
        print(f"[Raksha Guard AutoUpdate] Download failed: {e}")
        return False

    tmp_dir = os.path.join(tempfile.gettempdir(), "raksha_guard_autoupdate_mac")
    try:
        shutil.rmtree(tmp_dir, ignore_errors=True)
        os.makedirs(tmp_dir, exist_ok=True)
        with zipfile.ZipFile(tmp_zip, "r") as z:
            for info in z.infolist():
                name = info.filename.replace("\\", "/")
                if name.startswith("/") or ".." in name.split("/"):
                    print(f"[Raksha Guard AutoUpdate] Skipping unsafe zip path: {info.filename}")
                    continue
                dest = os.path.realpath(os.path.join(tmp_dir, name))
                root_real = os.path.realpath(tmp_dir)
                if not dest.startswith(root_real + os.sep) and dest != root_real:
                    print(f"[Raksha Guard AutoUpdate] Skipping zip-slip path: {info.filename}")
                    continue
                if info.is_dir():
                    os.makedirs(dest, exist_ok=True)
                else:
                    os.makedirs(os.path.dirname(dest), exist_ok=True)
                    with z.open(info) as src, open(dest, "wb") as out:
                        out.write(src.read())
    except Exception as e:
        print(f"[Raksha Guard AutoUpdate] Extract failed: {e}")
        return False
    finally:
        try:
            os.remove(tmp_zip)
        except Exception:
            pass

    app_path = None
    for root, dirs, _ in os.walk(tmp_dir):
        for d in dirs:
            if d == "Raksha_Guard.app" or d.endswith(".app"):
                app_path = os.path.join(root, d)
                break
        if app_path:
            break

    if not app_path:
        install_cmd = None
        for root, _, files in os.walk(tmp_dir):
            for f in files:
                if f == "Install_Raksha_Guard.command":
                    install_cmd = os.path.join(root, f)
                    break
            if install_cmd:
                break
        if install_cmd:
            _mac_stop_guard()
            os.chmod(install_cmd, 0o755)
            subprocess.Popen(["bash", install_cmd])
            print("[Raksha Guard AutoUpdate] Install script launched. Guard will restart.")
            time.sleep(5)
            os._exit(0)
        print("[Raksha Guard AutoUpdate] No .app or install script found")
        return False

    print(f"[Raksha Guard AutoUpdate] Installing .app from {app_path}")
    try:
        _mac_stop_guard()
        dest = _mac_install_dest()
        shutil.rmtree(dest, ignore_errors=True)
        parent = os.path.dirname(dest)
        os.makedirs(parent, exist_ok=True)
        shutil.copytree(app_path, dest, symlinks=True)

        # Keep VERSION.txt in Resources so next check matches server (sidecar).
        res = os.path.join(dest, "Contents", "Resources")
        os.makedirs(res, exist_ok=True)
        ver_src = os.path.join(tmp_dir, "VERSION.txt")
        ver_dst = os.path.join(res, "VERSION.txt")
        if os.path.isfile(ver_src):
            shutil.copy2(ver_src, ver_dst)
        elif server_version and server_version != "latest":
            with open(ver_dst, "w", encoding="utf-8", newline="\n") as f:
                f.write(server_version.strip() + "\n")

        # Clear Gatekeeper quarantine so open works without user click.
        try:
            subprocess.run(
                ["xattr", "-dr", "com.apple.quarantine", dest],
                check=False,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        except Exception:
            pass

        shutil.rmtree(tmp_dir, ignore_errors=True)
        subprocess.Popen(["open", dest])
        print(f"[Raksha Guard AutoUpdate] v{server_version} installed → restarting.")
        time.sleep(3)
        os._exit(0)
    except Exception as e:
        print(f"[Raksha Guard AutoUpdate] Install failed: {e}")
        return False


def nudge_update_check(server_version: str) -> None:
    """Heartbeat says the server serves a newer Guard: run the update check now, not in up to 1h."""
    global _last_check, _last_nudge
    if not server_version or not _versions_differ(AGENT_VERSION, server_version):
        return
    now = time.time()
    if now - _last_nudge < _NUDGE_MIN_INTERVAL:
        return
    _last_nudge = now
    _last_check = 0.0
    print(f"[Raksha Guard AutoUpdate] Server published v{server_version} — checking now")


def check_and_update_if_needed() -> None:
    """
    Startup + heartbeat loop-ல call பண்ணு.
    Server newer version இருந்தா → background thread-ல update பண்ணும்.
    """
    global _last_check

    now = time.time()
    if now - _last_check < _CHECK_INTERVAL:
        return
    _last_check = now

    if not RAKSHA_BACKEND_URL:
        return
    if not _IS_WIN and not _IS_MAC:
        return

    def _run():
        if not _update_lock.acquire(blocking=False):
            return
        try:
            server_ver = _get_server_version()
            if not server_ver:
                return
            if not _versions_differ(AGENT_VERSION, server_ver):
                print(f"[Raksha Guard AutoUpdate] Up to date (v{AGENT_VERSION})")
                return

            print(f"[Raksha Guard AutoUpdate] New version available: v{AGENT_VERSION} → v{server_ver}")
            if _IS_WIN:
                _download_and_install_windows(server_ver)
            elif _IS_MAC:
                _download_and_install_mac(server_ver)
            else:
                print("[Raksha Guard AutoUpdate] Unsupported OS for auto-update")
        except Exception as e:
            print(f"[Raksha Guard AutoUpdate] Unexpected error: {e}")
        finally:
            _update_lock.release()

    threading.Thread(target=_run, name="raksha-autoupdate", daemon=True).start()
