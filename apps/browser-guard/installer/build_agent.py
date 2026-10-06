#!/usr/bin/env python3
"""Build Raksha Guard with PyInstaller (Windows .exe or macOS .app)."""

from __future__ import annotations

import platform
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DIST = ROOT / "dist"
RELEASE = ROOT / "release"
CONFIG = ROOT / "config" / "raksha_guard_config.json"


def run(cmd: list[str]) -> None:
    try:
        print("+", " ".join(cmd))
    except UnicodeEncodeError:
        print("+", " ".join(cmd).encode(sys.stdout.encoding or "utf-8", errors="replace").decode(sys.stdout.encoding or "utf-8", errors="replace"))
    subprocess.run(cmd, cwd=ROOT, check=True)


def copy_config_into_app(app_path: Path) -> None:
    resources = app_path / "Contents" / "Resources"
    resources.mkdir(parents=True, exist_ok=True)
    shutil.copy2(CONFIG, resources / "raksha_guard_config.json")
    version_file = ROOT / "release" / "VERSION.txt"
    if version_file.is_file():
        shutil.copy2(version_file, resources / "VERSION.txt")


def build_windows() -> Path:
    spec = ROOT / "Raksha_Guard.spec"
    if not spec.is_file():
        raise SystemExit(f"Missing {spec}")
    run([sys.executable, "-m", "PyInstaller", "--noconfirm", "--clean", str(spec)])
    exe = DIST / "Raksha_Guard.exe"
    if not exe.is_file():
        raise SystemExit(f"Expected {exe} after PyInstaller")
    RELEASE.mkdir(parents=True, exist_ok=True)
    shutil.copy2(exe, RELEASE / "Raksha_Guard.exe")
    shutil.copy2(CONFIG, RELEASE / "raksha_guard_config.json")
    return exe


def build_macos() -> Path:
    spec = ROOT / "Raksha_Guard.macos.spec"
    if not spec.is_file():
        raise SystemExit(f"Missing {spec}")
    run([sys.executable, "-m", "PyInstaller", "--noconfirm", "--clean", str(spec)])
    app = DIST / "Raksha_Guard.app"
    if not app.is_dir():
        raise SystemExit(f"Expected {app} after PyInstaller")
    copy_config_into_app(app)
    RELEASE.mkdir(parents=True, exist_ok=True)
    release_app = RELEASE / "Raksha_Guard.app"
    if release_app.exists():
        shutil.rmtree(release_app)
    shutil.copytree(app, release_app, symlinks=True)
    return release_app


def main() -> int:
    if not (ROOT / "agent" / "raksha_agent.py").is_file():
        print("Missing agent/raksha_agent.py", file=sys.stderr)
        return 1
    if not (ROOT / "proxy" / "browser_ai_proxy.py").is_file():
        print("Missing proxy/browser_ai_proxy.py", file=sys.stderr)
        return 1
    if not CONFIG.is_file():
        print("Missing config/raksha_guard_config.json", file=sys.stderr)
        return 1

    # Always sync configs from .env before building
    sync_script = ROOT / "scripts" / "sync_config_from_env.py"
    if sync_script.is_file():
        subprocess.run([sys.executable, str(sync_script)], check=False)

    # Support --native flag for 100% C-compiled machine code binaries (anti-decompilation)
    if "--native" in sys.argv:
        from build_native_nuitka import main as native_main
        return native_main()

    system = platform.system().lower()
    if system == "windows":
        out = build_windows()
        safe_out = str(out).encode(sys.stdout.encoding or "ascii", errors="replace").decode(sys.stdout.encoding or "ascii")
        print(f"OK Windows build: {safe_out}")
    elif system == "darwin":
        out = build_macos()
        safe_out = str(out).encode(sys.stdout.encoding or "ascii", errors="replace").decode(sys.stdout.encoding or "ascii")
        print(f"OK macOS build: {safe_out}")
    else:
        print(f"Unsupported OS for Guard packaging: {system}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
