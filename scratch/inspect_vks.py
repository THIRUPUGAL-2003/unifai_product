import urllib.request
import json
import http.cookiejar

cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

# Login
req = urllib.request.Request(
    'http://127.0.0.1:6000/api/session/login',
    data=json.dumps({'username': 'admin@yespanchi.com', 'password': 'YP2025-2026yp'}).encode('utf-8'),
    headers={'Content-Type': 'application/json'},
    method='POST'
)
opener.open(req)

# Get virtual keys
resp = opener.open('http://127.0.0.1:6000/api/governance/virtual-keys')
data = json.loads(resp.read().decode('utf-8'))
vks = data.get('virtual_keys', [])
print(f"Total Virtual Keys: {len(vks)}")
for vk in vks:
    providers = [p.get('provider') for p in vk.get('provider_configs', [])]
    print(f"- Name: '{vk.get('name')}', ID: {vk.get('id')}, Active: {vk.get('is_active')}, Value: {vk.get('value', '')[:15]}..., Providers: {providers}, Origin: {vk.get('origin')}")
