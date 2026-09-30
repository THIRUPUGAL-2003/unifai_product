"""UnifAI Guard entry point — frozen into the EXE / .app and never hot-updated.

Rebuild & Publish on the dashboard ships the Guard's Python code (agent/ + proxy)
as a verified bundle. This launcher loads that bundle's agent modules in place of
the ones built into the installer, and falls back to the built-in code when the
bundle is missing, tampered with, marked bad, or keeps crashing on boot.
Uses only the standard library so it can never depend on hot-updated code.
"""

from __future__ import annotations

import hashlib
import importlib
import importlib.abc
import importlib.util
import json
import os
import subprocess
import sys
import time

RUNTIME_VERSION_BAKED = "1.1.15"
MAX_BOOT_ATTEMPTS = 3
_SERVICE_MODES = ("--mitm-worker", "--uninstall", "/uninstall", "--uninstall-prompt")


def data_dir() -> str:
    if sys.platform == "win32":
        base = os.environ.get("LOCALAPPDATA") or os.path.expanduser("~")
    elif sys.platform == "darwin":
        base = os.path.join(os.path.expanduser("~"), "Library", "Application Support")
    else:
        base = os.environ.get("XDG_DATA_HOME") or os.path.join(os.path.expanduser("~"), ".local", "share")
    return os.path.join(base, "UnifAI", "Guard")


def bundle_root() -> str:
    return os.path.join(data_dir(), "proxy_bundle")


def _read_version(path: str) -> str:
    try:
        with open(path, "r", encoding="utf-8-sig", errors="ignore") as f:
            return f.read().strip().lstrip("v").strip()
    except OSError:
        return ""


def runtime_version() -> str:
    """Version of the installed EXE / .app (what auto-update compares), not of hot-updated code."""
    if getattr(sys, "frozen", False):
        d = os.path.dirname(os.path.abspath(sys.executable))
        candidates = [os.path.join(d, "VERSION.txt")]
        if d.replace("\\", "/").endswith("/Contents/MacOS"):
            candidates.insert(0, os.path.join(d, "..", "Resources", "VERSION.txt"))
    else:
        candidates = [os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "release", "VERSION.txt")]
    for p in candidates:
        v = _read_version(p)
        if v:
            return v
    return RUNTIME_VERSION_BAKED


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


def mark_bad(sha: str, reason: str) -> None:
    path = os.path.join(bundle_root(), "bad.json")
    bad = _read_json(path)
    shas = [s for s in (bad.get("shas") or []) if s != sha][-19:] + [sha]
    try:
        _write_json(path, {"shas": shas, "last_reason": reason, "at": int(time.time())})
    except OSError:
        pass
    print(f"[UnifAI Guard WARNING] Server code {sha[:8]} disabled ({reason}) — using built-in code")


