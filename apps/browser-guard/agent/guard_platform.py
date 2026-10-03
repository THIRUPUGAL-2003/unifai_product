"""
OS-specific helpers for Raksha Guard (Windows + macOS).
Shared agent imports this module so Windows and Mac stay one product.
"""

from __future__ import annotations

import json
import os
import platform
import subprocess
import sys
import uuid

IS_WIN = sys.platform == "win32"
IS_MAC = sys.platform == "darwin"

if IS_WIN:
    import ctypes
    import winreg
else:
    ctypes = None  # type: ignore
    winreg = None  # type: ignore


def data_dir() -> str:
    """Writable per-user data (PAC, logs, agent_id)."""
    if IS_WIN:
        base = os.environ.get("LOCALAPPDATA") or os.path.expanduser("~")
    elif IS_MAC:
        base = os.path.join(os.path.expanduser("~"), "Library", "Application Support")
    else:
        base = os.environ.get("XDG_DATA_HOME") or os.path.join(os.path.expanduser("~"), ".local", "share")
    path = os.path.join(base, "Raksha", "Guard")
    os.makedirs(path, exist_ok=True)
    return path


def log_hint_path() -> str:
    if IS_WIN:
        return r"%LOCALAPPDATA%\Raksha\Guard"
    if IS_MAC:
        return "~/Library/Application Support/Raksha/Guard"
    return "~/.local/share/Raksha/Guard"


def _guid_from_transport(raw: str) -> str:
    import re

    m = re.search(
        r"\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}",
        raw or "",
    )
    return m.group(0).upper() if m else ""


