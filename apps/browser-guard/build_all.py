#!/usr/bin/env python3
"""
================================================================================
YP-Raksha / Browser Guard - Master Unified Build & Packaging Pipeline
================================================================================

One single script to build, encrypt, package, and verify everything:
  1. Sync configuration from repo root .env (sync_config_from_env.py)
  2. Package browser extensions (.crx, .zip, .xpi) (pack_all_extensions.py)
  3. Encrypt proxy runtime and agent code (encrypt_proxy_bundle.py)
  4. Compile Windows standalone agent YP_Raksha.exe (build_agent.py)
  5. Compile Inno Setup installer YP_Raksha_Setup.exe (ISCC.exe on YP_Raksha.iss)
  6. Package Windows employee zip (YP_Raksha_Windows.zip)
  7. Stage and package macOS suite (YP_Raksha_macOS.zip)
  8. Assemble master decoupled suite (YP_Raksha_Decoupled_Suite.zip)
     with:
       - windows/ (Setup EXE, portable EXE, config, uninstaller)
       - extension/ (.crx, unpacked extension zip, folder, whitelist bat)
       - mac/ (macOS zip, install/uninstall scripts, config)
       - README_INSTRUCTIONS.txt
  9. Run integrity verification checks on all generated artifacts.

Usage:
  python build_all.py
  python apps/browser-guard/build_all.py
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import time
import zipfile
from pathlib import Path

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")

# Resolve directories
SCRIPT_DIR = Path(__file__).resolve().parent
if SCRIPT_DIR.name == "browser-guard":
    BG_DIR = SCRIPT_DIR
    REPO_DIR = BG_DIR.parents[1]
else:
    BG_DIR = SCRIPT_DIR / "apps" / "browser-guard"
    REPO_DIR = SCRIPT_DIR

SCRIPTS_DIR = BG_DIR / "scripts"
INSTALLER_DIR = BG_DIR / "installer"
RELEASE_DIR = BG_DIR / "release"
PROXY_DIR = BG_DIR / "proxy"
CONFIG_DIR = BG_DIR / "config"
AGENT_DIR = BG_DIR / "agent"


def find_iscc() -> Path | None:
    candidates = [
        Path(os.path.expandvars(r"%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe")),
        Path(os.path.expandvars(r"%USERPROFILE%\AppData\Local\Programs\Inno Setup 6\ISCC.exe")),
        Path(r"C:\Program Files (x86)\Inno Setup 6\ISCC.exe"),
        Path(r"C:\Program Files\Inno Setup 6\ISCC.exe"),
    ]
    for c in candidates:
        if c.is_file():
            return c
    return None


def run_py(script_path: Path, args: list[str] | None = None, cwd: Path | None = None) -> None:
    cmd = [sys.executable, str(script_path)] + (args or [])
    res = subprocess.run(cmd, cwd=cwd or BG_DIR)
    if res.returncode != 0:
        raise SystemExit(f"\n[ERROR] Step failed with exit code {res.returncode}: {script_path.name}")


def step_banner(num: int, title: str) -> None:
    print("\n" + "=" * 70)
    print(f" STEP {num}: {title}")
    print("=" * 70)


def build_inno_setup(iscc: Path) -> Path:
    iss_file = INSTALLER_DIR / "YP_Raksha.iss"
    staging_dir = INSTALLER_DIR / "staging"
    output_dir = INSTALLER_DIR / "output"
    staging_dir.mkdir(parents=True, exist_ok=True)
    output_dir.mkdir(parents=True, exist_ok=True)

    # Stage required binaries & assets
    built_exe = BG_DIR / "build" / "exe" / "YP_Raksha.exe"
    if not built_exe.is_file():
        built_exe = RELEASE_DIR / "YP_Raksha.exe"
    if not built_exe.is_file():
        raise SystemExit("[ERROR] YP_Raksha.exe not found to stage for Inno Setup")

    shutil.copy2(built_exe, staging_dir / "YP_Raksha.exe")

    config_src = RELEASE_DIR / "gateway_guard_config.json"
    if not config_src.is_file():
        config_src = CONFIG_DIR / "gateway_guard_config.json"
    shutil.copy2(config_src, staging_dir / "gateway_guard_config.json")

    ico_src = BG_DIR / "yp_raksha.ico"
    if ico_src.is_file():
        shutil.copy2(ico_src, staging_dir / "yp_raksha.ico")
        shutil.copy2(ico_src, INSTALLER_DIR / "yp_raksha.ico")

    for opt in ["VERSION.txt", "INSTALL_WINDOWS.txt"]:
        f = RELEASE_DIR / opt
        if f.is_file():
            shutil.copy2(f, staging_dir / opt)

    uninstaller_src = INSTALLER_DIR / "Uninstall_YP_Raksha.bat"
    if uninstaller_src.is_file():
        shutil.copy2(uninstaller_src, staging_dir / "Uninstall_YP_Raksha.bat")

    readme_src = INSTALLER_DIR / "EMPLOYEE_README.txt"
    if readme_src.is_file():
        shutil.copy2(readme_src, staging_dir / "EMPLOYEE_README.txt")

    # Run ISCC
    print(f"Running Inno Setup compiler: {iscc}")
    out_setup = output_dir / "YP_Raksha_Setup.exe"
    if out_setup.is_file():
        try:
            out_setup.unlink()
        except Exception:
            pass

    cmd = [str(iscc), str(iss_file)]
    res = subprocess.run(cmd, cwd=INSTALLER_DIR)
    if res.returncode != 0:
        raise SystemExit(f"[ERROR] Inno Setup compiler failed with code {res.returncode}")

    if not out_setup.is_file():
        raise SystemExit(f"[ERROR] Inno Setup reported success but {out_setup} does not exist")

    # Copy to release
    dest = RELEASE_DIR / "YP_Raksha_Setup.exe"
    shutil.copy2(out_setup, dest)
    print(f"  [OK] Compiled: {dest} ({dest.stat().st_size:,} bytes)")
    return dest


def package_windows_zip() -> Path:
    out_zip = RELEASE_DIR / "YP_Raksha_Windows.zip"
    stage_dir = INSTALLER_DIR / "staging-win-zip"
    if stage_dir.is_dir():
        shutil.rmtree(stage_dir, ignore_errors=True)
    stage_dir.mkdir(parents=True, exist_ok=True)

    files_to_copy = [
        (RELEASE_DIR / "YP_Raksha_Setup.exe", "YP_Raksha_Setup.exe"),
        (RELEASE_DIR / "YP_Raksha.exe", "YP_Raksha.exe"),
        (RELEASE_DIR / "gateway_guard_config.json", "gateway_guard_config.json"),
        (RELEASE_DIR / "INSTALL_WINDOWS.txt", "INSTALL_WINDOWS.txt"),
        (RELEASE_DIR / "VERSION.txt", "VERSION.txt"),
        (INSTALLER_DIR / "EMPLOYEE_README.txt", "EMPLOYEE_README.txt"),
        (INSTALLER_DIR / "Uninstall_YP_Raksha.bat", "Uninstall_YP_Raksha.bat"),
        (INSTALLER_DIR / "Whitelist_Extension_In_Browsers.bat", "Whitelist_Extension_In_Browsers.bat"),
    ]

    for src, name in files_to_copy:
        if src.is_file():
            shutil.copy2(src, stage_dir / name)

    tmp_zip = out_zip.with_suffix(".tmp")
    if tmp_zip.is_file():
        tmp_zip.unlink()

    with zipfile.ZipFile(tmp_zip, "w", zipfile.ZIP_DEFLATED) as zf:
        for f in stage_dir.iterdir():
            if f.is_file():
                zf.write(f, f.name)

    shutil.rmtree(stage_dir, ignore_errors=True)
    if out_zip.is_file():
        out_zip.unlink()
    tmp_zip.replace(out_zip)
    print(f"  [OK] Windows Employee ZIP: {out_zip} ({out_zip.stat().st_size:,} bytes)")
    return out_zip


def package_macos_zip() -> Path:
    out_zip = RELEASE_DIR / "YP_Raksha_macOS.zip"
    app_path = RELEASE_DIR / "YP_Raksha.app"
    stage_dir = INSTALLER_DIR / "staging-mac-zip"

    if app_path.is_dir():
        # Update config & encrypted proxy code inside the app bundle
        res_dir = app_path / "Contents" / "Resources"
        res_dir.mkdir(parents=True, exist_ok=True)
        conf = RELEASE_DIR / "gateway_guard_config.json"
        if conf.is_file():
            shutil.copy2(conf, res_dir / "gateway_guard_config.json")
        code_enc = PROXY_DIR / "gateway_guard_code.enc"
        if code_enc.is_file():
            shutil.copy2(code_enc, res_dir / "gateway_guard_code.enc")
        stub = INSTALLER_DIR / "proxy_stub.py"
        if stub.is_file():
            shutil.copy2(stub, res_dir / "browser_ai_proxy.py")
        parts_enc = PROXY_DIR / "gateway_proxy_parts" / "gateway_proxy_parts.enc"
        if not parts_enc.is_file():
            parts_enc = PROXY_DIR / "gateway_proxy_parts" / "gateway_proxy_parts" / "gateway_proxy_parts.enc"
        if parts_enc.is_file():
            parts_dest = res_dir / "gateway_proxy_parts"
            parts_dest.mkdir(parents=True, exist_ok=True)
            shutil.copy2(parts_enc, parts_dest / "gateway_proxy_parts.enc")
        manifest_src = PROXY_DIR / "gateway_proxy_parts" / "gateway_proxy_parts" / "MANIFEST.txt"
        if not manifest_src.is_file():
            manifest_src = PROXY_DIR / "gateway_proxy_parts" / "MANIFEST.txt"
        if manifest_src.is_file():
            parts_dest = res_dir / "gateway_proxy_parts"
            parts_dest.mkdir(parents=True, exist_ok=True)
            shutil.copy2(manifest_src, parts_dest / "MANIFEST.txt")

    if stage_dir.is_dir():
        shutil.rmtree(stage_dir, ignore_errors=True)
    stage_dir.mkdir(parents=True, exist_ok=True)

    if app_path.is_dir():
        shutil.copytree(app_path, stage_dir / "YP_Raksha.app", dirs_exist_ok=True)

    extra_files = [
        "gateway_guard_config.json",
        "VERSION.txt",
        "EMPLOYEE_README_MAC.txt",
        "INSTALL_MACOS.txt",
        "UNINSTALL_MACOS.txt",
        "Install_YP_Raksha.command",
        "Uninstall_YP_Raksha.command",
        "Update_YP_Raksha_macOS.command",
    ]
    for name in extra_files:
        for base in [RELEASE_DIR, INSTALLER_DIR]:
            src = base / name
            if src.is_file():
                shutil.copy2(src, stage_dir / name)
                break

    tmp_zip = out_zip.with_suffix(".tmp")
    if tmp_zip.is_file():
        tmp_zip.unlink()

    with zipfile.ZipFile(tmp_zip, "w", zipfile.ZIP_DEFLATED) as zf:
        for root, dirs, files in os.walk(stage_dir):
            for file in files:
                full = Path(root) / file
                rel = full.relative_to(stage_dir)
                zf.write(full, str(rel))

    shutil.rmtree(stage_dir, ignore_errors=True)
    if out_zip.is_file():
        out_zip.unlink()
    tmp_zip.replace(out_zip)
    print(f"  [OK] macOS ZIP: {out_zip} ({out_zip.stat().st_size:,} bytes)")
    return out_zip


def verify_all_artifacts() -> None:
    print("\n" + "=" * 70)
    print(" FINAL VERIFICATION & INTEGRITY CHECK")
    print("=" * 70)

    suite_zip = RELEASE_DIR / "YP_Raksha_Decoupled_Suite.zip"
    if not suite_zip.is_file():
        raise SystemExit(f"[FAIL] Missing {suite_zip}")

    with zipfile.ZipFile(suite_zip) as zf:
        bad = zf.testzip()
        if bad:
            raise SystemExit(f"[FAIL] Suite zip is corrupted at entry: {bad}")
        names = zf.namelist()

    # Verify 3-folder structure
    has_win_setup = any("windows/YP_Raksha_Setup.exe" in n for n in names)
    has_win_exe = any("windows/YP_Raksha.exe" in n for n in names)
    has_ext = any(n.startswith("extension/") for n in names)
    has_crx = any(n.endswith(".crx") for n in names)
    has_mac = any(n.startswith("mac/") for n in names)
    has_readme = "README_INSTRUCTIONS.txt" in names

    print(f"Suite Archive: {suite_zip.name} ({suite_zip.stat().st_size / (1024*1024):.2f} MB)")
    print(f"  • windows/ folder present:    {'[OK]' if (has_win_setup and has_win_exe) else '[FAIL]'}")
    print(f"  • extension/ folder present:  {'[OK]' if (has_ext and has_crx) else '[FAIL]'}")
    print(f"  • mac/ folder present:        {'[OK]' if has_mac else '[FAIL]'}")
    print(f"  • README_INSTRUCTIONS.txt:    {'[OK]' if has_readme else '[FAIL]'}")

    assert has_win_setup and has_win_exe, "windows/YP_Raksha_Setup.exe and YP_Raksha.exe must be present"
    assert has_ext and has_crx, "extension/ folder and CRX package must be present"
    assert has_mac, "mac/ folder must be present"

    print("\n" + "=" * 70)
    print(" ALL BUILDS & PACKAGES GENERATED AND VERIFIED 100% SUCCESSFULLY!")
    print("=" * 70)


def main() -> int:
    t0 = time.time()
    print("=" * 70)
    print(" STARTING MASTER UNIFIED BUILD PIPELINE: YP-RAKSHA GUARD")
    print("=" * 70)

    # Step 1: Sync config from root .env
    step_banner(1, "Sync Configuration from root .env")
    run_py(SCRIPTS_DIR / "sync_config_from_env.py")

    # Step 2: Package all browser extensions
    step_banner(2, "Package Browser Extensions (.crx, .zip, .xpi)")
    run_py(SCRIPTS_DIR / "pack_all_extensions.py")

    # Step 3: Encrypt proxy runtime and agent code
    step_banner(3, "Encrypt Proxy Runtime & Guard Code Bundle")
    run_py(INSTALLER_DIR / "encrypt_proxy_bundle.py")

    # Step 4: Build standalone agent executable (PyInstaller)
    step_banner(4, "Build Standalone Executable (PyInstaller -> YP_Raksha.exe)")
    run_py(INSTALLER_DIR / "build_agent.py")

    # Step 5: Build Inno Setup installer
    step_banner(5, "Build Windows Setup Installer (Inno Setup -> YP_Raksha_Setup.exe)")
    iscc = find_iscc()
    if iscc:
        build_inno_setup(iscc)
    else:
        print("[WARNING] Inno Setup 6 (ISCC.exe) not found; skipping Setup EXE compilation.")

    # Step 6: Build Windows employee ZIP
    step_banner(6, "Package Windows Employee Distribution (YP_Raksha_Windows.zip)")
    package_windows_zip()

    # Step 7: Package macOS ZIP
    step_banner(7, "Package macOS Distribution (YP_Raksha_macOS.zip)")
    package_macos_zip()

    # Step 8: Assemble Master Decoupled Suite (windows/, extension/, mac/)
    step_banner(8, "Assemble Master Decoupled Suite (YP_Raksha_Decoupled_Suite.zip)")
    run_py(SCRIPTS_DIR / "build_master_suite.py", args=["--skip-prep"])

    # Step 9: Final verification
    verify_all_artifacts()

    elapsed = time.time() - t0
    print(f"\nPipeline finished in {elapsed:.1f} seconds.\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
