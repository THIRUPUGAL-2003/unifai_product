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
    raw = raw.strip().strip(".")
    if raw.startswith("www."):
        raw = raw[4:]
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
                    _sites_cache = (now, hosts)
                    _blocked_sites_cache = (now, b_hosts)
                    return list(hosts)
            except Exception:
                pass
        _sites_cache = (now, [])
        _blocked_sites_cache = (now, [])
    return []


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
}


def get_governance_controls() -> dict:
    """Return live data governance settings (clipboard, drag-drop, download)."""
    global _governance_cache
    # Ensure cache is refreshed by calling dynamic_rules() if empty or old
    try:
        dynamic_rules()
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
                    }
                break
        except Exception:
            pass

    disk_rules_file = os.path.join(data_dir(), "rules_cache.json")
    with _rules_lock:
        if fetch_ok:
            _rules_cache = (now, active_rules)
            try:
                with open(disk_rules_file, "w", encoding="utf-8") as f:
                    json.dump(active_rules, f)
            except Exception:
                pass
            return active_rules
        if _rules_cache[1]:
            return list(_rules_cache[1])
        if os.path.isfile(disk_rules_file):
            try:
                with open(disk_rules_file, "r", encoding="utf-8") as f:
                    active_rules = json.load(f)
                    _rules_cache = (now, active_rules)
                    return active_rules
            except Exception:
                pass
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
    if (not allowed) or act in ("blocked", "block", "bot answered", "replied"):
        return {
            "blocked": True,
            "action": "Blocked" if act not in ("bot answered", "replied") else action,
            "message": reply or warning or "This request was blocked by YP-Raksha.",
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


def _record_extension_eval(host: str, prompt: str, files: list | None = None, platform: str = "", eval_id: str = "") -> None:
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
            # Prune entries older than 30 seconds
            _RECENT_EXTENSION_EVALS = [e for e in _RECENT_EXTENSION_EVALS if now - float(e.get("time") or 0) < 30]
            for h in hosts_to_record:
                entry = {
                    "host": h,
                    "platform": (platform or "").strip(),
                    "preview": p_clean[:150],
                    "files": fnames,
                    "time": now,
                    "eval_id": str(eval_id or "").strip(),
                    "id": str(eval_id or "").strip(),
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
    dedupe_key = f"{platform}|{file_name}|{len(file_payload)}|{prompt[:100]}"
    if _is_duplicate_file_post(dedupe_key, ttl=15.0):
        print(f"[agent_extension] Skipping duplicate file post for {file_name}")
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
    files_list = files if isinstance(files, list) else []
    images_list = upload_images if isinstance(upload_images, list) else []

    file_names = []
    for f in files_list:
        if isinstance(f, dict):
            fname = str(f.get("name") or "").strip()
            if fname and fname not in file_names:
                file_names.append(fname)
            fext = os.path.splitext(fname)[1].lower()
            ftext = str(f.get("text") or "").strip()
            if ftext:
                text += f"\n[File: {fname}]\n" + ftext[:15000]
            elif fname and f"[File: {fname}]" not in text:
                text += f"\n[File: {fname}]\n"
            fb64 = str(f.get("base64") or "").strip()
            if fb64 and not ftext:
                try:
                    import base64
                    raw_b = base64.b64decode(fb64)
                    extracted = ""
                    try:
                        p_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "proxy"))
                        if p_dir not in sys.path:
                            sys.path.insert(0, p_dir)
                        import browser_ai_proxy
                        extracted = getattr(browser_ai_proxy, "extract_upload_text_for_rules", lambda *a, **k: "")(raw_b, file_name=fname)
                    except Exception:
                        pass
                    if not extracted:
                        try:
                            extracted = raw_b.decode("utf-8", errors="ignore")
                        except Exception:
                            pass
                    if extracted and len(extracted.strip()) > 3:
                        text += f"\n[Extracted File: {fname}]\n" + extracted[:25000]
                except Exception:
                    pass
            if fext in (".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".svg") and fb64:
                img_data = fb64 if fb64.startswith("data:") else f"data:image/{fext.lstrip('.')};base64,{fb64}"
                if img_data not in images_list:
                    images_list.append(img_data)
        elif isinstance(f, str) and f.strip():
            fn = f.strip()
            if fn not in file_names:
                file_names.append(fn)
            if f"[File: {fn}]" not in text:
                text += f"\n[File: {fn}]\n"

    if file_names and not text.strip():
        text = f"[FILE UPLOAD] {', '.join(file_names)}"

    # Instant in-memory DLP regex match on prompt + extracted file text
    if text and not is_blocked:
        try:
            p_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "proxy"))
            if p_dir not in sys.path:
                sys.path.insert(0, p_dir)
            import browser_ai_proxy
            m_matched, m_rule, m_act = getattr(browser_ai_proxy, "match_guard_rules_on_text", lambda *a: (False, "", ""))(text)
            if m_matched and m_act.upper() in ("BLOCK", "BLOCKED"):
                is_blocked = True
                rule = m_rule
        except Exception:
            pass

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
        return {
            "blocked": True,
            "action": "Blocked",
            "passthrough": False,
            "message": "Gateway security backend is not configured. This request is blocked until verification is available.",
            "rule": "Gateway Backend Unavailable",
            "redacted_prompt": "",
        }

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

    _record_extension_eval(clean_host or host, user_caption or text, file_names, platform=platform, eval_id=eval_id)

    # Multi-file support: If files are attached, post each file individually to /api/browser-ai/intercept-file
    if files_list:
        total_files = len(files_list)
        for idx, f in enumerate(files_list):
            fname = ""
            raw_b = b""
            ctype = "application/octet-stream"
            ftext = ""
            if isinstance(f, dict):
                fname = str(f.get("name") or "").strip()
                ctype = str(f.get("type") or "application/octet-stream").strip()
                ftext = str(f.get("text") or "").strip()
                fb64 = str(f.get("base64") or "").strip()
                if fb64:
                    try:
                        import base64
                        raw_b = base64.b64decode(fb64)
                    except Exception:
                        pass
                elif ftext:
                    raw_b = ftext.encode("utf-8")
            elif isinstance(f, str) and f.strip():
                fname = f.strip()

            if not fname:
                fname = f"file_{idx+1}"

            file_label = f"{fname}({idx+1}/{total_files})" if total_files > 1 else fname
            row_prompt = f"{file_label} -- {user_caption}" if user_caption else f"[FILE UPLOAD] {file_label}"

            f_meta = dict(meta)
            f_meta["file_name"] = fname
            f_meta["user_caption"] = user_caption
            f_meta["upload_scan"] = True
            if is_blocked:
                f_meta["is_blocked"] = True
                f_meta["blocked_reason"] = rule or "Client Security Rule"

            # Post file attachment asynchronously in background thread so check_prompt returns in 5ms without blocking
            threading.Thread(
                target=_post_intercept_file,
                kwargs=dict(
                    platform=platform,
                    prompt=row_prompt,
                    file_name=fname,
                    content_type=ctype,
                    file_payload=raw_b,
                    metadata=f_meta,
                    client_ip="127.0.0.1",
                    agent_id=agent_id,
                    agent_hostname=agent_hostname,
                ),
                daemon=True,
            ).start()

        if is_blocked:
            return {
                "blocked": True,
                "action": "Blocked",
                "message": f"Blocked by security policy: {rule}" if rule else "Blocked by security policy.",
                "rule": rule or "Data Loss Prevention Rule",
                "redacted_prompt": "",
            }
        return {"blocked": False, "action": "Allowed", "message": "", "rule": ""}

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
        for base in unique_candidates:
            threading.Thread(
                target=_http_json,
                args=("POST", f"{base}/api/browser-ai/intercept", payload),
                kwargs=dict(timeout=5),
                daemon=True,
            ).start()
            break
        return {
            "blocked": True,
            "action": "Blocked",
            "message": f"Blocked by security policy: {rule}" if rule else "Blocked by security policy.",
            "rule": rule or "Data Loss Prevention Rule",
            "redacted_prompt": "",
        }

    # Filter out noise prompts and internal RPC protobuf blobs
    t_clean = text.strip()
    if t_clean.lower() in ("set", "ping", "pong", "heartbeat", "null", "me", "true", "false"):
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}
    if len(t_clean) > 40 and " " not in t_clean and re.match(r"^[A-Za-z0-9+/=]+$", t_clean):
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}

    # Deduplicate non-blocked prompts within 8 seconds
    prompt_dedupe_key = f"{platform}|{text[:100]}"
    if _is_duplicate_file_post("prompt|" + prompt_dedupe_key, ttl=8.0):
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
                break
        except Exception:
            continue
    if code != 200 or not isinstance(data, dict):
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}
    return interpret_guard_decision(data, text, rule)



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
        status2, resp2 = _http_json("POST", url_direct, payload, timeout=4)
        if status2 == 200 and isinstance(resp2, dict) and "decision" in resp2:
            return resp2
    return {
        "decision": "block",
        "reason_code": "BACKEND_UNREACHABLE",
        "message": "Backend session verification unavailable; access is blocked until verification succeeds.",
        "policy_version": int(payload.get("policy_version") or 1),
    }


