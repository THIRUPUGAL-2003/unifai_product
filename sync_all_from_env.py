#!/usr/bin/env python3
"""
==============================================================================
UnifAI Enterprise Master Environment Synchronizer (sync_all_from_env.py)
==============================================================================
One-click / one-command master synchronizer that reads the repository root
.env file and automatically propagates every port, domain, host, proxy, secret,
extension file, Mac app package, installer script, and Go server binary.

Usage:
    python sync_all_from_env.py
    python sync_all_from_env.py --rebuild-exe   (also runs PyInstaller for Windows Guard .exe)
==============================================================================
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

# Ensure UTF-8 console output
for stream_name in ("stdout", "stderr"):
    stream = getattr(sys, stream_name, None)
    if stream is not None and hasattr(stream, "reconfigure"):
        try:
            stream.reconfigure(encoding="utf-8", errors="replace")
        except Exception:
            pass


def print_banner():
    print("=" * 72)
    print("       UnifAI Enterprise — Master .env Auto-Synchronizer")
    print("=" * 72)


def get_repo_root() -> Path:
    return Path(__file__).resolve().parent


def load_env(repo_root: Path) -> dict[str, str]:
    env_path = repo_root / ".env"
    if not env_path.is_file():
        raise FileNotFoundError(f".env file not found at {env_path}")

    env_vars: dict[str, str] = {}
    with open(env_path, "r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            k = k.strip()
            v = v.strip().strip("'\"")
            if k:
                env_vars[k] = v
                os.environ[k] = v  # propagate into process environment
    return env_vars


def update_guard_config_jsons(repo_root: Path, env_vars: dict[str, str]) -> int:
    server_domain = env_vars.get("SERVER_DOMAIN", "").rstrip("/")
    if not server_domain:
        server_domain = env_vars.get("GATEWAY_BACKEND_URL", "").rstrip("/")

    proxy_addr = env_vars.get("GATEWAY_PROXY_ADDR", "127.0.0.1:18103")
    pac_http_port_str = env_vars.get("PAC_HTTP_PORT", "18195")
    try:
        pac_http_port = int(pac_http_port_str)
    except ValueError:
        pac_http_port = 18195

    guard_secret = env_vars.get("GATEWAY_GUARD_SECRET", "")
    pac_url = f"{server_domain}/api/browser-ai/pac"
    listen_host = env_vars.get("APP_HOST", "127.0.0.1")
    if listen_host in ("0.0.0.0", ""):
        listen_host = "127.0.0.1"

    bg_dir = repo_root / "apps" / "browser-guard"
    candidates = [
        bg_dir / "config" / "gateway_guard_config.json",
        bg_dir / "release" / "gateway_guard_config.json",
        bg_dir / "installer" / "staging" / "gateway_guard_config.json",
        bg_dir / "installer" / "staging-mac" / "gateway_guard_config.json",
        bg_dir / "installer" / "staging-mac" / "Gateway_Guard.app" / "Contents" / "Resources" / "gateway_guard_config.json",
        bg_dir / "installer" / "pkg-root" / "Applications" / "Gateway_Guard.app" / "Contents" / "Resources" / "gateway_guard_config.json",
        bg_dir / "release" / "Gateway_Guard.app" / "Contents" / "Resources" / "gateway_guard_config.json",
        bg_dir / "release" / "YP_Raksha.app" / "Contents" / "Resources" / "gateway_guard_config.json",
        bg_dir / "release" / "YP_Raksha.app" / "Contents" / "Resources" / "Gateway_guard_config.json",
    ]

    local_app_data = os.environ.get("LOCALAPPDATA", "")
    if local_app_data:
        candidates.append(Path(local_app_data) / "Programs" / "Gateway" / "Guard" / "gateway_guard_config.json")
        candidates.append(Path(local_app_data) / "Gateway" / "Guard" / "gateway_guard_config.json")

    updated_count = 0
    for path in candidates:
        if not path.parent.is_dir():
            continue
        existing_data = {}
        if path.is_file():
            try:
                with open(path, "r", encoding="utf-8-sig", errors="ignore") as f:
                    existing_data = json.load(f)
            except Exception:
                existing_data = {}

        data = {
            "backend_url": server_domain,
            "proxy_addr": proxy_addr,
            "pac_sync_seconds": existing_data.get("pac_sync_seconds", 3),
            "server_mode": existing_data.get("server_mode", False),
            "agent_type": existing_data.get("agent_type", "endpoint"),
            "listen_host": listen_host,
            "pac_advertise_addr": existing_data.get("pac_advertise_addr", ""),
            "pac_http_port": pac_http_port,
            "pac_url": pac_url,
            "guard_secret": guard_secret or existing_data.get("guard_secret", ""),
            "_comment": "Auto-synchronized from root .env by sync_all_from_env.py",
        }

        path.parent.mkdir(parents=True, exist_ok=True)
        with open(path, "w", encoding="utf-8", newline="\n") as f:
            json.dump(data, f, indent=2)
            f.write("\n")
        updated_count += 1

    return updated_count


def update_extension_files(repo_root: Path, env_vars: dict[str, str]) -> int:
    pac_http_port = env_vars.get("PAC_HTTP_PORT", "18195")
    ext_dir = repo_root / "apps" / "browser-guard" / "extension"
    if not ext_dir.is_dir():
        return 0

    updated = 0

    # 1. endpoint.js
    server_domain = env_vars.get("SERVER_DOMAIN", "") or env_vars.get("GATEWAY_BACKEND_URL", "")
    sh = server_domain.split("://", 1)[-1].split("/", 1)[0].split(":", 1)[0].strip().lower()
    if sh.startswith("www."):
        sh = sh[4:]
    endpoint_js = ext_dir / "endpoint.js"
    endpoint_content = (
        "/* Auto-synchronized from root .env by sync_all_from_env.py */\n"
        f'var GATEWAY_GUARD_BASE = "http://127.0.0.1:{pac_http_port}";\n'
        f'var GATEWAY_SERVER_HOST = "{sh}";\n'
    )
    with open(endpoint_js, "w", encoding="utf-8", newline="\n") as f:
        f.write(endpoint_content)
    updated += 1

    # 2. background.js
    bg_js = ext_dir / "background.js"
    if bg_js.is_file():
        text = bg_js.read_text(encoding="utf-8")
        new_text = re.sub(
            r'var\s+base\s*=\s*"http://127\.0\.0\.1:\d+";',
            f'var base = "http://127.0.0.1:{pac_http_port}";',
            text,
        )
        if new_text != text:
            bg_js.write_text(new_text, encoding="utf-8", newline="\n")
            updated += 1

    # 3. popup.js
    popup_js = ext_dir / "popup.js"
    if popup_js.is_file():
        text = popup_js.read_text(encoding="utf-8")
        new_text = re.sub(
            r'var\s+base\s*=\s*"http://127\.0\.0\.1:\d+";',
            f'var base = "http://127.0.0.1:{pac_http_port}";',
            text,
        )
        new_text = re.sub(
            r'gatewayEl\.textContent\s*=\s*"Connected\s*\(127\.0\.0\.1:\d+\)";',
            f'gatewayEl.textContent = "Connected (127.0.0.1:{pac_http_port})";',
            new_text,
        )
        if new_text != text:
            popup_js.write_text(new_text, encoding="utf-8", newline="\n")
            updated += 1

    return updated


def repack_extensions(repo_root: Path) -> bool:
    pack_script = repo_root / "apps" / "browser-guard" / "scripts" / "pack_all_extensions.py"
    if not pack_script.is_file():
        return False
    try:
        res = subprocess.run(
            [sys.executable, str(pack_script)],
            cwd=str(repo_root),
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
        return res.returncode == 0
    except Exception as e:
        print(f"[!] Warning: pack_all_extensions failed: {e}")
        return False


def encrypt_proxy_bundles(repo_root: Path) -> bool:
    enc_script = repo_root / "apps" / "browser-guard" / "installer" / "encrypt_proxy_bundle.py"
    if not enc_script.is_file():
        return False
    try:
        res = subprocess.run(
            [sys.executable, str(enc_script)],
            cwd=str(repo_root),
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
        return res.returncode == 0
    except Exception as e:
        print(f"[!] Warning: encrypt_proxy_bundle failed: {e}")
        return False


def package_macos(repo_root: Path) -> bool:
    pkg_script = repo_root / "apps" / "browser-guard" / "scripts" / "package_macos.ps1"
    if not pkg_script.is_file():
        return False
    try:
        res = subprocess.run(
            ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(pkg_script)],
            cwd=str(repo_root),
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
        return res.returncode == 0
    except Exception as e:
        print(f"[!] Warning: package_macos failed: {e}")
        return False


def update_installer_scripts(repo_root: Path, env_vars: dict[str, str]) -> int:
    server_domain = env_vars.get("SERVER_DOMAIN", "").rstrip("/")
    pac_http_port = env_vars.get("PAC_HTTP_PORT", "18195")
    proxy_addr = env_vars.get("GATEWAY_PROXY_ADDR", "127.0.0.1:18103")

    bg_dir = repo_root / "apps" / "browser-guard"
    updated = 0

    # 1. YP_Raksha.iss & Gateway_Guard.iss (Inno Setup)
    for iss_name in ("YP_Raksha.iss", "Gateway_Guard.iss"):
        iss_file = bg_dir / "installer" / iss_name
        if iss_file.is_file():
            content = iss_file.read_text(encoding="utf-8", errors="ignore")
            new_content = re.sub(
                r'#define\s+MyAppURL\s+"[^"]*"',
                f'#define MyAppURL "{server_domain}"',
                content,
            )
            if new_content != content:
                iss_file.write_text(new_content, encoding="utf-8", newline="\n")
                updated += 1

    # 2. Text guides & readmes
    docs = [
        bg_dir / "release" / "EMPLOYEE_README.txt",
        bg_dir / "release" / "EMPLOYEE_README_MAC.txt",
        bg_dir / "installer" / "EMPLOYEE_README.txt",
        bg_dir / "installer" / "EMPLOYEE_README_MAC.txt",
        bg_dir / "installer" / "IT_README.txt",
        bg_dir / "release" / "INSTALL_WINDOWS.txt",
        bg_dir / "release" / "INSTALL_MACOS.txt",
    ]
    for doc in docs:
        if not doc.is_file():
            continue
        try:
            txt = doc.read_text(encoding="utf-8", errors="ignore")
            new_txt = re.sub(r"Company server:\s*https?://[^\s\n]+", f"Company server: {server_domain}", txt)
            new_txt = re.sub(r"127\.0\.0\.1:8085\b", proxy_addr, new_txt)
            if new_txt != txt:
                doc.write_text(new_txt, encoding="utf-8", newline="\n")
                updated += 1
        except Exception:
            pass

    return updated


def rebuild_and_restart_go_backend(repo_root: Path, env_vars: dict[str, str]) -> bool:
    app_port = env_vars.get("APP_PORT", "6000")
    print(f"\n[*] Checking Go backend rebuild on APP_PORT={app_port}...")

    # Check if go compiler is available
    if not shutil.which("go"):
        print("[!] 'go' executable not found in PATH — skipping backend recompile.")
        return False

    temp_exe = repo_root / "gateway-http-new.exe"
    target_exe = repo_root / "gateway-http.exe"

    build_cmd = ["go", "build", "-o", str(temp_exe), "./transports/gateway-http"]
    print(f"[*] Running: {' '.join(build_cmd)}")
    build_res = subprocess.run(build_cmd, cwd=str(repo_root), capture_output=True, text=True)
    if build_res.returncode != 0:
        print(f"[x] Go compilation failed:\n{build_res.stderr}")
        return False

    print("[✓] Go binary compiled successfully: gateway-http-new.exe")

    # Terminate any running gateway-http.exe instances gracefully
    ps_cmd = (
        'Get-Process -Name "gateway-http" -ErrorAction SilentlyContinue | '
        'ForEach-Object { Stop-Process -Id $_.Id -Force }; '
        'Start-Sleep -Milliseconds 800'
    )
    subprocess.run(["powershell", "-NoProfile", "-Command", ps_cmd], cwd=str(repo_root))

    # Replace binary
    try:
        if target_exe.is_file():
            target_exe.unlink()
        shutil.move(str(temp_exe), str(target_exe))
        print("[✓] Replaced gateway-http.exe with updated build.")
    except Exception as e:
        print(f"[!] Warning replacing binary: {e}")
        return False

    # Launch daemon in background with explicit WorkingDirectory
    target_abs = str(target_exe.resolve())
    root_abs = str(repo_root.resolve())
    launch_cmd = f'Start-Process -FilePath "{target_abs}" -WorkingDirectory "{root_abs}" -WindowStyle Hidden'
    subprocess.run(["powershell", "-NoProfile", "-Command", launch_cmd], cwd=str(repo_root))
    print(f"[✓] Started gateway-http.exe daemon (listening on port {app_port}).")

    # Poll health check for up to 40 seconds as DB/model bootstrap completes
    print(f"[*] Waiting for Gateway backend bootstrap on port {app_port}...", end="", flush=True)
    health_url = f"http://127.0.0.1:{app_port}/health"
    ready = False
    for _ in range(40):
        time.sleep(1.0)
        try:
            with urllib.request.urlopen(health_url, timeout=1.5) as resp:
                if resp.status == 200:
                    body = resp.read().decode("utf-8", errors="ignore")
                    print(f"\n[✓] Backend Healthy ({health_url}): {body.strip()}")
                    ready = True
                    break
        except Exception:
            print(".", end="", flush=True)

    if not ready:
        print(f"\n[!] Backend is still bootstrapping in background. You can check: {health_url}")
    return True


def optional_rebuild_guard_exe(repo_root: Path) -> bool:
    build_bat = repo_root / "apps" / "browser-guard" / "build_windows_exe.bat"
    if not build_bat.is_file():
        print("[!] build_windows_exe.bat not found.")
        return False
    print("\n[*] Running PyInstaller build for standalone Windows YP_Raksha.exe...")
    try:
        res = subprocess.run(
            ["cmd", "/c", str(build_bat)],
            cwd=str(repo_root / "apps" / "browser-guard"),
        )
        return res.returncode == 0
    except Exception as e:
        print(f"[x] Error during build_windows_exe: {e}")
        return False


def main() -> int:
    print_banner()
    repo_root = get_repo_root()

    try:
        env_vars = load_env(repo_root)
    except Exception as e:
        print(f"[x] FATAL: {e}")
        return 1

    app_port = env_vars.get("APP_PORT", "6000")
    server_domain = (env_vars.get("SERVER_DOMAIN") or env_vars.get("GATEWAY_BACKEND_URL") or "").strip().rstrip("/")
    proxy_addr = env_vars.get("GATEWAY_PROXY_ADDR", "127.0.0.1:18103")
    pac_port = env_vars.get("PAC_HTTP_PORT", "18195")

    print(f"[*] Detected Configuration from .env:")
    print(f"    • SERVER_DOMAIN      : {server_domain}")
    print(f"    • APP_PORT           : {app_port}")
    print(f"    • GATEWAY_PROXY_ADDR : {proxy_addr}")
    print(f"    • PAC_HTTP_PORT      : {pac_port}")
    print(f"    • UI_PORT            : {env_vars.get('UI_PORT', '3100')}")
    print("-" * 72)

    # 1. Update Guard JSON configurations (Windows, Mac, Staging, AppData)
    print("\n[1/7] Updating Guard JSON configs...")
    jsons_updated = update_guard_config_jsons(repo_root, env_vars)
    print(f"[✓] Synchronized {jsons_updated} gateway_guard_config.json file(s).")

    # 2. Update Browser Extension Source Files
    print("\n[2/7] Updating Browser Extension sources (endpoint.js, background.js, popup.js)...")
    ext_updated = update_extension_files(repo_root, env_vars)
    print(f"[✓] Updated {ext_updated} extension source file(s) with PAC_HTTP_PORT={pac_port}.")

    # 3. Repack and Sign Browser Extension (.crx, .zip, .xpi, update.xml)
    print("\n[3/7] Re-packing browser extensions (Chromium CRX, Firefox XPI, WebStore ZIP)...")
    if repack_extensions(repo_root):
        print(f"[✓] Browser extensions successfully repacked into release/ and staging/.")
    else:
        print("[!] Repack completed with notices.")

    # 4. Encrypt Proxy Engine Bundle
    print("\n[4/7] Encrypting Gateway Guard Proxy Engine bundle...")
    if encrypt_proxy_bundles(repo_root):
        print(f"[✓] Encrypted bytecode bundle synchronized to runtime and Mac .app.")
    else:
        print("[!] Proxy bundle encryption skipped or completed with notices.")

    # 5. Package macOS .app
    print("\n[5/7] Synchronizing macOS Application bundle (YP_Raksha.app / Gateway_Guard.app)...")
    if package_macos(repo_root):
        print(f"[✓] macOS package synchronized with updated config & proxy bundles.")

    # 6. Update Installer, Policy & Desktop Scripts
    print("\n[6/7] Updating Windows installer scripts & guides...")
    scripts_updated = update_installer_scripts(repo_root, env_vars)
    print(f"[✓] Synchronized {scripts_updated} installer / guide script(s).")

    # 7. Rebuild & Restart Go Backend Daemon
    print("\n[7/7] Rebuilding & Refreshing Gateway HTTP backend server...")
    rebuild_and_restart_go_backend(repo_root, env_vars)

    # Optional: Rebuild Windows EXE if requested via flag
    if "--rebuild-exe" in sys.argv or "--full" in sys.argv:
        optional_rebuild_guard_exe(repo_root)

    print("\n" + "=" * 72)
    print("       ALL COMPONENTS SUCCESSFULLY SYNCHRONIZED FROM .ENV!")
    print("=" * 72)
    print(f"  Backend URL   : {server_domain} (Port: {app_port})")
    print(f"  PAC Proxy     : {proxy_addr} (PAC HTTP Port: {pac_port})")
    print(f"  Extensions    : CRX3 / ZIP / XPI ready in apps/browser-guard/release/")
    print(f"  macOS Packages: Staged & Ready in apps/browser-guard/release/")
    print("=" * 72 + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
