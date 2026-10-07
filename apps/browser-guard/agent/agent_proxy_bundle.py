"""Rebuild & Publish → installed Guards run the server's latest Guard code.

The server publishes the Guard's Python code (agent/*.py + browser_ai_proxy.py +
gateway_proxy_parts/) as a zip + SHA-256 and every heartbeat advertises it. This
module downloads it, verifies the hash, self-tests a full load in a subprocess
(guard_bootstrap --bundle-selftest), records per-file hashes, then restarts the
Guard so guard_bootstrap loads the new code. Bundles that fail are remembered as
bad and the Guard stays on / returns to the code built into its installer.
"""

from __future__ import annotations

import hashlib
import io
import json
import os
import re
import shutil
import subprocess
import sys
import threading
import time
import urllib.parse
import urllib.request
import zipfile

from agent_config import AGENT_VERSION, GATEWAY_BACKEND_URL
from agent_http import _guard_headers
from guard_platform import data_dir

ENTRY = "browser_ai_proxy.py"
PARTS = "gateway_proxy_parts"
AGENT = "agent"
_MAX_BUNDLE_BYTES = 20 * 1024 * 1024
_RETRY_FAILED_AFTER = 600
_SHA_RE = re.compile(r"^[0-9a-f]{64}$")
_PART_NAME_RE = re.compile(r"^[A-Za-z0-9_\-]+\.py$")
_MODULE_NAME_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*\.py$")

_apply_lock = threading.Lock()
_failed_at: dict[str, float] = {}


def bundle_root() -> str:
    return os.path.join(data_dir(), "proxy_bundle")


def _state_path() -> str:
    return os.path.join(bundle_root(), "active.json")


def _bad_path() -> str:
    return os.path.join(bundle_root(), "bad.json")


def _code_dir(sha: str) -> str:
    return os.path.join(bundle_root(), sha[:16])


def _read_json(path: str) -> dict:
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


def _write_json(path: str, data: dict) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)
    os.replace(tmp, path)


def _bad_shas() -> set[str]:
    return set(_read_json(_bad_path()).get("shas") or [])


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def running_bundle_sha() -> str:
    """SHA of the server code this Guard runs now ("" = code built into its installer)."""
    code_dir = os.environ.get("GATEWAY_GUARD_CODE_DIR") or os.environ.get("GATEWAY_GUARD_CODE_DIR") or ""
    return (os.environ.get("GATEWAY_GUARD_CODE_SHA") or os.environ.get("GATEWAY_GUARD_CODE_SHA") or "") if code_dir else ""


def active_addon(default_addon: str) -> str:
    """Proxy addon for the MitM worker: the bundle guard_bootstrap loaded, else built-in."""
    code_dir = os.environ.get("GATEWAY_GUARD_CODE_DIR") or os.environ.get("GATEWAY_GUARD_CODE_DIR") or ""
    addon = os.path.join(code_dir, ENTRY) if code_dir else ""
    return addon if addon and os.path.isfile(addon) else default_addon


def _backend_allows_code_download() -> bool:
    try:
        u = urllib.parse.urlparse(GATEWAY_BACKEND_URL or "")
    except Exception:
        return False
    if u.scheme == "https":
        return True
    return u.scheme == "http" and (u.hostname or "") in ("localhost", "127.0.0.1", "::1")


