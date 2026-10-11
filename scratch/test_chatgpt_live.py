import urllib.request
import ssl
import json

proxy_handler = urllib.request.ProxyHandler({
    'https': 'http://127.0.0.1:18103',
    'http': 'http://127.0.0.1:18103'
})
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
opener = urllib.request.build_opener(proxy_handler, urllib.request.HTTPSHandler(context=ctx))

# Test 1: Upload staging request (POST /backend-api/files)
boundary = '----WebKitFormBoundary7MA4YWxkTrZu0gW'
body = (
    f'--{boundary}\r\n'
    f'Content-Disposition: form-data; name="file"; filename="sample_project_notes.txt"\r\n'
    f'Content-Type: text/plain\r\n\r\n'
    f'Sample confidential project notes content\r\n'
    f'--{boundary}--\r\n'
).encode('utf-8')

req1 = urllib.request.Request(
    'https://chatgpt.com/backend-api/files',
    data=body,
    headers={
        'Content-Type': f'multipart/form-data; boundary={boundary}',
        'Origin': 'https://chatgpt.com'
    }
)

try:
    with opener.open(req1, timeout=5) as r:
        print('Upload staging status:', r.status)
except urllib.error.HTTPError as e:
    print('Upload staging response code:', e.code)
except Exception as e:
    print('Upload staging result:', type(e).__name__, e)

# Test 2: Chat send with the file attached
conv = {
    'model': 'gpt-4o',
    'messages': [{
        'role': 'user',
        'content': {
            'content_type': 'text',
            'parts': ['summarize this note']
        },
        'metadata': {
            'attachments': [{'name': 'sample_project_notes.txt', 'id': 'file-12345'}]
        }
    }]
}
req2 = urllib.request.Request(
    'https://chatgpt.com/backend-api/conversation',
    data=json.dumps(conv).encode('utf-8'),
    headers={
        'Content-Type': 'application/json',
        'Accept': 'text/event-stream',
        'Origin': 'https://chatgpt.com'
    }
)

try:
    with opener.open(req2, timeout=5) as r:
        print('Chat send status:', r.status)
        print('Chat send preview:', r.read(200).decode('utf-8', errors='replace'))
except urllib.error.HTTPError as e:
    print('Chat send response code:', e.code)
except Exception as e:
    print('Chat send result:', type(e).__name__, e)
