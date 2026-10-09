import urllib.request
import ssl
import json

proxy_handler = urllib.request.ProxyHandler({'http': 'http://127.0.0.1:18103', 'https': 'http://127.0.0.1:18103'})
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
opener = urllib.request.build_opener(proxy_handler, urllib.request.HTTPSHandler(context=ctx))

payload = {
    "prompt": "explain this code",
    "attachments": [
        {
            "file_name": "app.py",
            "extracted_content": "def foo():\n    pin = 600028\n    return pin\n",
            "file_type": "text/x-python"
        }
    ]
}

req = urllib.request.Request(
    'https://claude.ai/api/organizations/123/chat_conversations/456/completion',
    data=json.dumps(payload).encode('utf-8'),
    headers={'Content-Type': 'application/json', 'Accept': 'text/event-stream'}
)
try:
    with opener.open(req, timeout=5) as resp:
        body = resp.read().decode('utf-8', errors='ignore')
        print(f"Status: {resp.status}")
        print("Body:", body[:500])
except Exception as e:
    print('Error:', e)
