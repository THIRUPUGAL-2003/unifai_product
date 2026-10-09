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
req = urllib.request.Request("http://127.0.0.1:8080/api/browser-ai/logs?limit=30", headers=auth_headers)
with urllib.request.urlopen(req, timeout=3) as resp:
    data = json.loads(resp.read().decode())
    logs = data.get("logs", [])
    for l in logs:
        ts = l.get("created_at", "")
        if "00:0" in ts:
            print("ID:", l.get("id"))
            print("Platform:", l.get("platform"))
            print("Domain:", l.get("domain"))
            print("Preview:", l.get("user_prompt_preview"))
            print("Full:", l.get("user_prompt_full"))
            print("Status:", l.get("status"))
            print("Metadata:", l.get("metadata"))
            print("-" * 50)
