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

payload = {
    'model': 'openrouter/cohere/north-mini-code:free',
    'messages': [{'role': 'user', 'content': 'Hello, say test in one word'}],
    'max_tokens': 10
}
req = urllib.request.Request(
    f'{base_url}/v1/chat/completions',
    data=json.dumps(payload).encode(),
    headers={'Content-Type': 'application/json'}
)
try:
    with opener.open(req) as resp:
        data = json.loads(resp.read().decode())
        print('Openrouter Cohere response:', data.get('choices', [{}])[0].get('message', {}).get('content'))
except urllib.error.HTTPError as e:
    print('Openrouter Cohere HTTP Error:', e.code, e.read().decode())