def validate_bundle(data: bytes) -> dict[str, bytes]:
    """Return {relative path: bytes} for an allowed, compilable Guard code bundle or raise ValueError."""
    files: dict[str, bytes] = {}
    with zipfile.ZipFile(io.BytesIO(data)) as zf:
        for info in zf.infolist():
            name = info.filename
            if info.is_dir():
                continue
            if name == ENTRY:
                pass
            elif name.startswith(PARTS + "/"):
                leaf = name[len(PARTS) + 1:]
                if leaf != "MANIFEST.txt" and not _PART_NAME_RE.match(leaf):
                    raise ValueError(f"unexpected file in bundle: {name!r}")
            elif name.startswith(AGENT + "/"):
                leaf = name[len(AGENT) + 1:]
                if not _MODULE_NAME_RE.match(leaf) or leaf == "guard_bootstrap.py":
                    raise ValueError(f"unexpected file in bundle: {name!r}")
            else:
                raise ValueError(f"unexpected file in bundle: {name!r}")
            files[name] = zf.read(info)
    if ENTRY not in files:
        raise ValueError("bundle has no browser_ai_proxy.py")
    if not any(n.endswith(".py") and n.startswith(PARTS + "/") for n in files):
        raise ValueError("bundle has no proxy parts")
    if any(n.startswith(AGENT + "/") for n in files) and f"{AGENT}/gateway_agent.py" not in files:
        raise ValueError("bundle agent code has no gateway_agent.py")
    for name, body in files.items():
        if name.endswith(".py"):
            compile(body.decode("utf-8"), name, "exec")
    manifest = files.get(PARTS + "/MANIFEST.txt")
    if manifest is not None:
        for line in manifest.decode("utf-8-sig").splitlines():
            part = line.strip()
            if part and f"{PARTS}/{part}" not in files:
                raise ValueError(f"MANIFEST lists missing part {part!r}")
    return files


def _extract(files: dict[str, bytes], dest: str) -> None:
    tmp = dest + ".tmp"
    shutil.rmtree(tmp, ignore_errors=True)
    for name, body in files.items():
        path = os.path.join(tmp, *name.split("/"))
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as f:
            f.write(body)
    shutil.rmtree(dest, ignore_errors=True)
    os.replace(tmp, dest)


def _selftest_cmd(code_dir: str) -> list[str]:
    if getattr(sys, "frozen", False):
        return [sys.executable, "--bundle-selftest", code_dir]
    bootstrap = os.path.join(os.path.dirname(os.path.abspath(__file__)), "guard_bootstrap.py")
    return [sys.executable, bootstrap, "--bundle-selftest", code_dir]


def selftest_bundle(code_dir: str) -> tuple[bool, str]:
    """Import every agent module and load the proxy addon from the bundle, in a throwaway process."""
    env = os.environ.copy()
    env.pop("GATEWAY_GUARD_CODE_DIR", None)
    env.pop("GATEWAY_GUARD_CODE_SHA", None)
    env["GATEWAY_PROXY_SELFTEST"] = "1"
    try:
        proc = subprocess.run(
            _selftest_cmd(code_dir),
            env=env,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=120,
            creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
        )
    except Exception as e:
        return False, f"self-test did not run: {e}"
    tail = ((proc.stdout or "") + (proc.stderr or "")).strip()[-800:]
    return proc.returncode == 0, tail


def mark_bundle_bad(sha: str, reason: str) -> None:
    if not sha:
        return
    bad = _read_json(_bad_path())
    shas = [s for s in (bad.get("shas") or []) if s != sha][-19:] + [sha]
    _write_json(_bad_path(), {"shas": shas, "last_reason": reason, "at": int(time.time())})
    print(f"[Gateway Guard WARNING] Server code {sha[:8]} disabled ({reason}) — using built-in code")


def confirm_bundle_healthy() -> None:
    """Running bundle survived startup: reset guard_bootstrap's crash-loop counter."""
    sha = running_bundle_sha()
    if not sha:
        return
    path = os.path.join(bundle_root(), "boot.json")
    boot = _read_json(path)
    if boot.get("sha256") == sha and int(boot.get("attempts") or 0) == 0:
        return
    try:
        _write_json(path, {"sha256": sha, "attempts": 0, "at": int(time.time())})
    except OSError:
        pass


def relaunch_guard(reason: str) -> None:
    """Restart the whole Guard so guard_bootstrap picks the code to run again."""
    print(f"[Gateway Guard] Restarting Guard: {reason}")
    try:
        from agent_pac_orchestration import pac_fail_open_direct

        pac_fail_open_direct(reason)
    except Exception:
        pass
    try:
        from agent_proxy_engine import stop_proxy_worker

        stop_proxy_worker()
    except Exception:
        pass
    _relaunch_fresh()
    sys.stdout.flush()
    os._exit(0)


