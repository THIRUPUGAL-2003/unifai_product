#!/usr/bin/env python3
"""Build Gateway Guard with PyInstaller (Windows .exe or macOS .app)."""

from __future__ import annotations

import platform
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DIST = ROOT / "dist"
RELEASE = ROOT / "release"
CONFIG = ROOT / "config" / "gateway_guard_config.json"


def run(cmd: list[str]) -> None:
    try:
        print("+", " ".join(cmd))
    except UnicodeEncodeError:
        print("+", " ".join(cmd).encode(sys.stdout.encoding or "utf-8", errors="replace").decode(sys.stdout.encoding or "utf-8", errors="replace"))
    subprocess.run(cmd, cwd=ROOT, check=True)


def copy_config_into_app(app_path: Path) -> None:
    resources = app_path / "Contents" / "Resources"
    resources.mkdir(parents=True, exist_ok=True)
    shutil.copy2(CONFIG, resources / "gateway_guard_config.json")
    version_file = ROOT / "release" / "VERSION.txt"
    if version_file.is_file():
        shutil.copy2(version_file, resources / "VERSION.txt")


def build_windows() -> Path:
    spec = ROOT / "Gateway_Guard.spec"
    if not spec.is_file():
        raise SystemExit(f"Missing {spec}")
    # dist\Gateway_Guard.exe can stay locked by a previous copy. Write the new
    # binary beside the PyInstaller work dir, then publish copies that file.
    out_dir = ROOT / "build" / "exe"
    work_dir = ROOT / "build" / "pyi"
    out_dir.mkdir(parents=True, exist_ok=True)
    run(
        [
            sys.executable,
            "-m",
            "PyInstaller",
            "--noconfirm",
            "--clean",
            "--distpath",
            str(out_dir),
            "--workpath",
            str(work_dir),
            str(spec),
        ]
    )
    exe = out_dir / "Gateway_Guard.exe"
    if not exe.is_file():
        raise SystemExit(f"Expected {exe} after PyInstaller")
    RELEASE.mkdir(parents=True, exist_ok=True)
    _replace_copy(exe, DIST / "Gateway_Guard.exe")
    _replace_copy(exe, RELEASE / "Gateway_Guard.exe")
    shutil.copy2(CONFIG, RELEASE / "gateway_guard_config.json")
    return exe


def _replace_copy(src: Path, dest: Path) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    try:
        shutil.copy2(src, dest)
    except OSError as exc:
        print(f"WARNING: could not replace {dest}: {exc}", file=sys.stderr)


def strip_plain_text_sources_from_bundle(app_path: Path) -> None:
    """Ensure release bundle contains ONLY the encrypted container, removing plain text .py files."""
    enc_src = ROOT / "proxy" / "gateway_proxy_parts" / "gateway_proxy_parts.enc"
    manifest_src = ROOT / "proxy" / "gateway_proxy_parts" / "MANIFEST.txt"
    code_enc = ROOT / "proxy" / "gateway_guard_code.enc"
    stub = ROOT / "installer" / "proxy_stub.py"
    if not enc_src.is_file():
        return

    resources = app_path / "Contents" / "Resources"
    if resources.is_dir() and stub.is_file():
        shutil.copy2(stub, resources / "browser_ai_proxy.py")
    if resources.is_dir() and code_enc.is_file():
        shutil.copy2(code_enc, resources / "gateway_guard_code.enc")

    # Check Frameworks, Resources, and app root
    for base in [app_path / "Contents" / "Frameworks", app_path / "Contents" / "Resources", app_path]:
        parts_target = base / "gateway_proxy_parts"
        if parts_target.is_dir():
            shutil.copy2(enc_src, parts_target / "gateway_proxy_parts.enc")
            loose_crypto = parts_target / "bundle_crypto.py"
            if loose_crypto.is_file():
                loose_crypto.unlink()
            if manifest_src.is_file():
                shutil.copy2(manifest_src, parts_target / "MANIFEST.txt")

            # Strip all plain text source files
            for py_file in parts_target.glob("*.py"):
                if py_file.is_file():
                    py_file.unlink()
                    print(f"  [Security] Stripped plain text source from bundle: {py_file.name}")

            # Strip any cached pyc files of parts
            pycache = parts_target / "__pycache__"
            if pycache.is_dir():
                shutil.rmtree(pycache, ignore_errors=True)


def build_macos() -> Path:
    # Ensure proxy parts are encrypted before packaging
    enc_script = ROOT / "installer" / "encrypt_proxy_bundle.py"
    if enc_script.is_file():
        subprocess.run([sys.executable, str(enc_script)], check=True)

    spec = ROOT / "Gateway_Guard.macos.spec"
    if not spec.is_file():
        raise SystemExit(f"Missing {spec}")
    run([sys.executable, "-m", "PyInstaller", "--noconfirm", "--clean", str(spec)])
    app = DIST / "Gateway_Guard.app"
    if not app.is_dir():
        raise SystemExit(f"Expected {app} after PyInstaller")
    copy_config_into_app(app)
    strip_plain_text_sources_from_bundle(app)

    RELEASE.mkdir(parents=True, exist_ok=True)
    release_app = RELEASE / "Gateway_Guard.app"
    if release_app.exists():
        if platform.system().lower() == "darwin":
            subprocess.run(["chflags", "-R", "nouchg", str(release_app)], check=False)
        shutil.rmtree(release_app)
    shutil.copytree(app, release_app, symlinks=True)
    strip_plain_text_sources_from_bundle(release_app)
    return release_app


def main() -> int:
    if not (ROOT / "agent" / "gateway_agent.py").is_file():
        print("Missing agent/gateway_agent.py", file=sys.stderr)
        return 1
    if not (ROOT / "proxy" / "browser_ai_proxy.py").is_file():
        print("Missing proxy/browser_ai_proxy.py", file=sys.stderr)
        return 1
    if not CONFIG.is_file():
        print("Missing config/gateway_guard_config.json", file=sys.stderr)
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
