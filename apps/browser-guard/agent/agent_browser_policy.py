"""Chromium/Firefox/Windows browser PAC and QUIC policy helpers."""

from __future__ import annotations

import json
import os

from guard_platform import (
    IS_MAC,
    IS_WIN,
    firefox_profiles_dirs,
    set_system_proxy_pac,
    write_chrome_mac_proxy_policy,
)

if IS_WIN:
    import ctypes
    import winreg
else:
    ctypes = None  # type: ignore
    winreg = None  # type: ignore

# Chromium-family policy keys (PAC + QuicAllowed). Safari is macOS-only — not on Windows Guard.
_CHROMIUM_POLICY_PATHS = (
    r"Software\Policies\Google\Chrome",
    r"Software\Policies\Google\Chrome Beta",
    r"Software\Policies\Google\Chrome Dev",
    r"Software\Policies\Google\Chrome SxS",
    r"Software\Policies\Microsoft\Edge",
    r"Software\Policies\Microsoft\Edge Beta",
    r"Software\Policies\Microsoft\Edge Dev",
    r"Software\Policies\Microsoft\Edge SxS",
    r"Software\Policies\BraveSoftware\Brave",
    r"Software\Policies\BraveSoftware\Brave-Browser",
    r"Software\Policies\Opera Software\Opera",
    r"Software\Policies\Opera Software\Opera Stable",
    r"Software\Policies\Opera Software\Opera GX",
    r"Software\Policies\Vivaldi",
    r"Software\Policies\Chromium",
    r"Software\Google\Chrome",
    r"Software\Microsoft\Edge",
    r"Software\BraveSoftware\Brave",
    r"Software\Opera Software\Opera",
    r"Software\Vivaldi",
)

_FIREFOX_PREF_MARKER_BEGIN = "// --- Gateway Guard BEGIN ---"
_FIREFOX_PREF_MARKER_END = "// --- Gateway Guard END ---"


def _notify_wininet() -> None:
    if not IS_WIN or ctypes is None:
        return
    try:
        ctypes.windll.Wininet.InternetSetOptionW(0, 39, 0, 0)
        ctypes.windll.Wininet.InternetSetOptionW(0, 37, 0, 0)
    except Exception:
        pass


def _set_reg_dword(root, path: str, name: str, value: int) -> bool:
    if not IS_WIN or winreg is None:
        return False
    try:
        try:
            key = winreg.CreateKey(root, path)
        except Exception:
            key = winreg.CreateKeyEx(root, path, 0, winreg.KEY_SET_VALUE | winreg.KEY_CREATE_SUB_KEY)
        winreg.SetValueEx(key, name, 0, winreg.REG_DWORD, value)
        winreg.CloseKey(key)
        return True
    except Exception:
        return False


def _set_reg_sz(root, path: str, name: str, value: str) -> bool:
    if not IS_WIN or winreg is None:
        return False
    try:
        try:
            key = winreg.CreateKey(root, path)
        except Exception:
            key = winreg.CreateKeyEx(root, path, 0, winreg.KEY_SET_VALUE | winreg.KEY_CREATE_SUB_KEY)
        winreg.SetValueEx(key, name, 0, winreg.REG_SZ, value)
        winreg.CloseKey(key)
        return True
    except Exception:
        return False


def _delete_reg_value(root, path: str, name: str) -> None:
    if not IS_WIN or winreg is None:
        return
    try:
        key = winreg.OpenKey(root, path, 0, winreg.KEY_SET_VALUE)
        winreg.DeleteValue(key, name)
        winreg.CloseKey(key)
    except Exception:
        pass


def _apply_chromium_browser_policies(enable: bool, pac_url: str | None = None) -> int:
    """Apply the same PAC + QUIC/DoH settings Chrome uses to every Chromium-family browser."""
    from agent_pac_server import pac_http_url

    if not IS_WIN or winreg is None:
        return 0
    if pac_url is None:
        pac_url = pac_http_url()
    applied = 0
    for root in (winreg.HKEY_CURRENT_USER, winreg.HKEY_LOCAL_MACHINE):
        for path in _CHROMIUM_POLICY_PATHS:
            try:
                if enable:
                    if _set_reg_sz(root, path, "ProxyMode", "pac_script"):
                        applied += 1
                    _set_reg_sz(root, path, "ProxyPacUrl", pac_url)
                    _set_reg_dword(root, path, "QuicAllowed", 0)
                    # Keep DNS-over-HTTPS off so Edge/Brave/Opera route like Chrome through PAC.
                    _set_reg_sz(root, path, "DnsOverHttpsMode", "off")
                else:
                    for name in ("ProxyMode", "ProxyPacUrl", "DnsOverHttpsMode", "QuicAllowed"):
                        _delete_reg_value(root, path, name)
            except Exception as e:
                print(f"[Gateway Guard WARNING] Could not update browser policy on {path}: {e}")
    return applied


