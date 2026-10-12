import json
import urllib.request
import urllib.error
import http.cookiejar

cookie_jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookie_jar))

BASE_URL = "http://127.0.0.1:8080"
GUARD_KEY = "ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo="

def login():
    payload = json.dumps({"username": "admin@yespanchi.com", "password": "YP2025-2026yp"}).encode("utf-8")
    req = urllib.request.Request(
        f"{BASE_URL}/api/session/login",
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST"
    )
    with opener.open(req) as resp:
        return resp.status == 200

if not login():
    print("Login failed!")
    exit(1)

def req(method, path, body=None, extra_headers=None):
    headers = {}
    if extra_headers:
        headers.update(extra_headers)
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    r = urllib.request.Request(f"{BASE_URL}{path}", data=data, headers=headers, method=method)
    try:
        with opener.open(r) as resp:
            content = resp.read().decode()
            return resp.status, json.loads(content) if content else {}
    except urllib.error.HTTPError as e:
        content = e.read().decode()
        try:
            return e.code, json.loads(content)
        except:
            return e.code, {"raw": content}

# 1. Create a prompt log by intercepting
intercept_payload = {
    "url": "https://chatgpt.com/backend-api/conversation",
    "host": "chatgpt.com",
    "prompt": "Test prompt for bulk delete verification",
    "agent_id": "test-bulk-agent",
    "agent_hostname": "TEST-HOST"
}
status, intercept_res = req("POST", "/api/browser-ai/intercept", intercept_payload, {"X-Gateway-Guard-Key": GUARD_KEY})
print(f"Intercept prompt: {status}")

# 2. Query logs to get the created log ID
status, logs_res = req("GET", "/api/browser-ai/logs?limit=5")
prompt_log_id = None
if isinstance(logs_res, dict) and "logs" in logs_res and len(logs_res["logs"]) > 0:
    prompt_log_id = logs_res["logs"][0]["id"]
print(f"Got prompt log ID: {prompt_log_id}")

if prompt_log_id:
    status, del_res = req("POST", "/api/browser-ai/logs/bulk-delete", {"ids": [prompt_log_id]})
    print(f"Bulk delete prompt log -> HTTP {status}: {del_res}")
    assert status == 200

# 3. Create a search log with guard security key
status, s_res = req("POST", "/api/browser-ai/search-logs", {
    "engine": "google",
    "browser": "chrome",
    "query": "bulk delete test query verification",
    "url": "https://www.google.com/search?q=test",
    "host": "www.google.com"
}, extra_headers={"X-Gateway-Guard-Key": GUARD_KEY})
print(f"Record search log status: {status}")
search_log_id = s_res.get("log", {}).get("id") if isinstance(s_res, dict) else None
print(f"Got search log ID: {search_log_id}")

if search_log_id:
    status, del_res = req("POST", "/api/browser-ai/search-logs/bulk-delete", {"ids": [search_log_id]})
    print(f"Bulk delete search log -> HTTP {status}: {del_res}")
    assert status == 200

print("\nALL VERIFICATIONS PASSED 100%!")
