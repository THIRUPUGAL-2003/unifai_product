#!/usr/bin/env python3
"""
Sync Guard configs + docs from repository root .env.

Usage (from gateway_product):
    python apps/browser-guard/scripts/sync_config_from_env.py
"""

from __future__ import annotations

import json
import os
import re
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


def read_env_var(repo_root: str, key: str, default: str = "") -> str:
    """Read key; also try GATEWAY_/GATEWAY_ sibling when one prefix is given."""
    keys = [key]
    if key.startswith("GATEWAY_"):
        keys.append("GATEWAY_" + key[len("GATEWAY_") :])
    elif key.startswith("GATEWAY_"):
        keys.insert(0, "GATEWAY_" + key[len("GATEWAY_") :])

    for ktry in keys:
        env_val = os.environ.get(ktry, "").strip()
        if env_val:
            return env_val

    for name in (".env", ".env.example"):
        env_path = os.path.join(repo_root, name)
        if not os.path.isfile(env_path):
            continue
        try:
            with open(env_path, "r", encoding="utf-8", errors="ignore") as f:
                file_vals: dict[str, str] = {}
                for line in f:
                    line = line.strip()
                    if line.startswith("#") or not line or "=" not in line:
                        continue
                    k, v = line.split("=", 1)
                    file_vals[k.strip()] = v.strip().strip("'\"")
                for ktry in keys:
                    val = file_vals.get(ktry, "").strip()
                    if val:
                        return val
        except Exception as e:
            print(f"[sync_config_from_env] Error reading {name}: {e}", file=sys.stderr)
    return default


def rel(path: str, repo_root: str) -> str:
    try:
        return os.path.relpath(path, repo_root).replace("\\", "/")
    except ValueError:
        return path.replace("\\", "/")


def update_json_config(
    path: str,
    server_domain: str,
    proxy_addr: str,
    pac_http_port: str,
    repo_root: str,
    guard_secret: str = "",
) -> bool:
    if not os.path.isfile(path):
        return False
    try:
        with open(path, "r", encoding="utf-8-sig", errors="ignore") as f:
            data = json.load(f)
        if not isinstance(data, dict):
            return False

        old_url = data.get("backend_url", "")
        changed = False
        if data.get("backend_url") != server_domain:
            data["backend_url"] = server_domain
            changed = True
        if proxy_addr and data.get("proxy_addr") != proxy_addr:
            data["proxy_addr"] = proxy_addr
            changed = True
        if pac_http_port:
            want = int(pac_http_port) if pac_http_port.isdigit() else pac_http_port
            if data.get("pac_http_port") != want:
                data["pac_http_port"] = want
                changed = True
        expected_pac = f"{server_domain}/api/browser-ai/pac"
        if data.get("pac_url") != expected_pac:
            data["pac_url"] = expected_pac
            changed = True
        if guard_secret and data.get("guard_secret") != guard_secret:
            data["guard_secret"] = guard_secret
            changed = True
        comment = (
            "Generated from .env — run sync_config_from_env.py after changing "
            "SERVER_DOMAIN / GATEWAY_PROXY_ADDR / PAC_HTTP_PORT / GATEWAY_GUARD_SECRET"
        )
        if data.get("_comment") != comment:
            data["_comment"] = comment
            changed = True

        if not changed:
            return False

        with open(path, "w", encoding="utf-8", newline="\n") as f:
            json.dump(data, f, indent=2)
            f.write("\n")
        print(
            f"[sync_config_from_env] Updated {rel(path, repo_root)}: "
            f"{old_url} -> {server_domain} (proxy: {data.get('proxy_addr')}, pac: {data.get('pac_http_port')})"
        )
        return True
    except Exception as e:
        print(f"[sync_config_from_env] Failed updating {rel(path, repo_root)}: {e}", file=sys.stderr)
        return False


