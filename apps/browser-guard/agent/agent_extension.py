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
import re
import shutil
import subprocess
import sys
import threading
import time
import urllib.request

from agent_config import AGENT_VERSION, GATEWAY_BACKEND_URL, PAC_HTTP_PORT
from agent_http import _http_json
from guard_platform import IS_WIN, data_dir

_COMPROMISED_EXTENSION_ID = "mipfhcmlcjfhmgldkokdckhndkebkllj"

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

def get_extension_version() -> str:
    """Return synchronized extension version.
    Automatically stays monotonically increasing with Guard EXE releases or code edits,
    so Chrome's enterprise update.xml detects the update and auto-updates the extension in all browsers.
    """
    base = "1.0.3"
    try:
        m_path = os.path.join(extension_source_dir(), "manifest.json")
        if os.path.isfile(m_path):
            with open(m_path, "r", encoding="utf-8") as f:
                data = json.load(f)
                v = str(data.get("version") or "").strip()
                if v:
                    base = v
    except Exception:
        pass

    def parse_v(s: str):
        try:
            return tuple(int(x) for x in s.strip().lstrip("v").split(".") if x.isdigit())
        except Exception:
            return (0,)

    if AGENT_VERSION and parse_v(AGENT_VERSION) > parse_v(base):
        base = AGENT_VERSION.strip().lstrip("v")

    counter_path = os.path.join(data_dir(), "extension_build.txt")
    hash_path = os.path.join(data_dir(), "extension_source_hash.txt")

    src = extension_source_dir()
    h = hashlib.sha256()
    for name in sorted(("manifest.json", "content.js", "page_hook.js", "endpoint.js", "background.js", "popup.js", "guard_base.js")):
        p = os.path.join(src, name)
        if os.path.isfile(p):
            try:
                with open(p, "rb") as f:
                    h.update(f.read())
            except Exception:
                pass
    curr_hash = h.hexdigest()

    saved_hash = ""
    build_num = 1
    if os.path.isfile(hash_path):
        try:
            with open(hash_path, "r", encoding="utf-8") as f:
                saved_hash = f.read().strip()
        except Exception:
            pass
    if os.path.isfile(counter_path):
        try:
            with open(counter_path, "r", encoding="utf-8") as f:
                raw_c = f.read().strip()
                if raw_c.isdigit():
                    build_num = int(raw_c)
        except Exception:
            pass

    if curr_hash != saved_hash:
        build_num += 1
        try:
            with open(hash_path, "w", encoding="utf-8") as f:
                f.write(curr_hash)
            with open(counter_path, "w", encoding="utf-8") as f:
                f.write(str(build_num))
        except Exception:
            pass

    base_parts = [p for p in base.split(".") if p.isdigit()]
    if build_num == 0:
        if len(base_parts) >= 3:
            return f"{base_parts[0]}.{base_parts[1]}.{base_parts[2]}"
        elif len(base_parts) == 2:
            return f"{base_parts[0]}.{base_parts[1]}.0"
    if len(base_parts) >= 3:
        return f"{base_parts[0]}.{base_parts[1]}.{base_parts[2]}.{build_num}"
    elif len(base_parts) == 2:
        return f"{base_parts[0]}.{base_parts[1]}.{build_num}"
    return f"{base}.{build_num}"

EXTENSION_VERSION = get_extension_version()
_sites_lock = threading.Lock()
_sites_cache: tuple[float, list[str]] = (0.0, [])
_blocked_sites_cache: tuple[float, list[str]] = (0.0, [])
_domain_platform_map: dict[str, str] = {}


def _pem_path() -> str:
    configured = os.environ.get("GATEWAY_GUARD_EXTENSION_PEM", "").strip()
    if configured:
        return os.path.abspath(os.path.expandvars(os.path.expanduser(configured)))
    if IS_WIN:
        common_pem = os.path.join(os.environ.get("ProgramData") or r"C:\ProgramData", "Gateway", "Guard", "guard_extension.pem")
        if os.path.isfile(common_pem):
            return common_pem
    return os.path.join(data_dir(), "guard_extension.pem")


def _crx_path() -> str:
    return os.path.join(data_dir(), "guard_extension.crx")


def _unpacked_dir() -> str:
    return os.path.join(data_dir(), "guard_extension")


def common_extension_dir() -> str:
    """Enterprise common extension directory shared across all users without personal profile paths."""
    if IS_WIN:
        base = os.environ.get("ProgramData") or r"C:\ProgramData"
        return os.path.join(base, "Gateway", "Guard", "guard_extension")
    elif sys.platform == "darwin":
        return "/Library/Application Support/Gateway/Guard/guard_extension"
    return os.path.join(data_dir(), "guard_extension")


def _normalize_host_domain(value: str) -> str:
    """Normalize a hostname, URL, or host:port while preserving IPv6 literals."""
    from urllib.parse import urlsplit

    raw = str(value or "").strip().lower()
    if not raw:
        return ""
    if "://" in raw:
        try:
            raw = urlsplit(raw).hostname or ""
        except ValueError:
            return ""
    else:
        raw = raw.split("/", 1)[0].split("?", 1)[0].split("#", 1)[0]
        if raw.startswith("[") and "]" in raw:
            raw = raw[1:raw.index("]")]
        elif raw.count(":") == 1:
            host, port = raw.rsplit(":", 1)
            if port.isdigit():
                raw = host
    raw = re.sub(r"^[*@.]+", "", raw).replace("www.", "")
    raw = re.sub(r"^[*@.]+", "", raw).rstrip(".")
    return raw


def host_monitored(host: str, domains: list[str]) -> bool:
    host = _normalize_host_domain(host)
    if not host:
        return False
    for domain in domains:
        d = _normalize_host_domain(str(domain or "").lstrip("*."))
        if not d:
            continue
        if host == d or host.endswith("." + d):
            return True
    return False


def is_internal_host(host: str) -> bool:
    """Check if host is internal/localhost/private network or the Unifai portal itself."""
    h = _normalize_host_domain(host)
    if not h or h in ("localhost", "127.0.0.1", "::1", "0.0.0.0"):
        return True
    if h.endswith(".local") or h.endswith(".internal") or h.endswith(".lan") or h.endswith(".home") or h.endswith(".corp"):
        return True
    import ipaddress
    try:
        ip = ipaddress.ip_address(h)
        if ip.is_loopback or ip.is_private or ip.is_link_local or ip.is_unspecified:
            return True
    except Exception:
        pass
    backend = GATEWAY_BACKEND_URL or ""
    if backend:
        b_host = _normalize_host_domain(backend)
        if b_host and (h == b_host or h.endswith("." + b_host)):
            return True
    server_dom = os.environ.get("SERVER_DOMAIN") or ""
    if server_dom:
        s_host = _normalize_host_domain(server_dom)
        if s_host and (h == s_host or h.endswith("." + s_host)):
            return True
    return False


def monitored_hosts() -> list[str]:
    """Domains the gateway monitors. Strictly synchronized live with PostgreSQL Target Websites."""
    import time

    global _sites_cache, _blocked_sites_cache, _domain_platform_map
    from agent_http import _http_get_text

    now = time.time()
    with _sites_lock:
        cached_at, cached = _sites_cache
        if cached and now - cached_at < 5:
            return list(cached)
    hosts: list[str] = []
    b_hosts: list[str] = []
    platform_map: dict[str, str] = {}
    fetch_ok = False
    body = ""
    candidates = [GATEWAY_BACKEND_URL.rstrip("/")] if GATEWAY_BACKEND_URL else []
    seen_urls = set()
    for base in candidates:
        if base in seen_urls:
            continue
        seen_urls.add(base)
        body = _http_get_text(
            f"{base}/api/browser-ai/targets?for=agent",
            "application/json",
            timeout=3,
        ) or ""
        if "targets" not in body:
            body = _http_get_text(
                f"{base}/api/browser-ai/targets",
                "application/json",
                timeout=3,
            ) or ""
        if "targets" in body:
            break
    try:
        data = json.loads(body) if body else {}
    except Exception:
        data = {}
    rows = data.get("targets") if isinstance(data, dict) else None
    if isinstance(rows, list):
        fetch_ok = True
        for row in rows:
            if not isinstance(row, dict):
                continue
            is_mon = bool(row.get("monitored")) or str(row.get("status") or "").upper() in ("MONITORED", "BLOCKED") or str(row.get("action") or "").lower() in ("monitor", "monitored", "block")
            is_blk = bool(row.get("block_site")) or str(row.get("status") or "").upper() == "BLOCKED" or str(row.get("action") or "").lower() == "block"
            if not is_mon and not is_blk:
                continue
            domain = _normalize_host_domain(str(row.get("domain") or "").lstrip("*."))
            if domain:
                if domain not in hosts:
                    hosts.append(domain)
                if is_blk and domain not in b_hosts:
                    b_hosts.append(domain)
                plat = str(row.get("platform_name") or "").strip()
                if plat:
                    platform_map[domain] = plat

    disk_cache_file = os.path.join(data_dir(), "targets_cache.json")
    if fetch_ok:
        # Backend reachable: strictly honor the database target websites list and persist to disk cache
        with _sites_lock:
            _sites_cache = (now, hosts)
            _blocked_sites_cache = (now, b_hosts)
            _domain_platform_map = platform_map
        try:
            with open(disk_cache_file, "w", encoding="utf-8") as f:
                json.dump({"hosts": hosts, "blocked_hosts": b_hosts, "platform_map": platform_map}, f)
        except Exception:
            pass
        return list(hosts)

    # Backend unreachable (offline startup): read last known DB targets from disk cache
    with _sites_lock:
        if _sites_cache[1]:
            return list(_sites_cache[1])
        if os.path.isfile(disk_cache_file):
            try:
                with open(disk_cache_file, "r", encoding="utf-8") as f:
                    saved = json.load(f)
                    hosts = saved.get("hosts", [])
                    b_hosts = saved.get("blocked_hosts", [])
                    _domain_platform_map = saved.get("platform_map", {})
                    if hosts:
                        _sites_cache = (now, hosts)
                        _blocked_sites_cache = (now, b_hosts)
                        return list(hosts)
            except Exception:
                pass
        # Fallback: check Chrome local storage mirror
        chrome_cache_file = os.path.join(data_dir(), "chrome_storage_cache.json")
        if os.path.isfile(chrome_cache_file):
            try:
                with open(chrome_cache_file, "r", encoding="utf-8") as f:
                    c_saved = json.load(f)
                    c_hosts = c_saved.get("cached_sites") or []
                    c_b_hosts = c_saved.get("cached_blocked_sites") or []
                    for h in c_hosts:
                        d = _normalize_host_domain(str(h).lstrip("*."))
                        if d and d not in hosts:
                            hosts.append(d)
                    for h in c_b_hosts:
                        d = _normalize_host_domain(str(h).lstrip("*."))
                        if d and d not in b_hosts:
                            b_hosts.append(d)
                    if hosts:
                        _sites_cache = (now, hosts)
                        _blocked_sites_cache = (now, b_hosts)
                        return list(hosts)
            except Exception:
                pass
        # Fallback: scan disk LevelDB for browser extension local storage
        try:
            scanned = scan_disk_for_browser_extension_storage()
            if scanned.get("cached_sites"):
                for h in scanned["cached_sites"]:
                    d = _normalize_host_domain(str(h).lstrip("*."))
                    if d and d not in hosts:
                        hosts.append(d)
                if hosts:
                    _sites_cache = (now, hosts)
                    _blocked_sites_cache = (now, b_hosts)
                    return list(hosts)
        except Exception:
            pass
        _sites_cache = (now, [])
        _blocked_sites_cache = (now, [])
    return []


def invalidate_policy_caches() -> None:
    """Force the next sites/rules read to hit the backend (keeps data for offline fallback)."""
    global _sites_cache, _rules_cache
    with _sites_lock:
        _sites_cache = (0.0, _sites_cache[1])
    with _rules_lock:
        _rules_cache = (0.0, _rules_cache[1])


def blocked_hosts() -> list[str]:
    """Return all target domains marked with block_site = True."""
    monitored_hosts()
    with _sites_lock:
        return list(_blocked_sites_cache[1] if len(_blocked_sites_cache) > 1 and _blocked_sites_cache[1] else [])


_rules_lock = threading.Lock()
_rules_cache: tuple[float, list[dict]] = (0.0, [])
_governance_cache: dict = {
    "block_upload": False,
    "upload_warning": "",
    "upload_target_websites": "",
    "file_extension_mode": "block_listed",
    "restricted_file_extensions": "",
    "allowed_file_extensions": "",
    "block_clipboard": False,
    "clipboard_target_websites": "",
    "block_drag_drop": False,
    "drag_drop_target_websites": "",
    "block_download": False,
    "download_target_websites": "",
    "governance_target_websites": "",
    "governance_block_message": "",
    "clipboard_block_message": "",
    "drag_drop_block_message": "",
    "download_block_message": "",
    "block_screenshot": False,
    "screenshot_block_message": "",
    "screenshot_target_websites": "",
    "enforce_corporate_domains": False,
    "corporate_email_domains": "",
    "allowed_workspace_ids": "",
    "corporate_target_websites": "",
    "corporate_block_message": "",
}


def scan_disk_for_browser_extension_storage() -> dict:
    """Scan local Chrome, Edge, and Chromium LevelDB folders for cached extension policy & uninstall keys."""
    result: dict = {}
    import glob
    base_dirs = []
    local_app_data = os.environ.get("LOCALAPPDATA") or ""
    app_data = os.environ.get("APPDATA") or ""
    if local_app_data:
        base_dirs.extend([
            os.path.join(local_app_data, "Google", "Chrome", "User Data"),
            os.path.join(local_app_data, "Microsoft", "Edge", "User Data"),
            os.path.join(local_app_data, "BraveSoftware", "Brave-Browser", "User Data"),
        ])
    if app_data:
        base_dirs.append(os.path.join(app_data, "Opera Software", "Opera Stable"))

    ldb_files = []
    for udir in base_dirs:
        if os.path.isdir(udir):
            for pattern in (
                os.path.join(udir, "*", "Local Extension Settings", "*neiemlideblokbpmnjblipmhgkklfjbd*", "*.log"),
                os.path.join(udir, "*", "Local Extension Settings", "*neiemlideblokbpmnjblipmhgkklfjbd*", "*.ldb"),
                os.path.join(udir, "*", "Local Extension Settings", "*", "*.log"),
                os.path.join(udir, "*", "Local Extension Settings", "*", "*.ldb"),
            ):
                for fpath in glob.glob(pattern):
                    if fpath not in ldb_files:
                        ldb_files.append(fpath)

    for lf in ldb_files[:60]:
        try:
            with open(lf, "rb") as f:
                content = f.read(2_000_000)
            if b"cached_sites" in content and "cached_sites" not in result:
                m = re.search(rb'cached_sites[^\w\[]+(\[[^\]]+\])', content)
                if m:
                    try:
                        arr = json.loads(m.group(1).decode("utf-8", errors="ignore"))
                        if isinstance(arr, list) and arr:
                            result["cached_sites"] = arr
                    except Exception:
                        pass
            if b"cached_rules" in content and "cached_rules" not in result:
                m = re.search(rb'cached_rules[^\w\[]+(\[[^\]]+\])', content)
                if m:
                    try:
                        arr = json.loads(m.group(1).decode("utf-8", errors="ignore"))
                        if isinstance(arr, list) and arr:
                            result["cached_rules"] = arr
                    except Exception:
                        pass
            for k in ("company_key_hash", "common_key_hash", "agent_key_hash", "daily_key_hash"):
                if k not in result and k.encode("ascii") in content:
                    km = re.search(rf'{k}[^\"]*\"([a-f0-9]{{64}})\"'.encode("ascii"), content, re.IGNORECASE)
                    if km:
                        val = km.group(1).decode("ascii")
                        result[k] = val
                        if k in ("company_key_hash", "common_key_hash"):
                            result["company_key_hash"] = val
                            result["common_key_hash"] = val
                        if k in ("agent_key_hash", "daily_key_hash"):
                            result["agent_key_hash"] = val
                            result["daily_key_hash"] = val
            if result.get("cached_sites") and result.get("company_key_hash") and result.get("agent_key_hash"):
                break
        except Exception:
            continue
    return result


