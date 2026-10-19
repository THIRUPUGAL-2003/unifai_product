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
_domain_platform_map: dict[str, str] = {}


def _pem_path() -> str:
    src_pem = os.path.join(extension_source_dir(), "guard_extension.pem")
    if os.path.isfile(src_pem):
        return src_pem
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


def is_internal_host(host: str) -> bool:
    """Check if host is internal/localhost/private network or the Unifai portal itself."""
    h = (host or "").lower().strip(".")
    if h.startswith("www."):
        h = h[4:]
    if ":" in h:
        h = h.split(":", 1)[0]
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
        b_host = backend.split("://", 1)[-1].split("/", 1)[0].split(":", 1)[0].lower()
        if b_host and (h == b_host or h.endswith("." + b_host)):
            return True
    server_dom = os.environ.get("SERVER_DOMAIN") or ""
    if server_dom:
        s_host = server_dom.split("://", 1)[-1].split("/", 1)[0].split(":", 1)[0].lower()
        if s_host and (h == s_host or h.endswith("." + s_host)):
            return True
    return False


def monitored_hosts() -> list[str]:
    """Domains the gateway monitors. Strictly synchronized live with PostgreSQL Target Websites."""
    import time

    global _sites_cache, _domain_platform_map
    from agent_http import _http_get_text

    now = time.time()
    with _sites_lock:
        cached_at, cached = _sites_cache
        if cached and now - cached_at < 15:
            return list(cached)
    hosts: list[str] = []
    platform_map: dict[str, str] = {}
    fetch_ok = False
    body = ""
    if GATEWAY_BACKEND_URL:
        body = _http_get_text(
            f"{GATEWAY_BACKEND_URL}/api/browser-ai/targets?for=agent",
            "application/json",
            timeout=6,
        ) or ""
        if "targets" not in body:
            body = _http_get_text(
                f"{GATEWAY_BACKEND_URL}/api/browser-ai/targets",
                "application/json",
                timeout=6,
            ) or ""
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
            if row.get("monitored") is False and str(row.get("status") or "").upper() not in ("MONITORED", "BLOCKED"):
                continue
            domain = str(row.get("domain") or "").strip().lower()
            if "://" in domain:
                domain = domain.split("://", 1)[1]
            domain = domain.split("/", 1)[0].split(":", 1)[0].strip(".")
            if domain.startswith("www."):
                domain = domain[4:]
            if domain:
                if domain not in hosts:
                    hosts.append(domain)
                plat = str(row.get("platform_name") or "").strip()
                if plat:
                    platform_map[domain] = plat

    disk_cache_file = os.path.join(data_dir(), "targets_cache.json")
    if fetch_ok:
        # Backend reachable: strictly honor the database target websites list and persist to disk cache
        with _sites_lock:
            _sites_cache = (now, hosts)
            _domain_platform_map = platform_map
        try:
            with open(disk_cache_file, "w", encoding="utf-8") as f:
                json.dump({"hosts": hosts, "platform_map": platform_map}, f)
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
                    _domain_platform_map = saved.get("platform_map", {})
                    _sites_cache = (now, hosts)
                    return list(hosts)
            except Exception:
                pass
        _sites_cache = (now, [])
    return []


_rules_lock = threading.Lock()
_rules_cache: tuple[float, list[dict]] = (0.0, [])


def dynamic_rules() -> list[dict]:
    """Fetch live DLP rules directly from the Gateway backend / PostgreSQL DB."""
    global _rules_cache
    import time
    now = time.time()
    with _rules_lock:
        cached_at, cached = _rules_cache
        if cached is not None and (now - cached_at) < 15:
            return list(cached)
    active_rules = []
    fetch_ok = False
    if GATEWAY_BACKEND_URL:
        from agent_http import _http_json
        try:
            code, data = _http_json("GET", f"{GATEWAY_BACKEND_URL}/api/browser-ai/rules?for=agent", timeout=3)
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


def _record_extension_eval(host: str, prompt: str, files: list | None = None) -> None:
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

        entry = {
            "host": clean_h,
            "preview": (prompt or "").strip()[:150],
            "files": fnames,
            "time": now,
        }
        with _RECENT_EXTENSION_EVALS_LOCK:
            global _RECENT_EXTENSION_EVALS
            # Prune entries older than 30 seconds
            _RECENT_EXTENSION_EVALS = [e for e in _RECENT_EXTENSION_EVALS if now - float(e.get("time") or 0) < 30]
            _RECENT_EXTENSION_EVALS.append(entry)
            tmp_path = cache_path + f".tmp.{os.getpid()}"
            with open(tmp_path, "w", encoding="utf-8") as fp:
                json.dump(_RECENT_EXTENSION_EVALS, fp)
            os.replace(tmp_path, cache_path)
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
        from agent_http import _guard_headers
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
        req = urllib.request.Request(
            f"{GATEWAY_BACKEND_URL}/api/browser-ai/intercept-file",
            data=body,
            headers=_guard_headers({"Content-Type": f"multipart/form-data; boundary={boundary}"}),
            method="POST",
        )
        timeout = 10 if file_payload else 4
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return 200 <= getattr(resp, "status", 200) < 300
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
) -> dict:
    """Evaluate prompt, files, and images against Gateway DLP policies."""
    text = (prompt or "").strip()
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
    if not GATEWAY_BACKEND_URL:
        return {"blocked": is_blocked, "passthrough": not is_blocked, "message": "", "rule": rule}

    # Format platform cleanly from database Target Websites mapping
    clean_host = (host or "").lower().strip(".")
    if clean_host.startswith("www."):
        clean_host = clean_host[4:]

    # STRICT BYPASS: Never monitor, check, or log internal websites / local portal
    if is_internal_host(clean_host) or is_internal_host(host):
        return {"blocked": False, "passthrough": True, "message": "", "rule": ""}

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
        hosts = monitored_hosts()
        if not host_monitored(clean_host, hosts):
            # Not an outside monitored AI website — ignore completely (NO logging)
            return {"blocked": False, "passthrough": True, "message": "", "rule": ""}
        # Fallback platform name only for confirmed monitored hosts
        parts = clean_host.split(".")
        main_part = parts[0] if parts and parts[0] not in ("www", "chat", "app", "api", "m") else (parts[1] if len(parts) > 1 else clean_host)
        platform = main_part.title() if main_part else (clean_host or "Browser AI")

    agent_hostname = os.environ.get("GATEWAY_AGENT_HOSTNAME") or os.environ.get("COMPUTERNAME") or ""
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

    _record_extension_eval(clean_host or host, user_caption or text, file_names)

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

            _post_intercept_file(
                platform=platform,
                prompt=row_prompt,
                file_name=fname,
                content_type=ctype,
                file_payload=raw_b,
                metadata=f_meta,
                client_ip="127.0.0.1",
                agent_id=agent_id,
                agent_hostname=agent_hostname,
            )

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
    if is_blocked:
        try:
            _http_json("POST", f"{GATEWAY_BACKEND_URL}/api/browser-ai/intercept", payload, timeout=3)
        except Exception:
            pass
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
    return interpret_guard_decision(data, text, rule)



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


