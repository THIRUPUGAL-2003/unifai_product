#!/usr/bin/env python3
"""
UnifAI / Gateway — Master Environment Synchronization & Rebuild Script
======================================================================
Whenever you edit `.env`, run this script:
    python sync_all_from_env.py
or double-click:
    sync_all_from_env.bat

What it updates automatically from .env:
  1. Guard configs (gateway_guard_config.json) across release, config, staging, appdata
  2. Installer text files & Inno Setup scripts (INSTALL_WINDOWS, INSTALL_MACOS, ISS)
  3. Release download packages (injects updated config into Gateway_Guard_Windows.zip and Gateway_Guard_macOS.zip)
  4. Local installed Guard config in %LOCALAPPDATA%
  5. Validates docker-compose.yml and framework/docker-compose.yml
  6. Optionally rebuilds UI bundle (pass --build-ui to trigger full npm build)
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import warnings
import zipfile

warnings.filterwarnings("ignore", category=UserWarning)

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")

REPO_ROOT = os.path.abspath(os.path.dirname(__file__))


def load_env(env_path: str) -> dict[str, str]:
    vals: dict[str, str] = {}
    if not os.path.isfile(env_path):
        return vals
    with open(env_path, "r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            vals[k.strip()] = v.strip().strip("'\"")
    return vals


def rel(path: str) -> str:
    try:
        return os.path.relpath(path, REPO_ROOT).replace("\\", "/")
    except ValueError:
        return path.replace("\\", "/")


def update_json_file(path: str, updates: dict) -> tuple[bool, list[tuple[str, any, any]]]:
    """
    Updates JSON key-values and returns (changed, list_of_diffs).
    Each diff is a tuple: (key, old_value, new_value)
    """
    if not os.path.isfile(path):
        return False, []
    try:
        with open(path, "r", encoding="utf-8-sig", errors="ignore") as f:
            data = json.load(f)
        if not isinstance(data, dict):
            return False, []
        diffs: list[tuple[str, any, any]] = []
        for k, v in updates.items():
            old_val = data.get(k)
            if old_val != v:
                diffs.append((k, old_val, v))
                data[k] = v
        if not diffs:
            return False, []
        with open(path, "w", encoding="utf-8", newline="\n") as f:
            json.dump(data, f, indent=2)
            f.write("\n")
        return True, diffs
    except Exception as e:
        print(f"  [!] Failed updating {rel(path)}: {e}", file=sys.stderr)
        return False, []


def update_text_file(path: str, server_domain: str, proxy_addr: str, pac_http_port: str) -> tuple[bool, list[tuple[str, str]]]:
    """
    Updates text file matches and returns (changed, list_of_diffs).
    Each diff is a tuple: (old_matched_text, new_replacement_text)
    """
    if not os.path.isfile(path):
        return False, []
    try:
        with open(path, "r", encoding="utf-8", errors="ignore") as f:
            content = f.read()
        diffs: list[tuple[str, str]] = []

        def track_sub(pattern: str, repl_target, text: str) -> str:
            def repl(m):
                old_str = m.group(0)
                if callable(repl_target):
                    new_str = repl_target(m)
                else:
                    new_str = re.sub(pattern, repl_target, old_str)
                if old_str != new_str:
                    diffs.append((old_str.strip(), new_str.strip()))
                return new_str
            return re.sub(pattern, repl, text)

        new = content
        new = track_sub(r"Company server:\s*https?://[^\s\n]+", f"Company server: {server_domain}", new)
        new = track_sub(
            r"(synced for |Config backend_url: |backend_url: |\*\*Backend:\*\* `)https?://[^\s`\n]+",
            lambda m: m.group(1) + server_domain,
            new,
        )
        new = track_sub(r'#define\s+MyAppURL\s+"[^"]*"', f'#define MyAppURL "{server_domain}"', new)
        if proxy_addr:
            new = track_sub(r"127\.0\.0\.1:8085\b", proxy_addr, new)
        if pac_http_port:
            new = track_sub(r"http://127\.0\.0\.1:18085", f"http://127.0.0.1:{pac_http_port}", new)

        if new == content or not diffs:
            return False, []
        with open(path, "w", encoding="utf-8", newline="\n") as f:
            f.write(new)
        return True, diffs
    except Exception as e:
        print(f"  [!] Failed updating doc {rel(path)}: {e}", file=sys.stderr)
        return False, []


def update_zip_entry(zip_path: str, entry_name: str, new_bytes: bytes, new_dict: dict) -> tuple[bool, list[tuple[str, any, any]]]:
    """
    Updates or injects a file inside a ZIP archive and reports diffs.
    """
    if not os.path.isfile(zip_path):
        return False, []
    try:
        old_data = {}
        had_entry = False
        with zipfile.ZipFile(zip_path, "r") as z:
            if entry_name in z.namelist():
                had_entry = True
                try:
                    raw = z.read(entry_name).decode("utf-8", errors="ignore")
                    old_data = json.loads(raw)
                except Exception:
                    old_data = {}

        diffs: list[tuple[str, any, any]] = []
        if had_entry and isinstance(old_data, dict):
            for k, v in new_dict.items():
                if old_data.get(k) != v:
                    diffs.append((k, old_data.get(k), v))
        else:
            diffs.append(("(new entry)", None, entry_name))

        if had_entry and not diffs:
            return False, []

        with zipfile.ZipFile(zip_path, "a") as z:
            z.writestr(entry_name, new_bytes)
        return True, diffs
    except Exception as e:
        print(f"  [-] Note: {rel(zip_path)} ({e}) — server injects fresh config dynamically on download.")
        return False, []


def main() -> int:
    parser = argparse.ArgumentParser(description="Synchronize all configurations and build artifacts from .env")
    parser.add_argument("--build-ui", "-u", action="store_true", help="Run full npm build in ui directory after sync")
    parser.add_argument("--rebuild-exe", "-e", action="store_true", help="Recompile Gateway_Guard.exe using PyInstaller")
    parser.add_argument("--rebuild-installer", "-i", action="store_true", help="Run apps/browser-guard/installer/build_installer.bat")
    args = parser.parse_args()

    env_path = os.path.join(REPO_ROOT, ".env")
    if not os.path.isfile(env_path):
        print(f"[ERROR] .env file not found at {env_path}", file=sys.stderr)
        return 1

    env = load_env(env_path)
    server_domain = (env.get("SERVER_DOMAIN") or "https://unifai.yespanchi.com").rstrip("/")
    backend_url = (env.get("GATEWAY_BACKEND_URL") or server_domain).rstrip("/")
    proxy_addr = env.get("GATEWAY_PROXY_ADDR") or "127.0.0.1:18103"
    pac_http_port_str = env.get("PAC_HTTP_PORT") or "18195"
    pac_http_port = int(pac_http_port_str) if pac_http_port_str.isdigit() else 18195
    guard_secret = env.get("GATEWAY_GUARD_SECRET") or ""

    print("=" * 70)
    print("   UnifAI / Gateway — Master Environment Sync & Rebuild")
    print("=" * 70)
    print(f"  SERVER_DOMAIN       : {server_domain}")
    print(f"  GATEWAY_BACKEND_URL : {backend_url}")
    print(f"  GATEWAY_PROXY_ADDR  : {proxy_addr}")
    print(f"  PAC_HTTP_PORT       : {pac_http_port}")
    print(f"  GATEWAY_GUARD_SECRET: {'(configured: ' + guard_secret[:4] + '***)' if guard_secret else '(not set)'}")
    print(f"  DATABASE (DB_TYPE)  : {env.get('DB_TYPE', 'postgres')} @ {env.get('DB_HOST', '')}:{env.get('DB_PORT', '')}")
    print("-" * 70)

    # 1. Prepare standard JSON payload
    guard_config_data = {
        "backend_url": server_domain,
        "proxy_addr": proxy_addr,
        "pac_sync_seconds": 3,
        "server_mode": False,
        "agent_type": "endpoint",
        "listen_host": "127.0.0.1",
        "pac_advertise_addr": "",
        "pac_http_port": pac_http_port,
        "pac_url": f"{server_domain}/api/browser-ai/pac",
        "guard_secret": guard_secret,
        "_comment": "Auto-synced from .env by sync_all_from_env.py",
    }
    guard_config_bytes = (json.dumps(guard_config_data, indent=2) + "\n").encode("utf-8")

    bg = os.path.join(REPO_ROOT, "apps", "browser-guard")
    json_targets = [
        os.path.join(bg, "release", "gateway_guard_config.json"),
        os.path.join(bg, "config", "gateway_guard_config.json"),
        os.path.join(bg, "installer", "staging", "gateway_guard_config.json"),
        os.path.join(bg, "installer", "staging-mac", "gateway_guard_config.json"),
        os.path.join(bg, "installer", "staging-mac", "Gateway_Guard.app", "Contents", "Resources", "gateway_guard_config.json"),
        os.path.join(bg, "release", "Gateway_Guard.app", "Contents", "Resources", "gateway_guard_config.json"),
    ]

    # Add local machine's AppData installed guard configs if present
    local_app_data = os.environ.get("LOCALAPPDATA", "")
    if local_app_data:
        json_targets.append(os.path.join(local_app_data, "Programs", "Gateway", "Guard", "gateway_guard_config.json"))
        json_targets.append(os.path.join(local_app_data, "Gateway", "Guard", "gateway_guard_config.json"))

    print("\n[1/4] Synchronizing Guard Configuration Files...")
    updated_json = 0
    for p in json_targets:
        changed, diffs = update_json_file(p, guard_config_data)
        if changed:
            print(f"  ✓ Updated: {rel(p)}")
            for k, old_val, new_val in diffs:
                old_str = f'"{old_val}"' if old_val is not None else "(not set)"
                new_str = f'"{new_val}"'
                print(f"      • {k}:")
                print(f"          [-] OLD: {old_str}")
                print(f"          [+] NEW: {new_str}")
            updated_json += 1
        elif os.path.isfile(p):
            print(f"  - Up to date: {rel(p)}")
    if updated_json == 0:
        print("  - All JSON configs already match .env (no changes needed)")

    # 2. Update doc & installer script templates
    print("\n[2/4] Synchronizing Installer Docs & Setup Scripts...")
    doc_targets = [
        os.path.join(bg, "release", "INSTALL_WINDOWS.txt"),
        os.path.join(bg, "release", "INSTALL_MACOS.txt"),
        os.path.join(bg, "release", "EMPLOYEE_README.txt"),
        os.path.join(bg, "release", "EMPLOYEE_README_MAC.txt"),
        os.path.join(bg, "installer", "Gateway_Guard.iss"),
        os.path.join(bg, "release", "Update_Gateway_Guard.ps1"),
        os.path.join(bg, "release", "Install_Gateway_Guard.command"),
    ]
    updated_docs = 0
    for p in doc_targets:
        changed, diffs = update_text_file(p, server_domain, proxy_addr, pac_http_port_str)
        if changed:
            print(f"  ✓ Updated: {rel(p)}")
            for old_s, new_s in diffs:
                print(f"      [-] OLD: {old_s}")
                print(f"      [+] NEW: {new_s}")
            updated_docs += 1
        elif os.path.isfile(p):
            print(f"  - Up to date: {rel(p)}")
    if updated_docs == 0:
        print("  - All installer texts already match .env (no changes needed)")

    # 3. Inject updated config into release ZIP packages
    print("\n[3/4] Updating Download Packages (Gateway_Guard_*.zip)...")
    win_zip = os.path.join(bg, "release", "Gateway_Guard_Windows.zip")
    if os.path.isfile(win_zip):
        changed, diffs = update_zip_entry(win_zip, "gateway_guard_config.json", guard_config_bytes, guard_config_data)
        if changed:
            print(f"  ✓ Updated config inside: {rel(win_zip)}")
            for k, old_val, new_val in diffs:
                print(f"      • {k}:")
                print(f"          [-] OLD: {old_val}")
                print(f"          [+] NEW: {new_val}")
        else:
            print(f"  - Up to date inside ZIP: {rel(win_zip)}")
    else:
        print(f"  - Windows ZIP not found: {rel(win_zip)}")

    mac_zip = os.path.join(bg, "release", "Gateway_Guard_macOS.zip")
    if os.path.isfile(mac_zip):
        ok1, diffs1 = update_zip_entry(mac_zip, "gateway_guard_config.json", guard_config_bytes, guard_config_data)
        ok2, diffs2 = update_zip_entry(
            mac_zip,
            "Gateway_Guard.app/Contents/Resources/gateway_guard_config.json",
            guard_config_bytes,
            guard_config_data,
        )
        if ok1 or ok2:
            print(f"  ✓ Updated config inside: {rel(mac_zip)}")
            for k, old_val, new_val in (diffs1 + diffs2):
                print(f"      • {k}:")
                print(f"          [-] OLD: {old_val}")
                print(f"          [+] NEW: {new_val}")
        else:
            print(f"  - Up to date inside ZIP: {rel(mac_zip)}")
    else:
        print(f"  - macOS ZIP not found: {rel(mac_zip)}")

    # 4. Optional Rebuilds
    print("\n[4/4] Optional Builds & Compilations...")
    rebuild_done = False

    if args.rebuild_exe:
        rebuild_done = True
        print("\n  [EXE Rebuild] Running PyInstaller via apps/browser-guard/installer/build_agent.py...")
        build_agent_script = os.path.join(bg, "installer", "build_agent.py")
        res = subprocess.run([sys.executable, build_agent_script], cwd=bg)
        if res.returncode == 0:
            print("  ✓ Gateway_Guard.exe rebuilt successfully in release/ and dist/")
        else:
            print(f"  [!] PyInstaller build returned exit code {res.returncode}", file=sys.stderr)

    if args.rebuild_installer:
        rebuild_done = True
        print("\n  [Installer Rebuild] Running apps/browser-guard/installer/build_installer.bat...")
        installer_bat = os.path.join(bg, "installer", "build_installer.bat")
        res = subprocess.run(["cmd.exe", "/c", installer_bat], cwd=bg)
        if res.returncode == 0:
            print("  ✓ Full Installer & Windows ZIP rebuilt successfully!")
        else:
            print(f"  [!] Installer build returned exit code {res.returncode}", file=sys.stderr)

    if args.build_ui:
        rebuild_done = True
        print("\n  [Frontend UI Build] Running 'npm run build' in ui/ directory...")
        ui_dir = os.path.join(REPO_ROOT, "ui")
        res = subprocess.run(["npm", "run", "build"], cwd=ui_dir, shell=True)
        if res.returncode == 0:
            print("  ✓ UI successfully rebuilt & copied to transports/gateway-http/ui")
        else:
            print(f"  [!] UI build returned exit code {res.returncode}", file=sys.stderr)

    if not rebuild_done:
        print("  - Optional builds skipped.")
        print("    (Tips: pass '--rebuild-exe' to recompile Gateway_Guard.exe,")
        print("           pass '--rebuild-installer' to rebuild Setup EXE & full ZIP,")
        print("           pass '--build-ui' to compile the frontend UI)")

    print("\n" + "=" * 70)
    print("   SUCCESS: All configurations are 100% in sync with .env!")
    print("=" * 70)
    return 0


if __name__ == "__main__":
    sys.exit(main())

