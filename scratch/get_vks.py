from db_config import get_base_url
base_url = get_base_url()
import urllib.request
import json
import http.cookiejar

cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

login_req = urllib.request.Request(
    f'{base_url}/api/session/login',
    data=json.dumps({'username': 'admin@yespanchi.com', 'password': 'YP2025-2026yp'}).encode(),
    headers={'Content-Type': 'application/json'}
)
opener.open(login_req)

req = urllib.request.Request(f'{base_url}/api/governance/virtual-keys')
try:
    with opener.open(req) as resp:
        data = json.loads(resp.read().decode())
        vks = data.get('virtual_keys', data) if isinstance(data, dict) else data
        for vk in vks:
            val = vk.get('value', '')
            masked = val[:10] + '...' + val[-5:] if len(val) > 15 else val
            print('VK:', vk.get('name'), '| ID:', vk.get('id'), '| Value:', masked)
except urllib.error.HTTPError as e:
    print('HTTP Error:', e.code, e.read().decode())
