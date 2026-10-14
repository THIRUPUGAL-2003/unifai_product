"""Browser extension that paints a blocked prompt inside the chat page.

The EXE packs this extension, serves it on the local PAC port, and force-installs
it into Chromium browsers. The page asks this process before Send. A block never
reaches the site, so the site stream does not have to be faked. The proxy remains
the backstop when the extension is not installed.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import subprocess
import sys
import threading

from agent_config import GATEWAY_BACKEND_URL
from agent_http import _http_json
from guard_platform import IS_WIN, data_dir

EXTENSION_VERSION = "1.0.0"
_sites_lock = threading.Lock()
_sites_cache: tuple[float, list[str]] = (0.0, [])


def extension_source_dir() -> str:
    """Dev tree, the installed EXE folder, the frozen bundle, or the Guard data dir."""
    meipass = getattr(sys, "_MEIPASS", "") or ""
    candidates = [
        os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "extension")),
        os.path.join(os.path.dirname(os.path.abspath(sys.executable)), "extension"),
        os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(sys.executable)), "..", "Resources", "extension")),
        os.path.join(meipass, "extension") if meipass else "",
        os.path.join(data_dir(), "extension"),
    ]
    for path in candidates:
        if path and os.path.isfile(os.path.join(path, "manifest.json")):
            return path
    return candidates[0]


def _pem_path() -> str:
    return os.path.join(data_dir(), "guard_extension.pem")


def _crx_path() -> str:
    return os.path.join(data_dir(), "guard_extension.crx")


def _unpacked_dir() -> str:
    return os.path.join(data_dir(), "guard_extension")


def host_monitored(host: str, domains: list[str]) -> bool:
    host = (host or "").lower().strip(".")
    if host.startswith("www."):
        host = host[4:]
    for domain in domains:
        d = (domain or "").lower().strip(".")
        if not d:
            continue
        if host == d or host.endswith("." + d):
            return True
    return False


def monitored_hosts() -> list[str]:
    """Domains the gateway already monitors. Cached for a few seconds."""
    import time

    global _sites_cache
    from agent_http import _http_get_text

    now = time.time()
    with _sites_lock:
        cached_at, cached = _sites_cache
        if cached and now - cached_at < 30:
            return list(cached)
    hosts: list[str] = []
    body = ""
    if GATEWAY_BACKEND_URL:
        body = _http_get_text(
            f"{GATEWAY_BACKEND_URL}/api/browser-ai/targets?for=agent",
            "application/json",
            timeout=8,
        ) or ""
        if "targets" not in body:
            body = _http_get_text(
                f"{GATEWAY_BACKEND_URL}/api/browser-ai/targets",
                "application/json",
                timeout=8,
            ) or ""
    try:
        data = json.loads(body) if body else {}
    except Exception:
        data = {}
    rows = data.get("targets") if isinstance(data, dict) else None
    if isinstance(rows, list):
        for row in rows:
            if not isinstance(row, dict):
                continue
            if row.get("monitored") is False and str(row.get("status") or "").upper() not in ("MONITORED", "BLOCKED"):
                continue
            domain = str(row.get("domain") or "").strip().lower()
            if "://" in domain:
                domain = domain.split("://", 1)[1]
            domain = domain.split("/", 1)[0].split(":", 1)[0].strip(".")
            if domain.startswith("www."):
                domain = domain[4:]
            if domain and domain not in hosts:
                hosts.append(domain)
    with _sites_lock:
        _sites_cache = (now, hosts)
    return list(hosts)


def check_prompt(host: str, url: str, prompt: str) -> dict:
    """Same gateway decision the proxy uses. Failure lets the proxy decide."""
    text = (prompt or "").strip()
    if not text:
        return {"blocked": False, "message": "", "rule": ""}
    if not GATEWAY_BACKEND_URL:
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}
    payload = {
        "platform": host or "Browser",
        "prompt": text[:20000],
        "client_ip": "127.0.0.1",
        "metadata": {
            "domain": host or "",
            "url": url or "",
            "method": "POST",
            "source": "extension",
        },
    }
    try:
        code, data = _http_json(
            "POST",
            f"{GATEWAY_BACKEND_URL}/api/browser-ai/intercept",
            payload,
            timeout=12,
        )
    except Exception:
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}
    if code != 200 or not isinstance(data, dict):
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}
    allowed = bool(data.get("allowed", True))
    action = str(data.get("action") or "")
    blocked = (not allowed) or action.lower() in ("blocked", "bot answered", "replied")
    message = str(data.get("reply_text") or data.get("warning_message") or data.get("security_message") or "").strip()
    if blocked and not message:
        message = "This request was blocked by Gateway Guard."
    return {
        "blocked": blocked,
        "message": message if blocked else "",
        "rule": str(data.get("rule_triggered") or ""),
    }


def _ensure_pem() -> str:
    path = _pem_path()
    if os.path.isfile(path) and os.path.getsize(path) > 0:
        return path
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric import rsa

    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    pem = key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption(),
    )
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(pem)
    return path


def extension_id_from_pem(pem_path: str) -> str:
    from cryptography.hazmat.primitives.serialization import load_pem_private_key

    with open(pem_path, "rb") as f:
        key = load_pem_private_key(f.read(), password=None)
    pub = key.public_key().public_bytes(
        serialization_encoding(),
        public_format(),
    )
    digest = hashlib.sha256(pub).digest()
    out = []
    for byte in digest[:16]:
        out.append(chr(ord("a") + ((byte >> 4) & 0xF)))
        out.append(chr(ord("a") + (byte & 0xF)))
    return "".join(out)


def serialization_encoding():
    from cryptography.hazmat.primitives import serialization
    return serialization.Encoding.DER


def public_format():
    from cryptography.hazmat.primitives import serialization
    return serialization.PublicFormat.SubjectPublicKeyInfo


def _find_chrome() -> str:
    roots = [
        os.environ.get("PROGRAMFILES", r"C:\Program Files"),
        os.environ.get("PROGRAMFILES(X86)", r"C:\Program Files (x86)"),
        os.environ.get("LOCALAPPDATA", ""),
    ]
    rels = [
        os.path.join("Google", "Chrome", "Application", "chrome.exe"),
        os.path.join("Microsoft", "Edge", "Application", "msedge.exe"),
        os.path.join("BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
    ]
    for root in roots:
        if not root:
            continue
        for rel in rels:
            path = os.path.join(root, rel)
            if os.path.isfile(path):
                return path
    return ""


def _write_unpacked(base_url: str) -> str:
    src = extension_source_dir()
    dst = _unpacked_dir()
    if os.path.isdir(dst):
        shutil.rmtree(dst, ignore_errors=True)
    shutil.copytree(src, dst)
    with open(os.path.join(dst, "endpoint.js"), "w", encoding="utf-8") as f:
        f.write(f'var GATEWAY_GUARD_BASE = "{base_url}";\n')
    return dst


def pack_extension(base_url: str) -> str:
    """Pack the extension. Returns the crx path, or empty when Chrome cannot pack it."""
    pem = _ensure_pem()
    unpacked = _write_unpacked(base_url)
    chrome = _find_chrome()
    if not chrome:
        print("[Gateway Guard] Extension files are ready. Chrome was not found to pack the install file.")
        return ""
    try:
        subprocess.run(
            [chrome, f"--pack-extension={unpacked}", f"--pack-extension-key={pem}", "--no-message-box"],
            timeout=60,
            check=False,
            capture_output=True,
        )
    except Exception as e:
        print(f"[Gateway Guard WARNING] Extension pack failed: {e}")
        return ""
    packed = unpacked + ".crx"
    if not os.path.isfile(packed):
        print("[Gateway Guard WARNING] Chrome did not write the extension package.")
        return ""
    if os.path.abspath(packed) != os.path.abspath(_crx_path()):
        shutil.copy2(packed, _crx_path())
        try:
            os.remove(packed)
        except Exception:
            pass
    return _crx_path()


def update_xml(extension_id: str, base_url: str) -> bytes:
    xml = (
        "<?xml version='1.0' encoding='UTF-8'?>\n"
        "<gupdate xmlns='http://www.google.com/update2/response' protocol='2.0'>\n"
        f"  <app appid='{extension_id}'>\n"
        f"    <updatecheck codebase='{base_url}/extension/guard.crx' version='{EXTENSION_VERSION}' />\n"
        "  </app>\n"
        "</gupdate>\n"
    )
    return xml.encode("utf-8")


FIREFOX_ID = "yp-raksha@gateway.guard"


def _extension_settings(extension_id: str, update_url: str) -> str:
    return json.dumps({
        extension_id: {
            "installation_mode": "force_installed",
            "update_url": update_url,
            "incognito": "spanning",
        }
    })


def _mac_managed_roots() -> list[str]:
    app_support = os.path.join(os.path.expanduser("~"), "Library", "Application Support")
    names = [
        os.path.join("Google", "Chrome"),
        os.path.join("Google", "Chrome Canary"),
        os.path.join("Microsoft Edge"),
        os.path.join("BraveSoftware", "Brave-Browser"),
        os.path.join("Chromium"),
        os.path.join("Vivaldi"),
        os.path.join("com.operasoftware.Opera"),
    ]
    return [os.path.join(app_support, name, "policies", "managed") for name in names]


def _write_mac_extension_policy(extension_id: str, update_url: str) -> None:
    policy = {
        "ExtensionInstallForcelist": [f"{extension_id};{update_url}"],
        "ExtensionSettings": json.loads(_extension_settings(extension_id, update_url)),
    }
    for root in _mac_managed_roots():
        path = os.path.join(root, "yp-raksha.json")
        try:
            os.makedirs(root, exist_ok=True)
            with open(path, "w", encoding="utf-8") as f:
                json.dump(policy, f, indent=2)
        except Exception as e:
            print(f"[Gateway Guard WARNING] YP-Raksha Mac policy failed ({root}): {e}")


def _remove_mac_extension_policy() -> None:
    for root in _mac_managed_roots():
        path = os.path.join(root, "yp-raksha.json")
        try:
            if os.path.isfile(path):
                os.remove(path)
        except Exception:
            pass


def _firefox_settings(base_url: str) -> str:
    return json.dumps({
        FIREFOX_ID: {
            "installation_mode": "force_installed",
            "install_url": f"{base_url}/extension/yp-raksha.xpi",
        }
    })


def _write_firefox_policy(base_url: str) -> None:
    raw = _firefox_settings(base_url)
    if IS_WIN:
        try:
            import winreg
            from agent_browser_policy import _set_reg_sz
            for root in (winreg.HKEY_CURRENT_USER, winreg.HKEY_LOCAL_MACHINE):
                _set_reg_sz(root, r"Software\Policies\Mozilla\Firefox", "ExtensionSettings", raw)
        except Exception as e:
            print(f"[Gateway Guard WARNING] YP-Raksha Firefox policy failed: {e}")
    dist_dirs = []
    if IS_WIN:
        for env in ("PROGRAMFILES", "PROGRAMFILES(X86)"):
            root = os.environ.get(env, "")
            if root:
                dist_dirs.append(os.path.join(root, "Mozilla Firefox", "distribution"))
    else:
        dist_dirs.append("/Applications/Firefox.app/Contents/Resources/distribution")
        dist_dirs.append(os.path.expanduser("~/Applications/Firefox.app/Contents/Resources/distribution"))
    body = json.dumps({"policies": {"ExtensionSettings": json.loads(raw)}}, indent=2)
    for folder in dist_dirs:
        path = os.path.join(folder, "policies.json")
        try:
            os.makedirs(folder, exist_ok=True)
            with open(path, "w", encoding="utf-8") as f:
                f.write(body)
        except Exception:
            pass


def _remove_firefox_policy() -> None:
    if IS_WIN:
        try:
            import winreg
            from agent_browser_policy import _delete_reg_value
            for root in (winreg.HKEY_CURRENT_USER, winreg.HKEY_LOCAL_MACHINE):
                _delete_reg_value(root, r"Software\Policies\Mozilla\Firefox", "ExtensionSettings")
        except Exception:
            pass


def build_xpi(unpacked_dir: str) -> str:
    """Firefox install package. Same files as the Chromium extension, without the signing key."""
    import zipfile
    dest = os.path.join(data_dir(), "yp-raksha.xpi")
    with zipfile.ZipFile(dest, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for name in os.listdir(unpacked_dir):
            if name.endswith(".pem"):
                continue
            path = os.path.join(unpacked_dir, name)
            if os.path.isfile(path):
                zf.write(path, name)
    return dest


def install_guard_extension(base_url: str) -> None:
    """Pack YP-Raksha and force-install it into every Chromium browser and Firefox."""
    if not base_url:
        return
    crx = _crx_path()
    source_mtime = 0.0
    for name in ("manifest.json", "content.js", "page_hook.js", "endpoint.js"):
        path = os.path.join(extension_source_dir(), name)
        if os.path.isfile(path):
            source_mtime = max(source_mtime, os.path.getmtime(path))
    if not os.path.isfile(crx) or os.path.getmtime(crx) < source_mtime:
        crx = pack_extension(base_url)
    unpacked = _unpacked_dir()
    if os.path.isdir(unpacked):
        try:
            build_xpi(unpacked)
        except Exception as e:
            print(f"[Gateway Guard WARNING] YP-Raksha Firefox package failed: {e}")
    pem = _pem_path()
    if not os.path.isfile(pem):
        return
    try:
        ext_id = extension_id_from_pem(pem)
    except Exception as e:
        print(f"[Gateway Guard WARNING] YP-Raksha id failed: {e}")
        return
    update_url = f"{base_url}/extension/update.xml"
    if IS_WIN:
        _write_force_install(ext_id, update_url)
    else:
        _write_mac_extension_policy(ext_id, update_url)
    _write_firefox_policy(base_url)
    if crx or not IS_WIN:
        print(f"[Gateway Guard] YP-Raksha ready ({ext_id}). Quit and reopen each browser once, including private windows.")


def remove_guard_extension_policy() -> None:
    """Drop YP-Raksha from every browser. Called when the Guard uninstall key is accepted."""
    if IS_WIN:
        _write_force_install("", "")
    else:
        _remove_mac_extension_policy()
    _remove_firefox_policy()
    print("[Gateway Guard] YP-Raksha removed from browser policy.")


def _write_force_install(extension_id: str, update_url: str) -> None:
    try:
        import winreg
    except Exception:
        return
    from agent_browser_policy import _CHROMIUM_POLICY_PATHS, _delete_reg_value, _set_reg_sz

    value = f"{extension_id};{update_url}" if extension_id and update_url else ""
    settings = _extension_settings(extension_id, update_url) if value else ""
    wrote = False
    for root in (winreg.HKEY_CURRENT_USER, winreg.HKEY_LOCAL_MACHINE):
        for path in _CHROMIUM_POLICY_PATHS:
            list_path = path + r"\ExtensionInstallForcelist"
            if value:
                if _set_reg_sz(root, list_path, "1", value):
                    wrote = True
                if _set_reg_sz(root, path, "ExtensionSettings", settings):
                    wrote = True
            else:
                _delete_reg_value(root, list_path, "1")
                _delete_reg_value(root, path, "ExtensionSettings")
                wrote = True
    if value and not wrote:
        print("[Gateway Guard WARNING] YP-Raksha policy was not written. Browser policy keys are locked for this user.")


def local_base_url() -> str:
    import agent_state
    url = agent_state._PAC_HTTP_URL or ""
    if "://" in url:
        return url.split("/proxy.pac", 1)[0].rstrip("/")
    return ""
