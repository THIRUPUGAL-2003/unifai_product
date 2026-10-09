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
req = urllib.request.Request("http://127.0.0.1:8080/api/browser-ai/logs?limit=15", headers=auth_headers)
with urllib.request.urlopen(req, timeout=3) as resp:
    data = json.loads(resp.read().decode())
    logs = data.get("logs", [])
    for l in logs:
        if l.get("platform") == "Claude" or "claude" in str(l.get("domain", "")).lower():
            print(json.dumps(l, indent=2))
            print("="*60)
