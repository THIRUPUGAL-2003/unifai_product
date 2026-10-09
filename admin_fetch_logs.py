import urllib.request
import json

# Login as admin
login_data = json.dumps({
    "username": "admin@yespanchi.com",
    "password": "YP2025-2026yp"
}).encode('utf-8')

req = urllib.request.Request("http://127.0.0.1:8080/api/session/login", data=login_data, headers={"Content-Type": "application/json"})
token = ""
cookies = ""
try:
    with urllib.request.urlopen(req, timeout=3) as resp:
        body = json.loads(resp.read().decode())
        token = body.get("token") or body.get("access_token") or ""
        cookies = resp.headers.get("Set-Cookie")
        print("Login OK. Token:", token[:20] if token else "No token in body, cookie:", cookies)
except Exception as e:
    print("Login error:", e)

# Fetch logs with token/cookie
auth_headers = {"Accept": "application/json"}
if token:
    auth_headers["Authorization"] = f"Bearer {token}"
if cookies:
    auth_headers["Cookie"] = cookies

req = urllib.request.Request("http://127.0.0.1:8080/api/browser-ai/logs?limit=25", headers=auth_headers)
try:
    with urllib.request.urlopen(req, timeout=3) as resp:
        data = json.loads(resp.read().decode())
        logs = data.get("logs", [])
        print(f"Total prompt logs returned: {len(logs)}")
        for l in logs:
            plat = l.get("platform")
            dom = l.get("domain")
            stat = l.get("status")
            p = l.get("prompt", "") or ""
            r = l.get("rule_triggered", "") or ""
            ts = l.get("created_at", "")
            print(f"  [{plat}] {dom} | status={stat} rule={r} | prompt={p[:60]!r} | {ts}")
except Exception as e:
    print("Fetch logs error:", e)
