#!/usr/bin/env python3
"""
Publish Guard fleet packages for employee laptop updates.

Used by Browser AI → Setup → Rebuild & Sync Packages.

Steps:
  1) Optionally bump VERSION.txt + agent AGENT_VERSION together
  2) sync_config_from_env.py (backend_url + guard_secret into all configs)
  3) Windows: build_installer.bat when on Windows (PyInstaller + Inno)
  4) macOS: package_macos.ps1 (inject config into .app + rezip)

Exit non-zero on hard failures so the HTTP rebuild API can surface errors.
"""

from __future__ import annotations

import os
import platform
import re
import shutil
import subprocess
import sys

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")


def find_repo_root() -> str:
    start = os.path.abspath(os.path.dirname(__file__))
    current = start
    for _ in range(6):
        if os.path.exists(os.path.join(current, ".env")) or os.path.exists(
            os.path.join(current, "docker-compose.yml")
        ):
            return current
        parent = os.path.dirname(current)
        if parent == current:
            break
        current = parent
    return os.path.abspath(os.path.join(start, "..", ".."))


def bump_patch(version: str) -> str:
    parts = version.strip().lstrip("v").split(".")
    nums: list[int] = []
    for p in parts:
        try:
            nums.append(int(p))
        except ValueError:
            nums.append(0)
    while len(nums) < 3:
        nums.append(0)
    nums[-1] += 1
    return ".".join(str(n) for n in nums)


def write_text(path: str, text: str) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write(text)


def read_text(path: str) -> str:
    with open(path, "r", encoding="utf-8-sig", errors="ignore") as f:
        return f.read()


def bump_versions(bg: str) -> str:
    version_path = os.path.join(bg, "release", "VERSION.txt")
    agent_path = os.path.join(bg, "agent", "agent_config.py")
    current = "1.0.0"
    if os.path.isfile(version_path):
        current = read_text(version_path).strip() or current
    new_ver = bump_patch(current)
    write_text(version_path, new_ver + "\n")
    if os.path.isfile(agent_path):
        src = read_text(agent_path)
        src2, n = re.subn(
            r'AGENT_VERSION_BAKED\s*=\s*"[^"]*"',
            f'AGENT_VERSION_BAKED = "{new_ver}"',
            src,
            count=1,
        )
        if not n:
            src2, n = re.subn(
                r'AGENT_VERSION\s*=\s*"[^"]*"',
                f'AGENT_VERSION = "{new_ver}"',
                src,
                count=1,
            )
        if n:
            write_text(agent_path, src2)
        else:
            print("[publish] WARNING: AGENT_VERSION_BAKED/AGENT_VERSION not found in agent_config.py", file=sys.stderr)
    bootstrap_path = os.path.join(bg, "agent", "guard_bootstrap.py")
    if os.path.isfile(bootstrap_path):
        boot_src, n_boot = re.subn(
            r'RUNTIME_VERSION_BAKED\s*=\s*"[^"]*"',
            f'RUNTIME_VERSION_BAKED = "{new_ver}"',
            read_text(bootstrap_path),
            count=1,
        )
        if n_boot:
            write_text(bootstrap_path, boot_src)
        else:
            print("[publish] WARNING: RUNTIME_VERSION_BAKED not found in guard_bootstrap.py", file=sys.stderr)
    iss_path = os.path.join(bg, "installer", "Raksha_Guard.iss")
    if os.path.isfile(iss_path):
        iss = read_text(iss_path)
        iss2, n_iss = re.subn(
            r'#define\s+MyAppVersion\s+"[^"]*"',
            f'#define MyAppVersion "{new_ver}"',
            iss,
            count=1,
        )
        if n_iss:
            write_text(iss_path, iss2)
            print(f"[publish] Inno MyAppVersion → {new_ver}")
        else:
            print("[publish] WARNING: MyAppVersion define not found in Raksha_Guard.iss", file=sys.stderr)
    print(f"[publish] version {current} → {new_ver}")
    return new_ver


def run(cmd: list[str], cwd: str, timeout: int = 1800) -> None:
    print(f"[publish] $ {' '.join(cmd)} (cwd={cwd})")
    proc = subprocess.run(cmd, cwd=cwd, timeout=timeout)
    if proc.returncode != 0:
        raise SystemExit(f"command failed ({proc.returncode}): {' '.join(cmd)}")


