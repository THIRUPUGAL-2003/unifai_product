#!/usr/bin/env python3
"""
Build the Master Decoupled Guard Suite:
- Windows: Standalone YP_Raksha.exe (Zero Setup Wizard, embedded config, zero extension bundling)
- macOS: Standalone YP_Raksha_macOS.zip + install scripts
- Extensions: Unpacked ZIP for Chrome/Brave/Edge, signed CRX, and Firefox XPI
"""

import os
import sys
import zipfile
import shutil

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")

script_dir = os.path.abspath(os.path.dirname(__file__))
bg_dir = os.path.dirname(script_dir)
release_dir = os.path.join(bg_dir, "release")
ext_dir = os.path.join(bg_dir, "extension")

print("============================================================")
print(" 1. Syncing config from .env")
print("============================================================")
sync_script = os.path.join(script_dir, "sync_config_from_env.py")
if os.path.isfile(sync_script):
    import subprocess
    subprocess.run([sys.executable, sync_script], check=True)

print("\n============================================================")
print(" 2. Packaging all browser extensions")
print("============================================================")
pack_script = os.path.join(script_dir, "pack_all_extensions.py")
if os.path.isfile(pack_script):
    import subprocess
    subprocess.run([sys.executable, pack_script], check=True)

# Copy policy allowlist bat
whitelist_bat = os.path.join(bg_dir, "installer", "Whitelist_Extension_In_Browsers.bat")
if os.path.isfile(whitelist_bat):
    shutil.copy2(whitelist_bat, os.path.join(release_dir, "Whitelist_Extension_In_Browsers.bat"))

print("\n============================================================")
print(" 3. Packaging Master Decoupled Suite ZIP")
print("============================================================")

suite_manifest = {
    "1_Windows": [
        ("YP_Raksha.exe", "YP_Raksha.exe"),
    ],
    "2_macOS": [
        ("YP_Raksha_macOS.zip", "YP_Raksha_macOS.zip"),
        ("Install_YP_Raksha.command", "Install_YP_Raksha.command"),
        ("Uninstall_YP_Raksha.command", "Uninstall_YP_Raksha.command"),
        ("INSTALL_MACOS.txt", "INSTALL_MACOS.txt"),
    ],
    "3_Browser_Extensions": [
        ("yp-raksha.crx", "Chromium_Extension_Chrome_Edge_Brave_Opera.crx"),
        ("yp-raksha.zip", "Unpacked_Extension_Chrome_Edge_Brave.zip"),
        ("Whitelist_Extension_In_Browsers.bat", "Whitelist_Extension_In_Browsers.bat"),
        ("yp-raksha.xpi", "Firefox_Extension.xpi"),
        ("Safari_Extension_README.txt", "Safari_Extension_README.txt"),
    ],
    "": [
        ("README_INSTRUCTIONS.txt", "README_INSTRUCTIONS.txt"),
    ]
}

# Ensure READMEs exist
readme_file = os.path.join(release_dir, "README_INSTRUCTIONS.txt")
if not os.path.isfile(readme_file):
    with open(readme_file, "w", encoding="utf-8") as f:
        f.write("UnifAI Enterprise AI Security Guard - Deployment Suite\n")

out_zip = os.path.join(release_dir, "YP_Raksha_Decoupled_Suite.zip")
with zipfile.ZipFile(out_zip, "w", zipfile.ZIP_DEFLATED) as zf:
    for folder, files in suite_manifest.items():
        for src_name, dst_name in files:
            src_path = os.path.join(release_dir, src_name)
            if not os.path.isfile(src_path):
                # Try installer folder for scripts
                installer_src = os.path.join(bg_dir, "installer", src_name)
                if os.path.isfile(installer_src):
                    src_path = installer_src
            if os.path.isfile(src_path):
                arc_name = f"{folder}/{dst_name}" if folder else dst_name
                zf.write(src_path, arc_name)
                print(f"  + Added: {arc_name} ({os.path.getsize(src_path):,} bytes)")
            else:
                print(f"  ! Warning: {src_name} not found, skipped")

# Verify zip integrity
with zipfile.ZipFile(out_zip, "r") as zf:
    bad_file = zf.testzip()
    if bad_file:
        raise RuntimeError(f"ZIP integrity test failed on {bad_file}")

size_mb = os.path.getsize(out_zip) / (1024 * 1024)
print(f"\n[SUCCESS] Master Suite ready: {out_zip} ({size_mb:.2f} MB)")
