"""Runtime configuration and module-level config globals for Raksha Guard."""

from __future__ import annotations

import json
import os
import sys

for _name in ("stdout", "stderr"):
    _stream = getattr(sys, _name, None)
    if _stream is not None and hasattr(_stream, "reconfigure"):
        try:
            _stream.reconfigure(encoding="utf-8", errors="replace")
        except Exception:
            pass

from guard_platform import data_dir

def _load_dotenv() -> None:
    """Find and load root .env into os.environ if not already set."""
    cur = os.path.dirname(os.path.abspath(__file__))
    for _ in range(5):
        env_path = os.path.join(cur, ".env")
        if os.path.isfile(env_path):
            try:
                with open(env_path, "r", encoding="utf-8", errors="ignore") as f:
                    for line in f:
                        line = line.strip()
                        if line and not line.startswith("#") and "=" in line:
                            k, v = line.split("=", 1)
                            k, v = k.strip(), v.strip().strip("'\"")
                            if k and k not in os.environ:
                                os.environ[k] = v
            except Exception:
                pass
            break
        parent = os.path.dirname(cur)
        if parent == cur:
            break
        cur = parent


# Always load .env first so environment takes precedence
_load_dotenv()

DEFAULT_BACKEND = (
    os.environ.get("SERVER_DOMAIN")
    or os.environ.get("RAKSHA_BACKEND_URL")
    or ""
).strip().rstrip("/")
AGENT_VERSION_BAKED = "1.1.16"


def _read_version_file(path: str) -> str:
    try:
        with open(path, "r", encoding="utf-8-sig", errors="ignore") as f:
            v = f.read().strip()
        return v.lstrip("v").strip() if v else ""
    except Exception:
        return ""


HEARTBEAT_SECONDS = 30
HEALTH_SECONDS = 45
_HEALTH_WHEN_PROXY_DOWN_SECONDS = 8
PAC_HTTP_HOST = os.environ.get("PAC_HTTP_HOST", "127.0.0.1")
_FIRST_RUN_FLAG = "first_run_done.flag"


def exe_dir() -> str:
    """Directory for config next to the Guard binary / .app.

    Frozen layouts:
    - Windows: folder containing Raksha_Guard.exe
    - macOS .app: Contents/Resources (preferred) or folder containing Raksha_Guard.app
    """
    if getattr(sys, "frozen", False):
        d = os.path.dirname(os.path.abspath(sys.executable))
        norm = d.replace("\\", "/")
        if norm.endswith("/Contents/MacOS"):
            resources = os.path.abspath(os.path.join(d, "..", "Resources"))
            if os.path.isdir(resources):
                return resources
            # Parent of Raksha_Guard.app (portable zip next to .app)
            return os.path.abspath(os.path.join(d, "..", "..", ".."))
        return d
    return os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))


def resolve_agent_version(baked: str = AGENT_VERSION_BAKED) -> str:
    """Prefer VERSION.txt beside the install (fleet publish can bump without re-freeze)."""
    try:
        base = exe_dir()
        for name in ("VERSION.txt", "version.txt"):
            v = _read_version_file(os.path.join(base, name))
            if v:
                return v
        norm = base.replace("\\", "/")
        if "/Contents/MacOS" in norm:
            res = os.path.join(os.path.dirname(base), "Resources", "VERSION.txt")
            v = _read_version_file(res)
            if v:
                return v
    except Exception:
        pass
    return (baked or "0.0.0").strip()


# Installed EXE/.app version from guard_bootstrap: this module may be server hot-updated code
# whose baked number differs, and auto-update must compare the real installer version.
AGENT_VERSION = (os.environ.get("RAKSHA_GUARD_RUNTIME_VERSION") or "").strip() or resolve_agent_version(AGENT_VERSION_BAKED)

def _read_json(path: str) -> dict:
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f) or {}
            return data if isinstance(data, dict) else {}
    except Exception as e:
        print(f"[Raksha Guard WARNING] Could not read {path}: {e}")
        return {}