def ingest_chrome_storage(data: dict) -> None:
    """Store Chrome extension local storage snapshot in agent memory and persistent disk files."""
    if not isinstance(data, dict) or not data:
        return
    import time
    now = time.time()
    from guard_platform import data_dir
    d_dir = data_dir()

    # 1. Save full Chrome local storage mirror
    chrome_storage_file = os.path.join(d_dir, "chrome_storage_cache.json")
    try:
        with open(chrome_storage_file, "w", encoding="utf-8") as f:
            json.dump(data, f, indent=2)
    except Exception:
        pass

    # 2. Extract and update target domains & blocked sites
    sites = data.get("cached_sites")
    b_sites = data.get("cached_blocked_sites")
    if isinstance(sites, list) and sites:
        h_list = []
        for s in sites:
            norm = _normalize_host_domain(str(s).lstrip("*."))
            if norm and norm not in h_list:
                h_list.append(norm)
        b_list = []
        if isinstance(b_sites, list):
            for s in b_sites:
                norm = _normalize_host_domain(str(s).lstrip("*."))
                if norm and norm not in b_list:
                    b_list.append(norm)
        with _sites_lock:
            global _sites_cache, _blocked_sites_cache
            _sites_cache = (now, h_list)
            _blocked_sites_cache = (now, b_list)
        try:
            with open(os.path.join(d_dir, "targets_cache.json"), "w", encoding="utf-8") as f:
                json.dump({"hosts": h_list, "blocked_hosts": b_list, "platform_map": _domain_platform_map}, f, indent=2)
        except Exception:
            pass
        try:
            from agent_pac_content import build_pac_from_targets, write_local_pac
            from agent_config import PROXY_ADDR
            pac = build_pac_from_targets(PROXY_ADDR)
            if pac:
                write_local_pac(pac)
        except Exception:
            pass

    # 3. Extract and update DLP rules
    rules = data.get("cached_rules")
    if isinstance(rules, list) and rules:
        active_rules = []
        for r in rules:
            if isinstance(r, dict):
                pat = str(r.get("pattern") or "").strip()
                if pat:
                    active_rules.append({
                        "id": str(r.get("id") or ""),
                        "name": str(r.get("name") or ""),
                        "pattern": pat,
                        "action": str(r.get("action") or "BLOCK"),
                        "warning_message": str(r.get("warning_message") or ""),
                        "description": str(r.get("description") or ""),
                    })
        if active_rules:
            with _rules_lock:
                global _rules_cache
                _rules_cache = (now, active_rules)
            try:
                with open(os.path.join(d_dir, "rules_cache.json"), "w", encoding="utf-8") as f:
                    json.dump(active_rules, f, indent=2)
            except Exception:
                pass

    # 4. Extract and update governance settings
    global _governance_cache
    gov_keys = (
        "block_upload", "upload_warning", "upload_target_websites",
        "file_extension_mode", "restricted_file_extensions", "allowed_file_extensions",
        "block_file_extension", "file_extension_block_message", "file_extension_target_websites",
        "block_clipboard", "clipboard_target_websites", "clipboard_block_message",
        "block_drag_drop", "drag_drop_target_websites", "drag_drop_block_message",
        "block_download", "download_target_websites", "download_block_message",
        "block_screenshot", "screenshot_block_message", "screenshot_target_websites",
        "enforce_corporate_domains", "corporate_email_domains", "allowed_workspace_ids",
        "corporate_target_websites", "corporate_block_message",
        "governance_target_websites", "governance_block_message",
    )
    has_gov = False
    for k in gov_keys:
        if k in data:
            has_gov = True
            _governance_cache[k] = data[k]
    if has_gov:
        try:
            with open(os.path.join(d_dir, "governance_cache.json"), "w", encoding="utf-8") as f:
                json.dump(_governance_cache, f, indent=2)
        except Exception:
            pass

    # 5. Extract uninstall keys
    if data.get("uninstall_key_configured") or data.get("company_key_hash") or data.get("agent_key_hash") or data.get("common_key_hash") or data.get("daily_key_hash"):
        try:
            cache_file = os.path.join(d_dir, "cached_uninstall_keys.json")
            existing = {}
            if os.path.isfile(cache_file):
                try:
                    with open(cache_file, "r", encoding="utf-8") as ef:
                        existing = json.load(ef)
                except Exception:
                    pass
            existing["uninstall_key_configured"] = bool(data.get("uninstall_key_configured"))
            if data.get("uninstall_key_rotated_at"):
                existing["rotated_at"] = data.get("uninstall_key_rotated_at")
            c_key = data.get("company_key_hash") or data.get("common_key_hash")
            if c_key:
                existing["company_key_hash"] = c_key
                existing["common_key_hash"] = c_key
            d_key = data.get("agent_key_hash") or data.get("daily_key_hash")
            if d_key:
                existing["agent_key_hash"] = d_key
                existing["daily_key_hash"] = d_key
            with open(cache_file, "w", encoding="utf-8") as f:
                json.dump(existing, f, indent=2)
        except Exception:
            pass

    print("[Gateway Guard] Synced & persisted policies from Chrome local storage.")


