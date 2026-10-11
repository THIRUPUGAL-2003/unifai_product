import os
import sys
import json
import time
import urllib.request
import ssl
import psycopg2

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

CA_CERT = os.path.expandvars(r"%USERPROFILE%\.mitmproxy\mitmproxy-ca-cert.pem")
PROXY_ADDR = "127.0.0.1:18103"

print("=" * 70)
print("LIVE CHATGPT TESTING VIA RUNNING GATEWAY_GUARD (PORT 18103)")
print("=" * 70)

# 1. Setup DB connection to verify live logs
env_vars = {}
with open('d:/unifai_project/.env', 'r', encoding='utf-8') as f:
    for line in f:
        line = line.strip()
        if line and not line.startswith('#') and '=' in line:
            k, v = line.split('=', 1)
            env_vars[k.strip()] = v.strip().strip('\'"')

conn = psycopg2.connect(
    dbname=env_vars.get('DB_NAME', 'unifai'),
    user=env_vars.get('DB_USER', 'postgres'),
    password=env_vars.get('DB_PASSWORD', 'postgres'),
    host=env_vars.get('DB_HOST', 'localhost'),
    port=env_vars.get('DB_PORT', '5432')
)

def get_latest_log():
    cur = conn.cursor()
    cur.execute("SELECT created_at, user_prompt_preview, attachment_name, metadata FROM browser_ai_logs ORDER BY created_at DESC LIMIT 1;")
    row = cur.fetchone()
    cur.close()
    return row

start_row = get_latest_log()
print("Current latest log in DB before tests:")
if start_row:
    print(f"  Time: {start_row[0]} | Prompt: {start_row[1]} | Attachment: {start_row[2]}")
else:
    print("  No previous logs.")

# 2. Test pure chat submit through proxy
proxy_handler = urllib.request.ProxyHandler({
    "http": f"http://{PROXY_ADDR}",
    "https": f"http://{PROXY_ADDR}"
})
ssl_context = ssl.create_default_context(cafile=CA_CERT)
ssl_context.check_hostname = False
ssl_context.verify_mode = ssl.CERT_NONE  # For local mitm testing
https_handler = urllib.request.HTTPSHandler(context=ssl_context)
opener = urllib.request.build_opener(proxy_handler, https_handler)

test_cases = [
    ("Pure Prompt - Tamil", "வணக்கம், நீங்கள் எப்படி இருக்கிறீர்கள்?"),
    ("Pure Prompt - English", "hii mama"),
    ("Pure Prompt - Numbers", "1000000 + 250000 = 1250000"),
    ("Pure Prompt - Symbols", "SELECT * FROM users WHERE active = true;"),
    ("Pure Prompt - Emojis", "🚀🔥🎉 Robot AI Super Testing! 🤖"),
]

for label, prompt_text in test_cases:
    print(f"\n--- Testing: {label} ---")
    payload = {
        "action": "next",
        "messages": [
            {
                "id": f"msg-{int(time.time()*1000)}",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": [prompt_text]}
            }
        ],
        "model": "gpt-4o"
    }
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(
        "https://chatgpt.com/backend-api/conversation",
        data=body,
        headers={
            "Content-Type": "application/json",
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
            "Referer": "https://chatgpt.com/"
        },
        method="POST"
    )
    try:
        # We expect a network error from upstream chatgpt.com because fake auth token,
        # but the request HAS ALREADY PASSED THROUGH the proxy request hook where Guard inspects it!
        opener.open(req, timeout=5)
    except Exception as e:
        # Expected upstream 401/403/connection error from real OpenAI server
        pass

    time.sleep(2)  # Give 2s for async intercept POST to reach backend
    latest = get_latest_log()
    if latest:
        logged_prompt = latest[1]
        attach = latest[2]
        print(f"Logged in DB -> Prompt: {logged_prompt!r} | Attachment: {attach!r}")
        
        # VERIFICATION:
        if "@" in logged_prompt or "📎" in logged_prompt or "sites-preview" in str(logged_prompt) or bool(attach):
            print(f"❌ FAILED! Erroneously attached file to pure prompt!")
        else:
            print(f"✅ PASSED! Clean pure prompt logged without any file attachment!")

conn.close()
print("\n" + "=" * 70)
print("LIVE PROXY TEST FINISHED")
print("=" * 70)