def main() -> int:
    bump = "--no-bump" not in sys.argv
    repo = find_repo_root()
    bg = os.path.join(repo, "apps", "browser-guard")
    scripts = os.path.join(bg, "scripts")
    system = platform.system().lower()

    print(f"[publish] repo={repo}")
    print(f"[publish] host={system}")

    # Installed Guards report the VERSION.txt bundled inside the installer at build time.
    # Bumping VERSION.txt without rebuilding the installer makes every Guard see a
    # "newer" version and reinstall the same package on each hourly update check.
    can_rebuild_binary = system == "windows" or (
        system == "darwin" and os.path.isfile(os.path.join(bg, "installer", "build_macos.sh"))
    )
    if bump and not can_rebuild_binary:
        print(f"[publish] version bump skipped: {system} host cannot rebuild the Guard binary")
        bump = False

    version = ""
    if bump:
        version = bump_versions(bg)
    else:
        vp = os.path.join(bg, "release", "VERSION.txt")
        if os.path.isfile(vp):
            version = read_text(vp).strip()

    sync = os.path.join(scripts, "sync_config_from_env.py")
    run([sys.executable, sync], cwd=repo, timeout=120)

    # Verify secret landed in release config when required.
    release_cfg = os.path.join(bg, "release", "raksha_guard_config.json")
    require = os.environ.get("RAKSHA_GUARD_REQUIRE_SECRET", "1").strip().lower()
    if require not in ("0", "false", "no", "off") and os.path.isfile(release_cfg):
        import json

        data = json.loads(read_text(release_cfg))
        if not str(data.get("guard_secret") or "").strip():
            raise SystemExit(
                "release/raksha_guard_config.json still has empty guard_secret after sync — "
                "set RAKSHA_GUARD_SECRET in .env"
            )

    steps_ok: list[str] = ["sync"]
    steps_skip: list[str] = []

    mac_app = os.path.join(bg, "release", "Raksha_Guard.app")
    mac_zip = os.path.join(bg, "release", "Raksha_Guard_macOS.zip")
    hard_errors: list[str] = []

    def repackage_macos() -> None:
        """Refresh config/proxy into existing .app and re-zip. Fails hard if .app is present."""
        ps1 = os.path.join(scripts, "package_macos.ps1")
        if not os.path.isfile(ps1):
            if os.path.isdir(mac_app):
                hard_errors.append("package_macos.ps1 missing but Raksha_Guard.app exists")
            else:
                steps_skip.append("macos_repackage (no script)")
            return
        if not os.path.isdir(mac_app):
            steps_skip.append("macos_repackage (no Raksha_Guard.app — run make build-guard-mac on a Mac once)")
            return
        ps_bin = shutil.which("powershell") or shutil.which("pwsh")
        if not ps_bin:
            steps_skip.append("macos_repackage (PowerShell not installed on this host)")
            return
        run(
            [
                ps_bin,
                "-NoProfile",
                "-ExecutionPolicy",
                "Bypass",
                "-File",
                ps1,
            ],
            cwd=bg,
            timeout=600,
        )
        if not os.path.isfile(mac_zip):
            hard_errors.append("macos_repackage finished but Raksha_Guard_macOS.zip was not created")
            return
        steps_ok.append("macos_repackage")

    if system == "windows":
        bat = os.path.join(bg, "installer", "build_installer.bat")
        if os.path.isfile(bat):
            run(["cmd.exe", "/c", bat], cwd=bg, timeout=2400)
            steps_ok.append("windows_build_installer")
        else:
            hard_errors.append("windows_build_installer missing (installer/build_installer.bat)")
        # Config/proxy refresh + rezip for Mac fleet (Darwin binary still needs a Mac host).
        try:
            repackage_macos()
        except SystemExit as e:
            hard_errors.append(f"macos_repackage failed: {e}")
    else:
        steps_skip.append("windows_build_installer (not a Windows host)")
        # On Mac, full rebuild is make build-guard-mac / build_macos.sh.
        sh = os.path.join(bg, "installer", "build_macos.sh")
        if system == "darwin" and os.path.isfile(sh):
            run(["bash", sh], cwd=bg, timeout=2400)
            steps_ok.append("macos_build")
        else:
            try:
                repackage_macos()
            except SystemExit as e:
                hard_errors.append(f"macos_repackage failed: {e}")
            except Exception as e:
                hard_errors.append(f"macos_repackage unavailable: {e}")

    print("[publish] OK steps:", ", ".join(steps_ok))
    if steps_skip:
        print("[publish] skipped:", ", ".join(steps_skip))
    print(f"[publish] version={version or '(unchanged)'}")

    win_setup = os.path.join(bg, "release", "Raksha_Guard_Setup.exe")
    win_exe = os.path.join(bg, "release", "Raksha_Guard.exe")
    if system == "windows" and not os.path.isfile(win_setup) and not os.path.isfile(win_exe):
        hard_errors.append("no Windows Setup/EXE in release/ after build — Windows fleet cannot update")
    if not os.path.isfile(mac_zip):
        if os.path.isdir(mac_app):
            hard_errors.append("Raksha_Guard_macOS.zip missing after publish but .app exists")
        else:
            print(
                "[publish] WARNING: Raksha_Guard_macOS.zip missing — Mac fleet cannot update "
                "(copy a Mac-built Raksha_Guard.app then rebuild, or run make build-guard-mac on a Mac)",
                file=sys.stderr,
            )
    else:
        print("[publish] macOS ZIP ready (config/proxy refreshed; Darwin binary needs Mac host for code rebuild)")

    if hard_errors:
        for msg in hard_errors:
            print(f"[publish] ERROR: {msg}", file=sys.stderr)
        raise SystemExit("; ".join(hard_errors))

    print(
        "[publish] Fleet update: Windows Guards silent-update when local version < "
        "VERSION.txt header. Mac silent-update needs Darwin binary version bump "
        "(make build-guard-mac); Windows-only rebuild refreshes Mac config/ZIP for Download only."
    )
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.TimeoutExpired:
        print("[publish] ERROR: step timed out", file=sys.stderr)
        raise SystemExit(1)