def get_cached_chrome_storage() -> dict:
    from guard_platform import data_dir
    d_dir = data_dir()
    cache_file = os.path.join(d_dir, "chrome_storage_cache.json")
    if os.path.isfile(cache_file):
        try:
            with open(cache_file, "r", encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            pass
    hosts = monitored_hosts()
    b_hosts = blocked_hosts()
    rules = dynamic_rules()
    gov = get_governance_controls()
    res = {
        "cached_sites": hosts,
        "cached_blocked_sites": b_hosts,
        "cached_rules": rules,
        "last_sync": time.time(),
    }
    res.update(gov)
    return res


def get_governance_controls() -> dict:
    """Return live data governance settings (clipboard, drag-drop, download)."""
    global _governance_cache
    # Ensure cache is refreshed by calling dynamic_rules() if empty or old
    try:
        dynamic_rules()
    except Exception:
        pass
    if not any(_governance_cache.values()):
        disk_gov_file = os.path.join(data_dir(), "governance_cache.json")
        if os.path.isfile(disk_gov_file):
            try:
                with open(disk_gov_file, "r", encoding="utf-8") as gf:
                    _governance_cache.update(json.load(gf))
            except Exception:
                pass
        elif os.path.isfile(os.path.join(data_dir(), "chrome_storage_cache.json")):
            try:
                with open(os.path.join(data_dir(), "chrome_storage_cache.json"), "r", encoding="utf-8") as cf:
                    c_data = json.load(cf)
                    for k in _governance_cache:
                        if k in c_data:
                            _governance_cache[k] = c_data[k]
            except Exception:
                pass
    return dict(_governance_cache)


def dynamic_rules() -> list[dict]:
    """Fetch live DLP rules directly from the Gateway backend / PostgreSQL DB."""
    global _rules_cache
    import time
    now = time.time()
    with _rules_lock:
        cached_at, cached = _rules_cache
        if cached is not None and (now - cached_at) < 5:
            return list(cached)
    active_rules = []
    fetch_ok = False
    candidates = [GATEWAY_BACKEND_URL.rstrip("/")] if GATEWAY_BACKEND_URL else []
    seen_urls = set()
    from agent_http import _http_json
    for base in candidates:
        if base in seen_urls:
            continue
        seen_urls.add(base)
        try:
            code, data = _http_json("GET", f"{base}/api/browser-ai/rules?for=agent", timeout=3)
            if code != 200 or not isinstance(data, dict):
                code, data = _http_json("GET", f"{base}/api/browser-ai/rules", timeout=3)
            if code == 200 and isinstance(data, dict):
                fetch_ok = True
                raw_rules = data.get("rules") or []
                for r in raw_rules:
                    if isinstance(r, dict) and r.get("active") is not False:
                        pattern = str(r.get("pattern") or "").strip()
                        if pattern:
                            active_rules.append({
                                "id": str(r.get("id") or ""),
                                "name": str(r.get("name") or ""),
                                "pattern": pattern,
                                "action": str(r.get("action") or "BLOCK"),
                                "warning_message": str(r.get("warning_message") or ""),
                                "description": str(r.get("description") or ""),
                            })
                global _governance_cache
                if "block_clipboard" in data or "block_upload" in data:
                    _governance_cache = {
                        "block_upload": bool(data.get("block_upload")),
                        "upload_warning": str(data.get("upload_warning") or ""),
                        "upload_target_websites": str(data.get("upload_target_websites") or ""),
                        "file_extension_mode": str(data.get("file_extension_mode") or "block_listed"),
                        "restricted_file_extensions": str(data.get("restricted_file_extensions") or ""),
                        "allowed_file_extensions": str(data.get("allowed_file_extensions") or ""),
                        "block_file_extension": bool(data.get("block_file_extension")),
                        "file_extension_block_message": str(data.get("file_extension_block_message") or ""),
                        "file_extension_target_websites": str(data.get("file_extension_target_websites") or ""),
                        "block_clipboard": bool(data.get("block_clipboard")),
                        "clipboard_target_websites": str(data.get("clipboard_target_websites") or ""),
                        "block_drag_drop": bool(data.get("block_drag_drop")),
                        "drag_drop_target_websites": str(data.get("drag_drop_target_websites") or ""),
                        "block_download": bool(data.get("block_download")),
                        "download_target_websites": str(data.get("download_target_websites") or ""),
                        "governance_target_websites": str(data.get("governance_target_websites") or ""),
                        "governance_block_message": str(data.get("governance_block_message") or ""),
                        "clipboard_block_message": str(data.get("clipboard_block_message") or ""),
                        "drag_drop_block_message": str(data.get("drag_drop_block_message") or ""),
                        "download_block_message": str(data.get("download_block_message") or ""),
                        "block_screenshot": bool(data.get("block_screenshot")),
                        "screenshot_block_message": str(data.get("screenshot_block_message") or ""),
                        "screenshot_target_websites": str(data.get("screenshot_target_websites") or ""),
                        "enforce_corporate_domains": bool(data.get("enforce_corporate_domains")),
                        "corporate_email_domains": str(data.get("corporate_email_domains") or ""),
                        "allowed_workspace_ids": str(data.get("allowed_workspace_ids") or ""),
                        "corporate_target_websites": str(data.get("corporate_target_websites") or ""),
                        "corporate_block_message": str(data.get("corporate_block_message") or ""),
                    }
                break
        except Exception:
            pass

    disk_rules_file = os.path.join(data_dir(), "rules_cache.json")
    disk_gov_file = os.path.join(data_dir(), "governance_cache.json")
    with _rules_lock:
        if fetch_ok:
            _rules_cache = (now, active_rules)
            try:
                with open(disk_rules_file, "w", encoding="utf-8") as f:
                    json.dump(active_rules, f, indent=2)
            except Exception:
                pass
            try:
                with open(disk_gov_file, "w", encoding="utf-8") as gf:
                    json.dump(_governance_cache, gf, indent=2)
            except Exception:
                pass
            return active_rules
        if _rules_cache[1]:
            return list(_rules_cache[1])
        if os.path.isfile(disk_rules_file):
            try:
                with open(disk_rules_file, "r", encoding="utf-8") as f:
                    active_rules = json.load(f)
                if active_rules:
                    _rules_cache = (now, active_rules)
            except Exception:
                pass
        # Fallback to Chrome local storage mirror
        if not active_rules:
            chrome_cache_file = os.path.join(data_dir(), "chrome_storage_cache.json")
            if os.path.isfile(chrome_cache_file):
                try:
                    with open(chrome_cache_file, "r", encoding="utf-8") as f:
                        c_saved = json.load(f)
                        c_rules = c_saved.get("cached_rules") or []
                        for r in c_rules:
                            if isinstance(r, dict) and r.get("pattern"):
                                active_rules.append(r)
                        if active_rules:
                            _rules_cache = (now, active_rules)
                except Exception:
                    pass
        # Fallback to scanning disk LevelDB
        if not active_rules:
            try:
                scanned = scan_disk_for_browser_extension_storage()
                if scanned.get("cached_rules"):
                    active_rules = scanned["cached_rules"]
                    _rules_cache = (now, active_rules)
            except Exception:
                pass

        # Load cached governance offline
        if os.path.isfile(disk_gov_file):
            try:
                with open(disk_gov_file, "r", encoding="utf-8") as gf:
                    _governance_cache.update(json.load(gf))
            except Exception:
                pass
        elif os.path.isfile(os.path.join(data_dir(), "chrome_storage_cache.json")):
            try:
                with open(os.path.join(data_dir(), "chrome_storage_cache.json"), "r", encoding="utf-8") as cf:
                    c_data = json.load(cf)
                    for k in _governance_cache:
                        if k in c_data:
                            _governance_cache[k] = c_data[k]
            except Exception:
                pass

        if active_rules:
            return active_rules
        _rules_cache = (now, [])
    return []


def _compile_rule_pattern(raw: str):
    import re
    pattern = (raw or "").strip()
    flags = 0
    while pattern.startswith("(?i)") or pattern.startswith("(?m)"):
        if pattern.startswith("(?i)"):
            flags |= re.IGNORECASE
            pattern = pattern[4:]
        else:
            flags |= re.MULTILINE
            pattern = pattern[4:]
    return re.compile(pattern, flags)


def redact_prompt_text(text: str, rule_name: str = "") -> str:
    """Mask the text that matched a redact rule. The warning stays in the card, not in the site request."""
    rules = dynamic_rules()
    chosen = []
    for rule in rules:
        action = str(rule.get("action") or "").upper()
        if action not in ("REDACT", "REDACTED"):
            continue
        if rule_name and str(rule.get("name") or "") == rule_name:
            chosen = [rule]
            break
        chosen.append(rule)
    out = text or ""
    for rule in chosen:
        try:
            out = _compile_rule_pattern(str(rule.get("pattern") or "")).sub("[REDACTED]", out)
        except Exception:
            continue
    return out


def interpret_guard_decision(data: dict, text: str, rule: str = "") -> dict:
    """Map the gateway intercept result onto block, warning, or redact."""
    allowed = bool(data.get("allowed", True))
    action = str(data.get("action") or "")
    act = action.lower()
    rule_name = rule or str(data.get("rule_triggered") or "")
    warning = str(data.get("warning_message") or "").strip()
    reply = str(data.get("reply_text") or data.get("security_message") or "").strip()
    effective_msg = warning or reply or "This request was blocked by corporate security policy."
    if (not allowed) or act in ("blocked", "block", "bot answered", "replied"):
        return {
            "blocked": True,
            "action": "Blocked" if act not in ("bot answered", "replied") else action,
            "message": effective_msg,
            "rule": rule_name,
            "redacted_prompt": "",
        }
    if act in ("redacted", "redact"):
        redacted = redact_prompt_text(text, rule_name)
        return {
            "blocked": False,
            "action": "Redacted",
            "message": warning or "Sensitive text was redacted by YP-Raksha.",
            "rule": rule_name,
            "redacted_prompt": redacted,
        }
    if act in ("warned", "warn", "alert"):
        return {
            "blocked": False,
            "action": "Warned",
            "message": warning or reply or "This request triggered a YP-Raksha warning.",
            "rule": rule_name,
            "redacted_prompt": "",
        }
    return {
        "blocked": False,
        "action": action or "Allowed",
        "message": "",
        "rule": rule_name,
        "redacted_prompt": "",
    }


_RECENT_EXTENSION_EVALS_LOCK = threading.Lock()
_RECENT_EXTENSION_EVALS: list[dict] = []


def _record_extension_eval(
    host: str,
    prompt: str,
    files: list | None = None,
    platform: str = "",
    eval_id: str = "",
    is_blocked: bool = False,
    action: str = "Allowed",
    rule: str = "",
    message: str = "",
) -> None:
    """Record that the browser extension handled this prompt/upload for proxy deduplication."""
    try:
        from guard_platform import data_dir
        cache_path = os.path.join(data_dir(), "recent_extension_evals.json")
        now = time.time()
        fnames = []
        if isinstance(files, list):
            for f in files:
                if isinstance(f, dict) and f.get("name"):
                    fnames.append(str(f["name"]).strip())
                elif isinstance(f, str) and f.strip():
                    fnames.append(f.strip())

        clean_h = (host or "").lower().strip(".")
        if clean_h.startswith("www."):
            clean_h = clean_h[4:]

        # Strip invisible zero-width formatting characters
        p_norm = re.sub(r'[\u200B-\u200F\uFEFF\u202A-\u202E\u2060-\u206F]', '', prompt or '').replace('\u00A0', ' ')
        p_clean = " ".join(p_norm.strip().split())

        COPILOT_FAMILY = {
            "copilot.com", "www.copilot.com", "copilot.microsoft.com",
            "copilot.cloud.microsoft", "substrate.office.com", "sydneyp.bing.com",
            "edgeservices.bing.com", "bing.com",
        }
        hosts_to_record = [clean_h]
        if clean_h in COPILOT_FAMILY:
            for fam in COPILOT_FAMILY:
                if fam not in hosts_to_record:
                    hosts_to_record.append(fam)

        with _RECENT_EXTENSION_EVALS_LOCK:
            global _RECENT_EXTENSION_EVALS
            # Prune entries older than 60 seconds
            _RECENT_EXTENSION_EVALS = [e for e in _RECENT_EXTENSION_EVALS if now - float(e.get("time") or 0) < 60]
            for h in hosts_to_record:
                entry = {
                    "host": h,
                    "platform": (platform or "").strip(),
                    "preview": p_clean[:150],
                    "files": fnames,
                    "time": now,
                    "eval_id": str(eval_id or "").strip(),
                    "id": str(eval_id or "").strip(),
                    "is_blocked": bool(is_blocked),
                    "action": str(action or ("Blocked" if is_blocked else "Allowed")),
                    "rule": str(rule or ""),
                    "message": str(message or ""),
                }
                _RECENT_EXTENSION_EVALS.append(entry)
            cache_paths = [cache_path]
            yp_dir = os.path.join(os.path.dirname(os.path.dirname(cache_path)), "YP", "Raksha")
            if os.path.isdir(yp_dir):
                cache_paths.append(os.path.join(yp_dir, "recent_extension_evals.json"))
            for cp in cache_paths:
                try:
                    tmp_path = cp + f".tmp.{os.getpid()}"
                    with open(tmp_path, "w", encoding="utf-8") as fp:
                        json.dump(_RECENT_EXTENSION_EVALS, fp)
                    os.replace(tmp_path, cp)
                except Exception:
                    try:
                        os.remove(tmp_path)
                    except Exception:
                        pass
    except Exception:
        pass


_RECENT_FILE_POSTS_LOCK = threading.Lock()
_RECENT_FILE_POSTS: dict[str, float] = {}

_RECENT_FILE_CAPTIONS_LOCK = threading.Lock()
_RECENT_FILE_CAPTIONS: dict[str, float] = {}


def _is_duplicate_file_post(key: str, ttl: float = 15.0) -> bool:
    now = time.time()
    with _RECENT_FILE_POSTS_LOCK:
        last = _RECENT_FILE_POSTS.get(key, 0)
        if now - last < ttl:
            return True
        _RECENT_FILE_POSTS[key] = now
        if len(_RECENT_FILE_POSTS) > 200:
            for k in list(_RECENT_FILE_POSTS):
                if now - _RECENT_FILE_POSTS[k] > 60:
                    del _RECENT_FILE_POSTS[k]
        return False


def _post_intercept_file(
    platform: str,
    prompt: str,
    file_name: str,
    content_type: str,
    file_payload: bytes,
    metadata: dict,
    client_ip: str = "127.0.0.1",
    agent_id: str = "",
    agent_hostname: str = "",
) -> bool:
    """Post single file upload to /api/browser-ai/intercept-file so backend stores attachment and activates View."""
    eval_id = metadata.get("eval_id") if isinstance(metadata, dict) else ""
    dedupe_key = f"file_eval|{eval_id}|{file_name}" if eval_id else f"file_post|{platform}|{file_name}|{len(file_payload)}|{hash(prompt)}"
    if _is_duplicate_file_post(dedupe_key, ttl=1.5):
        print(f"[agent_extension] Skipping burst duplicate file post for {file_name}")
        return True
    try:
        from agent_http import _DIRECT_OPENER, _guard_headers
        boundary = f"----Gateway{int(time.time() * 1000)}"
        parts: list[bytes] = []

        def add_field(name: str, value: str) -> None:
            parts.append(
                f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"\r\n\r\n{value}\r\n".encode("utf-8")
            )

        add_field("platform", platform)
        add_field("prompt", prompt)
        add_field("client_ip", client_ip)
        add_field("agent_id", agent_id or "")
        add_field("agent_hostname", agent_hostname or "")
        add_field("agent_type", "endpoint")
        add_field("file_name", file_name)
        add_field("content_type", content_type or "application/octet-stream")
        ext_text = str((metadata or {}).get("extracted_text") or "")
        if ext_text:
            add_field("extracted_text", ext_text)
        add_field("metadata", json.dumps(metadata))
        if file_payload and len(file_payload) <= 20 * 1024 * 1024:
            hdr = (
                f"--{boundary}\r\n"
                f"Content-Disposition: form-data; name=\"file\"; filename=\"{file_name}\"\r\n"
                f"Content-Type: {content_type or 'application/octet-stream'}\r\n\r\n"
            ).encode("utf-8")
            parts.append(hdr)
            parts.append(file_payload)
            parts.append(b"\r\n")
        parts.append(f"--{boundary}--\r\n".encode("utf-8"))
        body = b"".join(parts)
        candidates = [GATEWAY_BACKEND_URL.rstrip("/")] if GATEWAY_BACKEND_URL else []
        seen_cands = set()
        for base in candidates:
            if base in seen_cands:
                continue
            seen_cands.add(base)
            try:
                req = urllib.request.Request(
                    f"{base}/api/browser-ai/intercept-file",
                    data=body,
                    headers=_guard_headers({"Content-Type": f"multipart/form-data; boundary={boundary}"}),
                    method="POST",
                )
                timeout = 10 if file_payload else 4
                with _DIRECT_OPENER.open(req, timeout=timeout) as resp:
                    if 200 <= getattr(resp, "status", 200) < 300:
                        return True
            except Exception as e:
                print(f"[agent_extension] _post_intercept_file ({base}) failed: {e}")
        return False
    except Exception as e:
        print(f"[agent_extension] _post_intercept_file failed: {e}")
        return False


def detect_real_file_type(raw_b: bytes, ftext: str = "", file_name: str = "", ctype: str = "") -> str:
    """Detect real file type by magic bytes, MIME, and text content signatures to prevent file renaming bypass."""
    if raw_b:
        if raw_b.startswith(b"MZ"):
            return "exe"
        if raw_b.startswith(b"\x7fELF"):
            return "elf"
        if raw_b.startswith(b"PK\x03\x04") or raw_b.startswith(b"PK\x05\x06"):
            if b"word/" in raw_b[:2048] or b"[Content_Types].xml" in raw_b[:2048]:
                return "docx"
            if b"xl/" in raw_b[:2048]:
                return "xlsx"
            if b"ppt/" in raw_b[:2048]:
                return "pptx"
            return "zip"
        if raw_b.startswith(b"7z\xbc\xaf\x27\x1c"):
            return "7z"
        if raw_b.startswith(b"\x1f\x8b"):
            return "gz"
        if raw_b.startswith(b"Rar!\x1a\x07"):
            return "rar"
        if raw_b.startswith(b"%PDF-"):
            return "pdf"
        if raw_b.startswith(b"-----BEGIN ") or b"-----BEGIN " in raw_b[:256]:
            return "pem"

    txt = (ftext or (raw_b[:2048].decode('utf-8', errors='ignore') if raw_b else "")).strip()
    if txt:
        if txt.startswith("-----BEGIN ") or "-----BEGIN RSA PRIVATE KEY" in txt or "-----BEGIN PRIVATE KEY" in txt:
            return "pem"
        if re.search(r'^(?:--|/\*|CREATE\s+TABLE|INSERT\s+INTO|DROP\s+TABLE|ALTER\s+TABLE|USE\s+|SELECT\s+.+\s+FROM|LOCK\s+TABLES)\b', txt, re.IGNORECASE):
            return "sql"
        if re.search(r'^(?:export\s+)?[A-Z0-9_]{3,}\s*=\s*[\w\-\.]{4,}', txt, re.MULTILINE):
            if any(k in txt.upper() for k in ("API_KEY", "SECRET", "PASSWORD", "DATABASE_URL", "TOKEN", "AWS_ACCESS_KEY")):
                return "env"

    ct = (ctype or "").lower().strip()
    mime_map = {
        "application/x-msdownload": "exe", "application/x-dosexec": "exe", "application/x-executable": "exe",
        "application/zip": "zip", "application/x-zip-compressed": "zip", "application/x-7z-compressed": "7z",
        "application/x-tar": "tar", "application/gzip": "gz", "application/x-gzip": "gz", "application/pdf": "pdf",
        "application/sql": "sql", "application/x-sql": "sql", "text/x-sql": "sql",
        "application/x-pem-file": "pem", "application/x-x509-ca-cert": "pem",
    }
    if ct in mime_map:
        return mime_map[ct]
    return os.path.splitext(file_name)[1].lstrip('.').lower()


def _is_telemetry_payload(prompt: str, url: str = "") -> bool:
    """Detect background push notifications, telemetry, analytics, and noise payloads."""
    low_u = (url or "").lower()
    if (
        "/uc/" in low_u
        or "/lat/" in low_u
        or "/telemetry" in low_u
        or "/beacon" in low_u
        or "/analytics" in low_u
        or "/ces/" in low_u
    ):
        return True
    if not prompt:
        return False
    t = str(prompt).strip()
    if not t:
        return False
    low_t = t.lower()
    if low_t in ("set", "ping", "pong", "heartbeat", "null", "me", "true", "false", "ready", "ack", "sync"):
        return True
    if low_t.startswith("{") and (
        "app_name" in low_t
        or "has_web_push" in low_t
        or "web_push_" in low_t
        or "is_dark_mode" in low_t
        or "pixel_ratio" in low_t
        or "screen_height" in low_t
        or "screen_width" in low_t
        or "notification_permission" in low_t
        or "device_pixel_ratio" in low_t
    ):
        return True
    return False


def check_prompt(
    host: str,
    url: str,
    prompt: str,
    files: list | None = None,
    upload_images: list | None = None,
    is_blocked: bool = False,
    rule: str = "",
    eval_id: str = "",
) -> dict:
    """Evaluate prompt, files, and images against Gateway DLP policies."""
    if _is_telemetry_payload(prompt, url):
        return {"blocked": False, "passthrough": True, "action": "Allowed", "message": "", "rule": ""}
    # Immediate site block check
    clean_h = (host or "").strip().lower()
    if clean_h.startswith("www."):
        clean_h = clean_h[4:]
    if ":" in clean_h:
        clean_h = clean_h.split(":", 1)[0]
    b_hosts = blocked_hosts()
    for bh in b_hosts:
        bh_clean = bh.lower().strip()
        if bh_clean and (clean_h == bh_clean or clean_h.endswith("." + bh_clean)):
            return {
                "blocked": True,
                "action": "Blocked",
                "rule": "Site Blocked",
                "message": f"Access to {clean_h} is blocked by company security policy."
            }

    prompt = re.sub(r'[\u200B-\u200F\uFEFF\u202A-\u202E\u2060-\u206F]', '', prompt or '').replace('\u00A0', ' ').strip()
    text = prompt
    eval_text = prompt
    raw_files_list = files if isinstance(files, list) else []
    images_list = upload_images if isinstance(upload_images, list) else []

    # Clean and deduplicate files_list, ignoring truncated DOM artifacts (containing "..." or ellipsis)
    cleaned_files = []
    seen_fnames = set()
    for f in raw_files_list:
        fn = (f.get("name") if isinstance(f, dict) else str(f)).strip()
        if not fn:
            continue
        fn_lower = fn.lower()
        if fn_lower in seen_fnames or "..." in fn or "\u2026" in fn:
            continue
        seen_fnames.add(fn_lower)
        cleaned_files.append(f)
    files_list = cleaned_files

    # Instant check deduplication for identical requests within 10 seconds (bypass for explicit governance actions)
    is_governance_action = bool(
        prompt.startswith("--drag drop")
        or prompt.startswith("--copy paste")
        or prompt.startswith("--download")
        or prompt.startswith("--screenshot")
        or prompt.startswith("--corporate account")
        or "[screenshot" in prompt.lower()
        or "[screen capture" in prompt.lower()
        or is_blocked
    )
    file_sig = ",".join(sorted(seen_fnames))
    prompt_req_key = f"{clean_h}|{file_sig}|{prompt[:150]}"
    now_ts = time.time()
    if eval_id:
        with _RECENT_FILE_POSTS_LOCK:
            cached_dec = _RECENT_FILE_POSTS.get("eval_dec|" + eval_id)
            cached_ts = _RECENT_FILE_POSTS.get("eval_ts|" + eval_id, 0)
            if cached_dec and (now_ts - cached_ts < 1.0):
                return dict(cached_dec)
    elif not is_governance_action:
        with _RECENT_FILE_POSTS_LOCK:
            cached_dec = _RECENT_FILE_POSTS.get("decision|" + prompt_req_key)
            cached_ts = _RECENT_FILE_POSTS.get("ts|" + prompt_req_key, 0)
            if cached_dec and (now_ts - cached_ts < 0.8):
                return dict(cached_dec)

    # If this exact text was already handled as a caption for attached files on this host within the last 15 seconds,
    # return the cached decision and skip creating a duplicate standalone text row in Prompt Logs.
    if not files_list and not images_list and text and not is_governance_action:
        clean_text = " ".join(re.sub(r'[\u200B-\u200F\uFEFF\u202A-\u202E\u2060-\u206F]', '', text).replace('\u00A0', ' ').strip().split()).lower()
        if clean_text:
            with _RECENT_FILE_CAPTIONS_LOCK:
                now_c = time.time()
                for cap_k, cap_ts in list(_RECENT_FILE_CAPTIONS.items()):
                    if now_c - cap_ts > 20.0:
                        _RECENT_FILE_CAPTIONS.pop(cap_k, None)
                    elif cap_k == clean_text or clean_text.startswith(cap_k) or cap_k.startswith(clean_text):
                        return {
                            "blocked": False,
                            "action": "Allowed",
                            "rule": "",
                            "message": "",
                            "redacted_prompt": "",
                        }
        with _RECENT_EXTENSION_EVALS_LOCK:
            for ev in _RECENT_EXTENSION_EVALS:
                if (now_ts - float(ev.get("time") or 0) < 15.0):
                    ev_preview = (ev.get("preview") or "").lower()
                    ev_files = ev.get("files") or []
                    if ev_files and ev_preview and (ev_preview == clean_text[:150] or clean_text[:150].startswith(ev_preview) or ev_preview.startswith(clean_text[:150])):
                        return {
                            "blocked": bool(ev.get("is_blocked")),
                            "action": str(ev.get("action") or "Allowed"),
                            "rule": str(ev.get("rule") or ""),
                            "message": str(ev.get("message") or ""),
                            "redacted_prompt": "",
                        }

    file_names = []
    file_extracted_texts = []
    file_text_map = {}
    for f in files_list:
        if isinstance(f, dict):
            fname = str(f.get("name") or "").strip()
            ctype = str(f.get("type") or "application/octet-stream").strip()
            if fname and fname not in file_names:
                file_names.append(fname)
            fext = os.path.splitext(fname)[1].lower()
            norm_fext = fext.lstrip('.').lower()
            ftext = str(f.get("text") or "").strip()
            fb64 = str(f.get("base64") or "").strip()
            raw_b = b""
            if fb64:
                try:
                    import base64
                    raw_b = base64.b64decode(fb64)
                except Exception:
                    pass
            if not raw_b and ftext:
                try:
                    raw_b = ftext.encode("utf-8", errors="ignore")
                except Exception:
                    pass
            if not raw_b and fext in (".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".svg") and images_list:
                for img_cand in images_list:
                    if isinstance(img_cand, str) and img_cand:
                        cand_b64 = img_cand.split(",")[-1] if "," in img_cand else img_cand
                        try:
                            cand_bytes = base64.b64decode(cand_b64)
                            if len(cand_bytes) >= 8:
                                raw_b = cand_bytes
                                fb64 = cand_b64
                                f["base64"] = fb64
                                if not ctype or ctype == "application/octet-stream":
                                    ctype = f"image/{norm_fext or 'png'}"
                                    f["type"] = ctype
                                break
                        except Exception:
                            pass

            extracted = str(f.get("extracted_text") or "").strip()
            if not extracted and (raw_b or ftext):
                try:
                    p_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "proxy"))
                    if p_dir not in sys.path:
                        sys.path.insert(0, p_dir)
                    import browser_ai_proxy
                    extract_fn = getattr(browser_ai_proxy, "extract_upload_text_for_rules", None)
                    if extract_fn:
                        try:
                            extracted = extract_fn(raw_b, content_type=ctype, raw_text=ftext, file_name=fname)
                        except TypeError:
                            try:
                                extracted = extract_fn(raw_b, file_name=fname)
                            except Exception:
                                pass
                except Exception:
                    pass

                # Multi-encoding fallback if primary extractors returned empty
                if not extracted and raw_b:
                    for enc in ("utf-8", "utf-16", "utf-16-le", "utf-16-be", "latin-1", "cp1252"):
                        try:
                            t_dec = raw_b.decode(enc, errors="strict").strip()
                            if len(t_dec) > 3 and not any(ord(c) == 0 for c in t_dec[:100]):
                                extracted = t_dec
                                break
                        except Exception:
                            continue

                # Printable ASCII + UTF-16LE strings extraction for .exe, PE binaries, archives, and arbitrary data
                is_bin = (
                    raw_b.startswith(b"MZ")
                    or norm_fext in ("exe", "dll", "sys", "bin", "dat", "so", "dylib", "elf", "class")
                    or (b"\x00" in raw_b[:512])
                )
                if raw_b and (is_bin or not extracted) and len(raw_b) >= 4:
                    bin_parts = []
                    try:
                        strs = re.findall(rb'[\x20-\x7E]{4,}', raw_b)
                        for s_bytes in strs[:3000]:
                            s_dec = s_bytes.decode("ascii", errors="ignore").strip()
                            if len(s_dec) >= 4:
                                bin_parts.append(s_dec)
                    except Exception:
                        pass
                    try:
                        u16_strs = re.findall(rb'(?:[\x20-\x7E]\x00){4,}', raw_b)
                        for u_bytes in u16_strs[:3000]:
                            u_dec = u_bytes.decode("utf-16le", errors="ignore").strip()
                            if len(u_dec) >= 4 and u_dec not in bin_parts:
                                bin_parts.append(u_dec)
                    except Exception:
                        pass
                    if bin_parts:
                        extra_str = " ".join(bin_parts[:4000])
                        if extracted:
                            extracted += "\n" + extra_str
                        else:
                            extracted = extra_str

            f_content = extracted or ftext or ""
            if f_content:
                f["extracted_text"] = f_content
                file_extracted_texts.append(f_content)
                if fname:
                    file_text_map[fname] = f_content

            if ftext and len(ftext.strip()) > 0:
                eval_text += f"\n{ftext[:50000]}"
            if extracted and len(extracted.strip()) > 3:
                eval_text += f"\n{extracted[:100000]}"

            if fext in (".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".svg") and fb64:
                img_data = fb64 if fb64.startswith("data:") else f"data:image/{norm_fext or 'png'};base64,{fb64}"
                if img_data not in images_list:
                    images_list.append(img_data)
        elif isinstance(f, str) and f.strip():
            fn = f.strip()
            if fn not in file_names:
                file_names.append(fn)

    # Anti-spoofing real file type check against restricted or allowed extensions
    restricted_str = str(_governance_cache.get("restricted_file_extensions") or "")
    allowed_str = str(_governance_cache.get("allowed_file_extensions") or "")
    mode = str(_governance_cache.get("file_extension_mode") or "block_listed")

    violating_ext_files = []
    violating_ext_types = []
    for cf in files_list:
        cf_name = (cf.get("name") if isinstance(cf, dict) else str(cf)).strip()
        cf_ext = os.path.splitext(cf_name)[1].lower()
        cf_norm = cf_ext.lstrip('.').lower()
        cf_raw = b""
        cf_text = ""
        cf_type = ""
        if isinstance(cf, dict):
            cf_type = str(cf.get("type") or "application/octet-stream").strip()
            cf_b64 = str(cf.get("base64") or "").strip()
            cf_text = str(cf.get("text") or "").strip()
            if cf_b64:
                try:
                    import base64
                    cf_raw = base64.b64decode(cf_b64)
                except Exception:
                    pass
            if not cf_raw and cf_text:
                try:
                    cf_raw = cf_text.encode("utf-8", errors="ignore")
                except Exception:
                    pass
        cf_real = detect_real_file_type(cf_raw, cf_text, cf_name, cf_type)
        if mode == "allow_listed" and allowed_str:
            allowed_list = [x.strip().lstrip('.').lower() for x in re.split(r'[,;\s]+', allowed_str) if x.strip()]
            is_allowed = (cf_real and cf_real in allowed_list) or (cf_norm and cf_norm in allowed_list)
            if not is_allowed:
                if cf_name and cf_name not in violating_ext_files:
                    violating_ext_files.append(cf_name)
        elif mode != "allow_listed" and restricted_str:
            restricted_list = [x.strip().lstrip('.').lower() for x in re.split(r'[,;\s]+', restricted_str) if x.strip()]
            m_ext = cf_real if (cf_real and cf_real in restricted_list) else (cf_norm if (cf_norm and cf_norm in restricted_list) else "")
            if m_ext:
                if cf_name and cf_name not in violating_ext_files:
                    violating_ext_files.append(cf_name)
                ext_tag = f".{m_ext}"
                if ext_tag not in violating_ext_types:
                    violating_ext_types.append(ext_tag)

    if violating_ext_files:
        f_str = ", ".join(f"'{fn}'" for fn in violating_ext_files)
        types_str = ", ".join(violating_ext_types) if violating_ext_types else ""
        if mode == "allow_listed":
            msg = (
                f"File {f_str} is not permitted. Only approved file types ({', '.join('.' + e for e in allowed_list)}) may be uploaded. Please remove this file before sending."
                if len(violating_ext_files) == 1 else
                f"Files {f_str} are not permitted. Only approved file types ({', '.join('.' + e for e in allowed_list)}) may be uploaded. Please remove these files before sending."
            )
        else:
            msg = (
                f"File {f_str} ({types_str}) is restricted by corporate DLP policy. Please remove this file before sending."
                if len(violating_ext_files) == 1 else
                f"Files {f_str} ({types_str}) are restricted by corporate DLP policy. Please remove these files before sending."
            )
        return {
            "blocked": True,
            "action": "Blocked",
            "message": msg,
            "rule": f"Restricted File Type ({types_str.upper()})" if types_str else "Restricted File Type",
            "violating_files": violating_ext_files,
            "redacted_prompt": "",
        }

    violating_files = []
    prompt_violated = False
    block_message = ""
    matched_rule_dict = None
    raw_prompt_text = (prompt or "").strip()

    # Instant in-memory DLP regex match on prompt and individual files
    if (eval_text or file_text_map) and not is_blocked:
        try:
            p_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "proxy"))
            if p_dir not in sys.path:
                sys.path.insert(0, p_dir)
            import browser_ai_proxy
            if raw_prompt_text:
                m_matched, m_rule, m_act = getattr(browser_ai_proxy, "match_guard_rules_on_text", lambda *a: (False, "", ""))(raw_prompt_text)
                if m_matched and m_act.upper() in ("BLOCK", "BLOCKED"):
                    is_blocked = True
                    rule = m_rule
                    prompt_violated = True
            for fn, fcontent in file_text_map.items():
                if fcontent:
                    fm_matched, fm_rule, fm_act = getattr(browser_ai_proxy, "match_guard_rules_on_text", lambda *a: (False, "", ""))(fcontent)
                    if fm_matched and fm_act.upper() in ("BLOCK", "BLOCKED"):
                        is_blocked = True
                        if not rule:
                            rule = fm_rule
                        if fn not in violating_files:
                            violating_files.append(fn)
        except Exception:
            pass

        matched_rule_dict = None
        if not is_blocked:
            for r in dynamic_rules():
                pat = str(r.get("pattern") or "")
                if pat:
                    try:
                        rx = _compile_rule_pattern(pat)
                        p_match = bool(raw_prompt_text and rx.search(raw_prompt_text))
                        f_matches = []
                        for fn, fcontent in file_text_map.items():
                            if fcontent and rx.search(fcontent):
                                if fn not in f_matches:
                                    f_matches.append(fn)
                        if not p_match and not f_matches and rx.search(eval_text):
                            p_match = True

                        if p_match or f_matches:
                            act = str(r.get("action") or "BLOCK").upper()
                            if act in ("BLOCK", "BLOCKED"):
                                is_blocked = True
                                rule = str(r.get("name") or "Security Policy")
                                prompt_violated = p_match
                                violating_files = f_matches
                                matched_rule_dict = r
                                break
                    except Exception:
                        pass

    if is_blocked:
        r_name = rule or "Restricted Content"
        if not matched_rule_dict and r_name:
            for dr in dynamic_rules():
                if dr.get("name") == r_name or dr.get("id") == r_name:
                    matched_rule_dict = dr
                    break
        rule_reason = ""
        if matched_rule_dict:
            rule_reason = str(matched_rule_dict.get("warning_message") or matched_rule_dict.get("description") or "").strip()

        # If user configured a custom blocked/warning message on the rule, use it directly!
        if rule_reason:
            if violating_files:
                f_str = ", ".join(f"'{fn}'" for fn in violating_files)
                block_message = f"{rule_reason} (File: {f_str}). Please remove before sending."
            else:
                block_message = rule_reason
        else:
            if violating_files and prompt_violated:
                f_str = ", ".join(f"'{fn}'" for fn in violating_files)
                block_message = f"Prompt and file(s) {f_str} contain restricted data: {r_name}. Request blocked by security policy. Please remove the file(s) before sending."
            elif violating_files:
                f_str = ", ".join(f"'{fn}'" for fn in violating_files)
                block_message = f"File(s) {f_str} contain restricted data: {r_name}. Upload blocked by corporate DLP policy. Please remove before sending."
            elif prompt_violated:
                block_message = f"Prompt contains restricted data: {r_name}. Request blocked by security policy."
            else:
                block_message = f"Blocked by corporate DLP policy: {r_name}."
        if rule_reason and rule_reason.lower() != r_name.lower():
            rule = r_name

    if not text and not images_list and not files_list:
        return {"blocked": False, "message": "", "rule": ""}
    # Format platform cleanly from database Target Websites mapping
    clean_host = (host or "").lower().strip(".")
    if clean_host.startswith("www."):
        clean_host = clean_host[4:]

    # An internal/private target remains protected when an administrator explicitly
    # adds it to Target Websites. Unlisted local services still bypass inspection.
    target_hosts = monitored_hosts()
    is_target_host = host_monitored(clean_host, target_hosts) or host_monitored(host, target_hosts)
    if (is_internal_host(clean_host) or is_internal_host(host)) and not is_target_host:
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}

    # STRICT BLOCK: If domain is marked as "Block entire website", block immediately!
    b_hosts = blocked_hosts()
    if host_monitored(clean_host, b_hosts) or host_monitored(host, b_hosts):
        return {
            "blocked": True,
            "action": "Blocked",
            "message": f"Access to {clean_host} is blocked by company security policy.",
            "rule": "Block Entire Website",
            "redacted_prompt": "",
        }

    platform = ""
    with _sites_lock:
        platform = _domain_platform_map.get(clean_host, "")
        if not platform:
            for d, p in _domain_platform_map.items():
                if clean_host == d or clean_host.endswith("." + d):
                    platform = p
                    break

    # STRICT BYPASS: ONLY outside monitored websites in Target Websites are evaluated/logged!
    if not platform:
        if not is_target_host:
            # Not an outside monitored AI website — ignore completely (NO logging)
            return {"blocked": False, "passthrough": True, "message": "", "rule": ""}
        # Fallback platform name only for confirmed monitored hosts
        parts = clean_host.split(".")
        main_part = parts[0] if parts and parts[0] not in ("www", "chat", "app", "api", "m") else (parts[1] if len(parts) > 1 else clean_host)
        platform = main_part.title() if main_part else (clean_host or "Browser AI")

    if not GATEWAY_BACKEND_URL:
        # Offline mode: evaluate locally against cached DLP rules
        if is_blocked:
            return {
                "blocked": True,
                "action": "Blocked",
                "message": block_message or (f"Blocked by security policy: {rule}" if rule else "Blocked by security policy."),
                "rule": rule or "Data Loss Prevention Rule",
                "violating_files": violating_files,
                "redacted_prompt": "",
            }
        for r in dynamic_rules():
            pat = str(r.get("pattern") or "")
            if pat:
                try:
                    rx = _compile_rule_pattern(pat)
                    p_match = bool(raw_prompt_text and rx.search(raw_prompt_text))
                    f_matches = []
                    for fn, fcontent in file_text_map.items():
                        if fcontent and rx.search(fcontent):
                            if fn not in f_matches:
                                f_matches.append(fn)
                    if not p_match and not f_matches and rx.search(text):
                        p_match = True

                    if p_match or f_matches:
                        act = str(r.get("action") or "BLOCK").upper()
                        r_name = str(r.get("name") or "Security Policy")
                        r_reason = str(r.get("warning_message") or r.get("description") or "").strip()
                        reason_display = f" (Reason: {r_reason})" if (r_reason and r_reason.lower() != r_name.lower()) else (f" ({r_reason})" if r_reason else "")

                        warn_msg = ""
                        if f_matches and p_match:
                            warn_msg = f"Prompt and file(s) {', '.join(repr(x) for x in f_matches)} contain restricted data: {r_name}{reason_display}. Request blocked by security policy. Please remove the file(s) before sending."
                        elif f_matches:
                            if len(f_matches) == 1:
                                warn_msg = f"File '{f_matches[0]}' contains restricted data: {r_name}{reason_display}. Upload blocked by corporate DLP policy. Please remove this file before sending."
                            else:
                                warn_msg = f"Files {', '.join(repr(x) for x in f_matches)} contain restricted data: {r_name}{reason_display}. Upload blocked by corporate DLP policy. Please remove these files before sending."
                        elif p_match:
                            warn_msg = f"Prompt contains restricted data: {r_name}{reason_display}. Request blocked by security policy."

                        rule_label = f"{r_name}: {r_reason}" if (r_reason and r_reason.lower() != r_name.lower()) else (r_reason or r_name)

                        if act in ("BLOCK", "BLOCKED"):
                            return {"blocked": True, "action": "Blocked", "message": warn_msg or f"Blocked by {r_name}", "rule": rule_label, "violating_files": f_matches, "redacted_prompt": ""}
                        if act in ("REDACT", "REDACTED"):
                            return {"blocked": False, "action": "Redacted", "message": warn_msg or "Sensitive text redacted by YP-Raksha", "rule": rule_label, "violating_files": f_matches, "redacted_prompt": redact_prompt_text(text, r_name)}
                        if act in ("WARN", "WARNED", "ALERT"):
                            return {"blocked": False, "action": "Warned", "message": warn_msg or "This request triggered a YP-Raksha warning", "rule": rule_label, "violating_files": f_matches, "redacted_prompt": ""}
                except Exception:
                    pass
        return {"blocked": False, "passthrough": True, "action": "Allowed", "message": "", "rule": ""}

    agent_hostname = os.environ.get("GATEWAY_AGENT_HOSTNAME") or os.environ.get("COMPUTERNAME") or ""
    try:
        from agent_identity import get_or_create_agent_id
        agent_id = os.environ.get("GATEWAY_AGENT_ID") or get_or_create_agent_id()
    except Exception:
        agent_id = os.environ.get("GATEWAY_AGENT_ID") or ""

    meta = {
        "domain": host or "",
        "url": url or "",
        "method": "POST",
        "source": "extension",
        "desktop_name": agent_hostname,
        "file_count": len(files_list),
        "image_count": len(images_list),
    }
    user_caption = (prompt or "").strip()
    user_caption = re.sub(r'\[(?:File|Extracted File):.*?\]', '', user_caption).strip()

    if file_names:
        meta["file_name"] = ", ".join(file_names)
        meta["user_caption"] = user_caption
        meta["upload_scan"] = True
    if is_blocked:
        meta["is_blocked"] = True
        meta["blocked_reason"] = rule or "Client Security Rule"
    if prompt.startswith("--drag drop") or "[drag" in prompt.lower():
        meta["is_blocked"] = True
        meta["blocked_reason"] = rule or "Drag & Drop Policy"
        meta["status"] = "Blocked (Drag & Drop Restricted)"
        meta["predicted_category"] = "drag_drop_governance"
        rule = rule or "Drag & Drop Policy"
        is_blocked = True
    elif prompt.startswith("--copy paste") or "[clipboard" in prompt.lower():
        meta["is_blocked"] = True
        is_copy = "copy" in prompt.lower() and "paste" not in prompt.lower()
        if "[clipboard copy" in prompt.lower():
            is_copy = True
        elif "[clipboard paste" in prompt.lower():
            is_copy = False
        action_name = "Clipboard Copy Restricted" if is_copy else "Clipboard Paste Restricted"
        meta["blocked_reason"] = action_name
        meta["status"] = f"Blocked ({action_name})"
        meta["predicted_category"] = "clipboard_governance"
        rule = action_name
        is_blocked = True
    elif prompt.startswith("--screenshot") or "[screenshot" in prompt.lower() or "[screen capture" in prompt.lower():
        meta["is_blocked"] = True
        meta["blocked_reason"] = rule or "Screen Capture Policy"
        meta["status"] = "Blocked (Screen Capture Restricted)"
        meta["predicted_category"] = "screenshot_restriction"
        rule = rule or "Screen Capture Policy"
        is_blocked = True
    elif prompt.startswith("--download") or "[download" in prompt.lower():
        meta["is_blocked"] = True
        meta["blocked_reason"] = rule or "Download Policy"
        meta["status"] = "Blocked (Download Restricted)"
        meta["predicted_category"] = "download_governance"
        rule = rule or "Download Policy"
        is_blocked = True
    elif prompt.startswith("--corporate account") or "[unauthorized domain]" in prompt.lower() or "[corporate workspace" in prompt.lower():
        meta["is_blocked"] = True
        meta["blocked_reason"] = rule or "Unauthorized Personal Account"
        meta["status"] = "Blocked (Unauthorized Personal Account)"
        meta["predicted_category"] = "CORPORATE_WORKSPACE_BLOCKED"
        rule = "Corporate AI Workspace Policy"
        is_blocked = True

    _record_extension_eval(
        clean_host or host,
        user_caption or text,
        file_names,
        platform=platform,
        eval_id=eval_id,
        is_blocked=is_blocked,
        action=meta.get("status") or ("Blocked" if is_blocked else "Allowed"),
        rule=rule or meta.get("blocked_reason") or "",
        message=block_message or meta.get("blocked_reason") or "",
    )

    candidates = [GATEWAY_BACKEND_URL.rstrip("/")] if GATEWAY_BACKEND_URL else []
    seen_bases = set()
    unique_candidates = []
    for b in candidates:
        if b not in seen_bases:
            seen_bases.add(b)
            unique_candidates.append(b)

    # If images were attached without explicit file entries, synthesize file items so they enter full file pipeline
    if not files_list and images_list:
        files_list = []
        for img_idx, img_cand in enumerate(images_list):
            if isinstance(img_cand, str) and img_cand.strip():
                cand_b64 = img_cand.split(",")[-1].strip() if "," in img_cand else img_cand.strip()
                cand_mime = "image/png"
                if "data:image/jpeg" in img_cand.lower() or "data:image/jpg" in img_cand.lower():
                    cand_mime = "image/jpeg"
                elif "data:image/webp" in img_cand.lower():
                    cand_mime = "image/webp"
                ext = cand_mime.split("/")[-1]
                files_list.append({
                    "name": f"image_{img_idx + 1}.{ext}",
                    "type": cand_mime,
                    "base64": cand_b64,
                })

    # Multi-file support: Post individual clean record for each attached file (no phantom joined row!)
    if files_list:
        if user_caption:
            norm_cap = " ".join(user_caption.strip().split()).lower()
            if norm_cap:
                with _RECENT_FILE_CAPTIONS_LOCK:
                    _RECENT_FILE_CAPTIONS[norm_cap] = time.time()
        n_total_files = len(files_list)
        for f_idx, f in enumerate(files_list):
            fn = (f.get("name") if isinstance(f, dict) else str(f)).strip() or "attachment"
            f_label = f"{fn}({f_idx + 1}/{n_total_files})" if n_total_files > 1 else fn
            f_prompt = f"{f_label} -- {user_caption}" if user_caption else f_label

            f_bytes = b""
            f_ct = "application/octet-stream"
            f_ext_text = ""
            if isinstance(f, dict):
                f_ext_text = str(f.get("extracted_text") or "").strip()
                fb64 = str(f.get("base64") or "").strip()
                if fb64:
                    try:
                        import base64
                        f_bytes = base64.b64decode(fb64)
                    except Exception:
                        pass
                elif f.get("text"):
                    f_bytes = str(f["text"]).encode("utf-8")
                fn_ext = os.path.splitext(fn)[1].lower()
                if not f_bytes and fn_ext in (".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".svg") and images_list:
                    for img_cand in images_list:
                        if isinstance(img_cand, str) and img_cand:
                            cand_b64 = img_cand.split(",")[-1] if "," in img_cand else img_cand
                            try:
                                cand_bytes = base64.b64decode(cand_b64)
                                if len(cand_bytes) >= 8:
                                    f_bytes = cand_bytes
                                    break
                            except Exception:
                                pass
                f_ct = str(f.get("type") or "application/octet-stream").strip()
                if not f_ct or f_ct == "application/octet-stream":
                    if fn_ext in (".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".svg"):
                        f_ct = f"image/{fn_ext.lstrip('.').lower() or 'png'}"
                    elif fn_ext == ".docx":
                        f_ct = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
                    elif fn_ext == ".xlsx":
                        f_ct = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                    elif fn_ext == ".pdf":
                        f_ct = "application/pdf"

                # Extract OCR text for images if not already extracted
                if not f_ext_text and f_bytes and fn_ext in (".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".svg"):
                    try:
                        p_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "proxy"))
                        if p_dir not in sys.path:
                            sys.path.insert(0, p_dir)
                        import browser_ai_proxy
                        extract_fn = getattr(browser_ai_proxy, "extract_upload_text_for_rules", None)
                        if extract_fn:
                            f_ext_text = extract_fn(f_bytes, content_type=f_ct, file_name=fn)
                    except Exception:
                        pass

            f_meta = dict(meta)
            f_meta["file_name"] = fn
            f_meta["multi_file_count"] = n_total_files
            f_meta["multi_file_index"] = f_idx + 1
            if f_ext_text:
                f_meta["extracted_text"] = f_ext_text[:100000]
            if fb64:
                f_meta["upload_images"] = [f"data:{f_ct};base64,{fb64}"]
            elif images_list:
                f_meta["upload_images"] = images_list[:5]

            f_dedupe_key = f"ext_file|{eval_id}|{fn}" if eval_id else f"ext_file|{platform}|{fn}|{hash(user_caption)}"

            if not _is_duplicate_file_post(f_dedupe_key, ttl=1.0):
                if not f_bytes and fb64:
                    try:
                        import base64
                        f_bytes = base64.b64decode(fb64)
                    except Exception:
                        pass
                if not f_bytes and images_list:
                    for img_cand in images_list:
                        if isinstance(img_cand, str) and img_cand:
                            cand_b64 = img_cand.split(",")[-1] if "," in img_cand else img_cand
                            try:
                                cand_bytes = base64.b64decode(cand_b64)
                                if len(cand_bytes) >= 8:
                                    f_bytes = cand_bytes
                                    break
                            except Exception:
                                pass

                if f_bytes:
                    threading.Thread(
                        target=_post_intercept_file,
                        kwargs=dict(
                            platform=platform,
                            prompt=f_prompt,
                            file_name=fn,
                            content_type=f_ct,
                            file_payload=f_bytes,
                            metadata=f_meta,
                            client_ip="127.0.0.1",
                            agent_id=agent_id,
                            agent_hostname=agent_hostname,
                        ),
                        daemon=True,
                    ).start()
                else:
                    payload = {
                        "platform": platform,
                        "prompt": f_prompt,
                        "client_ip": "127.0.0.1",
                        "agent_id": agent_id,
                        "agent_hostname": agent_hostname,
                        "upload_images": images_list[:5],
                        "metadata": f_meta,
                    }
                    for base in unique_candidates:
                        threading.Thread(
                            target=_http_json,
                            args=("POST", f"{base}/api/browser-ai/intercept", payload),
                            kwargs=dict(timeout=5),
                            daemon=True,
                        ).start()
                        break

        if is_blocked:
            res = {
                "blocked": True,
                "action": "Blocked",
                "message": block_message or (f"Blocked by security policy: {rule}" if rule else "Blocked by security policy."),
                "rule": rule or "Data Loss Prevention Rule",
                "violating_files": violating_files,
                "redacted_prompt": "",
            }
            with _RECENT_FILE_POSTS_LOCK:
                _RECENT_FILE_POSTS["decision|" + prompt_req_key] = res
                _RECENT_FILE_POSTS["ts|" + prompt_req_key] = time.time()
            return res
        else:
            res = {
                "blocked": False,
                "action": "Allowed",
                "message": "",
                "rule": rule or "",
                "redacted_prompt": "",
            }
            with _RECENT_FILE_POSTS_LOCK:
                _RECENT_FILE_POSTS["decision|" + prompt_req_key] = res
                _RECENT_FILE_POSTS["ts|" + prompt_req_key] = time.time()
            return res

    if file_extracted_texts:
        meta["extracted_text"] = "\n\n".join(file_extracted_texts)[:100000]

    payload = {
        "platform": platform,
        "prompt": text[:40000],
        "client_ip": "127.0.0.1",
        "agent_id": agent_id,
        "agent_hostname": agent_hostname,
        "upload_images": images_list[:5],
        "metadata": meta,
    }
    candidates = [GATEWAY_BACKEND_URL.rstrip("/")] if GATEWAY_BACKEND_URL else []
    seen_bases = set()
    unique_candidates = []
    for b in candidates:
        if b not in seen_bases:
            seen_bases.add(b)
            unique_candidates.append(b)

    if is_blocked:
        if not files_list:
            for base in unique_candidates:
                threading.Thread(
                    target=_http_json,
                    args=("POST", f"{base}/api/browser-ai/intercept", payload),
                    kwargs=dict(timeout=5),
                    daemon=True,
                ).start()
                break
        res = {
            "blocked": True,
            "action": "Blocked",
            "message": block_message or (f"Blocked by security policy: {rule}" if rule else "Blocked by security policy."),
            "rule": rule or "Data Loss Prevention Rule",
            "violating_files": violating_files,
            "redacted_prompt": "",
        }
        with _RECENT_FILE_POSTS_LOCK:
            _RECENT_FILE_POSTS["decision|" + prompt_req_key] = res
            _RECENT_FILE_POSTS["ts|" + prompt_req_key] = time.time()
            if eval_id:
                _RECENT_FILE_POSTS["eval_dec|" + eval_id] = res
                _RECENT_FILE_POSTS["eval_ts|" + eval_id] = time.time()
        return res

    # Filter out noise prompts and internal RPC protobuf blobs
    t_clean = text.strip()
    if t_clean.lower() in ("set", "ping", "pong", "heartbeat", "null", "me", "true", "false") or _is_telemetry_payload(t_clean, url):
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}
    if len(t_clean) > 40 and " " not in t_clean and re.match(r"^[A-Za-z0-9+/=]+$", t_clean):
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}

    # Deduplicate rapid burst retries only (within 0.8s) so identical send bursts don't double-log,
    # but every distinct user send or consecutive prompt is always predicted and recorded!
    p_dedupe_key = f"p_eval|{eval_id}" if eval_id else f"p_burst|{platform}|{','.join(file_names)}|{hash(text)}"
    if _is_duplicate_file_post(p_dedupe_key, ttl=0.8):
        return {"blocked": False, "action": "Allowed", "message": "", "rule": ""}

    code = 0
    data = None
    for base in unique_candidates:
        try:
            code, data = _http_json(
                "POST",
                f"{base}/api/browser-ai/intercept",
                payload,
                timeout=5,
            )
            if code == 200 and isinstance(data, dict):
                threading.Thread(target=_flush_pending_audit_logs, daemon=True).start()
                break
        except Exception:
            continue
    if code != 200 or not isinstance(data, dict):
        # Backend unreachable or network down: queue audit log locally for sync when network restores
        try:
            log_file = os.path.join(data_dir(), "pending_audit_logs.json")
            pending_logs = []
            if os.path.isfile(log_file):
                try:
                    with open(log_file, "r", encoding="utf-8") as pf:
                        pending_logs = json.load(pf)
                except Exception:
                    pass
            if isinstance(pending_logs, list) and len(pending_logs) < 500:
                pending_logs.append(payload)
                with open(log_file, "w", encoding="utf-8") as pf:
                    json.dump(pending_logs, pf)
        except Exception:
            pass

        # Enforce cached DLP rules locally
        for r in dynamic_rules():
            pat = str(r.get("pattern") or "")
            if pat:
                try:
                    rx = _compile_rule_pattern(pat)
                    if rx.search(text):
                        act = str(r.get("action") or "BLOCK").upper()
                        r_name = str(r.get("name") or "Security Policy")
                        warn_msg = str(r.get("warning_message") or "")
                        if act in ("BLOCK", "BLOCKED"):
                            return {"blocked": True, "action": "Blocked", "message": warn_msg or f"Blocked by {r_name}", "rule": r_name, "redacted_prompt": ""}
                        if act in ("REDACT", "REDACTED"):
                            return {"blocked": False, "action": "Redacted", "message": warn_msg or "Sensitive text redacted by YP-Raksha", "rule": r_name, "redacted_prompt": redact_prompt_text(text, r_name)}
                        if act in ("WARN", "WARNED", "ALERT"):
                            return {"blocked": False, "action": "Warned", "message": warn_msg or "This request triggered a YP-Raksha warning", "rule": r_name, "redacted_prompt": ""}
                except Exception:
                    pass
        # If clean prompt, allow passthrough cleanly without network error
        return {"blocked": False, "action": "Allowed", "passthrough": True, "message": "", "rule": ""}
    final_dec = interpret_guard_decision(data, text, rule)
    if final_dec.get("blocked") or str(final_dec.get("action") or "").lower() == "blocked":
        _record_extension_eval(
            clean_host or host,
            user_caption or text,
            file_names,
            platform=platform,
            eval_id=eval_id,
            is_blocked=True,
            action=str(final_dec.get("action") or "Blocked"),
            rule=str(final_dec.get("rule") or rule),
            message=str(final_dec.get("message") or ""),
        )
    if eval_id:
        with _RECENT_FILE_POSTS_LOCK:
            _RECENT_FILE_POSTS["eval_dec|" + eval_id] = final_dec
            _RECENT_FILE_POSTS["eval_ts|" + eval_id] = time.time()
    return final_dec


