import urllib.request
import urllib.parse
import http.cookiejar
import json

BASE_URL = "http://localhost:8080"
cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

# 1. Login as Admin
login_data = json.dumps({
    "username": "admin@yespanchi.com",
    "password": "YP2025-2026yp"
}).encode('utf-8')

req = urllib.request.Request(f"{BASE_URL}/api/session/login", data=login_data, headers={"Content-Type": "application/json"})
with opener.open(req) as resp:
    print(f"Admin Login: {resp.status}")

endpoints_to_test = [
    # Feature flags
    ("GET", "/api/feature-flags", None),
    # Config / Core
    ("GET", "/api/config", None),
    # Caching
    ("GET", "/api/caching-config", None),
    # Proxy
    ("GET", "/api/proxy-config", None),
    # SMTP / Security
    ("GET", "/api/smtp-config", None),
    ("GET", "/api/auth-config", None),
    # Vector store
    ("GET", "/api/vector-store-config", None),
    # Compatibility
    ("GET", "/api/compatibility-config", None),
    # Performance tuning / System info
    ("GET", "/api/system-info", None),
]

for method, path, payload in endpoints_to_test:
    data = json.dumps(payload).encode('utf-8') if payload else None
    headers = {"Content-Type": "application/json"} if payload else {}
    req = urllib.request.Request(f"{BASE_URL}{path}", data=data, headers=headers, method=method)
    try:
        with opener.open(req) as resp:
            body = resp.read().decode('utf-8')
            print(f"[{method}] {path} -> {resp.status} (body length: {len(body)})")
    except urllib.error.HTTPError as e:
        print(f"[{method}] {path} -> FAILED: {e.code} - {e.read().decode('utf-8')}")
    except Exception as e:
        print(f"[{method}] {path} -> ERROR: {e}")
