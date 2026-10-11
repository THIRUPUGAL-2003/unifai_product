import urllib.request, json, ssl

ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

proxy_handler = urllib.request.ProxyHandler({
    'http': 'http://127.0.0.1:18103',
    'https': 'http://127.0.0.1:18103'
})
opener = urllib.request.build_opener(proxy_handler, urllib.request.HTTPSHandler(context=ctx))

# 1. Test ChatGPT block response
payload = json.dumps({
    "action": "next",
    "messages": [
        {
            "id": "user-msg-123",
            "author": {"role": "user"},
            "content": {"content_type": "text", "parts": ["my post code is 613002"]}
        }
    ],
    "parent_message_id": "root-123",
    "model": "gpt-4o",
    "timezone_offset_min": -330
}).encode("utf-8")

req = urllib.request.Request(
    'https://chatgpt.com/backend-api/f/conversation',
    data=payload,
    headers={
        'Accept': 'text/event-stream',
        'Content-Type': 'application/json',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)'
    }
)

try:
    with opener.open(req, timeout=10) as resp:
        print("ChatGPT Block Status Code:", resp.status)
        print("ChatGPT Block Headers Content-Type:", resp.headers.get("content-type"))
        text = resp.read().decode("utf-8", errors="ignore")
        print("ChatGPT Block Response Text:\n", text[:600])
except Exception as e:
    print("ChatGPT test error:", e)

# 2. Test Claude block response
claude_payload = json.dumps({
    "prompt": "my post code is 613002",
    "timezone": "Asia/Calcutta",
    "model": "claude-3-5-sonnet-20241022"
}).encode("utf-8")

req2 = urllib.request.Request(
    'https://claude.ai/api/organizations/bdae9546-5469-4020-aa12-15ae2d8a3673/chat_conversations/05f0c14e-58d0-482b-bb1a-ec723be72dd2/completion',
    data=claude_payload,
    headers={
        'Accept': 'text/event-stream',
        'Content-Type': 'application/json',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)'
    }
)

try:
    with opener.open(req2, timeout=10) as resp2:
        print("\nClaude Block Status Code:", resp2.status)
        print("Claude Block Headers Content-Type:", resp2.headers.get("content-type"))
        text2 = resp2.read().decode("utf-8", errors="ignore")
        print("Claude Block Response Text:\n", text2[:600])
except Exception as e:
    print("Claude test error:", e)
