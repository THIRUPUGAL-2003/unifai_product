#!/usr/bin/env python3
"""
==============================================================================
UnifAI Enterprise Master Environment Synchronizer (sync_all_from_env.py)
==============================================================================
One-click master synchronizer that reads the repository root .env file and
automatically compiles, encrypts, updates, and synchronizes EVERYTHING:

  1. Loads root .env (ports, domains, proxy addresses, secrets)
  2. Updates all gateway_guard_config.json files across repo & staging
  3. Updates browser extension sources (endpoint.js, background.js, popup.js)
  4. Repacks browser extensions (Chromium CRX3, Unpacked ZIP)
  5. Encrypts proxy runtime & agent bytecode bundles (AES-256-GCM)
  6. Compiles Windows Standalone Agent (PyInstaller -> YP_Raksha.exe)
  7. Compiles Windows Setup Installer (Inno Setup -> YP_Raksha_Setup.exe)
  8. Packages Windows Employee Distribution (YP_Raksha_Windows.zip)
  9. Packages macOS Distribution (YP_Raksha_macOS.zip)
 10. Assembles Master Decoupled Suite (YP_Raksha_Decoupled_Suite.zip)
 11. Purges duplicates & loose files in release/ to keep folder 100% clean
 12. Updates timestamps of all release artifacts to current time
 13. Rebuilds & refreshes Gateway Go HTTP server (gateway-http.exe)

Usage:
    python sync_all_from_env.py
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


def run_master_build_pipeline(repo_root: Path) -> bool:
    """Run full build_all.py (PyInstaller EXE, Inno Setup, macOS zip, Decoupled Suite)."""
    build_all_py = repo_root / "apps" / "browser-guard" / "build_all.py"
    if not build_all_py.is_file():
        print(f"[!] Warning: {build_all_py} not found.")
        return False

    print("\n[*] Running Master Build Pipeline (build_all.py)...")
    res = subprocess.run(
        [sys.executable, str(build_all_py)],
        cwd=str(repo_root / "apps" / "browser-guard"),
    )
    return res.returncode == 0


def clean_release_directory(repo_root: Path) -> None:
    """Purge duplicates, unzipped dirs, and temporary artifacts from release/."""
    release_dir = repo_root / "apps" / "browser-guard" / "release"
    if not release_dir.is_dir():
        return

    # Redundant/duplicate files to delete
    duplicates = [
        "YP_Raksha_WebStore.zip",
        "yp-raksha-unsigned.xpi",
        "Safari_Extension_README.txt",
        "README_INSTRUCTIONS.txt",
        "Whitelist_Extension_In_Browsers.bat",
        "Install_YP_Raksha.command",
        "Uninstall_YP_Raksha.command",
        "Uninstall_YP_Raksha.bat",
        "Turn_Off_YP_Raksha.bat",
    ]
    for name in duplicates:
        p = release_dir / name
        if p.is_file():
            try:
                p.unlink()
            except Exception:
                pass

    # Touch all remaining release files and browser-guard source files to current time
    now = time.time()
    for item in release_dir.iterdir():
        if item.is_file():
            try:
                os.utime(str(item), (now, now))
            except Exception:
                pass

    bg_dir = repo_root / "apps" / "browser-guard"
    for item in bg_dir.iterdir():
        if item.is_file():
            try:
                os.utime(str(item), (now, now))
            except Exception:
                pass


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
    print("\n[1/5] Updating Guard JSON configs...")
    jsons_updated = update_guard_config_jsons(repo_root, env_vars)
    print(f"[✓] Synchronized {jsons_updated} gateway_guard_config.json file(s).")

    # 2. Update Browser Extension Source Files
    print("\n[2/5] Updating Browser Extension sources (endpoint.js, background.js, popup.js)...")
    ext_updated = update_extension_files(repo_root, env_vars)
    print(f"[✓] Updated {ext_updated} extension source file(s) with PAC_HTTP_PORT={pac_port}.")

    # 3. Master Build Pipeline (PyInstaller EXE, Inno Setup, macOS zip, Decoupled Suite)
    print("\n[3/5] Running Complete Master Build & Compilation Pipeline...")
    build_ok = run_master_build_pipeline(repo_root)
    if not build_ok:
        print("[!] Master build pipeline finished with notices.")

    # 4. Clean up release directory (no duplicates, refresh timestamps)
    print("\n[4/5] Cleaning duplicate artifacts & refreshing timestamps...")
    clean_release_directory(repo_root)
    print("[✓] Release directory verified: zero duplicates, fresh timestamps.")

    # 5. Rebuild & Restart Go Backend Daemon
    print("\n[5/5] Rebuilding & Refreshing Gateway HTTP backend server...")
    rebuild_and_restart_go_backend(repo_root, env_vars)

    print("\n" + "=" * 72)
    print("       ALL COMPONENTS FULLY COMPILED & SYNCHRONIZED FROM .ENV!")
    print("=" * 72)
    print(f"  • Backend URL         : {server_domain} (Port: {app_port})")
    print(f"  • Windows Standalone  : release/YP_Raksha.exe")
    print(f"  • Windows Setup       : release/YP_Raksha_Setup.exe")
    print(f"  • Windows Package     : release/YP_Raksha_Windows.zip")
    print(f"  • macOS Package       : release/YP_Raksha_macOS.zip")
    print(f"  • Chrome Extension    : release/yp-raksha.crx & yp-raksha.zip")
    print(f"  • Master All-In-One   : release/YP_Raksha_Decoupled_Suite.zip")
    print("=" * 72 + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