def get_extension_id() -> str:
    """Return stable extension ID, matching manifest.json key or fixed enterprise ID."""
    m_path = os.path.join(extension_source_dir(), "manifest.json")
    if os.path.isfile(m_path):
        try:
            with open(m_path, "r", encoding="utf-8") as f:
                data = json.load(f)
            key_b64 = data.get("key")
            if key_b64:
                import base64
                der = base64.b64decode(key_b64)
                digest = hashlib.sha256(der).digest()
                out = []
                for b in digest[:16]:
                    out.append(chr(ord("a") + ((b >> 4) & 0xF)))
                    out.append(chr(ord("a") + (b & 0xF)))
                return "".join(out)
        except Exception:
            pass
    return "mipfhcmlcjfhmgldkokdckhndkebkllj"



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
    shutil.copytree(src, dst, ignore=shutil.ignore_patterns("*.pem"))
    with open(os.path.join(dst, "endpoint.js"), "w", encoding="utf-8") as f:
        f.write(f'var GATEWAY_GUARD_BASE = "{base_url}";\n')
    # Synchronize manifest version in packed CRX with current extension version
    cur_ver = get_extension_version()
    man_path = os.path.join(dst, "manifest.json")
    if os.path.isfile(man_path):
        try:
            with open(man_path, "r", encoding="utf-8") as f:
                man_data = json.load(f)
            man_data["version"] = cur_ver
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
    try:
        pack_profile = os.path.join(data_dir(), "crx-pack-profile")
        subprocess.run(
            [chrome, f"--user-data-dir={pack_profile}", f"--pack-extension={unpacked}", f"--pack-extension-key={pem}", "--no-message-box"],
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
    return json.dumps({
        extension_id: {
            "installation_mode": "force_installed",
            "update_url": update_url,
            "incognito": "spanning",
            "toolbar_pin": "force_unpinned",
        }
    })


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
    for name in ("manifest.json", "content.js", "page_hook.js", "endpoint.js", "background.js", "popup.js"):
        path = os.path.join(extension_source_dir(), name)
        if os.path.isfile(path):
            try:
                source_mtime = max(source_mtime, os.path.getmtime(path))
            except Exception:
                pass
    cur_ver = get_extension_version()
    unpacked_man = os.path.join(_unpacked_dir(), "manifest.json")
    ver_mismatch = False
    if os.path.isfile(unpacked_man):
        try:
            with open(unpacked_man, "r", encoding="utf-8") as f:
                ver_mismatch = (json.load(f).get("version") != cur_ver)
        except Exception:
            ver_mismatch = True
    else:
        ver_mismatch = True

    if not os.path.isfile(crx) or os.path.getmtime(crx) < source_mtime or ver_mismatch:
        crx = pack_extension(base_url)
    unpacked = _unpacked_dir()
    if os.path.isdir(unpacked):
        try:
            build_xpi(unpacked)
        except Exception as e:
            print(f"[Gateway Guard WARNING] YP-Raksha Firefox package failed: {e}")
        # Sync to enterprise common directory (e.g. C:\ProgramData - zero user-specific paths)
        common_unpacked = common_extension_dir()
        try:
            os.makedirs(common_unpacked, exist_ok=True)
            import shutil
            for item in os.listdir(unpacked):
                s = os.path.join(unpacked, item)
                d = os.path.join(common_unpacked, item)
                if os.path.isfile(s):
                    shutil.copy2(s, d)
        except Exception:
            pass
    pem = _pem_path()
    if not os.path.isfile(pem):
        return
    try:
        ext_id = get_extension_id()
    except Exception as e:
        ext_id = "mipfhcmlcjfhmgldkokdckhndkebkllj"
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
    _detach_browser_shortcuts()
    if IS_WIN:
        _write_force_install("", "")
    else:
        _remove_mac_extension_policy()
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
        ext_id_for_keys = extension_id or "mipfhcmlcjfhmgldkokdckhndkebkllj"
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

