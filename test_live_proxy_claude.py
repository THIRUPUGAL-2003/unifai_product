import urllib.request
import ssl

proxy_handler = urllib.request.ProxyHandler({'http': 'http://127.0.0.1:18103', 'https': 'http://127.0.0.1:18103'})
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
opener = urllib.request.build_opener(proxy_handler, urllib.request.HTTPSHandler(context=ctx))

# Test request to claude.ai completion endpoint with post code 600028
req = urllib.request.Request(
    'https://claude.ai/api/organizations/123/chat_conversations/456/completion',
    data=b'{"prompt": "My pin is 600028"}',
    headers={'Content-Type': 'application/json', 'Accept': 'text/event-stream'}
)
try:
    with opener.open(req, timeout=5) as resp:
        print('Status:', resp.status)
        print('Headers:', dict(resp.headers))
        print('Body:', resp.read()[:500].decode('utf-8', errors='ignore'))
except Exception as e:
    print('Error:', e)