def load_runtime_config() -> dict:
    """
    Priority: ENV > exe-dir config > user data-dir config > defaults.
    """
    meipass = getattr(sys, "_MEIPASS", None)
    candidates = [
        os.path.join(exe_dir(), "raksha_guard_config.json"),
        os.path.join(meipass, "raksha_guard_config.json") if meipass else "",
        os.path.join(exe_dir(), "config", "raksha_guard_config.json"),
        os.path.join(exe_dir(), "release", "raksha_guard_config.json"),
        os.path.join(data_dir(), "raksha_guard_config.json"),
    ]
    candidates = [c for c in candidates if c]
    file_cfg: dict = {}
    loaded_from = ""
    for cfg_path in candidates:
        if os.path.isfile(cfg_path):
            data = _read_json(cfg_path)
            if data and any(data.get(k) for k in ("backend_url", "pac_http_port", "proxy_addr")):
                file_cfg = data
                loaded_from = cfg_path
                break
    if not file_cfg:
        for cfg_path in candidates:
            if os.path.isfile(cfg_path):
                file_cfg = _read_json(cfg_path)
                loaded_from = cfg_path
                break
    if loaded_from:
        print(f"[Raksha Guard] Loaded config: {loaded_from}")

    def pick(env_key: str, file_key: str, default: str) -> str:
        if os.environ.get(env_key):
            return os.environ[env_key].strip()
        val = file_cfg.get(file_key)
        if val is not None and str(val).strip():
            return str(val).strip()
        return default

    backend = pick("RAKSHA_BACKEND_URL", "backend_url", "").rstrip("/")
    if not backend:
        backend = (os.environ.get("SERVER_DOMAIN") or DEFAULT_BACKEND or "").strip().rstrip("/")
    if not backend:
        print(
            "[Raksha Guard ERROR] backend_url missing — set RAKSHA_BACKEND_URL / SERVER_DOMAIN "
            "or backend_url in raksha_guard_config.json"
        )
    default_proxy_port = (os.environ.get("PROXY_PORT") or "").strip()
    # Laptop Guard bind comes from RAKSHA_PROXY_ADDR / config — NOT docker PROXY_PORT.
    # PROXY_PORT is only a last-resort default when neither env nor config has proxy_addr.
    if default_proxy_port.isdigit():
        default_proxy_addr = f"127.0.0.1:{default_proxy_port}"
    else:
        default_proxy_addr = ""
    env_proxy = (os.environ.get("RAKSHA_PROXY_ADDR") or "").strip()
    if env_proxy:
        default_proxy_addr = env_proxy
    proxy_addr = pick("RAKSHA_PROXY_ADDR", "proxy_addr", default_proxy_addr)
    if not proxy_addr:
        print(
            "[Raksha Guard ERROR] proxy_addr missing — set RAKSHA_PROXY_ADDR in .env "
            "or proxy_addr in raksha_guard_config.json (run sync_config_from_env.py)"
        )
    pac_url = pick("RAKSHA_PAC_URL", "pac_url", f"{backend}/api/browser-ai/pac")
    # Default 3s: Monitor/Block host list enters PAC same few seconds (not 10–30s wait).
    sync_secs = pick("RAKSHA_PAC_SYNC_SECONDS", "pac_sync_seconds", "3")
    try:
        sync_i = int(float(sync_secs))
    except Exception:
        sync_i = 3
    # Floor 2s — 1s PAC churn felt like connection cuts; 2–3s still feels instant.
    sync_secs = str(max(2, min(sync_i, 600)))

    server_mode_raw = pick("RAKSHA_SERVER_MODE", "server_mode", "0").lower()
    server_mode = server_mode_raw in ("1", "true", "yes", "on", "server", "network")
    agent_type = pick("RAKSHA_AGENT_TYPE", "agent_type", "network" if server_mode else "endpoint").lower()
    if agent_type in ("server", "gateway", "corp", "shared"):
        agent_type = "network"
    if agent_type not in ("endpoint", "network"):
        agent_type = "network" if server_mode else "endpoint"
    listen_host = pick(
        "RAKSHA_LISTEN_HOST",
        "listen_host",
        "0.0.0.0" if server_mode else "127.0.0.1",
    )
    pac_advertise = pick("RAKSHA_PAC_ADVERTISE_ADDR", "pac_advertise_addr", "")
    if not pac_advertise:
        # Endpoint: PAC points at local bind. Network: prefer proxy_addr if it is a
        # reachable hostname; otherwise leave empty so IT sets advertise explicitly.
        if not server_mode:
            pac_advertise = proxy_addr
        elif proxy_addr and not proxy_addr.startswith(("0.0.0.0:", "*:")):
            pac_advertise = proxy_addr

    os.environ["RAKSHA_BACKEND_URL"] = backend
    os.environ["RAKSHA_PROXY_ADDR"] = proxy_addr
    os.environ["RAKSHA_PAC_URL"] = pac_url
    os.environ["RAKSHA_PAC_SYNC_SECONDS"] = str(sync_secs)
    os.environ["RAKSHA_SERVER_MODE"] = "1" if server_mode else "0"
    os.environ["RAKSHA_AGENT_TYPE"] = agent_type
    os.environ["RAKSHA_LISTEN_HOST"] = listen_host
    if pac_advertise:
        os.environ["RAKSHA_PAC_ADVERTISE_ADDR"] = pac_advertise

    pac_http_port = pick("PAC_HTTP_PORT", "pac_http_port", "")
    if pac_http_port:
        os.environ["PAC_HTTP_PORT"] = str(pac_http_port)

    guard_secret = pick("RAKSHA_GUARD_SECRET", "guard_secret", "")
    if guard_secret:
        os.environ["RAKSHA_GUARD_SECRET"] = guard_secret

    # Keep a copy in data_dir so logs/support can see active config (only when valid)
    if backend or (pac_http_port and str(pac_http_port).isdigit()):
        try:
            with open(os.path.join(data_dir(), "raksha_guard_config.json"), "w", encoding="utf-8") as f:
                json.dump(
                    {
                        "backend_url": backend,
                        "proxy_addr": proxy_addr,
                        "pac_url": pac_url,
                        "pac_sync_seconds": int(sync_secs),
                        "pac_http_port": int(pac_http_port) if str(pac_http_port).isdigit() else pac_http_port,
                        "server_mode": server_mode,
                        "agent_type": agent_type,
                        "listen_host": listen_host,
                        "pac_advertise_addr": pac_advertise,
                        "guard_secret": guard_secret,
                    },
                    f,
                    indent=2,
                )
        except Exception:
            pass

    return {
        "backend_url": backend,
        "proxy_addr": proxy_addr,
        "pac_url": pac_url,
        "pac_sync_seconds": int(sync_secs),
        "pac_http_port": pac_http_port,
        "server_mode": server_mode,
        "agent_type": agent_type,
        "listen_host": listen_host,
        "pac_advertise_addr": pac_advertise,
        "guard_secret": guard_secret,
    }