def set_browser_quic(enable_quic: bool) -> None:
    from agent_pac_server import pac_http_url

    if IS_MAC:
        # When disabling QUIC, (re)write managed policies including QuicAllowed=false.
        # Do not wipe PAC policies when re-enabling QUIC.
        if not enable_quic:
            try:
                write_chrome_mac_proxy_policy(enable=True, pac_url=pac_http_url())
            except Exception as e:
                print(f"[Gateway Guard WARNING] Mac QUIC policy update failed: {e}")
        return
    if not IS_WIN:
        return
    value = 1 if enable_quic else 0
    ok = False
    for root in (winreg.HKEY_CURRENT_USER, winreg.HKEY_LOCAL_MACHINE):
        for path in _CHROMIUM_POLICY_PATHS:
            if _set_reg_dword(root, path, "QuicAllowed", value):
                ok = True
    if not ok and not enable_quic:
        print("[Gateway Guard WARNING] Could not disable browser HTTP/3 (QUIC). Some sites may bypass the proxy.")


def set_browser_pac_policy(enable: bool, pac_url: str | None = None) -> None:
    """Force browsers onto the same Guard PAC (Windows Chromium policies + Firefox profiles on all OS)."""
    from agent_pac_server import pac_http_url

    if pac_url is None:
        pac_url = pac_http_url()
    applied = _apply_chromium_browser_policies(enable=enable, pac_url=pac_url)
    set_firefox_proxy_policy(enable=enable, pac_url=pac_url)
    if IS_MAC:
        write_chrome_mac_proxy_policy(enable=enable, pac_url=pac_url)
    if enable:
        if IS_WIN:
            if applied:
                print(
                    "[Gateway Guard] PAC applied to Chrome, Edge, Brave, Opera, Vivaldi (+ Firefox profiles). "
                    "Fully quit & reopen each browser once."
                )
            else:
                print("[Gateway Guard WARNING] Could not set Chromium PAC policy — try restarting Guard as admin.")
        else:
            print(
                "[Gateway Guard] System auto-proxy PAC + Firefox prefs applied. "
                "Fully quit & reopen Chrome/Safari/Firefox once."
            )
    else:
        print("[Gateway Guard] Browser PAC policies cleared.")


def _firefox_profiles_dirs() -> list[str]:
    return firefox_profiles_dirs()


def _firefox_guard_pref_block(pac_url: str) -> str:
    # network.proxy.type 2 = PAC / autoconfig URL
    esc = pac_url.replace("\\", "\\\\").replace('"', '\\"')
    return "\n".join(
        [
            _FIREFOX_PREF_MARKER_BEGIN,
            f'user_pref("network.proxy.type", 2);',
            f'user_pref("network.proxy.autoconfig_url", "{esc}");',
            'user_pref("network.proxy.share_proxy_settings", true);',
            'user_pref("security.enterprise_roots.enabled", true);',
            'user_pref("network.http.http3.enable", false);',
            'user_pref("network.http.http3.enable_0rtt", false);',
            _FIREFOX_PREF_MARKER_END,
            "",
        ]
    )


def _strip_firefox_guard_block(text: str) -> str:
    begin = text.find(_FIREFOX_PREF_MARKER_BEGIN)
    if begin < 0:
        return text
    end = text.find(_FIREFOX_PREF_MARKER_END, begin)
    if end < 0:
        return text[:begin].rstrip() + "\n"
    end += len(_FIREFOX_PREF_MARKER_END)
    while end < len(text) and text[end] in "\r\n":
        end += 1
    return (text[:begin].rstrip() + "\n" + text[end].lstrip()) if text[end:] else text[:begin].rstrip() + "\n"


def _write_firefox_user_js(profile_dir: str, enable: bool, pac_url: str) -> bool:
    path = os.path.join(profile_dir, "user.js")
    try:
        existing = ""
        if os.path.isfile(path):
            with open(path, "r", encoding="utf-8", errors="replace") as f:
                existing = f.read()
        cleaned = _strip_firefox_guard_block(existing)
        if enable:
            cleaned = cleaned.rstrip() + "\n" + _firefox_guard_pref_block(pac_url)
        with open(path, "w", encoding="utf-8", newline="\n") as f:
            f.write(cleaned if cleaned.endswith("\n") else cleaned + "\n")
        return True
    except Exception as e:
        print(f"[Gateway Guard WARNING] Firefox user.js update failed ({profile_dir}): {e}")
        return False


