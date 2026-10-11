import urllib.request
import json
import http.cookiejar

# 1. Login to get session cookie
cj = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))

login_req = urllib.request.Request(
    'http://127.0.0.1:8080/api/session/login',
    data=json.dumps({'username': 'admin@yespanchi.com', 'password': 'YP2025-2026yp'}).encode(),
    headers={'Content-Type': 'application/json'}
)

try:
    with opener.open(login_req) as resp:
        print('Login success!')
except Exception as e:
    print('Login failed:', e)
    exit(1)

# 2. Test chat completion with DEVELOPER KEY
payload = {
    'model': 'mistral/codestral-2508',
    'messages': [
        {'role': 'user', 'content': 'Hello, say test in one word'}
    ],
    'max_tokens': 10
}

chat_req = urllib.request.Request(
    'http://127.0.0.1:8080/v1/chat/completions',
    data=json.dumps(payload).encode(),
    headers={
        'Content-Type': 'application/json',
        'x-uf-api-key-id': '059738dc-e1ff-4914-ab19-e311d1973d1a'
    }
)

try:
    with opener.open(chat_req) as resp:
        data = json.loads(resp.read().decode())
        print('Chat Completion Success!')
        print('Response:', data.get('choices', [{}])[0].get('message', {}).get('content'))
except urllib.error.HTTPError as e:
    err_body = e.read().decode('utf-8', errors='replace')
    print(f'HTTP Error {e.code}: {err_body}')
except Exception as e:
    print('Unexpected error:', e)
