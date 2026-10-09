import urllib.request
import ssl

proxy_handler = urllib.request.ProxyHandler({'http': 'http://127.0.0.1:18103', 'https': 'http://127.0.0.1:18103'})
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
opener = urllib.request.build_opener(proxy_handler, urllib.request.HTTPSHandler(context=ctx))

for domain in ["claude.ai", "api.claude.ai", "api.anthropic.com", "files.claudeusercontent.com", "cdn.claude.ai"]:
    req = urllib.request.Request(
        f"https://{domain}/api/organizations/123/chat_conversations/456/completion",
        data=b'{"prompt": "My pin is 600028"}',
        headers={'Content-Type': 'application/json', 'Accept': 'text/event-stream'}
    )
    try:
        with opener.open(req, timeout=5) as resp:
            body = resp.read().decode('utf-8', errors='ignore')
            print(f"[{domain}] -> status={resp.status} len={len(body)}")
            print("BODY:", body[:300])
    except Exception as e:
        print(f"[{domain}] -> error: {e}")