def _flush_pending_audit_logs() -> None:
    """Flush audit logs that were queued locally while the website network was down."""
    try:
        log_file = os.path.join(data_dir(), "pending_audit_logs.json")
        if not os.path.isfile(log_file) or not GATEWAY_BACKEND_URL:
            return
        with open(log_file, "r", encoding="utf-8") as f:
            logs = json.load(f)
        if not isinstance(logs, list) or not logs:
            return
        remaining = []
        for item in logs[:50]:
            try:
                c, _ = _http_json("POST", f"{GATEWAY_BACKEND_URL}/api/browser-ai/intercept", item, timeout=4)
                if c != 200:
                    remaining.append(item)
            except Exception:
                remaining.append(item)
        if remaining:
            with open(log_file, "w", encoding="utf-8") as f:
                json.dump(remaining, f)
        else:
            try:
                os.remove(log_file)
            except Exception:
                pass
    except Exception:
        pass



def _ensure_pem() -> str:
    path = _pem_path()
    if os.path.isfile(path) and os.path.getsize(path) > 0:
        try:
            if extension_id_from_pem(path) != _COMPROMISED_EXTENSION_ID:
                return path
        except Exception:
            if os.environ.get("GATEWAY_GUARD_EXTENSION_PEM"):
                raise ValueError("Configured browser extension signing key is invalid.")
        # The old repository key was exposed. Never use it to sign updates.
        if os.environ.get("GATEWAY_GUARD_EXTENSION_PEM"):
            raise ValueError("Configured signing key is compromised; provision a rotated key.")
        path = os.path.join(data_dir(), "guard_extension.pem")
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric import rsa

    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    pem = key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption(),
    )
    os.makedirs(os.path.dirname(path), mode=0o700, exist_ok=True)
    temp_path = path + f".tmp.{os.getpid()}"
    with open(temp_path, "wb") as f:
        f.write(pem)
    try:
        os.chmod(temp_path, 0o600)
    except OSError:
        pass
    os.replace(temp_path, path)
    return path