_CFG = load_runtime_config()
RAKSHA_BACKEND_URL = _CFG["backend_url"]
PAC_URL = _CFG["pac_url"]
PROXY_ADDR = _CFG["proxy_addr"]
PAC_SYNC_SECONDS = _CFG["pac_sync_seconds"]
SERVER_MODE = bool(_CFG.get("server_mode"))
AGENT_TYPE = str(_CFG.get("agent_type") or "endpoint")
LISTEN_HOST = str(_CFG.get("listen_host") or "127.0.0.1")
PAC_ADVERTISE_ADDR = str(_CFG.get("pac_advertise_addr") or PROXY_ADDR)
RAKSHA_GUARD_SECRET = str(_CFG.get("guard_secret") or os.environ.get("RAKSHA_GUARD_SECRET") or "").strip()
if RAKSHA_GUARD_SECRET:
    os.environ["RAKSHA_GUARD_SECRET"] = RAKSHA_GUARD_SECRET

_pac_port_raw = str(_CFG.get("pac_http_port") or os.environ.get("PAC_HTTP_PORT") or "").strip()
if not _pac_port_raw.isdigit():
    raise RuntimeError(
        "PAC_HTTP_PORT missing — set it in .env or pac_http_port in raksha_guard_config.json "
        "(run sync_config_from_env.py)"
    )
PAC_HTTP_PORT = int(_pac_port_raw)


def proxy_listen_port() -> int:
    """MitM listen port from RAKSHA_PROXY_ADDR / PROXY_ADDR — never a stale hardcoded 8085."""
    addr = (os.environ.get("RAKSHA_PROXY_ADDR") or PROXY_ADDR or "").strip()
    if ":" in addr:
        try:
            port = int(addr.rsplit(":", 1)[-1])
            if 1 <= port <= 65535:
                return port
        except ValueError:
            pass
    pp = (os.environ.get("PROXY_PORT") or "").strip()
    if pp.isdigit():
        return int(pp)
    raise RuntimeError(
        "proxy port unknown — set RAKSHA_PROXY_ADDR in .env or raksha_guard_config.json"
    )
