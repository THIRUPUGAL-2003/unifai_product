#!/usr/bin/env python3
"""
Build the single employee download (UnifAI_Guard_Suite.zip) served by Browser AI -> Setup:

  1_Windows/           YP_Raksha_Setup.exe (installs EXE, config, autostart, updates, secure uninstall)
                       YP_Raksha.exe       (portable EXE, same agent without the installer)
  2_Browser_Extension/ YP_Raksha_Extension/ (ready for "Load unpacked" in Chrome / Edge / Brave / Opera)
                       Chromium .crx, Firefox .xpi (only when Mozilla-signed), Safari guide, whitelist .bat
  3_macOS/             YP_Raksha_macOS.zip + Install / Uninstall .command
  README_INSTRUCTIONS.txt

The EXE and the extension are installed separately and find each other on 127.0.0.1:18195.

Usage:
  python scripts/build_master_suite.py              sync config, repack extensions, then build the ZIP
  python scripts/build_master_suite.py --skip-prep  only build the ZIP (installer build already did the prep)
"""

import json
import os
import subprocess
import sys
import zipfile

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")

script_dir = os.path.abspath(os.path.dirname(__file__))
bg_dir = os.path.dirname(script_dir)
release_dir = os.path.join(bg_dir, "release")
installer_dir = os.path.join(bg_dir, "installer")
skip_prep = "--skip-prep" in sys.argv[1:]

WINDOWS = "windows"
EXTENSION = "extension"
MACOS = "mac"
UNPACKED = f"{EXTENSION}/YP_Raksha_Extension"


def find(name: str) -> str:
    for base in (release_dir, installer_dir):
        path = os.path.join(base, name)
        if os.path.isfile(path):
            return path
    return ""


if not skip_prep:
    print("============================================================")
    print(" 1. Syncing config from .env and packaging browser extensions")
    print("============================================================")
    for step in ("sync_config_from_env.py", "pack_all_extensions.py"):
        path = os.path.join(script_dir, step)
        if os.path.isfile(path):
            subprocess.run([sys.executable, path], check=True)

# Required: a suite without these is not something an employee can install from.
required = {
    f"{WINDOWS}/YP_Raksha_Setup.exe": "YP_Raksha_Setup.exe",
    f"{WINDOWS}/YP_Raksha.exe": "YP_Raksha.exe",
    f"{WINDOWS}/gateway_guard_config.json": "gateway_guard_config.json",
    f"{EXTENSION}/Chromium_Extension_Chrome_Edge_Brave_Opera.crx": "yp-raksha.crx",
    f"{MACOS}/YP_Raksha_macOS.zip": "YP_Raksha_macOS.zip",
}
optional = {
    f"{WINDOWS}/INSTALL_WINDOWS.txt": "INSTALL_WINDOWS.txt",
    f"{EXTENSION}/Unpacked_Extension_Chrome_Edge_Brave.zip": "yp-raksha.zip",
    f"{EXTENSION}/Whitelist_Extension_In_Browsers.bat": "Whitelist_Extension_In_Browsers.bat",
    f"{EXTENSION}/Safari_Extension_README.txt": "Safari_Extension_README.txt",
    f"{MACOS}/Install_YP_Raksha.command": "Install_YP_Raksha.command",
    f"{MACOS}/Uninstall_YP_Raksha.command": "Uninstall_YP_Raksha.command",
    f"{MACOS}/INSTALL_MACOS.txt": "INSTALL_MACOS.txt",
    f"{MACOS}/gateway_guard_config.json": "gateway_guard_config.json",
}
# Firefox only accepts a Mozilla-signed add-on; never ship an unsigned one to employees.
xpi = find("yp-raksha.xpi")
if xpi:
    sys.path.insert(0, os.path.join(bg_dir, "agent"))
    try:
        from agent_extension import is_signed_firefox_xpi
        if is_signed_firefox_xpi(xpi):
            optional[f"{EXTENSION}/Firefox_Extension.xpi"] = "yp-raksha.xpi"
    except Exception as e:
        print(f"  ! Firefox package check skipped: {e}")