def detect_mac_and_transport() -> tuple[str, str]:
    """Physical MAC + optional adapter id."""
    if IS_WIN:
        try:
            completed = subprocess.run(
                ["getmac"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=8,
                creationflags=subprocess.CREATE_NO_WINDOW,
                check=False,
            )
            for line in (completed.stdout or "").splitlines():
                raw = line.strip()
                if not raw or raw.lower().startswith("physical") or set(raw) <= {"=", " ", "-"}:
                    continue
                if "media disconnected" in raw.lower():
                    continue
                parts = raw.split()
                if len(parts) < 2:
                    continue
                mac = parts[0].strip().upper()
                transport = _guid_from_transport(raw[len(parts[0]) :].strip())
                if mac.count("-") == 5 or mac.count(":") == 5:
                    return mac, transport
        except Exception:
            pass
    elif IS_MAC:
        try:
            completed = subprocess.run(
                ["ifconfig"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=8,
                check=False,
            )
            import re

            # Internal Apple interfaces to skip (not real network adapters)
            _SKIP_IFACE_PREFIXES = ("anpi", "bridge", "llw", "awdl", "utun", "lo", "gif", "stf", "ap1")

            lines = (completed.stdout or "").splitlines()
            current_iface = ""
            # Collect all candidates: list of (mac, transport, iface)
            candidates: list[tuple[str, str, str]] = []
            for line in lines:
                if line and not line[0].isspace():
                    current_iface = line.split(":", 1)[0].strip()
                m = re.search(r"ether\s+([0-9a-f:]{17})", line, re.I)
                if m:
                    if any(current_iface.startswith(p) for p in _SKIP_IFACE_PREFIXES):
                        continue
                    mac = m.group(1).upper().replace(":", "-")
                    transport = _mac_iface_to_service_name(current_iface) or current_iface
                    candidates.append((mac, transport, current_iface))

            if candidates:
                # Priority 1: Wi-Fi
                for mac, transport, iface in candidates:
                    if "wi-fi" in transport.lower() or transport.lower() == "wi-fi":
                        return mac, transport
                # Priority 2: Plain "Ethernet" (not numbered)
                for mac, transport, iface in candidates:
                    if transport.lower() == "ethernet":
                        return mac, transport
                # Priority 3: Any known named service (not raw interface name)
                for mac, transport, iface in candidates:
                    if transport and transport != iface:
                        return mac, transport
                # Fallback: first candidate
                return candidates[0][0], candidates[0][1]

            for m in re.finditer(r"ether\s+([0-9a-f:]{17})", completed.stdout or "", re.I):
                return m.group(1).upper().replace(":", "-"), ""
        except Exception:
            pass
    try:
        node = uuid.getnode()
        mac = "-".join(f"{(node >> ele) & 0xFF:02X}" for ele in range(40, -1, -8))
        return mac, ""
    except Exception:
        return "", ""


def _mac_iface_to_service_name(iface: str) -> str:
    """Map macOS interface (en0, en1, …) to human-readable network service name (Wi-Fi, Ethernet, …)."""
    if not iface:
        return ""
    try:
        completed = subprocess.run(
            ["networksetup", "-listallhardwareports"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=8,
            check=False,
        )
        # Output looks like:
        #   Hardware Port: Wi-Fi
        #   Device: en0
        #   Ethernet Address: ...
        current_service = ""
        for line in (completed.stdout or "").splitlines():
            line = line.strip()
            if line.startswith("Hardware Port:"):
                current_service = line.split(":", 1)[1].strip()
            elif line.startswith("Device:"):
                device = line.split(":", 1)[1].strip()
                if device == iface:
                    return current_service
    except Exception:
        pass
    return ""


def ensure_single_instance(mutex_name: str = "Raksha_Guard_Agent") -> bool:
    if IS_WIN:
        kernel32 = ctypes.windll.kernel32  # type: ignore[union-attr]
        handle = kernel32.CreateMutexW(None, False, f"Global\\{mutex_name}")
        if kernel32.GetLastError() == 183:
            return False
        ensure_single_instance._handle = handle  # type: ignore[attr-defined]
        return True
    # Unix: flock on a lockfile
    lock_path = os.path.join(data_dir(), "guard.lock")
    try:
        import fcntl

        fp = open(lock_path, "w", encoding="utf-8")
        fcntl.flock(fp.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        ensure_single_instance._fp = fp  # type: ignore[attr-defined]
        return True
    except Exception:
        return False


def show_message(title: str, text: str, error: bool = False) -> None:
    if IS_WIN:
        try:
            flags = 0x10 if error else 0x40
            ctypes.windll.user32.MessageBoxW(0, text, title, flags)  # type: ignore[union-attr]
            return
        except Exception as e:
            print(f"[Raksha Guard] Message: {title}: {text} ({e})")
            return
    if IS_MAC:
        icon = "stop" if error else "note"
        safe_title = title.replace("\\", "\\\\").replace('"', '\\"')
        safe_text = text.replace("\\", "\\\\").replace('"', '\\"')
        script = f'display dialog "{safe_text}" with title "{safe_title}" with icon {icon} buttons {{"OK"}} default button 1'
        try:
            subprocess.run(["osascript", "-e", script], check=False, timeout=120)
            return
        except Exception as e:
            print(f"[Raksha Guard] Message: {title}: {text} ({e})")
            return
    print(f"[Raksha Guard] {title}: {text}")


def prompt_uninstall_key() -> str | None:
    if IS_WIN:
        script = r"""
Add-Type -AssemblyName System.Windows.Forms
$form = New-Object System.Windows.Forms.Form
$form.Text = 'Raksha Guard Uninstall'
$form.Size = New-Object System.Drawing.Size(420,160)
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.MinimizeBox = $false
$label = New-Object System.Windows.Forms.Label
$label.Location = New-Object System.Drawing.Point(12,12)
$label.Size = New-Object System.Drawing.Size(380,30)
$label.Text = 'Enter company or device uninstall key:'
$form.Controls.Add($label)
$box = New-Object System.Windows.Forms.TextBox
$box.Location = New-Object System.Drawing.Point(12,50)
$box.Size = New-Object System.Drawing.Size(380,24)
$box.UseSystemPasswordChar = $true
$form.Controls.Add($box)
$ok = New-Object System.Windows.Forms.Button
$ok.Text = 'OK'
$ok.DialogResult = [System.Windows.Forms.DialogResult]::OK
$ok.Location = New-Object System.Drawing.Point(220,90)
$form.Controls.Add($ok)
$cancel = New-Object System.Windows.Forms.Button
$cancel.Text = 'Cancel'
$cancel.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$cancel.Location = New-Object System.Drawing.Point(310,90)
$form.Controls.Add($cancel)
$form.AcceptButton = $ok
$form.CancelButton = $cancel
$form.TopMost = $true
$form.ShowInTaskbar = $true
$form.Add_Shown({ $form.Activate(); $box.Focus() })
$result = $form.ShowDialog()
if ($result -ne [System.Windows.Forms.DialogResult]::OK) { exit 3 }
[Console]::Out.Write($box.Text.Trim())
"""
        try:
            # No STARTUPINFO SW_HIDE here: Windows applies it to the process's first
            # ShowWindow, which is the key form, leaving it invisible. CREATE_NO_WINDOW
            # already keeps the PowerShell console from appearing.
            completed = subprocess.run(
                ["powershell.exe", "-NoProfile", "-NonInteractive", "-STA", "-Command", script],
                capture_output=True,
                text=True,
                timeout=300,
                creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
            )
            if completed.returncode == 3:
                return None
            return (completed.stdout or "").strip()
        except Exception as e:
            print(f"[Raksha Guard ERROR] Uninstall prompt failed: {e}")
            return None
    if IS_MAC:
        script = (
            'try\n'
            'set r to display dialog "Enter company uninstall key (leave blank if not required):" '
            'default answer "" with title "Raksha Guard Uninstall" with hidden answer '
            'buttons {"Cancel", "OK"} default button "OK"\n'
            'return text returned of r\n'
            'on error\n'
            'return "__CANCEL__"\n'
            'end try'
        )
        try:
            completed = subprocess.run(
                ["osascript", "-e", script],
                capture_output=True,
                text=True,
                timeout=300,
                check=False,
            )
            out = (completed.stdout or "").strip()
            if out == "__CANCEL__" or completed.returncode != 0:
                return None
            return out
        except Exception as e:
            print(f"[Raksha Guard ERROR] Uninstall prompt failed: {e}")
            return None
    return ""


def ca_trusted(status_path: str) -> bool:
    """True only when the mitmproxy CA is actually trusted for SSL (not merely present)."""
    mitm_dir = os.path.expanduser("~/.mitmproxy")
    pem = os.path.join(mitm_dir, "mitmproxy-ca-cert.pem")

    if IS_WIN:
        try:
            if os.path.isfile(status_path):
                with open(status_path, "r", encoding="utf-8", errors="replace") as f:
                    if f.read().strip().upper().startswith("OK"):
                        # Confirm still in store — status OK alone can go stale after user removes cert.
                        completed = subprocess.run(
                            ["certutil.exe", "-user", "-store", "Root"],
                            stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE,
                            text=True,
                            timeout=12,
                            creationflags=subprocess.CREATE_NO_WINDOW,
                            check=False,
                        )
                        out = ((completed.stdout or "") + (completed.stderr or "")).lower()
                        return "mitmproxy" in out
        except Exception:
            pass
        try:
            completed = subprocess.run(
                ["certutil.exe", "-user", "-store", "Root"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=12,
                creationflags=subprocess.CREATE_NO_WINDOW,
                check=False,
            )
            out = ((completed.stdout or "") + (completed.stderr or "")).lower()
            return "mitmproxy" in out
        except Exception:
            return False

    if IS_MAC:
        # Prefer real SSL trust verification — "cert exists" alone is NOT enough
        # (Keychain can hold mitmproxy with Use System Defaults → HTTPS MITM fails).
        if os.path.isfile(pem):
            try:
                completed = subprocess.run(
                    ["security", "verify-cert", "-c", pem, "-p", "ssl"],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=12,
                    check=False,
                )
                if completed.returncode == 0:
                    return True
            except Exception:
                pass
        # Stale OK status without verify must NOT skip reinstall.
        try:
            if os.path.isfile(status_path):
                with open(status_path, "r", encoding="utf-8", errors="replace") as f:
                    if f.read().strip().upper().startswith("OK"):
                        # Downgrade stale status so next install_ca runs again.
                        with open(status_path, "w", encoding="utf-8") as wf:
                            wf.write("STALE: cert present but SSL trust not verified\n")
        except Exception:
            pass
        return False

    try:
        if os.path.isfile(status_path):
            with open(status_path, "r", encoding="utf-8", errors="replace") as f:
                return f.read().strip().upper().startswith("OK")
    except Exception:
        pass
    return False


def install_ca_certificate(status_path: str) -> bool:
    """Install mitmproxy CA into the user trust store (Windows Root / macOS login keychain)."""
    try:
        from pathlib import Path
        from mitmproxy.certs import CertStore

        mitm_dir = Path(os.path.expanduser("~/.mitmproxy"))
        mitm_dir.mkdir(parents=True, exist_ok=True)
        CertStore.from_store(path=mitm_dir, basename="mitmproxy", key_size=2048)
    except Exception as e:
        print(f"[Raksha Guard WARNING] Could not ensure mitm certs: {e}")

    mitm_dir = os.path.expanduser("~/.mitmproxy")
    candidates = [
        os.path.join(mitm_dir, "mitmproxy-ca-cert.pem"),
        os.path.join(mitm_dir, "mitmproxy-ca-cert.cer"),
        os.path.join(mitm_dir, "mitmproxy-ca-cert.crt"),
    ]
    target_cert = next((p for p in candidates if os.path.exists(p)), "")
    if not target_cert:
        msg = "CA cert file not found yet — HTTPS intercept will fail until cert exists."
        print(f"[Raksha Guard ERROR] {msg}")
        try:
            with open(status_path, "w", encoding="utf-8") as f:
                f.write("FAILED: " + msg + "\n")
        except Exception:
            pass
        return False

    try:
        if IS_WIN:
            print("[Raksha Guard] Installing mitmproxy Root CA into Windows Trusted Root Store...")
            completed = subprocess.run(
                ["certutil.exe", "-user", "-addstore", "Root", target_cert],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                creationflags=subprocess.CREATE_NO_WINDOW,
                check=False,
            )
            out = ((completed.stdout or "") + (completed.stderr or "")).strip()
            if completed.returncode != 0 and "already in store" not in out.lower():
                print(f"[Raksha Guard ERROR] CA install failed (code={completed.returncode}): {out}")
                with open(status_path, "w", encoding="utf-8") as f:
                    f.write(f"FAILED code={completed.returncode}\n{out}\n")
                return False
        elif IS_MAC:
            print("[Raksha Guard] Installing mitmproxy CA into macOS login keychain...")
            keychain = os.path.expanduser("~/Library/Keychains/login.keychain-db")
            if not os.path.exists(keychain):
                keychain = os.path.expanduser("~/Library/Keychains/login.keychain")

            def _add_trusted(with_admin_flag: bool) -> tuple[int, str]:
                cmd = [
                    "security",
                    "add-trusted-cert",
                    "-r",
                    "trustRoot",
                    "-p",
                    "ssl",
                    "-p",
                    "basic",
                    "-k",
                    keychain,
                    target_cert,
                ]
                # Prefer non-admin first (login keychain). -d can hang on password dialog.
                if with_admin_flag:
                    cmd.insert(2, "-d")
                completed = subprocess.run(
                    cmd,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=120,
                    check=False,
                )
                return completed.returncode, ((completed.stdout or "") + (completed.stderr or "")).strip()

            code, out = _add_trusted(False)
            if code != 0 and "already" not in out.lower() and "exists" not in out.lower():
                code2, out2 = _add_trusted(True)
                if code2 != 0 and "already" not in out2.lower() and "exists" not in out2.lower():
                    print(f"[Raksha Guard ERROR] CA install failed: {out or out2}")
                    print("[Raksha Guard ERROR] Approve the cert in Keychain Access → Trust → Always Trust (SSL).")
                    with open(status_path, "w", encoding="utf-8") as f:
                        f.write(f"FAILED\n{out}\n{out2}\n")
                    return False

            # Require SSL verify before writing OK — otherwise HTTPS intercept stays broken.
            verify = subprocess.run(
                ["security", "verify-cert", "-c", target_cert, "-p", "ssl"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=12,
                check=False,
            )
            if verify.returncode != 0:
                print("[Raksha Guard WARNING] CA added but SSL trust not verified yet — open Keychain Access and set Always Trust.")
                with open(status_path, "w", encoding="utf-8") as f:
                    f.write("PARTIAL: installed but SSL trust not verified\n")
                # Still return True so Guard starts; health will keep warning until verify OK.
                # Caller uses ca_trusted() which will stay False until verify succeeds.
                return False
        else:
            print("[Raksha Guard WARNING] Auto CA install not supported on this OS — trust mitmproxy CA manually.")
            with open(status_path, "w", encoding="utf-8") as f:
                f.write("MANUAL\n")
            return False

        print("[Raksha Guard] Certificate trust step completed.")
        with open(status_path, "w", encoding="utf-8") as f:
            f.write("OK\n")
        return True
    except Exception as e:
        print(f"[Raksha Guard ERROR] Could not auto-install CA Cert: {e}")
        try:
            with open(status_path, "w", encoding="utf-8") as f:
                f.write(f"FAILED: {e}\n")
        except Exception:
            pass
        return False


def _mac_network_services() -> list[str]:
    try:
        completed = subprocess.run(
            ["networksetup", "-listallnetworkservices"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=10,
            check=False,
        )
        services = []
        for line in (completed.stdout or "").splitlines():
            line = line.strip()
            if not line or line.startswith("An asterisk") or line.startswith("*"):
                # "*Ethernet" means disabled — skip disabled
                if line.startswith("*"):
                    continue
                continue
            if line.startswith("*"):
                continue
            services.append(line)
        # Also include lines that don't start with asterisk from first skip logic
        services = []
        for line in (completed.stdout or "").splitlines():
            s = line.strip()
            if not s or "asterisk" in s.lower():
                continue
            if s.startswith("*"):
                continue
            services.append(s)
        return services
    except Exception as e:
        print(f"[Raksha Guard WARNING] list network services: {e}")
        return ["Wi-Fi", "Ethernet"]


def set_system_proxy_pac(enable: bool, pac_url: str, silent: bool = False) -> bool:
    """Enable/disable system PAC (Windows Internet Settings / macOS networksetup)."""
    if IS_WIN:
        return _set_windows_proxy_pac(enable, pac_url, silent)
    if IS_MAC:
        return _set_mac_proxy_pac(enable, pac_url, silent)
    print("[Raksha Guard WARNING] System PAC not implemented for this OS.")
    return False


def _set_windows_proxy_pac(enable: bool, pac_url: str, silent: bool) -> bool:
    key_path = r"Software\Microsoft\Windows\CurrentVersion\Internet Settings"
    try:
        key = winreg.OpenKey(winreg.HKEY_CURRENT_USER, key_path, 0, winreg.KEY_SET_VALUE)  # type: ignore
        if enable:
            winreg.SetValueEx(key, "ProxyEnable", 0, winreg.REG_DWORD, 0)  # type: ignore
            winreg.SetValueEx(key, "AutoConfigURL", 0, winreg.REG_SZ, pac_url)  # type: ignore
            try:
                winreg.DeleteValue(key, "ProxyServer")  # type: ignore
            except FileNotFoundError:
                pass
            winreg.SetValueEx(key, "ProxyOverride", 0, winreg.REG_SZ, "localhost;127.0.0.1;<local>")  # type: ignore
            if not silent:
                print(f"[Raksha Guard] Windows PAC ENABLED -> {pac_url}")
        else:
            winreg.SetValueEx(key, "ProxyEnable", 0, winreg.REG_DWORD, 0)  # type: ignore
            try:
                winreg.DeleteValue(key, "AutoConfigURL")  # type: ignore
            except FileNotFoundError:
                pass
            if not silent:
                print("[Raksha Guard] Windows Proxy / PAC DISABLED.")
        winreg.CloseKey(key)  # type: ignore
        try:
            ctypes.windll.Wininet.InternetSetOptionW(0, 39, 0, 0)  # type: ignore
            ctypes.windll.Wininet.InternetSetOptionW(0, 37, 0, 0)  # type: ignore
        except Exception as _ie_err:
            # Non-critical: browsers re-read registry on next navigation anyway.
            print(f"[Raksha Guard DEBUG] InternetSetOptionW notify skipped: {_ie_err}")
        return True
    except Exception as e:
        print(f"[Raksha Guard ERROR] Failed to update Windows Proxy settings: {e}")
        return False


def _set_mac_proxy_pac(enable: bool, pac_url: str, silent: bool) -> bool:
    ok_any = False
    for service in _mac_network_services():
        try:
            if enable:
                set_url = subprocess.run(
                    ["networksetup", "-setautoproxyurl", service, pac_url],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=15,
                    check=False,
                )
                set_state = subprocess.run(
                    ["networksetup", "-setautoproxystate", service, "on"],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=15,
                    check=False,
                )
                if set_url.returncode != 0:
                    err = ((set_url.stderr or "") + (set_url.stdout or "")).strip()
                    print(f"[Raksha Guard WARNING] setautoproxyurl '{service}' failed: {err or set_url.returncode}")
                    continue
                if set_state.returncode != 0:
                    err = ((set_state.stderr or "") + (set_state.stdout or "")).strip()
                    print(f"[Raksha Guard WARNING] setautoproxystate '{service}' failed: {err or set_state.returncode}")
                    continue
                # Confirm URL stuck on the interface (false OK previously ignored setautoproxyurl failures).
                got = subprocess.run(
                    ["networksetup", "-getautoproxyurl", service],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=15,
                    check=False,
                )
                got_out = (got.stdout or "").strip()
                if pac_url.split("?")[0] not in got_out.replace("URL:", "").replace(" ", ""):
                    # Soft check — some macOS versions print multi-line; accept if URL substring present.
                    pac_port_str = os.environ.get("PAC_HTTP_PORT") or ""
                    if not str(pac_port_str).isdigit():
                        from agent_config import PAC_HTTP_PORT as _p

                        pac_port_str = str(_p)
                    if pac_url not in got_out and f":{pac_port_str}" not in got_out and pac_port_str not in got_out:
                        print(f"[Raksha Guard WARNING] PAC URL not confirmed on '{service}': {got_out[:200]}")
                        continue
                ok_any = True
            else:
                completed = subprocess.run(
                    ["networksetup", "-setautoproxystate", service, "off"],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    timeout=15,
                    check=False,
                )
                if completed.returncode == 0:
                    ok_any = True
        except Exception as e:
            print(f"[Raksha Guard WARNING] PAC on '{service}': {e}")
    if enable and not silent:
        print(f"[Raksha Guard] macOS auto-proxy PAC {'ENABLED' if ok_any else 'FAILED'} -> {pac_url}")
    elif not enable and not silent:
        print("[Raksha Guard] macOS auto-proxy PAC DISABLED.")
    return ok_any


def firefox_profiles_dirs() -> list[str]:
    dirs: list[str] = []
    if IS_WIN:
        roaming = os.environ.get("APPDATA") or ""
        base = os.path.join(roaming, "Mozilla", "Firefox") if roaming else ""
    elif IS_MAC:
        base = os.path.join(os.path.expanduser("~"), "Library", "Application Support", "Firefox")
    else:
        base = os.path.join(os.path.expanduser("~"), ".mozilla", "firefox")
    if not base or not os.path.isdir(base):
        return dirs
    ini = os.path.join(base, "profiles.ini")
    if os.path.isfile(ini):
        try:
            with open(ini, "r", encoding="utf-8", errors="replace") as f:
                lines = f.read().splitlines()
        except Exception:
            lines = []
        current: dict[str, str] = {}

        def flush() -> None:
            path = current.get("Path")
            if not path:
                return
            is_rel = current.get("IsRelative", "1") != "0"
            full = path if not is_rel else os.path.join(base, path.replace("/", os.sep))
            if os.path.isdir(full):
                dirs.append(full)

        for raw in lines:
            line = raw.strip()
            if line.startswith("[") and line.endswith("]"):
                flush()
                current = {}
                continue
            if "=" in line:
                k, v = line.split("=", 1)
                current[k.strip()] = v.strip()
        flush()
    profiles = os.path.join(base, "Profiles")
    if os.path.isdir(profiles):
        for name in os.listdir(profiles):
            p = os.path.join(profiles, name)
            if os.path.isdir(p):
                dirs.append(p)
    out: list[str] = []
    seen: set[str] = set()
    for d in dirs:
        n = os.path.normcase(os.path.abspath(d))
        if n not in seen:
            seen.add(n)
            out.append(d)
    return out


def register_autostart(exe_path: str) -> None:
    """Register Guard to start at user login."""
    if IS_WIN:
        try:
            key = winreg.OpenKey(  # type: ignore
                winreg.HKEY_CURRENT_USER,  # type: ignore
                r"Software\Microsoft\Windows\CurrentVersion\Run",
                0,
                winreg.KEY_SET_VALUE,  # type: ignore
            )
            winreg.SetValueEx(key, "Raksha_Guard", 0, winreg.REG_SZ, f'"{exe_path}"')  # type: ignore
            winreg.CloseKey(key)  # type: ignore
            print("[Raksha Guard] Autostart registered (Windows Run key).")
        except Exception as e:
            print(f"[Raksha Guard WARNING] Autostart register failed: {e}")
        return
    if IS_MAC:
        agents = os.path.join(os.path.expanduser("~"), "Library", "LaunchAgents")
        os.makedirs(agents, exist_ok=True)
        plist_path = os.path.join(agents, "com.raksha.guard.plist")
        # Prefer .app Contents/MacOS binary when frozen as app bundle
        program = exe_path
        plist = f"""<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.raksha.guard</string>
  <key>ProgramArguments</key>
  <array>
    <string>{program}</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key>
    <false/>
  </dict>
  <key>ThrottleInterval</key>
  <integer>5</integer>
  <key>StandardOutPath</key>
  <string>{os.path.join(data_dir(), "launchd.out.log")}</string>
  <key>StandardErrorPath</key>
  <string>{os.path.join(data_dir(), "launchd.err.log")}</string>
</dict>
</plist>
"""
        try:
            with open(plist_path, "w", encoding="utf-8") as f:
                f.write(plist)
            # Write plist only — do not launchctl load while this process is alive
            # (would spawn a second copy). KeepAlive applies on next login when
            # launchd starts Guard and restarts it after crash.
            print(f"[Raksha Guard] Autostart registered (LaunchAgent KeepAlive on next login): {plist_path}")
        except Exception as e:
            print(f"[Raksha Guard WARNING] LaunchAgent register failed: {e}")


def clear_autostart() -> None:
    if IS_WIN:
        try:
            key = winreg.OpenKey(  # type: ignore
                winreg.HKEY_CURRENT_USER,  # type: ignore
                r"Software\Microsoft\Windows\CurrentVersion\Run",
                0,
                winreg.KEY_SET_VALUE,  # type: ignore
            )
            try:
                winreg.DeleteValue(key, "Raksha_Guard")  # type: ignore
            except FileNotFoundError:
                pass
            winreg.CloseKey(key)  # type: ignore
        except Exception as e:
            print(f"[Raksha Guard WARNING] Could not clear autostart: {e}")
        return
    if IS_MAC:
        plist_path = os.path.join(os.path.expanduser("~"), "Library", "LaunchAgents", "com.raksha.guard.plist")
        try:
            subprocess.run(["launchctl", "unload", plist_path], check=False, capture_output=True)
            if os.path.isfile(plist_path):
                os.remove(plist_path)
            print("[Raksha Guard] LaunchAgent removed.")
        except Exception as e:
            print(f"[Raksha Guard WARNING] Could not clear LaunchAgent: {e}")


def os_label() -> str:
    return platform.platform()


def write_chrome_mac_proxy_policy(enable: bool, pac_url: str) -> None:
    """Write Chromium managed policies on macOS (PAC + disable QUIC/DoH) so Chrome/Edge/Brave
    do not bypass Guard via HTTP/3. Also keep a support note."""
    if not IS_MAC:
        return
    import json

    note = os.path.join(data_dir(), "mac_browser_note.txt")
    try:
        with open(note, "w", encoding="utf-8") as f:
            f.write(
                "Raksha Guard on macOS uses system Auto Proxy URL (networksetup).\n"
                "Chrome / Edge / Brave / Firefox typically follow system proxy.\n"
                "Managed policies also disable QUIC (HTTP/3) where Chrome supports it.\n"
                "Fully quit & reopen browsers after install.\n"
                "Safari: disable iCloud Private Relay for reliable intercept.\n"
                f"PAC: {pac_url if enable else '(cleared)'}\n"
            )
    except Exception:
        pass

    # Chrome 90+ user managed policies (no admin / MDM required).
    policy = {
        "ProxyMode": "pac_script",
        "ProxyPacUrl": pac_url,
        "QuicAllowed": False,
        "DnsOverHttpsMode": "off",
    }
    app_support = os.path.join(os.path.expanduser("~"), "Library", "Application Support")
    managed_roots = [
        os.path.join(app_support, "Google", "Chrome", "policies", "managed"),
        os.path.join(app_support, "Google", "Chrome Canary", "policies", "managed"),
        os.path.join(app_support, "Microsoft Edge", "policies", "managed"),
        os.path.join(app_support, "BraveSoftware", "Brave-Browser", "policies", "managed"),
        os.path.join(app_support, "Chromium", "policies", "managed"),
    ]
    for root in managed_roots:
        path = os.path.join(root, "raksha_guard.json")
        try:
            if enable:
                os.makedirs(root, exist_ok=True)
                with open(path, "w", encoding="utf-8") as f:
                    json.dump(policy, f, indent=2)
            elif os.path.isfile(path):
                os.remove(path)
        except Exception as e:
            print(f"[Raksha Guard WARNING] Mac Chromium policy write failed ({root}): {e}")