def _extension_id_from_public_key(pub: bytes) -> str:
    digest = hashlib.sha256(pub).digest()
    out = []
    for byte in digest[:16]:
        out.append(chr(ord("a") + ((byte >> 4) & 0xF)))
        out.append(chr(ord("a") + (byte & 0xF)))
    return "".join(out)


def extension_id_from_pem(pem_path: str) -> str:
    from cryptography.hazmat.primitives.serialization import load_pem_private_key

    with open(pem_path, "rb") as f:
        key = load_pem_private_key(f.read(), password=None)
    pub = key.public_key().public_bytes(
        serialization_encoding(),
        public_format(),
    )
    return _extension_id_from_public_key(pub)


def _public_key_b64(pem_path: str) -> str:
    import base64
    from cryptography.hazmat.primitives.serialization import load_pem_private_key

    with open(pem_path, "rb") as f:
        key = load_pem_private_key(f.read(), password=None)
    pub = key.public_key().public_bytes(
        serialization_encoding(),
        public_format(),
    )
    return base64.b64encode(pub).decode("ascii")


def get_extension_id() -> str:
    """Return the ID for the locally provisioned, persistent signing key."""
    return extension_id_from_pem(_ensure_pem())



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
    os.makedirs(dst, exist_ok=True)
    shutil.copytree(
        src,
        dst,
        dirs_exist_ok=True,
        ignore=shutil.ignore_patterns("*.pem", "*.crx", "*.zip", "*.xpi"),
    )
    srv_host = ""
    try:
        from agent_config import GATEWAY_BACKEND_URL
        if GATEWAY_BACKEND_URL:
            from urllib.parse import urlsplit

            parsed = urlsplit(
                GATEWAY_BACKEND_URL
                if "://" in GATEWAY_BACKEND_URL
                else "//" + GATEWAY_BACKEND_URL
            )
            srv_host = parsed.netloc.rsplit("@", 1)[-1]
    except Exception:
        pass
    with open(os.path.join(dst, "endpoint.js"), "w", encoding="utf-8") as f:
        f.write(
            "var GATEWAY_GUARD_BASE = "
            + json.dumps(base_url)
            + ";\nvar GATEWAY_SERVER_HOST = "
            + json.dumps(srv_host)
            + ";\n"
        )
    # Keep the extension identity and version aligned with the private key used
    # to sign the CRX. The private key never enters the extension bundle.
    pem = _ensure_pem()
    cur_ver = get_extension_version()
    man_path = os.path.join(dst, "manifest.json")
    if os.path.isfile(man_path):
        try:
            with open(man_path, "r", encoding="utf-8") as f:
                man_data = json.load(f)
            man_data["version"] = cur_ver
            man_data["key"] = _public_key_b64(pem)
            with open(man_path, "w", encoding="utf-8") as f:
                json.dump(man_data, f, indent=2)
        except Exception:
            pass
    return dst