def _relaunch_fresh() -> None:
    """Same as guard_bootstrap.relaunch, kept here because Guard EXEs up to 1.1.14 froze a
    copy that cannot restart on Windows (escaped quotes + inherited PyInstaller _MEI dir)."""
    env = os.environ.copy()
    env.pop("GATEWAY_GUARD_CODE_DIR", None)
    env.pop("GATEWAY_GUARD_CODE_SHA", None)
    env["PYINSTALLER_RESET_ENVIRONMENT"] = "1"
    if getattr(sys, "frozen", False):
        target = [sys.executable]
    else:
        import guard_bootstrap

        target = [sys.executable, os.path.abspath(guard_bootstrap.__file__)]
    if sys.platform == "win32":
        quoted = " ".join(f'"{p}"' for p in target)
        subprocess.Popen(
            f'cmd.exe /c ping -n 4 127.0.0.1 >nul & start "" {quoted}',
            env=env,
            creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0) | getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0),
            close_fds=True,
        )
    else:
        import shlex

        cmd = "sleep 3; " + " ".join(shlex.quote(p) for p in target) + " >/dev/null 2>&1 &"
        subprocess.Popen(["/bin/sh", "-c", cmd], env=env, start_new_session=True)


def _cleanup(keep_shas: set[str]) -> None:
    keep = {s[:16] for s in keep_shas if s} | {"active.json", "bad.json", "boot.json"}
    try:
        for name in os.listdir(bundle_root()):
            if name in keep:
                continue
            path = os.path.join(bundle_root(), name)
            if os.path.isdir(path):
                shutil.rmtree(path, ignore_errors=True)
            else:
                try:
                    os.remove(path)
                except OSError:
                    pass
    except OSError:
        pass


def _download(sha: str) -> bytes:
    url = f"{GATEWAY_BACKEND_URL}/api/browser-ai/setup/proxy-bundle.zip"
    req = urllib.request.Request(url, headers=_guard_headers({"Accept": "application/zip"}))
    with urllib.request.urlopen(req, timeout=60) as resp:
        data = resp.read(_MAX_BUNDLE_BYTES + 1)
    if len(data) > _MAX_BUNDLE_BYTES:
        raise ValueError("bundle too large")
    got = _sha256(data)
    if got != sha:
        raise ValueError(f"SHA-256 mismatch (expected {sha[:8]}, got {got[:8]})")
    return data


def apply_bundle(info: dict | None, restart) -> bool:
    """Stage + self-test the advertised bundle and restart onto it. Returns True when staged."""
    if not isinstance(info, dict):
        return False
    sha = str(info.get("sha256") or "").strip().lower()
    versions = [str(v).strip() for v in (info.get("guard_versions") or [])]
    if not _SHA_RE.match(sha) or AGENT_VERSION not in versions:
        return False
    if sha == running_bundle_sha():
        return False
    state = _read_json(_state_path())
    if state.get("sha256") == sha and state.get("guard_version") == AGENT_VERSION:
        return False
    if sha in _bad_shas() or time.time() - _failed_at.get(sha, 0) < _RETRY_FAILED_AFTER:
        return False
    if not _backend_allows_code_download():
        return False
    if not _apply_lock.acquire(blocking=False):
        return False
    try:
        print(f"[Gateway Guard] New Guard code {sha[:8]} published by admin — downloading...")
        files = validate_bundle(_download(sha))
        _cleanup({sha, running_bundle_sha()})
        _extract(files, _code_dir(sha))
        ok, detail = selftest_bundle(_code_dir(sha))
        if not ok:
            mark_bundle_bad(sha, f"self-test failed: {detail}")
            return False
        _write_json(_state_path(), {
            "sha256": sha,
            "guard_version": AGENT_VERSION,
            "published_at": info.get("published_at"),
            "applied_at": int(time.time()),
            "files": {name: _sha256(body) for name, body in files.items()},
        })
        print(f"[Gateway Guard] Guard code {sha[:8]} verified.")
        restart(f"switching to Guard code {sha[:8]}")
        return True
    except Exception as e:
        _failed_at[sha] = time.time()
        print(f"[Gateway Guard WARNING] Guard code {sha[:8]} not applied: {e}")
        return False
    finally:
        _apply_lock.release()


def maybe_apply_bundle_async(info: dict | None, restart=relaunch_guard) -> None:
    if not isinstance(info, dict) or _apply_lock.locked():
        return
    threading.Thread(target=apply_bundle, args=(info, restart), name="gateway-code-bundle", daemon=True).start()