missing = [src for src in required.values() if not find(src)]
unpacked_zip = find("yp-raksha.zip")
if not unpacked_zip:
    missing.append("yp-raksha.zip (unpacked extension)")
if missing:
    raise SystemExit("[ERROR] Cannot build suite, missing: " + ", ".join(missing))

version = ""
version_file = find("VERSION.txt")
if version_file:
    with open(version_file, encoding="utf-8") as f:
        version = f.read().strip()

README = f"""UnifAI YP-Raksha Enterprise AI Security Guard {version}
==============================================================

The Desktop Agent (EXE) and the Browser Extension are installed separately.
Each works on its own; installed together they connect automatically within a second.

1. WINDOWS  (folder: windows/)
   Recommended: double-click YP_Raksha_Setup.exe. One install sets up everything:
     - YP_Raksha.exe, company configuration, start at Windows login
     - automatic updates from the company server
     - secure uninstall (needs the company uninstall key or IT removal from the website)
   Silent / mass deployment: YP_Raksha_Setup.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART
   YP_Raksha.exe is the same agent without the installer (portable).
   Without the extension, the agent alone still checks prompts and file uploads on the
   company Target Websites and records Prompt Logs and Search Logs.

2. BROWSER EXTENSION  (folder: extension/)
   Chrome / Edge / Brave / Opera:
     1. Open chrome://extensions (edge://extensions, brave://extensions).
     2. Turn ON 'Developer mode'.
     3. Click 'Load unpacked' and select the 'extension' folder (or extension/YP_Raksha_Extension).
   The extension only acts on the company Target Websites and connects to the agent on
   127.0.0.1:18195. Policy changes made on the website reach the browser in about a second.
   Firefox: install Firefox_Extension.xpi (only included when Mozilla-signed).
   Safari: see Safari_Extension_README.txt.

3. macOS  (folder: mac/)
   Extract YP_Raksha_macOS.zip and double-click Install_YP_Raksha.command
   (right-click -> Open if Gatekeeper blocks it). Uninstall: Uninstall_YP_Raksha.command.

4. UNINSTALL
   - From the website: Browser AI -> Guard Agents -> Uninstall. The laptop removes the agent
     within a few seconds and the extension drops all cached policy.
   - On the laptop: Settings -> Apps -> YP Raksha -> Uninstall, then enter the company uninstall key.
"""

print("\n============================================================")
print(" 2. Packaging UnifAI_Guard_Suite (YP_Raksha_Decoupled_Suite.zip)")
print("============================================================")

out_zip = os.path.join(release_dir, "YP_Raksha_Decoupled_Suite.zip")
tmp_zip = out_zip + ".tmp"
with zipfile.ZipFile(tmp_zip, "w", zipfile.ZIP_DEFLATED) as zf:
    for arc, src in list(required.items()) + list(optional.items()):
        path = find(src)
        if path:
            zf.write(path, arc)
            print(f"  + {arc} ({os.path.getsize(path):,} bytes)")
        else:
            print(f"  ! {src} not found, skipped")
    # Extension files placed both directly in extension/ and extension/YP_Raksha_Extension/
    # so employees can select either when clicking 'Load unpacked'.
    with zipfile.ZipFile(unpacked_zip) as ext:
        manifest = json.loads(ext.read("manifest.json"))
        for info in ext.infolist():
            if info.is_dir():
                continue
            data = ext.read(info.filename)
            zf.writestr(f"{EXTENSION}/{info.filename}", data)
            zf.writestr(f"{UNPACKED}/{info.filename}", data)
        print(f"  + {EXTENSION}/ (extension {manifest.get('version')}, {len(ext.infolist())} files)")
    zf.writestr("README_INSTRUCTIONS.txt", README)
    print("  + README_INSTRUCTIONS.txt")

with zipfile.ZipFile(tmp_zip) as zf:
    bad = zf.testzip()
    if bad:
        os.remove(tmp_zip)
        raise RuntimeError(f"ZIP integrity test failed on {bad}")
os.replace(tmp_zip, out_zip)

size_mb = os.path.getsize(out_zip) / (1024 * 1024)
print(f"\n[SUCCESS] Suite ready: {out_zip} ({size_mb:.2f} MB)")

