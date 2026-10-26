"""Local HTTP server that serves proxy.pac and status pages to browsers."""

from __future__ import annotations

import http.server
import json
import os
import threading

import agent_state
from agent_config import AGENT_VERSION, PAC_HTTP_HOST, PAC_HTTP_PORT, GATEWAY_BACKEND_URL
from agent_pac_content import local_pac_path


def pac_http_url() -> str:
    return agent_state._PAC_HTTP_URL


def html_escape(s: str) -> str:
    return (
        (s or "")
        .replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
    )


class _PACRequestHandler(http.server.BaseHTTPRequestHandler):
    def _cors(self) -> None:
        # Chrome treats an https AI site calling 127.0.0.1 as a private-network
        # request. Without this opt-in the extension's /extension/check never
        # reaches the EXE, and the prompt is allowed through.
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Access-Control-Request-Private-Network, *")
        self.send_header("Access-Control-Allow-Private-Network", "true")
        self.send_header("Access-Control-Max-Age", "86400")

    def _json(self, code: int, payload: dict) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self._cors()
        self.end_headers()
        self.wfile.write(body)

    def do_OPTIONS(self) -> None:
        self.send_response(204)
        self._cors()
        self.end_headers()

    def do_POST(self) -> None:
        path = (self.path or "/").split("?", 1)[0]
        if path in ("/extension/authorize_session", "/authorize_ai_session"):
            try:
                length = int(self.headers.get("Content-Length") or "0")
            except Exception:
                length = 0
            raw = self.rfile.read(min(length, 100_000)) if length > 0 else b""
            try:
                data = json.loads(raw.decode("utf-8", errors="ignore") or "{}")
            except Exception:
                data = {}
            from agent_extension import authorize_ai_session
            result = authorize_ai_session(data)
            self._json(200, result)
            return

        if path != "/extension/check":
            self.send_error(404)
            return
        try:
            length = int(self.headers.get("Content-Length") or "0")
        except Exception:
            length = 0
        raw = self.rfile.read(min(length, 100_000)) if length > 0 else b""
        try:
            data = json.loads(raw.decode("utf-8", errors="ignore") or "{}")
        except Exception:
            data = {}
        if not isinstance(data, dict):
            data = {}
        from agent_extension import check_prompt
        result = check_prompt(
            str(data.get("host") or ""),
            str(data.get("url") or ""),
            str(data.get("prompt") or data.get("text") or ""),
            files=data.get("files") if isinstance(data.get("files"), list) else None,
            upload_images=data.get("upload_images") if isinstance(data.get("upload_images"), list) else None,
            rule=str(data.get("rule") or data.get("blocked_reason") or ""),
            eval_id=str(data.get("eval_id") or data.get("id") or ""),
        )
        self._json(200, result)

    def do_GET(self) -> None:
        path = (self.path or "/").split("?", 1)[0]
        if path in ("/status", "/health", "/ping", "/extension/ping"):
            report = agent_state._LAST_HEALTH if isinstance(agent_state._LAST_HEALTH, dict) and agent_state._LAST_HEALTH else {"status": "ok", "agent_version": AGENT_VERSION, "connected": True}
            self._json(200, report)
            return
        if path in ("/", "/status.html"):
            report = agent_state._LAST_HEALTH if isinstance(agent_state._LAST_HEALTH, dict) else {}
            st = html_escape(str(report.get("status") or "starting"))
            ver = html_escape(AGENT_VERSION)
            backend = html_escape(GATEWAY_BACKEND_URL)
            checks = report.get("checks") if isinstance(report.get("checks"), dict) else {}
            rows = "".join(
                f"<tr><td>{html_escape(k)}</td><td>{'OK' if v else 'FAIL'}</td></tr>" for k, v in checks.items()
            )
            details = report.get("details") if isinstance(report.get("details"), list) else []
            det = "<br/>".join(html_escape(str(d)) for d in details) or "—"
            html = f"""<!DOCTYPE html><html><head><meta charset="utf-8"/><title>Gateway Guard</title>
<style>body{{font-family:Segoe UI,sans-serif;background:#0b1220;color:#e2e8f0;padding:24px}}
.card{{background:#111827;border:1px solid #334155;border-radius:12px;padding:20px;max-width:720px}}
h1{{margin:0 0 8px;font-size:20px}} .ok{{color:#34d399}} .bad{{color:#f87171}} .deg{{color:#fbbf24}}
table{{width:100%;border-collapse:collapse;margin-top:12px}} td,th{{border-bottom:1px solid #334155;padding:8px;text-align:left;font-size:13px}}
</style></head><body><div class="card">
<h1>Gateway Guard {ver}</h1>
<p>Status: <strong class="{'ok' if st=='ok' else 'deg' if st=='degraded' else 'bad'}">{st}</strong></p>
<p>PAC mode: <code>{html_escape(str(report.get("pac_mode") or "unknown"))}</code> (strict_proxy = intercepts; fail_open_direct / bypass_chain = Prompt Logs stay 0)</p>
<p>Backend: <code>{backend}</code></p>
<p>Local status API: <code>/status</code></p>
<table><thead><tr><th>Check</th><th>Result</th></tr></thead><tbody>{rows}</tbody></table>
<p style="margin-top:16px;font-size:12px;color:#94a3b8">Details: {det}</p>
<p style="font-size:12px;color:#94a3b8">Chrome, Edge, Brave, Opera, Vivaldi, and Firefox use the same Guard PAC. Fully quit &amp; reopen after install. Safari is macOS-only (not supported by Windows Guard).</p>
</div></body></html>"""
            body = html.encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if path == "/extension/wait":
            # Long-poll: the extension background and the MitM worker park here and
            # re-pull policy as soon as the version changes (admin edit on the website).
            import agent_policy_sync
            from urllib.parse import parse_qs

            query = parse_qs((self.path or "").split("?", 1)[1] if "?" in (self.path or "") else "")
            try:
                since = int((query.get("v") or ["-1"])[0])
            except ValueError:
                since = -1
            try:
                timeout = float((query.get("t") or [str(agent_policy_sync.WAIT_MAX_SECONDS)])[0])
            except ValueError:
                timeout = agent_policy_sync.WAIT_MAX_SECONDS
            version, uninstalling = agent_policy_sync.wait_for_change(since, timeout)
            self._json(200, {"policy_version": version, "uninstalling": uninstalling})
            return
        if path == "/extension/sites":
            import agent_policy_sync
            from agent_extension import monitored_hosts, blocked_hosts
            from agent_config import GATEWAY_BACKEND_URL
            server_host = ""
            if GATEWAY_BACKEND_URL:
                sh = GATEWAY_BACKEND_URL.split("://", 1)[-1].split("/", 1)[0].split(":", 1)[0].strip().lower()
                if sh.startswith("www."):
                    sh = sh[4:]
                server_host = sh
            self._json(200, {
                "hosts": monitored_hosts(),
                "blocked_hosts": blocked_hosts(),
                "server_host": server_host,
                "policy_version": agent_policy_sync.current_version(),
                "uninstalling": agent_policy_sync.is_uninstalling(),
            })
            return
        if path == "/extension/rules":
            import agent_policy_sync
            from agent_extension import dynamic_rules
            from agent_config import GATEWAY_BACKEND_URL
            from guard_platform import data_dir
            server_host = ""
            if GATEWAY_BACKEND_URL:
                sh = GATEWAY_BACKEND_URL.split("://", 1)[-1].split("/", 1)[0].split(":", 1)[0].strip().lower()
                if sh.startswith("www."):
                    sh = sh[4:]
                server_host = sh
            tamper_flag = os.path.join(data_dir(), "tamper.flag")
            is_tampered = os.path.isfile(tamper_flag)
            from agent_extension import get_governance_controls
            gov = get_governance_controls()
            payload = {
                "rules": dynamic_rules(),
                "server_host": server_host,
                "status": "tampered" if is_tampered else "active",
                "tampered": is_tampered,
                "policy_version": agent_policy_sync.current_version(),
                "uninstalling": agent_policy_sync.is_uninstalling(),
            }
            cache_file = os.path.join(data_dir(), "cached_uninstall_keys.json")
            if os.path.isfile(cache_file):
                try:
                    with open(cache_file, "r", encoding="utf-8") as f:
                        cached_k = json.load(f)
                    payload["uninstall_key_configured"] = bool(cached_k.get("agent_key_hash") or cached_k.get("company_key_hash"))
                    payload["uninstall_key_rotated_at"] = cached_k.get("rotated_at") or ""
                except Exception:
                    pass
            payload.update(gov)
            self._json(200, payload)
            return
        if path == "/extension/update.xml":
            from agent_extension import get_extension_id, local_base_url, update_xml
            body = update_xml(get_extension_id(), local_base_url())
            self.send_response(200)
            self.send_header("Content-Type", "text/xml; charset=utf-8")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(body)))
            self._cors()
            self.end_headers()
            self.wfile.write(body)
            return
        if path == "/extension/yp-raksha.xpi":
            from agent_extension import data_dir as _guard_data, is_signed_firefox_xpi
            xpi = os.path.join(_guard_data(), "yp-raksha.xpi")
            if not is_signed_firefox_xpi(xpi):
                self.send_error(404)
                return
            with open(xpi, "rb") as f:
                body = f.read()
            self.send_response(200)
            self.send_header("Content-Type", "application/x-xpinstall")
            self.send_header("Content-Length", str(len(body)))
            self._cors()
            self.end_headers()
            self.wfile.write(body)
            return
        if path in ("/extension/guard.crx", "/extension/guard_extension.crx", "/extension/yp-raksha.crx"):
            from agent_extension import _crx_path
            crx = _crx_path()
            if not os.path.isfile(crx):
                self.send_error(404)
                return
            with open(crx, "rb") as f:
                body = f.read()
            self.send_response(200)
            self.send_header("Content-Type", "application/x-chrome-extension")
            self.send_header("Content-Length", str(len(body)))
            self._cors()
            self.end_headers()
            self.wfile.write(body)
            return
        if path not in ("/proxy.pac", "/pac"):
            self.send_error(404)
            return
        body = (
            b"function FindProxyForURL(url, host) { return \"DIRECT\"; }\n"
        )
        try:
            with open(local_pac_path(), "rb") as f:
                body = f.read() or body
        except Exception:
            pass
        self.send_response(200)
        self.send_header("Content-Type", "application/x-ns-proxy-autoconfig")
        self.send_header("Cache-Control", "no-store, no-cache, must-revalidate")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args) -> None:  # noqa: A003
        return


def start_local_pac_http_server() -> str:
    """Chrome ignores file:// PAC. Serve it over HTTP on localhost instead."""
    last_err = None
    for port in (PAC_HTTP_PORT, PAC_HTTP_PORT + 1, PAC_HTTP_PORT + 2):
        try:
            httpd = http.server.ThreadingHTTPServer((PAC_HTTP_HOST, port), _PACRequestHandler)
            thread = threading.Thread(target=httpd.serve_forever, daemon=True)
            thread.start()
            agent_state._PAC_HTTP_URL = f"http://{PAC_HTTP_HOST}:{port}/proxy.pac"
            # The MitM worker inherits this and long-polls /extension/wait for instant policy.
            os.environ["GATEWAY_GUARD_LOCAL_BASE"] = f"http://{PAC_HTTP_HOST}:{port}"
            print(f"[Gateway Guard] Local PAC HTTP server: {agent_state._PAC_HTTP_URL}")
            return agent_state._PAC_HTTP_URL
        except Exception as e:
            last_err = e
    print(f"[Gateway Guard ERROR] Could not bind local PAC HTTP server: {last_err}")
    return agent_state._PAC_HTTP_URL