def update_doc_file(
    path: str,
    server_domain: str,
    proxy_addr: str,
    pac_http_port: str,
    repo_root: str,
) -> bool:
    """Rewrite domain + local proxy/PAC ports in docs from .env values."""
    if not os.path.isfile(path):
        return False
    try:
        with open(path, "r", encoding="utf-8", errors="ignore") as f:
            content = f.read()
        new = content

        # Company server / backend URL lines
        new = re.sub(
            r"Company server:\s*https?://[^\s\n]+",
            f"Company server: {server_domain}",
            new,
        )
        new = re.sub(
            r"(synced for |Config backend_url: |backend_url: |\*\*Backend:\*\* `)https?://[^\s`\n]+",
            lambda m: m.group(1) + server_domain,
            new,
        )
        new = re.sub(
            r"`https?://[^`\s]+`",
            lambda m: f"`{server_domain}`"
            if "yespanchi" in m.group(0) or "gateway." in m.group(0) or "dev-yp" in m.group(0)
            else m.group(0),
            new,
        )
        new = re.sub(
            r"(e\.g\.\s+)https?://[^\s\"')\]]+",
            rf"\1{server_domain}",
            new,
        )
        new = re.sub(
            r'#define\s+MyAppURL\s+"[^"]*"',
            f'#define MyAppURL "{server_domain}"',
            new,
        )

        if proxy_addr:
            # Stale laptop proxy examples (8085 and any 127.0.0.1:NNNN used as local proxy bind)
            new = re.sub(r"127\.0\.0\.1:8085\b", proxy_addr, new)
            new = re.sub(
                r"(local proxy on this (?:PC|Mac) only \()127\.0\.0\.1:\d{2,5}(\))",
                rf"\g<1>{proxy_addr}\2",
                new,
            )
            new = re.sub(
                r"(Listens:\s*)127\.0\.0\.1:\d{2,5}",
                rf"\g<1>{proxy_addr}",
                new,
            )
            new = re.sub(
                r"(proxy\.pac\?proxy=)127\.0\.0\.1:\d{2,5}",
                rf"\g<1>{proxy_addr}",
                new,
            )

        if pac_http_port:
            # Health / PAC status HTTP (must NOT rewrite proxy_addr if same host)
            def _pac_url(m: re.Match) -> str:
                suffix = m.group(1) or ""
                return f"http://127.0.0.1:{pac_http_port}{suffix}"

            # Only rewrite known health/status ports (18085 or previous sync values), not proxy_addr
            new = re.sub(
                rf"http://127\.0\.0\.1:(?:18085|{re.escape(pac_http_port)})(/status)?/?",
                _pac_url,
                new,
            )
            # Broader: "Health check: http://127.0.0.1:PORT/"
            new = re.sub(
                r"(Health(?: check)?:?\s*)http://127\.0\.0\.1:\d{2,5}/?",
                rf"\g<1>http://127.0.0.1:{pac_http_port}/",
                new,
                flags=re.I,
            )
            new = re.sub(
                r"(Open\s+)http://127\.0\.0\.1:\d{2,5}/",
                rf"\g<1>http://127.0.0.1:{pac_http_port}/",
                new,
            )

        if new == content:
            return False
        with open(path, "w", encoding="utf-8", newline="\n") as f:
            f.write(new)
        print(f"[sync_config_from_env] Updated docs {rel(path, repo_root)}")
        return True
    except Exception as e:
        print(f"[sync_config_from_env] Failed docs {rel(path, repo_root)}: {e}", file=sys.stderr)
        return False