def pack_extension(base_url: str) -> str:
    """Pack the extension. Returns the crx path, or empty when Chrome cannot pack it."""
    pem = _ensure_pem()
    unpacked = _write_unpacked(base_url)
    chrome = _find_chrome()
    if not chrome:
        print("[Gateway Guard] Extension files are ready. Chrome was not found to pack the install file.")
        return ""
    packed = unpacked + ".crx"
    try:
        if os.path.isfile(packed):
            os.remove(packed)
    except OSError as e:
        print(f"[Gateway Guard WARNING] Stale extension package could not be removed: {e}")
        return ""

    pack_args = [f"--pack-extension={unpacked}", f"--pack-extension-key={pem}", "--no-message-box"]
    try:
        pack_profile = os.path.join(data_dir(), "crx-pack-profile")
        subprocess.run(
            [chrome, f"--user-data-dir={pack_profile}", *pack_args],
            timeout=60,
            check=False,
            capture_output=True,
        )
        if not os.path.isfile(packed):
            # Some managed/headless Windows hosts abort Chrome's normal startup
            # before the pack-only task runs. Retry in a private headless profile.
            fallback_profile = pack_profile + "-headless"
            subprocess.run(
                [
                    chrome,
                    f"--user-data-dir={fallback_profile}",
                    *pack_args,
                    "--no-sandbox",
                    "--disable-gpu",
                    "--headless=new",
                ],
                timeout=60,
                check=False,
                capture_output=True,
            )
    except Exception as e:
        print(f"[Gateway Guard WARNING] Extension pack failed: {e}")
        return ""
    if not os.path.isfile(packed):
        print("[Gateway Guard WARNING] Chrome did not write the extension package.")
        return ""
    try:
        import base64
        import io
        import struct
        import zipfile

        with open(packed, "rb") as f:
            raw = f.read()
        if raw[:4] != b"Cr24" or len(raw) < 12:
            raise ValueError("Invalid CRX header")
        format_version, header_length = struct.unpack_from("<II", raw, 4)
        if format_version != 3:
            raise ValueError("Unsupported CRX version")
        with zipfile.ZipFile(io.BytesIO(raw[12 + header_length:]), "r") as zf:
            packaged_manifest = json.loads(zf.read("manifest.json").decode("utf-8"))
        packaged_key = base64.b64decode(packaged_manifest.get("key") or "", validate=True)
        if extension_id_from_pem(pem) != _extension_id_from_public_key(packaged_key):
            raise ValueError("CRX manifest key does not match the signing key")
        with open(os.path.join(unpacked, "manifest.json"), "r", encoding="utf-8") as f:
            expected_manifest = json.load(f)
        if packaged_manifest.get("version") != expected_manifest.get("version"):
            raise ValueError("CRX version does not match the current extension source")
    except Exception as e:
        print(f"[Gateway Guard WARNING] Chrome wrote an invalid or mismatched CRX: {e}")
        try:
            os.remove(packed)
        except OSError:
            pass
        return ""
    if os.path.abspath(packed) != os.path.abspath(_crx_path()):
        shutil.copy2(packed, _crx_path())
        try:
            os.remove(packed)
        except Exception:
            pass
    return _crx_path()


def update_xml(extension_id: str, base_url: str) -> bytes:
    cur_ver = get_extension_version()
    xml = (
        "<?xml version='1.0' encoding='UTF-8'?>\n"
        "<gupdate xmlns='http://www.google.com/update2/response' protocol='2.0'>\n"
        f"  <app appid='{extension_id}'>\n"
        f"    <updatecheck codebase='{base_url}/extension/guard.crx' version='{cur_ver}' />\n"
        "  </app>\n"
        "</gupdate>\n"
    )
    return xml.encode("utf-8")


FIREFOX_ID = "yp-raksha@gateway.guard"


def _extension_settings(extension_id: str, update_url: str) -> str:
    settings = {
        extension_id: {
            "installation_mode": "force_installed",
            "update_url": update_url,
            "incognito": "spanning",
            "toolbar_pin": "force_unpinned",
        }
    }
    if extension_id != _COMPROMISED_EXTENSION_ID:
        settings[_COMPROMISED_EXTENSION_ID] = {"installation_mode": "blocked"}
    return json.dumps(settings)


def _mac_managed_roots() -> list[str]:
    names = [
        os.path.join("Google", "Chrome"),
        os.path.join("Google", "Chrome Canary"),
        os.path.join("Microsoft Edge"),
        os.path.join("BraveSoftware", "Brave-Browser"),
        os.path.join("Chromium"),
        os.path.join("Vivaldi"),
        os.path.join("com.operasoftware.Opera"),
    ]
    roots = []
    # User-level managed policy (works without root)
    user_support = os.path.join(os.path.expanduser("~"), "Library", "Application Support")
    roots.extend([os.path.join(user_support, name, "policies", "managed") for name in names])
    # Machine-level managed policy (for Jamf / Kandji / Intune MDM root installs)
    machine_support = "/Library/Application Support"
    roots.extend([os.path.join(machine_support, name, "policies", "managed") for name in names])
    return roots


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
    signed_xpi = os.path.join(data_dir(), "yp-raksha.xpi")
    if not is_signed_firefox_xpi(signed_xpi):
        _remove_firefox_policy()
        print("[Gateway Guard WARNING] Firefox install skipped: a Mozilla-signed XPI is not provisioned.")
        return
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


def is_signed_firefox_xpi(path: str) -> bool:
    """Return true only for an XPI containing Mozilla's detached signatures."""
    import zipfile
    try:
        with zipfile.ZipFile(path, "r") as zf:
            names = {name.upper() for name in zf.namelist()}
        return (
            "META-INF/MOZILLA.RSA" in names
            and "META-INF/MOZILLA.SF" in names
            and "META-INF/MANIFEST.MF" in names
        )
    except Exception:
        return False


def build_xpi(unpacked_dir: str) -> str:
    """Copy a pre-signed Mozilla XPI; never label a raw ZIP as installable."""
    dest = os.path.join(data_dir(), "yp-raksha.xpi")
    candidates = [
        os.environ.get("GATEWAY_GUARD_SIGNED_XPI", "").strip(),
        os.path.join(extension_source_dir(), "yp-raksha.xpi"),
        os.path.join(os.path.dirname(extension_source_dir()), "release", "yp-raksha.xpi"),
        os.path.join(os.path.dirname(os.path.abspath(sys.executable)), "yp-raksha.xpi"),
        os.path.join(getattr(sys, "_MEIPASS", "") or "", "yp-raksha.xpi"),
    ]
    for source in candidates:
        if source and os.path.isfile(source) and is_signed_firefox_xpi(source):
            if os.path.abspath(source) != os.path.abspath(dest):
                shutil.copy2(source, dest)
            return dest
    if os.path.isfile(dest) and not is_signed_firefox_xpi(dest):
        try:
            os.remove(dest)
        except OSError:
            pass
    return ""


