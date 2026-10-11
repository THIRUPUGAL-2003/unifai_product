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
with opener.open(req) as resp:
    data = json.loads(resp.read().decode())
    vks = data.get('virtual_keys', data)
    dev_vk = next(vk for vk in vks if vk.get('id') == '059738dc-e1ff-4914-ab19-e311d1973d1a')
    dev_vk_value = dev_vk['value']
    print('Found DEVELOPER KEY full value:', dev_vk_value[:15] + '...')

# Now make chat completion using DEVELOPER KEY via x-uf-vk header
payload = {
    'model': 'mistral/codestral-2508',
    'messages': [{'role': 'user', 'content': 'Say hello'}],
    'max_tokens': 5
}
chat_req = urllib.request.Request(
    f'{base_url}/v1/chat/completions',
    data=json.dumps(payload).encode(),
    headers={
        'Content-Type': 'application/json',
        'x-uf-vk': dev_vk_value
    }
)
try:
    with opener.open(chat_req) as resp:
        print('Chat completion success with DEVELOPER KEY!')
        print('Resp:', resp.read().decode()[:200])
except urllib.error.HTTPError as e:
    print('HTTP Error:', e.code, e.read().decode())
