"""PAC file content: fetch, build, normalize, and persist local proxy.pac."""

from __future__ import annotations

import json
import os
import re

import agent_config
from agent_config import PAC_URL, PROXY_ADDR, RAKSHA_BACKEND_URL
from agent_http import _http_get_text
from guard_platform import data_dir


def local_pac_path() -> str:
    return os.path.join(data_dir(), "proxy.pac")


def check_backend() -> bool:
    """Return True only when Browser AI API is reachable (not just /health)."""
    targets = _http_get_text(
        f"{RAKSHA_BACKEND_URL}/api/browser-ai/targets?for=agent",
        "application/json",
        timeout=45,
    )
    if not (targets and ("targets" in targets or targets.strip().startswith("{"))):
        targets = _http_get_text(
            f"{RAKSHA_BACKEND_URL}/api/browser-ai/targets",
            "application/json",
            timeout=45,
        )
    if targets and ("targets" in targets or targets.strip().startswith("{")):
        print(f"[Raksha Guard] Backend Browser AI API OK: {RAKSHA_BACKEND_URL}")
        return True
    health = _http_get_text(f"{RAKSHA_BACKEND_URL}/health", "application/json", timeout=8)
    if health and '"status"' in health:
        print("[Raksha Guard WARNING] /health OK but /api/browser-ai/targets failed — Browser AI may be missing on this deploy.")
    print(f"[Raksha Guard ERROR] Cannot reach Browser AI API at {RAKSHA_BACKEND_URL}")
    print("[Raksha Guard ERROR] Deploy latest Raksha with /api/browser-ai/* routes, then restart Guard.")
    return False


def _normalize_domain(raw: str) -> str:
    domain = (raw or "").strip().lower()
    if not domain:
        return ""
    if "://" in domain:
        domain = domain.split("://", 1)[1]
    domain = domain.split("/", 1)[0].split("?", 1)[0].split("#", 1)[0]
    if domain.startswith("[") and "]" in domain:
        domain = domain[1:domain.index("]")]
    elif ":" in domain:
        domain = domain.rsplit(":", 1)[0]
    domain = domain.lstrip("*.").strip(".")
    if domain.startswith("www."):
        domain = domain[4:]
    return domain


# Keep in sync with searchEnginePACRule in framework/logstore/browser_ai.go (Search Logs).
SEARCH_ENGINE_PAC_RULE = [
    '    if (shExpMatch(host, "google.*") || shExpMatch(host, "www.google.*") ||',
    '        host === "bing.com" || host === "www.bing.com" ||',
    '        host === "duckduckgo.com" || host === "html.duckduckgo.com" ||',
    '        dnsDomainIs(host, "search.yahoo.com") || host === "search.brave.com") {',
    '        return "PROXY {proxy}";',
    "    }",
    "",
]


def build_pac_from_targets(proxy_addr: str) -> str | None:
    body = _http_get_text(
        f"{RAKSHA_BACKEND_URL}/api/browser-ai/targets?for=agent",
        "application/json",
        timeout=45,
    )
    if not body:
        body = _http_get_text(
            f"{RAKSHA_BACKEND_URL}/api/browser-ai/targets",
            "application/json",
            timeout=45,
        )
    if not body:
        return None
    try:
        data = json.loads(body)
    except Exception as e:
        print(f"[Raksha Guard WARNING] targets JSON parse failed: {e}")
        return None

    targets = data.get("targets") if isinstance(data, dict) else None
    if not isinstance(targets, list):
        return None

    hosts: list[str] = []
    seen: set[str] = set()
    for t in targets:
        if not isinstance(t, dict):
            continue
        monitored = bool(t.get("monitored"))
        block_site = bool(t.get("block_site"))
        if not monitored and not block_site:
            continue
        d = _normalize_domain(str(t.get("domain") or ""))
        if not d or d in seen:
            continue
        seen.add(d)
        hosts.append(d)

    hosts.sort()
    # Collapse children covered by a parent already in the list (no product hardcoding).
    host_set = set(hosts)
    minimized: list[str] = []
    for d in hosts:
        parts = d.split(".")
        covered = False
        for i in range(1, len(parts)):
            parent = ".".join(parts[i:])
            if parent in host_set:
                covered = True
                break
        if not covered:
            minimized.append(d)
    hosts = minimized

    lines = [
        "// Raksha Browser AI Guard — admin Target Websites from dashboard only.",
        "// Parent domains preferred when children are covered by subdomain match.",
        "function FindProxyForURL(url, host) {",
        "    host = host.toLowerCase();",
        "",
        *(line.replace("{proxy}", proxy_addr) for line in SEARCH_ENGINE_PAC_RULE),
        "    var aiHosts = [",
    ]
    for d in hosts:
        lines.append(f'        "{d}",')
    lines += [
        "    ];",
        "    for (var i = 0; i < aiHosts.length; i++) {",
        "        var d = aiHosts[i];",
        '        if (host === d || dnsDomainIs(host, "." + d) || shExpMatch(host, "*." + d)) {',
        "            // Strict: monitored hosts MUST use Guard. Fail-open is health_loop all-DIRECT only.",
        f'            return "PROXY {proxy_addr}";',
        "        }",
        "    }",
        '    return "DIRECT";',
        "}",
        "",
    ]
    return "\n".join(lines)


def ensure_pac_strict_proxy(pac: str) -> str:
    """Monitored hosts use PROXY only while Guard is healthy.

    ``PROXY …; DIRECT`` caused silent bypass (sites work, Prompt Logs stay 0).
    When the local listener is down, health_loop rewrites PAC to all-DIRECT instead.
    """
    if not pac or "FindProxyForURL" not in pac:
        return pac

    # Normalize any "PROXY host:port; DIRECT" → "PROXY host:port"
    pac = re.sub(
        r'return\s*"\s*PROXY\s+([^";]+?)\s*;\s*DIRECT\s*"',
        lambda m: f'return "PROXY {m.group(1).strip()}"',
        pac,
        flags=re.IGNORECASE,
    )
    return pac


def fetch_proxy_pac() -> str | None:
    advertise = (agent_config.PAC_ADVERTISE_ADDR or PROXY_ADDR or "").strip()
    urls = [
        f"{PAC_URL}?proxy={advertise}",
        f"{RAKSHA_BACKEND_URL}/api/browser-ai/pac?proxy={advertise}",
        f"{RAKSHA_BACKEND_URL}/api/browser-ai/proxy.pac?proxy={advertise}",
    ]
    seen = set()
    for url in urls:
        if url in seen:
            continue
        seen.add(url)
        body = _http_get_text(url, "application/x-ns-proxy-autoconfig,*/*")
        if body and "FindProxyForURL" in body:
            return ensure_pac_strict_proxy(body)
    print("[Raksha Guard] Server PAC unavailable — building PAC from /api/browser-ai/targets")
    return build_pac_from_targets(advertise or PROXY_ADDR)


def write_local_pac(content: str) -> None:
    path = local_pac_path()
    try:
        # Never persist PROXY;DIRECT except intentional all-DIRECT fail-open from health_loop.
        if content and "PROXY " in content.upper():
            content = ensure_pac_strict_proxy(content)
        with open(path, "w", encoding="utf-8", newline="\n") as f:
            f.write(content)
        print(f"[Raksha Guard] Wrote local proxy.pac ({path})")
    except Exception as e:
        print(f"[Raksha Guard WARNING] Could not write local proxy.pac: {e}")