def install_guard_extension(base_url: str) -> None:
    """Decoupled mode: Extension is independently installed from 3_Browser_Extensions."""
    if not base_url:
        return
    print(f"[Gateway Guard] Decoupled mode active. YP-Raksha endpoint listening on {base_url}. Extensions connect independently.")



def remove_guard_extension_policy() -> None:
    """Drop YP-Raksha from every browser. Called when the Guard uninstall key is accepted."""
    if IS_WIN:
        _write_force_install("", "")
    else:
        _remove_mac_extension_policy()
    _detach_browser_shortcuts()
    _remove_firefox_policy()
    print("[Gateway Guard] YP-Raksha removed from browser policy and shortcuts.")


def _attach_browser_shortcuts(unpacked_dir: str) -> None:
    if not IS_WIN or not os.path.isdir(unpacked_dir):
        return
    clean_path = os.path.abspath(unpacked_dir).replace('"', '')
    ps_script = (
        '$extPath = "$env:ProgramData\\Gateway\\Guard\\guard_extension"\n'
        'if (!(Test-Path $extPath)) { $extPath = "$env:LOCALAPPDATA\\Gateway\\Guard\\guard_extension" }\n'
        'if (!(Test-Path $extPath)) { $extPath = "$env:ProgramFiles\\Gateway\\Guard\\extension" }\n'
        f'if (!(Test-Path $extPath)) {{ $extPath = \'{clean_path}\' }}\n'
        '$argToAdd = "--load-extension=`"" + $extPath + "`""\n'
        '$wsh = New-Object -ComObject WScript.Shell\n'
        '$shApp = New-Object -ComObject Shell.Application\n'
        '$searchDirs = @(\n'
        '    [Environment]::GetFolderPath("Desktop"),\n'
        '    [Environment]::GetFolderPath("CommonDesktopDirectory"),\n'
        '    [Environment]::GetFolderPath("Programs"),\n'
        '    [Environment]::GetFolderPath("CommonPrograms"),\n'
        '    "$env:APPDATA\\Microsoft\\Internet Explorer\\Quick Launch\\User Pinned\\TaskBar",\n'
        '    "$env:APPDATA\\Microsoft\\Internet Explorer\\Quick Launch"\n'
        ')\n'
        '$oneDrive = "$env:USERPROFILE\\OneDrive"\n'
        'if (Test-Path $oneDrive) {\n'
        '    Get-ChildItem -Path $oneDrive -Filter "*Desktop*" -Directory -Depth 2 -ErrorAction SilentlyContinue | ForEach-Object {\n'
        '        $searchDirs += $_.FullName\n'
        '    }\n'
        '}\n'
        'foreach ($dir in $searchDirs) {\n'
        '    if (Test-Path $dir) {\n'
        '        try {\n'
        '            $folder = $shApp.Namespace($dir)\n'
        '            if ($folder) {\n'
        '                foreach ($item in $folder.Items()) {\n'
        '                    if ($item.Name -match "chrome|edge|brave") {\n'
        '                        if ($item.GetLink) {\n'
        '                            $link = $item.GetLink\n'
        '                            $cur = $link.Arguments\n'
        '                            $clean = ($cur -replace \'--load-extension=[^\\s"]+\', \'\' -replace \'--load-extension="[^"]*"\', \'\').Trim()\n'
        '                            $link.Arguments = ($clean + " " + $argToAdd).Trim()\n'
        '                            $link.Save()\n'
        '                        }\n'
        '                    }\n'
        '                }\n'
        '            }\n'
        '        } catch {}\n'
        '        Get-ChildItem -Path $dir -Filter *.lnk -Depth 2 -ErrorAction SilentlyContinue | ForEach-Object {\n'
        '            $name = $_.Name.ToLower()\n'
        '            if ($true) {\n'
        '                try {\n'
        '                    $linkPath = $_.FullName\n'
        '                    try {\n'
        '                        $fso = New-Object -ComObject Scripting.FileSystemObject\n'
        '                        $linkPath = $fso.GetFile($_.FullName).ShortPath\n'
        '                    } catch {}\n'
        '                    $sc = $wsh.CreateShortcut($linkPath)\n'
        '                    $target = $sc.TargetPath\n'
        '                    if ($target -match "chrome\\.exe" -or $target -match "msedge\\.exe" -or $target -match "brave\\.exe") {\n'
        '                        $cur = $sc.Arguments\n'
        '                        $clean = ($cur -replace \'--load-extension=[^\\s"]+\', \'\' -replace \'--load-extension="[^"]*"\', \'\').Trim()\n'
        '                        $sc.Arguments = ($clean + " " + $argToAdd).Trim()\n'
        '                        $sc.Save()\n'
        '                    }\n'
        '                } catch {}\n'
        '            }\n'
        '        }\n'
        '    }\n'
        '}\n'
        '# Ensure Microsoft Edge exists on user Desktop\n'
        '$desk = [Environment]::GetFolderPath("Desktop")\n'
        '$edgeDesk = Join-Path $desk "Microsoft Edge.lnk"\n'
        'if (!(Test-Path $edgeDesk)) {\n'
        '    $startEdge = "$env:APPDATA\\Microsoft\\Windows\\Start Menu\\Programs\\Microsoft Edge.lnk"\n'
        '    if (Test-Path $startEdge) { Copy-Item -Path $startEdge -Destination $edgeDesk -Force -ErrorAction SilentlyContinue }\n'
        '}\n'
    )
    try:
        subprocess.run(
            ["powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", ps_script],
            timeout=15,
            check=False,
            capture_output=True,
            creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
        )
    except Exception as e:
        print(f"[Gateway Guard WARNING] Browser shortcut attach failed: {e}")


def _detach_browser_shortcuts() -> None:
    if not IS_WIN:
        return
    ps_script = (
        '$wsh = New-Object -ComObject WScript.Shell\n'
        '$shApp = New-Object -ComObject Shell.Application\n'
        '$searchDirs = @(\n'
        '    [Environment]::GetFolderPath("Desktop"),\n'
        '    [Environment]::GetFolderPath("CommonDesktopDirectory"),\n'
        '    [Environment]::GetFolderPath("Programs"),\n'
        '    [Environment]::GetFolderPath("CommonPrograms"),\n'
        '    "$env:APPDATA\\Microsoft\\Internet Explorer\\Quick Launch\\User Pinned\\TaskBar",\n'
        '    "$env:APPDATA\\Microsoft\\Internet Explorer\\Quick Launch"\n'
        ')\n'
        '$oneDrive = "$env:USERPROFILE\\OneDrive"\n'
        'if (Test-Path $oneDrive) {\n'
        '    Get-ChildItem -Path $oneDrive -Filter "*Desktop*" -Directory -Depth 2 -ErrorAction SilentlyContinue | ForEach-Object {\n'
        '        $searchDirs += $_.FullName\n'
        '    }\n'
        '}\n'
        'foreach ($dir in $searchDirs) {\n'
        '    if (Test-Path $dir) {\n'
        '        try {\n'
        '            $folder = $shApp.Namespace($dir)\n'
        '            if ($folder) {\n'
        '                foreach ($item in $folder.Items()) {\n'
        '                    if ($item.Name -match "chrome|edge|brave") {\n'
        '                        if ($item.GetLink) {\n'
        '                            $link = $item.GetLink\n'
        '                            $cur = $link.Arguments\n'
        '                            if ($cur -match "load-extension") {\n'
        '                                $cleaned = ($cur -replace \'--load-extension=[^\\s"]+\', \'\' -replace \'--load-extension="[^"]*"\', \'\').Trim()\n'
        '                                $link.Arguments = $cleaned\n'
        '                                $link.Save()\n'
        '                            }\n'
        '                        }\n'
        '                    }\n'
        '                }\n'
        '            }\n'
        '        } catch {}\n'
        '        Get-ChildItem -Path $dir -Filter *.lnk -Depth 2 -ErrorAction SilentlyContinue | ForEach-Object {\n'
        '            $name = $_.Name.ToLower()\n'
        '            if ($true) {\n'
        '                try {\n'
        '                    $linkPath = $_.FullName\n'
        '                    try {\n'
        '                        $fso = New-Object -ComObject Scripting.FileSystemObject\n'
        '                        $linkPath = $fso.GetFile($_.FullName).ShortPath\n'
        '                    } catch {}\n'
        '                    $sc = $wsh.CreateShortcut($linkPath)\n'
        '                    $cur = $sc.Arguments\n'
        '                    if ($cur -match "load-extension") {\n'
        '                        $cleaned = ($cur -replace \'--load-extension=[^\\s"]+\', \'\' -replace \'--load-extension="[^"]*"\', \'\').Trim()\n'
        '                        $sc.Arguments = $cleaned\n'
        '                        $sc.Save()\n'
        '                    }\n'
        '                } catch {}\n'
        '            }\n'
        '        }\n'
        '    }\n'
        '}\n'
    )
    try:
        subprocess.run(
            ["powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", ps_script],
            timeout=15,
            check=False,
            capture_output=True,
            creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
        )
    except Exception as e:
        print(f"[Gateway Guard WARNING] Browser shortcut detach failed: {e}")


def _external_extension_keys(extension_id: str) -> tuple[str, ...]:
    """Registry keys Chrome reads for every profile (every signed-in email)."""
    return (
        r"Software\Google\Chrome\Extensions\\" + extension_id,
        r"Software\Microsoft\Edge\Extensions\\" + extension_id,
        r"Software\BraveSoftware\Brave-Browser\Extensions\\" + extension_id,
        r"Software\BraveSoftware\Brave\Extensions\\" + extension_id,
        r"Software\Chromium\Extensions\\" + extension_id,
    )


def _machine_policy_installed(extension_id: str) -> bool:
    if not IS_WIN or not extension_id:
        return False
    try:
        import winreg
        key = winreg.OpenKey(
            winreg.HKEY_LOCAL_MACHINE,
            r"Software\Policies\Google\Chrome\ExtensionInstallForcelist",
        )
        val, _ = winreg.QueryValueEx(key, "1")
        winreg.CloseKey(key)
        if extension_id not in str(val):
            return False
        # A folder path install is what Chrome shows as off, with Remove.
        # update_url is the all-profile install and must stay.
        try:
            ext = winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, r"Software\Google\Chrome\Extensions\\" + extension_id)
            try:
                winreg.QueryValueEx(ext, "path")
                winreg.CloseKey(ext)
                return False
            except OSError:
                winreg.CloseKey(ext)
                return True
        except OSError:
            return True
    except Exception:
        return False