def _write_firefox_policies_json(enable: bool, pac_url: str) -> None:
    """Best-effort enterprise policies.json next to Firefox installs (needs write access)."""
    policy = {
        "policies": {
            "Proxy": {
                "Mode": "autoConfig",
                "AutoConfigURL": pac_url,
                "Locked": True,
            },
            "Certificates": {"ImportEnterpriseRoots": True},
            "Preferences": {
                "network.http.http3.enable": {"Value": False, "Status": "locked"},
                "security.enterprise_roots.enabled": {"Value": True, "Status": "locked"},
            },
        }
    }
    roots: list[str] = []
    if IS_WIN:
        roots = [
            os.path.join(os.environ.get("ProgramFiles", r"C:\Program Files"), "Mozilla Firefox"),
            os.path.join(os.environ.get("ProgramFiles(x86)", r"C:\Program Files (x86)"), "Mozilla Firefox"),
        ]
        local = os.environ.get("LOCALAPPDATA") or ""
        if local:
            roots.append(os.path.join(local, "Mozilla Firefox"))
    elif IS_MAC:
        roots = [
            "/Applications/Firefox.app/Contents/Resources",
            os.path.expanduser("~/Applications/Firefox.app/Contents/Resources"),
        ]
    for root in roots:
        if not root or not os.path.isdir(root):
            continue
        dist = os.path.join(root, "distribution")
        path = os.path.join(dist, "policies.json")
        try:
            if not enable:
                if os.path.isfile(path):
                    try:
                        with open(path, "r", encoding="utf-8") as f:
                            existing = json.load(f)
                        proxy = ((existing or {}).get("policies") or {}).get("Proxy") or {}
                        if str(proxy.get("AutoConfigURL") or "").startswith("http://127.0.0.1:"):
                            os.remove(path)
                    except Exception:
                        pass
                continue
            os.makedirs(dist, exist_ok=True)
            with open(path, "w", encoding="utf-8") as f:
                json.dump(policy, f, indent=2)
            print(f"[Gateway Guard] Firefox policies.json -> {path}")
        except Exception:
            pass


def set_firefox_proxy_policy(enable: bool, pac_url: str | None = None) -> None:
    """Point Firefox at Guard PAC (+ Windows enterprise roots for MITM CA)."""
    from agent_pac_server import pac_http_url

    if pac_url is None:
        pac_url = pac_http_url()
    profiles = _firefox_profiles_dirs()
    ok = 0
    for p in profiles:
        if _write_firefox_user_js(p, enable=enable, pac_url=pac_url):
            ok += 1
    _write_firefox_policies_json(enable=enable, pac_url=pac_url)
    if IS_WIN and winreg is not None:
        try:
            key = winreg.CreateKey(winreg.HKEY_CURRENT_USER, r"Software\Policies\Mozilla\Firefox\Certificates")
            if enable:
                winreg.SetValueEx(key, "ImportEnterpriseRoots", 0, winreg.REG_DWORD, 1)
            else:
                try:
                    winreg.DeleteValue(key, "ImportEnterpriseRoots")
                except FileNotFoundError:
                    pass
            winreg.CloseKey(key)
        except PermissionError:
            # Standard users cannot write the Policies hive; user.js enterprise_roots covers trust.
            pass
        except Exception as e:
            print(f"[Gateway Guard WARNING] Firefox registry policy: {e}")
    if enable:
        if ok:
            print(f"[Gateway Guard] Firefox PAC applied to {ok} profile(s). Fully quit & reopen Firefox.")
        else:
            print("[Gateway Guard] Firefox not found yet — open Firefox once, then restart Guard to apply PAC.")
    else:
        print("[Gateway Guard] Firefox Guard prefs cleared (restart Firefox).")


def set_system_proxy_pac_and_browsers(enable: bool, pac_url: str | None = None, silent: bool = False) -> bool:
    """System PAC (Windows Internet Settings / macOS networksetup) + browser policies."""
    from agent_pac_server import pac_http_url

    if pac_url is None:
        pac_url = pac_http_url()
    ok = set_system_proxy_pac(enable=enable, pac_url=pac_url, silent=silent)
    set_browser_pac_policy(enable=enable, pac_url=pac_url)
    if IS_WIN:
        _notify_wininet()
    return ok


def set_windows_proxy_pac(enable: bool, pac_url: str | None = None, silent: bool = False) -> bool:
    return set_system_proxy_pac_and_browsers(enable=enable, pac_url=pac_url, silent=silent)
