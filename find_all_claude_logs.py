import urllib.request
import json

login_data = json.dumps({
    "username": "admin@yespanchi.com",
    "password": "YP2025-2026yp"
}).encode('utf-8')

req = urllib.request.Request("http://127.0.0.1:8080/api/session/login", data=login_data, headers={"Content-Type": "application/json"})
with urllib.request.urlopen(req, timeout=3) as resp:
    cookies = resp.headers.get("Set-Cookie")

auth_headers = {"Accept": "application/json", "Cookie": cookies}
req = urllib.request.Request("http://127.0.0.1:8080/api/browser-ai/logs?limit=100", headers=auth_headers)
with urllib.request.urlopen(req, timeout=3) as resp:
    data = json.loads(resp.read().decode())
    logs = data.get("logs", [])
    print(f"Total logs in DB: {len(logs)}")
    claude_logs = [l for l in logs if l.get("platform") == "Claude" or "claude" in str(l.get("domain", "")).lower()]
    print(f"Claude logs count: {len(claude_logs)}")
    for l in claude_logs:
        meta = json.loads(l.get("metadata", "{}") or "{}") if isinstance(l.get("metadata"), str) else (l.get("metadata") or {})
        print(f"ID={l.get('id')} | TS={l.get('created_at')} | Status={l.get('status')} | URL={meta.get('url')} | Prompt={l.get('user_prompt_preview')!r}")