def _file_sha(path: str) -> str:
    with open(path, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


def verified_code_dir(version: str) -> tuple[str, str]:
    """(code dir, sha) of the active bundle if it targets this EXE version and every file is intact."""
    state = _read_json(os.path.join(bundle_root(), "active.json"))
    sha = str(state.get("sha256") or "")
    files = state.get("files") if isinstance(state.get("files"), dict) else {}
    if len(sha) != 64 or state.get("guard_version") != version or not files:
        return "", ""
    if sha in set(_read_json(os.path.join(bundle_root(), "bad.json")).get("shas") or []):
        return "", ""
    code_dir = os.path.join(bundle_root(), sha[:16])
    try:
        for rel, want in files.items():
            if ".." in rel.split("/") or _file_sha(os.path.join(code_dir, *rel.split("/"))) != want:
                return "", ""
    except OSError:
        return "", ""
    return code_dir, sha


def _count_boot(sha: str) -> bool:
    """Count a main-mode boot on this bundle; False once it has failed to become healthy too often."""
    path = os.path.join(bundle_root(), "boot.json")
    boot = _read_json(path)
    attempts = int(boot.get("attempts") or 0) if boot.get("sha256") == sha else 0
    if attempts >= MAX_BOOT_ATTEMPTS:
        mark_bad(sha, f"did not become healthy after {attempts} starts")
        return False
    try:
        _write_json(path, {"sha256": sha, "attempts": attempts + 1, "at": int(time.time())})
    except OSError:
        pass
    return True


class _BundleFinder(importlib.abc.MetaPathFinder):
    """Serves the bundle's agent modules ahead of the copies frozen into the EXE."""

    def __init__(self, agent_dir: str) -> None:
        self.agent_dir = agent_dir
        self.names = {n[:-3] for n in os.listdir(agent_dir) if n.endswith(".py")}
        self.names.discard("guard_bootstrap")

    def find_spec(self, fullname, path=None, target=None):  # noqa: ANN001
        if fullname in self.names:
            return importlib.util.spec_from_file_location(fullname, os.path.join(self.agent_dir, fullname + ".py"))
        return None


def install_code_dir(code_dir: str, sha: str) -> _BundleFinder | None:
    os.environ["UNIFAI_GUARD_CODE_DIR"] = code_dir
    os.environ["UNIFAI_GUARD_CODE_SHA"] = sha
    agent_dir = os.path.join(code_dir, "agent")
    if not os.path.isdir(agent_dir):
        return None
    finder = _BundleFinder(agent_dir)
    sys.meta_path.insert(0, finder)
    return finder


def uninstall_code_dir(finder: _BundleFinder | None) -> None:
    os.environ.pop("UNIFAI_GUARD_CODE_DIR", None)
    os.environ.pop("UNIFAI_GUARD_CODE_SHA", None)
    if finder is None:
        return
    if finder in sys.meta_path:
        sys.meta_path.remove(finder)
    for name in list(sys.modules):
        if name in finder.names:
            del sys.modules[name]


def relaunch(extra_env: dict | None = None) -> None:
    """Start a fresh Guard a few seconds after this process exits (single-instance lock is released)."""
    env = os.environ.copy()
    env.pop("UNIFAI_GUARD_CODE_DIR", None)
    env.pop("UNIFAI_GUARD_CODE_SHA", None)
    # Without this the new onefile EXE reuses this process's _MEI dir, which is
    # deleted when we exit ("failed to obtain executable path for parent process").
    env["PYINSTALLER_RESET_ENVIRONMENT"] = "1"
    env.update(extra_env or {})
    if getattr(sys, "frozen", False):
        target = [sys.executable]
    else:
        target = [sys.executable, os.path.abspath(__file__)]
    if sys.platform == "win32":
        quoted = " ".join(f'"{p}"' for p in target)
        # String, not list: list2cmdline turns the quotes into \" which cmd.exe
        # does not understand ("Windows cannot find '\\'").
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


def selftest(code_dir: str) -> int:
    """--bundle-selftest <dir>: import every agent module and load the proxy addon from the bundle."""
    import runpy

    code = 3
    try:
        finder = install_code_dir(code_dir, "selftest")
        if finder is not None:
            for name in sorted(finder.names):
                mod = importlib.import_module(name)
                if not os.path.abspath(getattr(mod, "__file__", "")).startswith(os.path.abspath(code_dir)):
                    raise RuntimeError(f"{name} did not load from the bundle")
            if not callable(getattr(sys.modules.get("unifai_agent"), "main", None)):
                raise RuntimeError("unifai_agent.main missing")
        ns = runpy.run_path(os.path.join(code_dir, "browser_ai_proxy.py"), run_name="unifai_proxy_selftest")
        addons = ns.get("addons") or []
        if not (addons and type(addons[0]).__name__ == "BrowserAIInterceptor" and callable(getattr(addons[0], "request", None))):
            raise RuntimeError("BrowserAIInterceptor addon missing")
        print("[UnifAI Guard] Server code self-test OK")
        code = 0
    except BaseException as e:  # noqa: BLE001 - any failure means "do not activate"
        print(f"[UnifAI Guard ERROR] Server code self-test failed: {type(e).__name__}: {e}")
    sys.stdout.flush()
    os._exit(code)


def _utf8_stdio() -> None:
    """Piped stdout on Windows defaults to cp1252; non-ASCII user/OneDrive paths in log lines would crash."""
    for name in ("stdout", "stderr"):
        stream = getattr(sys, name, None)
        if stream is not None and hasattr(stream, "reconfigure"):
            try:
                stream.reconfigure(encoding="utf-8", errors="replace")
            except Exception:
                pass


def main() -> None:
    _utf8_stdio()
    version = runtime_version()
    os.environ["UNIFAI_GUARD_RUNTIME_VERSION"] = version
    args = sys.argv[1:]
    if len(args) >= 2 and args[0] == "--bundle-selftest":
        selftest(args[1])

    main_mode = not args or args[0] not in _SERVICE_MODES
    finder = None
    sha = ""
    if os.environ.pop("UNIFAI_GUARD_FORCE_BUILTIN", "") != "1":
        code_dir, sha = verified_code_dir(version)
        if code_dir and (not main_mode or _count_boot(sha)):
            finder = install_code_dir(code_dir, sha)
        else:
            sha = ""

    try:
        import unifai_agent
    except Exception as e:
        if not sha:
            raise
        mark_bad(sha, f"import failed: {type(e).__name__}: {e}")
        uninstall_code_dir(finder)
        sha = ""
        import unifai_agent

    try:
        unifai_agent.main()
    except (SystemExit, KeyboardInterrupt):
        raise
    except Exception as e:
        if not sha or not main_mode:
            raise
        mark_bad(sha, f"crashed: {type(e).__name__}: {e}")
        relaunch({"UNIFAI_GUARD_FORCE_BUILTIN": "1"})
        os._exit(1)


if __name__ == "__main__":
    main()
