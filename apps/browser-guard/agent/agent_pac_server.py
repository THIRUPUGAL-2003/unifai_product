"""Local HTTP server that serves proxy.pac and status pages to browsers."""

from __future__ import annotations

import http.server
import json
import threading

import agent_state
from agent_config import AGENT_VERSION, PAC_HTTP_HOST, PAC_HTTP_PORT, UNIFAI_BACKEND_URL
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
    def do_GET(self) -> None:
        path = (self.path or "/").split("?", 1)[0]
        if path in ("/status", "/health"):
            report = agent_state._LAST_HEALTH if isinstance(agent_state._LAST_HEALTH, dict) and agent_state._LAST_HEALTH else {"status": "starting", "agent_version": AGENT_VERSION}
            body = json.dumps(report, indent=2).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if path in ("/", "/status.html"):
            report = agent_state._LAST_HEALTH if isinstance(agent_state._LAST_HEALTH, dict) else {}
            st = html_escape(str(report.get("status") or "starting"))
            ver = html_escape(AGENT_VERSION)
            backend = html_escape(UNIFAI_BACKEND_URL)
            checks = report.get("checks") if isinstance(report.get("checks"), dict) else {}
            rows = "".join(
                f"<tr><td>{html_escape(k)}</td><td>{'OK' if v else 'FAIL'}</td></tr>" for k, v in checks.items()
            )
            details = report.get("details") if isinstance(report.get("details"), list) else []
            det = "<br/>".join(html_escape(str(d)) for d in details) or "—"
            html = f"""<!DOCTYPE html><html><head><meta charset="utf-8"/><title>UnifAI Guard</title>
<style>body{{font-family:Segoe UI,sans-serif;background:#0b1220;color:#e2e8f0;padding:24px}}
.card{{background:#111827;border:1px solid #334155;border-radius:12px;padding:20px;max-width:720px}}
h1{{margin:0 0 8px;font-size:20px}} .ok{{color:#34d399}} .bad{{color:#f87171}} .deg{{color:#fbbf24}}
table{{width:100%;border-collapse:collapse;margin-top:12px}} td,th{{border-bottom:1px solid #334155;padding:8px;text-align:left;font-size:13px}}
</style></head><body><div class="card">
<h1>UnifAI Guard {ver}</h1>
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
            print(f"[UnifAI Guard] Local PAC HTTP server: {agent_state._PAC_HTTP_URL}")
            return agent_state._PAC_HTTP_URL
        except Exception as e:
            last_err = e
    print(f"[UnifAI Guard ERROR] Could not bind local PAC HTTP server: {last_err}")
    return agent_state._PAC_HTTP_URL