def main() -> int:
    repo_root = find_repo_root()
    server_domain = read_env_var(repo_root, "SERVER_DOMAIN", "").rstrip("/")
    if not server_domain:
        print(
            "[sync_config_from_env ERROR] SERVER_DOMAIN is not set in .env!",
            file=sys.stderr,
        )
        return 1

    proxy_addr = read_env_var(repo_root, "GATEWAY_PROXY_ADDR", "")
    if not proxy_addr:
        print(
            "[sync_config_from_env ERROR] GATEWAY_PROXY_ADDR is not set in .env!",
            file=sys.stderr,
        )
        return 1

    pac_http_port = read_env_var(repo_root, "PAC_HTTP_PORT", "")
    if not pac_http_port:
        print(
            "[sync_config_from_env ERROR] PAC_HTTP_PORT is not set in .env!",
            file=sys.stderr,
        )
        return 1

    print(f"[sync_config_from_env] SERVER_DOMAIN = {server_domain}")
    print(f"[sync_config_from_env] GATEWAY_PROXY_ADDR = {proxy_addr}")
    print(f"[sync_config_from_env] PAC_HTTP_PORT = {pac_http_port}")

    guard_secret = read_env_var(repo_root, "GATEWAY_GUARD_SECRET", "")
    require_secret = read_env_var(repo_root, "GATEWAY_GUARD_REQUIRE_SECRET", "1").lower()
    require_secret_on = require_secret not in ("0", "false", "no", "off")
    if guard_secret:
        print("[sync_config_from_env] GATEWAY_GUARD_SECRET = (set)")
    else:
        print(
            "[sync_config_from_env] WARNING: GATEWAY_GUARD_SECRET not set — "
            "agent APIs stay open until you set it (required for VAPT / production)",
            file=sys.stderr,
        )
        if require_secret_on:
            print(
                "[sync_config_from_env] ERROR: GATEWAY_GUARD_REQUIRE_SECRET is on but "
                "GATEWAY_GUARD_SECRET is empty — refuse to publish empty secret into packages",
                file=sys.stderr,
            )
            return 1

    bg = os.path.join(repo_root, "apps", "browser-guard")

    config_files = [
        os.path.join(bg, "config", "gateway_guard_config.json"),
        os.path.join(bg, "release", "gateway_guard_config.json"),
        os.path.join(bg, "installer", "staging", "gateway_guard_config.json"),
        os.path.join(bg, "installer", "staging-mac", "gateway_guard_config.json"),
        os.path.join(
            bg, "installer", "staging-mac", "Gateway_Guard.app", "Contents", "Resources", "gateway_guard_config.json"
        ),
        os.path.join(
            bg,
            "installer",
            "pkg-root",
            "Applications",
            "Gateway_Guard.app",
            "Contents",
            "Resources",
            "gateway_guard_config.json",
        ),
        os.path.join(bg, "release", "Gateway_Guard.app", "Contents", "Resources", "gateway_guard_config.json"),
    ]
    local_app_data = os.environ.get("LOCALAPPDATA", "")
    if local_app_data:
        config_files.append(os.path.join(local_app_data, "Programs", "Gateway", "Guard", "gateway_guard_config.json"))
        config_files.append(os.path.join(local_app_data, "Gateway", "Guard", "gateway_guard_config.json"))

    doc_files = [
        os.path.join(bg, "release", "EMPLOYEE_README.txt"),
        os.path.join(bg, "release", "EMPLOYEE_README_MAC.txt"),
        os.path.join(bg, "installer", "EMPLOYEE_README.txt"),
        os.path.join(bg, "installer", "EMPLOYEE_README_MAC.txt"),
        os.path.join(bg, "installer", "staging", "EMPLOYEE_README.txt"),
        os.path.join(bg, "installer", "staging", "EMPLOYEE_README_MAC.txt"),
        os.path.join(bg, "installer", "staging-mac", "EMPLOYEE_README_MAC.txt"),
        os.path.join(bg, "installer", "MAC_ONLY_TODO.txt"),
        os.path.join(bg, "installer", "MACOS_PRODUCTION.md"),
        os.path.join(bg, "installer", "IT_README.txt"),
        os.path.join(bg, "HYBRID_DEPLOY.txt"),
        os.path.join(bg, "installer", "Gateway_Guard.iss"),
        os.path.join(bg, "release", "Update_Gateway_Guard.ps1"),
        os.path.join(bg, "release", "Update_Gateway_Guard_macOS.command"),
        os.path.join(bg, "installer", "Install_Gateway_Guard.command"),
        os.path.join(bg, "release", "Install_Gateway_Guard.command"),
        os.path.join(bg, "installer", "staging-mac", "Install_Gateway_Guard.command"),
        os.path.join(bg, "release", "INSTALL_WINDOWS.txt"),
        os.path.join(bg, "release", "INSTALL_MACOS.txt"),
        os.path.join(bg, "installer", "staging", "INSTALL_WINDOWS.txt"),
        os.path.join(bg, "installer", "staging-mac", "INSTALL_MACOS.txt"),
    ]

    updated = 0
    for cfg in config_files:
        if update_json_config(cfg, server_domain, proxy_addr, pac_http_port, repo_root, guard_secret):
            updated += 1
    for doc in doc_files:
        if update_doc_file(doc, server_domain, proxy_addr, pac_http_port, repo_root):
            updated += 1

    print(f"[sync_config_from_env] Done — {updated} file(s) updated.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
