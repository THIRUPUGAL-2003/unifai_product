from db_config import get_base_url
base_url = get_base_url()
import urllib.request
import json
import http.cookiejar

# 1. Login
cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

login_req = urllib.request.Request(
    f'{base_url}/api/session/login',
    data=json.dumps({'username': 'admin@yespanchi.com', 'password': 'YP2025-2026yp'}).encode(),
    headers={'Content-Type': 'application/json'}
)
opener.open(login_req)

rule_id = '898d275c-4181-40a9-8945-816632f7443f'

# 2. Get current rule
get_req = urllib.request.Request(ff'{base_url}/api/governance/routing-rules/{rule_id}')
with opener.open(get_req) as resp:
    curr_rule = json.loads(resp.read().decode())
    print('Current rule before update:', curr_rule.get('name'), '| CEL:', curr_rule.get('cel_expression'), '| Scope:', curr_rule.get('scope'))

# 3. Update CEL expression to correct condition: provider == "mistral"
update_payload = {
    'cel_expression': 'provider == "mistral"',
    'scope': 'virtual_key',
    'scope_id': '059738dc-e1ff-4914-ab19-e311d1973d1a'
}
put_req = urllib.request.Request(
    ff'{base_url}/api/governance/routing-rules/{rule_id}',
    data=json.dumps(update_payload).encode(),
    headers={'Content-Type': 'application/json'},
    method='PUT'
)
with opener.open(put_req) as resp:
    updated = json.loads(resp.read().decode())
    print('Updated rule:', updated.get('name'), '| CEL:', updated.get('cel_expression'))

# 4. Get DEVELOPER KEY value
req = urllib.request.Request(f'{base_url}/api/governance/virtual-keys')
with opener.open(req) as resp:
    data = json.loads(resp.read().decode())
    vks = data.get('virtual_keys', data)
    dev_vk = next(vk for vk in vks if vk.get('id') == '059738dc-e1ff-4914-ab19-e311d1973d1a')
    dev_vk_val = dev_vk['value']

# 5. Send test chat request with DEVELOPER KEY
chat_payload = {
    'model': 'mistral/codestral-2508',
    'messages': [{'role': 'user', 'content': 'Test word: Antigravity'}],
    'max_tokens': 5
}
chat_req = urllib.request.Request(
    f'{base_url}/v1/chat/completions',
    data=json.dumps(chat_payload).encode(),
    headers={
        'Content-Type': 'application/json',
        'x-uf-vk': dev_vk_val
    }
)
with opener.open(chat_req) as resp:
    print('Chat response status:', resp.status)