def _elevate_machine_policy(extension_id: str, update_url: str) -> None:
    """Chrome only force-installs this extension from the machine policy. That key needs administrator once."""
    if not IS_WIN or not extension_id or not update_url or _machine_policy_installed(extension_id):
        return
    safe_id = "".join(ch for ch in extension_id if ch.isascii() and ch.islower())
    safe_url = update_url.replace("'", "")
    if safe_id != extension_id or not safe_url.startswith("http://127.0.0.1:") and not safe_url.startswith("http://localhost:"):
        print("[Gateway Guard WARNING] YP-Raksha machine policy was not written.")
        return
    settings = _extension_settings(safe_id, safe_url)
    value = f"{safe_id};{safe_url}"
    script = (
        "$val = '" + value + "'\n"
        "$settings = '" + settings.replace("'", "''") + "'\n"
        "$paths = @(\n"
        "  'HKLM:\\Software\\Policies\\Google\\Chrome',\n"
        "  'HKLM:\\Software\\Policies\\Microsoft\\Edge',\n"
        "  'HKLM:\\Software\\Policies\\BraveSoftware\\Brave',\n"
        "  'HKLM:\\Software\\Policies\\BraveSoftware\\Brave-Browser',\n"
        "  'HKLM:\\Software\\Policies\\Chromium',\n"
        "  'HKLM:\\Software\\WOW6432Node\\Policies\\Google\\Chrome',\n"
        "  'HKLM:\\Software\\WOW6432Node\\Policies\\Microsoft\\Edge',\n"
        "  'HKLM:\\Software\\WOW6432Node\\Policies\\BraveSoftware\\Brave',\n"
        "  'HKLM:\\Software\\WOW6432Node\\Policies\\Chromium'\n"
        ")\n"
        "foreach ($p in $paths) {\n"
        "  New-Item -Path ($p + '\\ExtensionInstallForcelist') -Force | Out-Null\n"
        "  New-ItemProperty -Path ($p + '\\ExtensionInstallForcelist') -Name '1' -Value $val -PropertyType String -Force | Out-Null\n"
        "  New-ItemProperty -Path $p -Name 'ExtensionSettings' -Value $settings -PropertyType String -Force | Out-Null\n"
        "}\n"
        "$exts = @(\n"
        "  'HKLM:\\Software\\Google\\Chrome\\Extensions\\" + safe_id + "',\n"
        "  'HKLM:\\Software\\Microsoft\\Edge\\Extensions\\" + safe_id + "',\n"
        "  'HKLM:\\Software\\BraveSoftware\\Brave\\Extensions\\" + safe_id + "',\n"
        "  'HKLM:\\Software\\BraveSoftware\\Brave-Browser\\Extensions\\" + safe_id + "',\n"
        "  'HKCU:\\Software\\Google\\Chrome\\Extensions\\" + safe_id + "',\n"
        "  'HKCU:\\Software\\Microsoft\\Edge\\Extensions\\" + safe_id + "',\n"
        "  'HKCU:\\Software\\BraveSoftware\\Brave-Browser\\Extensions\\" + safe_id + "'\n"
        ")\n"
        "$url = '" + safe_url + "'\n"
        "foreach ($e in $exts) {\n"
        "  New-Item -Path $e -Force | Out-Null\n"
        "  New-ItemProperty -Path $e -Name 'update_url' -Value $url -PropertyType String -Force | Out-Null\n"
        "  Remove-ItemProperty -Path $e -Name 'path' -ErrorAction SilentlyContinue\n"
        "  Remove-ItemProperty -Path $e -Name 'version' -ErrorAction SilentlyContinue\n"
        "}\n"
    )
    path = os.path.join(data_dir(), "yp-raksha-machine-policy.ps1")
    try:
        os.makedirs(data_dir(), exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            f.write(script)
        subprocess.run(
            ["powershell", "-NoProfile", "-Command",
             "Start-Process -FilePath powershell.exe -Verb RunAs -Wait -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-File','" + path.replace("'", "''") + "')"],
            timeout=120,
            check=False,
            creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
        )
    except Exception as e:
        print(f"[Gateway Guard WARNING] YP-Raksha machine policy failed: {e}")
    if _machine_policy_installed(extension_id):
        print(f"[Gateway Guard] YP-Raksha machine policy installed ({extension_id}).")
    else:
        print("[Gateway Guard WARNING] YP-Raksha is not force-installed. Approve the administrator prompt, then reopen each browser.")


def _write_force_install(extension_id: str, update_url: str) -> None:
    try:
        import winreg
    except Exception:
        return
    from agent_browser_policy import _CHROMIUM_POLICY_PATHS, _delete_reg_value, _set_reg_sz

    # Block the compromised legacy identity so Chromium cannot keep accepting
    # updates signed with the private key that was previously tracked in git.
    for root in (winreg.HKEY_CURRENT_USER, winreg.HKEY_LOCAL_MACHINE):
        for policy_path in _CHROMIUM_POLICY_PATHS:
            list_path = policy_path + r"\ExtensionInstallForcelist"
            stale_value_names = []
            key = None
            try:
                key = winreg.OpenKey(root, list_path, 0, winreg.KEY_READ)
                index = 0
                while True:
                    value_name, value_data, _ = winreg.EnumValue(key, index)
                    if _COMPROMISED_EXTENSION_ID in str(value_data).lower():
                        stale_value_names.append(value_name)
                    index += 1
            except OSError:
                pass
            finally:
                if key is not None:
                    try:
                        winreg.CloseKey(key)
                    except OSError:
                        pass
            for value_name in stale_value_names:
                _delete_reg_value(root, list_path, value_name)
        for ext_sub in _external_extension_keys(_COMPROMISED_EXTENSION_ID):
            for name in ("path", "version", "update_url"):
                _delete_reg_value(root, ext_sub, name)

    value = f"{extension_id};{update_url}" if extension_id and update_url else ""
    settings = _extension_settings(extension_id, update_url) if value else ""
    crx = _crx_path()
    wrote = False

    for root in (winreg.HKEY_CURRENT_USER, winreg.HKEY_LOCAL_MACHINE):
        for path in _CHROMIUM_POLICY_PATHS:
            list_path = path + r"\ExtensionInstallForcelist"
            if value:
                if _set_reg_sz(root, list_path, "1", value):
                    wrote = True
                if _set_reg_sz(root, path, "ExtensionSettings", settings):
                    wrote = True
                _set_reg_sz(root, path + r"\ExtensionInstallSources", "1", f"http://127.0.0.1:{PAC_HTTP_PORT}/*")
                _set_reg_sz(root, path + r"\ExtensionInstallSources", "2", f"http://localhost:{PAC_HTTP_PORT}/*")
            else:
                _delete_reg_value(root, list_path, "1")
                _delete_reg_value(root, path, "ExtensionSettings")
                _delete_reg_value(root, path + r"\ExtensionInstallSources", "1")
                _delete_reg_value(root, path + r"\ExtensionInstallSources", "2")
                wrote = True

        # update_url installs this one extension into every Chrome profile on this PC.
        # A path value is what Chrome shows as off, with Remove, so that value is removed.
        ext_id_for_keys = extension_id or _COMPROMISED_EXTENSION_ID
        for ext_sub in _external_extension_keys(ext_id_for_keys):
            _delete_reg_value(root, ext_sub, "path")
            _delete_reg_value(root, ext_sub, "version")
            if value:
                if _set_reg_sz(root, ext_sub, "update_url", update_url):
                    wrote = True
            else:
                _delete_reg_value(root, ext_sub, "update_url")

    # Also automatically attach/detach shortcuts so user launches always carry the extension
    unpacked = _unpacked_dir()
    if value and os.path.isdir(unpacked):
        _attach_browser_shortcuts(unpacked)
    elif not value:
        _detach_browser_shortcuts()

    if value and not _machine_policy_installed(extension_id):
        _elevate_machine_policy(extension_id, update_url)
    if value and not wrote and not _machine_policy_installed(extension_id):
        print("[Gateway Guard WARNING] YP-Raksha policy was not written. Browser policy keys are locked for this user.")


def local_base_url() -> str:
    import agent_state
    url = agent_state._PAC_HTTP_URL or ""
    if "://" in url:
        return url.split("/proxy.pac", 1)[0].rstrip("/")
    return ""


def get_root_domain(host: str) -> str:
    if not host:
        return ""
    h = host.lower().replace("www.", "").strip(".")
    parts = h.split(".")
    if len(parts) <= 2:
        return h
    two_part_tlds = {"co.uk", "org.uk", "com.au", "net.au", "co.in", "co.jp", "com.br", "gov.in"}
    last_two = ".".join(parts[-2:])
    if last_two in two_part_tlds and len(parts) > 2:
        return ".".join(parts[-3:])
    return ".".join(parts[-2:])


def are_related_ai_hosts(h1: str, h2: str) -> bool:
    h1 = (h1 or "").lower().replace("www.", "").strip(".")
    h2 = (h2 or "").lower().replace("www.", "").strip(".")
    if not h1 or not h2:
        return False
    if h1 == h2:
        return True
    r1 = get_root_domain(h1)
    r2 = get_root_domain(h2)
    if r1 and r2 and r1 == r2:
        return True
    pairs = [
        ("chatgpt.com", "openai.com"),
        ("chat.openai.com", "chatgpt.com"),
        ("ab.chatgpt.com", "chatgpt.com"),
        ("claude.ai", "anthropic.com"),
        ("gemini.google.com", "aistudio.google.com"),
        ("gemini.google.com", "bard.google.com"),
        ("gemini.google.com", "google.com"),
        ("copilot.microsoft.com", "copilot.com"),
        ("copilot.microsoft.com", "copilot.cloud.microsoft"),
        ("copilot.microsoft.com", "bing.com"),
        ("copilot.microsoft.com", "githubcopilot.com"),
        ("deepseek.com", "chat.deepseek.com"),
        ("mistral.ai", "chat.mistral.ai"),
        ("perplexity.ai", "www.perplexity.ai"),
        ("lovable.dev", "lovable.app"),
        ("lovable.dev", "app.lovable.dev"),
        ("poe.com", "quora.com"),
    ]
    for p0, p1 in pairs:
        if (h1 == p0 or h1.endswith("." + p0)) and (h2 == p1 or h2.endswith("." + p1)):
            return True
        if (h1 == p1 or h1.endswith("." + p1)) and (h2 == p0 or h2.endswith("." + p0)):
            return True
    return False


def authorize_ai_session(payload: dict) -> dict:
    """Forward an AI session authorization check to the Gateway backend."""
    if not isinstance(payload, dict):
        payload = {}
    candidates = [GATEWAY_BACKEND_URL.rstrip("/")] if GATEWAY_BACKEND_URL else []
    seen_urls = set()
    for backend in candidates:
        if backend in seen_urls:
            continue
        seen_urls.add(backend)
        url = f"{backend}/api/browser-ai/session/authorize"
        status, resp = _http_json("POST", url, payload, timeout=4)
        if status == 200 and isinstance(resp, dict) and "decision" in resp:
            return resp
        url_direct = f"{backend}/authorize_ai_session"
        status2, resp2 = _http_json("POST", url_direct, payload, timeout=3)
        if status2 == 200 and isinstance(resp2, dict) and "decision" in resp2:
            return resp2

    # Offline / backend unavailable fallback: evaluate using local governance cache
    gov = get_governance_controls() or {}
    enforce_corp = bool(gov.get("enforce_corporate_domains"))
    policy_ver = int(payload.get("policy_version") or 1)
    if not enforce_corp:
        return {
            "decision": "allow",
            "reason_code": "POLICY_NOT_ENFORCED",
            "message": "Corporate domain enforcement is currently inactive.",
            "policy_version": policy_ver,
        }

    # Check target website filter
    corp_targets = str(gov.get("corporate_target_websites") or "").strip().lower()
    page_url = str(payload.get("page_url") or "").lower()
    provider = str(payload.get("provider") or "").lower()
    if not corp_targets or corp_targets == "none":
        return {
            "decision": "allow",
            "reason_code": "NO_TARGET_WEBSITES",
            "message": "Corporate domain enforcement is not configured for any target websites.",
            "policy_version": policy_ver,
        }
    if corp_targets not in ("all", "*"):
        from urllib.parse import urlparse
        page_host = ""
        try:
            page_host = urlparse(page_url).hostname or ""
        except Exception:
            page_host = page_url
        page_host = page_host.lower().replace("www.", "")
        parts = [p.strip().lstrip("*.").strip().lower().replace("www.", "") for p in corp_targets.replace(";", ",").split(",") if p.strip()]
        matched_target = False
        for p in parts:
            if not p:
                continue
            if p == provider or (page_host and (page_host == p or page_host.endswith("." + p) or p in page_host or are_related_ai_hosts(page_host, p))):
                matched_target = True
                break
        if not matched_target:
            return {
                "decision": "allow",
                "reason_code": "TARGET_NOT_RESTRICTED",
                "message": "Corporate domain enforcement is not required on this target website.",
                "policy_version": policy_ver,
            }

    raw_domains = str(gov.get("corporate_email_domains") or "")
    allowed_domains = set()
    for d in raw_domains.replace(";", ",").replace("\n", ",").split(","):
        cl = re.sub(r"^[*@.]+", "", d.strip().lower()).replace("www.", "").rstrip(".")
        if cl:
            allowed_domains.add(cl)

    if not allowed_domains:
        return {
            "decision": "allow",
            "reason_code": "NO_DOMAINS_CONFIGURED",
            "message": "No corporate email domains have been configured yet.",
            "policy_version": policy_ver,
        }

    email = str(payload.get("user_email") or "").strip().lower()
    email_domain = str(payload.get("email_domain") or "").strip().lower()
    email_domain = re.sub(r"^[*@.]+", "", email_domain).replace("www.", "").rstrip(".")
    if not email_domain and "@" in email:
        email_domain = re.sub(r"^[*@.]+", "", email.split("@")[-1].strip().lower()).replace("www.", "").rstrip(".")

    if not email_domain:
        return {
            "decision": "allow",
            "reason_code": "PENDING_AUTHENTICATION",
            "message": "Awaiting user authentication to verify domain.",
            "policy_version": policy_ver,
        }

    # Check if domain matches configured corporate domains
    domain_allowed = (
        (email_domain in allowed_domains)
        or any(email_domain.endswith("." + ad) for ad in allowed_domains)
        or any((not "." in ad) and (email_domain.startswith(ad + ".") or email_domain == ad + ".com" or f".{ad}." in email_domain) for ad in allowed_domains)
    )
    if not domain_allowed:
        block_msg = str(gov.get("corporate_block_message") or "").strip()
        if not block_msg:
            block_msg = f"Corporate policy restricts access to authorized domains (@{', @'.join(sorted(allowed_domains))}). Personal account (@{email_domain}) is blocked."
        return {
            "decision": "block",
            "reason_code": "UNAUTHORIZED_DOMAIN",
            "message": block_msg,
            "policy_version": policy_ver,
        }

    return {
        "decision": "allow",
        "reason_code": "APPROVED_WORKSPACE",
        "message": f"Corporate {provider.title() or 'AI'} workspace verified.",
        "policy_version": policy_ver,
    }


_last_extension_ping: float = 0.0
_extension_status_lock = threading.Lock()


def record_extension_activity() -> None:
    """Record that the browser extension just communicated with the Guard agent."""
    global _last_extension_ping
    now = time.time()
    with _extension_status_lock:
        was_off = (now - _last_extension_ping) > 25 if _last_extension_ping > 0 else True
        _last_extension_ping = now
    try:
        flag_file = os.path.join(data_dir(), "extension_uninstalled.flag")
        if os.path.isfile(flag_file):
            os.remove(flag_file)
    except Exception:
        pass
    try:
        seen_file = os.path.join(data_dir(), "extension_last_seen.txt")
        with open(seen_file, "w", encoding="utf-8") as f:
            f.write(str(int(now)))
    except Exception:
        pass
    if was_off:
        try:
            import threading
            from agent_identity import get_or_create_agent_id
            from agent_heartbeat import send_heartbeat
            threading.Thread(target=lambda: send_heartbeat(get_or_create_agent_id()), daemon=True).start()
        except Exception:
            pass


def mark_extension_uninstalled() -> None:
    """Record that the browser extension was uninstalled or removed by the user."""
    global _last_extension_ping
    with _extension_status_lock:
        _last_extension_ping = 0.0
    try:
        seen_file = os.path.join(data_dir(), "extension_last_seen.txt")
        with open(seen_file, "w", encoding="utf-8") as f:
            f.write("uninstalled")
    except Exception:
        pass
    try:
        flag_file = os.path.join(data_dir(), "extension_uninstalled.flag")
        with open(flag_file, "w", encoding="utf-8") as f:
            f.write(str(int(time.time())))
    except Exception:
        pass
    try:
        import threading
        from agent_identity import get_or_create_agent_id
        from agent_heartbeat import send_heartbeat
        threading.Thread(target=lambda: send_heartbeat(get_or_create_agent_id()), daemon=True).start()
    except Exception:
        pass


def mark_extension_disconnected() -> None:
    """Record that the browser extension was turned off or disconnected."""
    global _last_extension_ping
    with _extension_status_lock:
        _last_extension_ping = 0.0
    try:
        seen_file = os.path.join(data_dir(), "extension_last_seen.txt")
        with open(seen_file, "w", encoding="utf-8") as f:
            f.write("off")
    except Exception:
        pass
    try:
        import threading
        from agent_identity import get_or_create_agent_id
        from agent_heartbeat import send_heartbeat
        threading.Thread(target=lambda: send_heartbeat(get_or_create_agent_id()), daemon=True).start()
    except Exception:
        pass


_last_reported_ext_status: str | None = None
_watcher_thread_started = False


def _ext_status_watcher_loop() -> None:
    global _last_reported_ext_status
    while True:
        try:
            time.sleep(3)
            current = get_extension_status()
            if _last_reported_ext_status is not None and current != _last_reported_ext_status:
                _last_reported_ext_status = current
                try:
                    from agent_identity import get_or_create_agent_id
                    from agent_heartbeat import send_heartbeat
                    send_heartbeat(get_or_create_agent_id())
                except Exception:
                    pass
            else:
                _last_reported_ext_status = current
        except Exception:
            pass


def ensure_status_watcher() -> None:
    global _watcher_thread_started
    if not _watcher_thread_started:
        _watcher_thread_started = True
        try:
            t = threading.Thread(target=_ext_status_watcher_loop, daemon=True, name="ExtensionStatusWatcher")
            t.start()
        except Exception:
            pass


def get_extension_status() -> str:
    """Determine browser extension connection status for the agent heartbeat.

    Returns:
      'connected': Extension actively polling/checking in (within last 25s).
      'off': Extension was connected before, but is now turned off or disconnected (>25s).
      'installed': Extension / Guard agent installed on device, but not yet connected to browser.
      'uninstalled': Extension was uninstalled or removed by user.
    """
    ensure_status_watcher()
    global _last_extension_ping
    now = time.time()
    with _extension_status_lock:
        last = _last_extension_ping

    # Check uninstalled flag first
    try:
        if os.path.isfile(os.path.join(data_dir(), "extension_uninstalled.flag")):
            return "uninstalled"
    except Exception:
        pass

    # Check in-memory ping
    if last > 0 and (now - last) <= 25:
        return "connected"

    # Check persisted last seen file
    try:
        seen_file = os.path.join(data_dir(), "extension_last_seen.txt")
        if os.path.isfile(seen_file):
            with open(seen_file, "r", encoding="utf-8") as f:
                content = f.read().strip()
            if content == "uninstalled":
                return "uninstalled"
            if content in ("disconnected", "off", "disabled"):
                return "off"
            if content in ("installed", "install", "not_connected"):
                return "installed"
            try:
                ts = float(content)
                if (now - ts) <= 25:
                    with _extension_status_lock:
                        _last_extension_ping = ts
                    return "connected"
                return "off"
            except (ValueError, TypeError):
                return "off"
    except Exception:
        pass

    return "installed"




