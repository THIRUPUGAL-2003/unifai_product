"""Runtime configuration and module-level config globals for Gateway Guard."""

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
    or os.environ.get("GATEWAY_BACKEND_URL")
    or ""
).strip().rstrip("/")
AGENT_VERSION_BAKED = "1.1.30"


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
    - Windows: folder containing Gateway_Guard.exe
    - macOS .app: Contents/Resources (preferred) or folder containing Gateway_Guard.app
    """
    if getattr(sys, "frozen", False):
        d = os.path.dirname(os.path.abspath(sys.executable))
        norm = d.replace("\\", "/")
        if norm.endswith("/Contents/MacOS"):
            resources = os.path.abspath(os.path.join(d, "..", "Resources"))
            if os.path.isdir(resources):
                return resources
            # Parent of Gateway_Guard.app (portable zip next to .app)
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
AGENT_VERSION = (
    os.environ.get("GATEWAY_GUARD_RUNTIME_VERSION")
    or os.environ.get("GATEWAY_GUARD_RUNTIME_VERSION")
    or ""
).strip() or resolve_agent_version(AGENT_VERSION_BAKED)

import hashlib
import hmac
import zlib

_MAGIC_CONFIG = b"GATEWAYENC02\n"
_MASK = 0x5C
_SEED_MIXED = (
    27, 12, 10, 234, 215, 208, 187, 148, 145, 148, 116, 115, 18, 3, 15, 53, 24, 24, 252, 239,
    194, 168, 161, 134, 176, 73, 70, 29, 2, 10, 53, 25, 25, 223, 235, 195, 165, 162, 167, 157,
    99, 124, 72, 82, 114, 99, 88, 75, 190, 171, 128, 143, 233, 219, 200, 80, 113, 70, 68, 34,
    44, 34, 31, 242, 207, 200, 219, 171,
)
_CTX_MIXED = (
    27, 12, 10, 234, 215, 208, 187, 140, 166, 135, 105, 96, 91, 92, 56, 4, 45, 52, 209, 207,
    194, 174, 170, 154, 171, 71, 99, 73, 92, 37, 63, 52, 42, 191, 193, 234, 174, 165, 135, 129,
    116, 103, 79, 68, 45,
)

def _unwrap_bytes(mixed: tuple[int, ...]) -> bytes:
    raw = bytearray(len(mixed))
    for i, n in enumerate(mixed):
        raw[i] = n ^ ((_MASK + i * 17) & 0xFF)
    return bytes(raw)

def _derive_config_key(salt: bytes) -> bytes:
    seed = _unwrap_bytes(_SEED_MIXED)
    ctx = _unwrap_bytes(_CTX_MIXED)
    prk = hmac.new(salt, seed, hashlib.sha256).digest()
    return hmac.new(prk, ctx + b"\x01", hashlib.sha256).digest()

def _cipher_stream_bytes(key: bytes, nonce: bytes, data: bytes) -> bytes:
    blocks = []
    needed = len(data)
    produced = 0
    counter = 0
    while produced < needed:
        counter += 1
        blk = hmac.new(key, nonce + counter.to_bytes(4, "big"), hashlib.sha256).digest()
        blocks.append(blk)
        produced += len(blk)
    keystream = b"".join(blocks)[:needed]
    return bytes(d ^ k for d, k in zip(data, keystream))

def encrypt_guard_config(cfg_dict: dict) -> bytes:
    """Encrypt runtime configuration into tamper-proof authenticated binary blob."""
    raw = json.dumps(cfg_dict, separators=(",", ":")).encode("utf-8")
    comp = zlib.compress(raw, level=9)
    salt = os.urandom(16)
    nonce = os.urandom(16)
    key = _derive_config_key(salt)
    ct = _cipher_stream_bytes(key, nonce, comp)
    tag = hmac.new(key, salt + nonce + ct + _unwrap_bytes(_CTX_MIXED), hashlib.sha256).digest()
    return _MAGIC_CONFIG + salt + nonce + tag + ct

def decrypt_guard_config(blob: bytes) -> dict:
    """Decrypt runtime configuration directly in RAM with HMAC integrity verification."""
    if not blob.startswith(_MAGIC_CONFIG):
        raise ValueError("Invalid magic header")
    off = len(_MAGIC_CONFIG)
    salt = blob[off:off+16]; off += 16
    nonce = blob[off:off+16]; off += 16
    tag = blob[off:off+32]; off += 32
    ct = blob[off:]
    key = _derive_config_key(salt)
    expected_tag = hmac.new(key, salt + nonce + ct + _unwrap_bytes(_CTX_MIXED), hashlib.sha256).digest()
    if not hmac.compare_digest(tag, expected_tag):
        raise PermissionError("Guard config integrity check failed — tampered or corrupt config")
    comp = _cipher_stream_bytes(key, nonce, ct)
    raw = zlib.decompress(comp)
    return json.loads(raw.decode("utf-8"))

def _read_json(path: str) -> dict:
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f) or {}
            return data if isinstance(data, dict) else {}
    except Exception as e:
        print(f"[Gateway Guard WARNING] Could not read {path}: {e}")
        return {}


def load_runtime_config() -> dict:
    """
    Priority: ENV > encrypted config (.enc) > plaintext config (.json, auto-encrypts on load) > defaults.
    """
    meipass = getattr(sys, "_MEIPASS", None)
    file_cfg: dict = {}
    loaded_from = ""

    # 1. Try encrypted config (.enc) first across candidate paths
    enc_candidates = [
        os.path.join(exe_dir(), "gateway_guard_config.enc"),
        os.path.join(data_dir(), "gateway_guard_config.enc"),
        os.path.join(meipass, "gateway_guard_config.enc") if meipass else "",
        os.path.join(exe_dir(), "config", "gateway_guard_config.enc"),
    ]
    for enc_path in [c for c in enc_candidates if c]:
        if os.path.isfile(enc_path):
            try:
                with open(enc_path, "rb") as f:
                    blob = f.read()
                data = decrypt_guard_config(blob)
                if data and any(data.get(k) for k in ("backend_url", "pac_http_port", "proxy_addr")):
                    file_cfg = data
                    loaded_from = enc_path
                    break
            except Exception as e:
                print(f"[Gateway Guard WARNING] Failed to decrypt {enc_path}: {e}")

    # 2. If no .enc found, check for plaintext .json (and auto-encrypt it immediately)
    if not file_cfg:
        json_candidates = [
            os.path.join(exe_dir(), "gateway_guard_config.json"),
            os.path.join(data_dir(), "gateway_guard_config.json"),
            os.path.join(meipass, "gateway_guard_config.json") if meipass else "",
            os.path.join(exe_dir(), "config", "gateway_guard_config.json"),
            os.path.join(exe_dir(), "release", "gateway_guard_config.json"),
        ]
        for cfg_path in [c for c in json_candidates if c]:
            if os.path.isfile(cfg_path):
                data = _read_json(cfg_path)
                if data and any(data.get(k) for k in ("backend_url", "pac_http_port", "proxy_addr")):
                    file_cfg = data
                    loaded_from = cfg_path
                    # Auto-encrypt upon installation so plaintext credentials are wiped from disk
                    try:
                        enc_bytes = encrypt_guard_config(file_cfg)
                        target_enc = os.path.join(os.path.dirname(cfg_path), "gateway_guard_config.enc")
                        with open(target_enc, "wb") as f_enc:
                            f_enc.write(enc_bytes)
                        # Securely wipe plaintext JSON
                        try:
                            file_size = os.path.getsize(cfg_path)
                            with open(cfg_path, "wb") as f_shred:
                                f_shred.write(b"\x00" * file_size)
                            os.remove(cfg_path)
                            print(f"[Gateway Guard SECURE] Auto-encrypted {cfg_path} -> {target_enc} and wiped plaintext file")
                        except Exception:
                            pass
                    except Exception as enc_err:
                        print(f"[Gateway Guard WARNING] Could not auto-encrypt config: {enc_err}")
                    break

    if loaded_from:
        print(f"[Gateway Guard] Loaded config: {loaded_from}")

    def pick(env_key: str, file_key: str, default: str) -> str:
        # Prefer GATEWAY_* (common), fall back to legacy GATEWAY_*.
        suffix = env_key
        for prefix in ("GATEWAY_", "GATEWAY_"):
            if suffix.startswith(prefix):
                suffix = suffix[len(prefix) :]
                break
        for key in (f"GATEWAY_{suffix}", f"GATEWAY_{suffix}", env_key):
            if os.environ.get(key):
                return os.environ[key].strip()
        val = file_cfg.get(file_key)
        if val is not None and str(val).strip():
            return str(val).strip()
        return default

    server_dom = (os.environ.get("SERVER_DOMAIN") or "").strip().rstrip("/")
    backend = pick("GATEWAY_BACKEND_URL", "backend_url", "").rstrip("/")
    if server_dom and (not backend or "localhost" in backend or "127.0.0.1" in backend):
        backend = server_dom
    elif not backend:
        backend = (server_dom or DEFAULT_BACKEND or "").strip().rstrip("/")
    if not backend:
        print(
            "[Gateway Guard ERROR] backend_url missing — set GATEWAY_BACKEND_URL / SERVER_DOMAIN "
            "or backend_url in gateway_guard_config.json"
        )
    default_proxy_port = (os.environ.get("PROXY_PORT") or "").strip()
    # Laptop Guard bind comes from GATEWAY_PROXY_ADDR / config — NOT docker PROXY_PORT.
    # PROXY_PORT is only a last-resort default when neither env nor config has proxy_addr.
    if default_proxy_port.isdigit():
        default_proxy_addr = f"127.0.0.1:{default_proxy_port}"
    else:
        default_proxy_addr = ""
    env_proxy = (os.environ.get("GATEWAY_PROXY_ADDR") or os.environ.get("GATEWAY_PROXY_ADDR") or "").strip()
    if env_proxy:
        default_proxy_addr = env_proxy
    proxy_addr = pick("GATEWAY_PROXY_ADDR", "proxy_addr", default_proxy_addr)
    if not proxy_addr:
        print(
            "[Gateway Guard ERROR] proxy_addr missing — set GATEWAY_PROXY_ADDR in .env "
            "or proxy_addr in gateway_guard_config.json (run sync_config_from_env.py)"
        )
    pac_url = pick("GATEWAY_PAC_URL", "pac_url", f"{backend}/api/browser-ai/pac")
    # Default 3s: Monitor/Block host list enters PAC same few seconds (not 10–30s wait).
    sync_secs = pick("GATEWAY_PAC_SYNC_SECONDS", "pac_sync_seconds", "3")
    try:
        sync_i = int(float(sync_secs))
    except Exception:
        sync_i = 3
    # Floor 2s — 1s PAC churn felt like connection cuts; 2–3s still feels instant.
    sync_secs = str(max(2, min(sync_i, 600)))

    server_mode_raw = pick("GATEWAY_SERVER_MODE", "server_mode", "0").lower()
    server_mode = server_mode_raw in ("1", "true", "yes", "on", "server", "network")
    agent_type = pick("GATEWAY_AGENT_TYPE", "agent_type", "network" if server_mode else "endpoint").lower()
    if agent_type in ("server", "gateway", "corp", "shared"):
        agent_type = "network"
    if agent_type not in ("endpoint", "network"):
        agent_type = "network" if server_mode else "endpoint"
    listen_host = pick(
        "GATEWAY_LISTEN_HOST",
        "listen_host",
        "0.0.0.0" if server_mode else "127.0.0.1",
    )
    pac_advertise = pick("GATEWAY_PAC_ADVERTISE_ADDR", "pac_advertise_addr", "")
    if not pac_advertise:
        # Endpoint: PAC points at local bind. Network: prefer proxy_addr if it is a
        # reachable hostname; otherwise leave empty so IT sets advertise explicitly.
        if not server_mode:
            pac_advertise = proxy_addr
        elif proxy_addr and not proxy_addr.startswith(("0.0.0.0:", "*:")):
            pac_advertise = proxy_addr

    def _set_both(suffix: str, value: str) -> None:
        os.environ[f"GATEWAY_{suffix}"] = value
        os.environ[f"GATEWAY_{suffix}"] = value  # legacy readers in older proxy parts

    _set_both("BACKEND_URL", backend)
    _set_both("PROXY_ADDR", proxy_addr)
    _set_both("PAC_URL", pac_url)
    _set_both("PAC_SYNC_SECONDS", str(sync_secs))
    _set_both("SERVER_MODE", "1" if server_mode else "0")
    _set_both("AGENT_TYPE", agent_type)
    _set_both("LISTEN_HOST", listen_host)
    if pac_advertise:
        _set_both("PAC_ADVERTISE_ADDR", pac_advertise)

    pac_http_port = pick("PAC_HTTP_PORT", "pac_http_port", "")
    if pac_http_port:
        os.environ["PAC_HTTP_PORT"] = str(pac_http_port)

    guard_secret = pick("GATEWAY_GUARD_SECRET", "guard_secret", "")
    if guard_secret:
        _set_both("GUARD_SECRET", guard_secret)

    # Securely maintain encrypted config in data_dir (NEVER write plaintext credentials to disk)
    if backend or (pac_http_port and str(pac_http_port).isdigit()):
        try:
            active_dict = {
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
            }
            enc_data = encrypt_guard_config(active_dict)
            with open(os.path.join(data_dir(), "gateway_guard_config.enc"), "wb") as f:
                f.write(enc_data)
            # Remove legacy plaintext json if present
            legacy_json = os.path.join(data_dir(), "gateway_guard_config.json")
            if os.path.isfile(legacy_json):
                try:
                    os.remove(legacy_json)
                except Exception:
                    pass
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
GATEWAY_BACKEND_URL = _CFG["backend_url"]
PAC_URL = _CFG["pac_url"]
PROXY_ADDR = _CFG["proxy_addr"]
PAC_SYNC_SECONDS = _CFG["pac_sync_seconds"]
SERVER_MODE = bool(_CFG.get("server_mode"))
AGENT_TYPE = str(_CFG.get("agent_type") or "endpoint")
LISTEN_HOST = str(_CFG.get("listen_host") or "127.0.0.1")
PAC_ADVERTISE_ADDR = str(_CFG.get("pac_advertise_addr") or PROXY_ADDR)
GATEWAY_GUARD_SECRET = str(
    _CFG.get("guard_secret")
    or os.environ.get("GATEWAY_GUARD_SECRET")
    or os.environ.get("GATEWAY_GUARD_SECRET")
    or ""
).strip()
if GATEWAY_GUARD_SECRET:
    os.environ["GATEWAY_GUARD_SECRET"] = GATEWAY_GUARD_SECRET
    os.environ["GATEWAY_GUARD_SECRET"] = GATEWAY_GUARD_SECRET

_pac_port_raw = str(_CFG.get("pac_http_port") or os.environ.get("PAC_HTTP_PORT") or "").strip()
if not _pac_port_raw.isdigit():
    raise RuntimeError(
        "PAC_HTTP_PORT missing — set it in .env or pac_http_port in gateway_guard_config.json "
        "(run sync_config_from_env.py)"
    )
PAC_HTTP_PORT = int(_pac_port_raw)


def proxy_listen_port() -> int:
    """MitM listen port from GATEWAY_PROXY_ADDR / PROXY_ADDR — never a stale hardcoded 8085."""
    addr = (
        os.environ.get("GATEWAY_PROXY_ADDR")
        or os.environ.get("GATEWAY_PROXY_ADDR")
        or PROXY_ADDR
        or ""
    ).strip()
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
        "proxy port unknown — set GATEWAY_PROXY_ADDR in .env or gateway_guard_config.json"
    )
