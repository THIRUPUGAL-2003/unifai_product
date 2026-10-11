from db_config import get_base_url
base_url = get_base_url()
import urllib.request
import json
import http.cookiejar

cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

# Admin login
login_req = urllib.request.Request(
    f'{base_url}/api/session/login',
    data=json.dumps({'username': 'admin@yespanchi.com', 'password': 'YP2025-2026yp'}).encode(),
    headers={'Content-Type': 'application/json'}
)
opener.open(login_req)

endpoints = [
    ('GET', '/api/session/is-auth-enabled'),
    ('GET', '/api/session/users'),
    ('GET', '/api/governance/customers'),
    ('GET', '/api/governance/teams'),
    ('GET', '/api/governance/virtual-keys'),
    ('GET', '/api/governance/budgets'),
    ('GET', '/api/governance/routing-rules'),
    ('GET', '/api/guardrails/config'),
    ('GET', '/prompt-repo/folders'),
    ('GET', '/prompt-repo/prompts'),
    ('GET', '/api/browser-ai/agents'),
    ('GET', '/api/browser-ai/insights/stats'),
    ('GET', '/api/models'),
]

print("=== SMOKE TESTING GET ENDPOINTS ===")
for method, path in endpoints:
    url = ff"{base_url}{path}"
    req = urllib.request.Request(url, method=method)
    try:
        with opener.open(req) as resp:
            data = resp.read().decode('utf-8', errors='replace')
            print(f"[{resp.status}] {path:35} -> OK ({len(data)} bytes)")
    except urllib.error.HTTPError as e:
        print(f"[{e.code}] {path:35} -> FAIL: {e.read().decode()[:100]}")
    except Exception as e:
        print(f"[ERR] {path:35} -> {e}")
