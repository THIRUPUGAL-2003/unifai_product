#!/usr/bin/env python3
"""
Raksha Guard Native C Compilation (Nuitka)
===========================================
Compiles Python source code into 100% Native C/C++ Machine Code Binaries
(PE .exe on Windows, Mach-O executable inside .app on macOS).

Why this stops all decryption / decompilation:
- Translates Python AST directly into C/C++ source code.
- Uses system C compiler (MSVC / GCC / Clang) to produce native machine code.
- NO Python bytecode (.pyc) or source code (.py) is shipped.
- Decompilers (pyinstxtractor, uncompyle6, pycdc) CANNOT decompile native machine code.
- Fully compatible with Apple Hardened Runtime, Gatekeeper, and Windows Defender.
"""

from __future__ import annotations

import os
import platform
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DIST = ROOT / "dist"
RELEASE = ROOT / "release"
CONFIG = ROOT / "config" / "raksha_guard_config.json"
ICON_ICO = ROOT / "raksha_guard.ico"
ICON_ICNS = ROOT / "raksha_guard.icns"


def ensure_nuitka() -> None:
    try:
        import nuitka  # noqa: F401
    except ImportError:
        print("[Raksha Guard] Installing Nuitka compiler and build dependencies...")
        subprocess.run([sys.executable, "-m", "pip", "install", "nuitka", "zstandard", "ordered-set"], check=True)


def build_native_windows() -> Path:
    entry = ROOT / "agent" / "guard_bootstrap.py"
    output_exe = DIST / "Raksha_Guard.exe"

    cmd = [
        sys.executable,
        "-m",
        "nuitka",
        "--standalone",
        "--onefile",
        "--assume-yes-for-downloads",
        "--remove-output",
        f"--output-dir={DIST}",
        "--output-filename=Raksha_Guard.exe",
        f"--windows-icon-from-ico={ICON_ICO}",
        "--windows-disable-console",
        # Include data files and configurations
        f"--include-data-file={CONFIG}=raksha_guard_config.json",
        f"--include-data-dir={ROOT / 'proxy'}=proxy",
        # Optimization & code protection
        "--lto=yes",
        "--python-flag=no_docstrings",
        "--python-flag=-O",
        str(entry),
    ]

    print("[Raksha Guard] Compiling native Windows PE binary with Nuitka C compiler...")
    subprocess.run(cmd, cwd=ROOT, check=True)

    RELEASE.mkdir(parents=True, exist_ok=True)
    if output_exe.is_file():
        shutil.copy2(output_exe, RELEASE / "Raksha_Guard.exe")
        shutil.copy2(CONFIG, RELEASE / "raksha_guard_config.json")
        print(f"[Raksha Guard SUCCESS] Native C binary created: {RELEASE / 'Raksha_Guard.exe'}")
    return output_exe


def build_native_macos() -> Path:
    entry = ROOT / "agent" / "guard_bootstrap.py"
    output_app = DIST / "Raksha_Guard.app"

    cmd = [
        sys.executable,
        "-m",
        "nuitka",
        "--standalone",
        "--macos-create-app-bundle",
        "--assume-yes-for-downloads",
        "--remove-output",
        f"--output-dir={DIST}",
        "--macos-app-name=Raksha_Guard",
        # Optimization & protection
        "--lto=yes",
        "--python-flag=no_docstrings",
        "--python-flag=-O",
        # Include configs and proxy assets
        f"--include-data-file={CONFIG}=Contents/Resources/raksha_guard_config.json",
        f"--include-data-dir={ROOT / 'proxy'}=Contents/Resources/proxy",
        str(entry),
    ]

    if ICON_ICNS.is_file():
        cmd.append(f"--macos-app-icon={ICON_ICNS}")

    print("[Raksha Guard] Compiling native macOS Mach-O bundle with Nuitka C compiler...")
    subprocess.run(cmd, cwd=ROOT, check=True)

    RELEASE.mkdir(parents=True, exist_ok=True)
    release_app = RELEASE / "Raksha_Guard.app"
    if release_app.exists():
        shutil.rmtree(release_app)
    shutil.copytree(output_app, release_app, symlinks=True)
    print(f"[Raksha Guard SUCCESS] Native C macOS app created: {release_app}")
    return release_app


def main() -> int:
    ensure_nuitka()
    system = platform.system().lower()
    if system == "windows":
        build_native_windows()
    elif system == "darwin":
        build_native_macos()
    else:
        print(f"Unsupported OS: {system}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
