import os
import sys
import json
import time
import ssl
import urllib.request
import psycopg2

PROXY_HANDLER = urllib.request.ProxyHandler({
    "http": "http://127.0.0.1:18103",
    "https": "http://127.0.0.1:18103"
})
SSL_CTX = ssl.create_default_context()
SSL_CTX.check_hostname = False
SSL_CTX.verify_mode = ssl.CERT_NONE
OPENER = urllib.request.build_opener(PROXY_HANDLER, urllib.request.HTTPSHandler(context=SSL_CTX))

# 1. Connect to DB to check baseline
env_vars = {}
with open('d:/unifai_project/.env', 'r', encoding='utf-8') as f:
    for line in f:
        line = line.strip()
        if line and not line.startswith('#') and '=' in line:
            k, v = line.split('=', 1)
            env_vars[k.strip()] = v.strip('\'"')

conn = psycopg2.connect(
    dbname=env_vars.get('DB_NAME', 'unifai'),
    user=env_vars.get('DB_USER', 'postgres'),
    password=env_vars.get('DB_PASSWORD', 'postgres'),
    host=env_vars.get('DB_HOST', 'localhost'),
    port=env_vars.get('DB_PORT', '5432')
)
cur = conn.cursor()

test_run_tag = f"livecheck-{int(time.time())}"
print(f"=== STARTING LIVE CHECK ACROSS ALL AI DOMAINS & SUBDOMAINS ({test_run_tag}) ===")

domains_to_test = [
    {
        "domain": "claude.ai",
        "platform": "Claude",
        "url": "https://claude.ai/api/organizations/org_test/chat_conversations/conv_test/completion",
        "headers": {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"},
        "body": json.dumps({"prompt": f"{test_run_tag} on claude.ai", "attachments": []}).encode("utf-8"),
        "should_log": True
    },
    {
        # Noise filter test: Cloudflare turnstile on /login
        "domain": "claude.ai",
        "platform": "Claude",
        "url": "https://claude.ai/login?from=logout&reauth=1",
        "headers": {"Content-Type": "application/x-www-form-urlencoded", "User-Agent": "Mozilla/5.0"},
        "body": b"89721075028dcbd210651b465dc1bc92ce37dccafe74f3f2b73148fb56bae8c4=7_IPp0a.aME5yH50V0gOF4AwjSzG1Ze_VU8OYLioZxU-1791713886-1.2.1.1-2GOD3bJEzqgZVI8",
        "should_log": False # MUST NOT LOG
    },
    {
        "domain": "chatgpt.com",
        "platform": "ChatGPT",
        "url": "https://chatgpt.com/backend-api/conversation",
        "headers": {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"},
        "body": json.dumps({
            "action": "next",
            "messages": [{
                "id": "aaa",
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": [f"{test_run_tag} on chatgpt.com"]}
            }],
            "model": "gpt-4o"
        }).encode("utf-8"),
        "should_log": True
    },
    {
        "domain": "copilot.microsoft.com",
        "platform": "Copilot",
        "url": "https://copilot.microsoft.com/c/api/chat",
        "headers": {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"},
        "body": json.dumps({"prompt": f"{test_run_tag} on copilot.microsoft.com"}).encode("utf-8"),
        "should_log": True
    },
    {
        "domain": "gemini.google.com",
        "platform": "Gemini",
        "url": "https://gemini.google.com/_/BardChatUi/data/batchexecute",
        "headers": {"Content-Type": "application/x-www-form-urlencoded", "User-Agent": "Mozilla/5.0"},
        "body": f"f.req=%5B%5B%5B%22fKjhEb%22%2C%22%5Bnull%2C%5C%22%5B%5B%5C%22{test_run_tag}+on+gemini.google.com%5C%22%5D%5D%5C%22%5D%22%5D%5D%5D".encode("utf-8"),
        "should_log": True
    },
    {
        "domain": "perplexity.ai",
        "platform": "Perplexity",
        "url": "https://www.perplexity.ai/rest/sse/ask",
        "headers": {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"},
        "body": json.dumps({"query": f"{test_run_tag} on perplexity.ai"}).encode("utf-8"),
        "should_log": True
    },
    {
        "domain": "chat.deepseek.com",
        "platform": "DeepSeek",
        "url": "https://chat.deepseek.com/api/v0/chat/completions",
        "headers": {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"},
        "body": json.dumps({
            "prompt": f"{test_run_tag} on chat.deepseek.com",
            "messages": [{"role": "user", "content": f"{test_run_tag} on chat.deepseek.com"}]
        }).encode("utf-8"),
        "should_log": True
    },
    {
        "domain": "chat.mistral.ai",
        "platform": "Mistral",
        "url": "https://chat.mistral.ai/api/chat",
        "headers": {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"},
        "body": json.dumps({"prompt": f"{test_run_tag} on chat.mistral.ai"}).encode("utf-8"),
        "should_log": True
    },
    {
        "domain": "blackbox.ai",
        "platform": "Blackbox AI",
        "url": "https://www.blackbox.ai/api/chat",
        "headers": {"Content-Type": "application/json", "User-Agent": "Mozilla/5.0"},
        "body": json.dumps({"messages": [{"role": "user", "content": f"{test_run_tag} on blackbox.ai"}]}).encode("utf-8"),
        "should_log": True
    }
]

for tc in domains_to_test:
    dom = tc["domain"]
    print(f"-> Sending request to {tc['platform']} ({dom})...")
    req = urllib.request.Request(
        tc["url"],
        data=tc["body"],
        headers=tc["headers"],
        method="POST"
    )
    try:
        with OPENER.open(req, timeout=4) as response:
            print(f"   Response Code: {response.getcode()}")
    except urllib.error.HTTPError as he:
        print(f"   Response Code: {he.code}")
    except Exception as e:
        print(f"   Upstream status: intercepted by proxy ({type(e).__name__})")

time.sleep(2)

print("\n=== AUDITING POSTGRES DATABASE FOR TEST RUN ===")
cur.execute("""
    SELECT platform, user_prompt_preview, domain, action, created_at
    FROM browser_ai_logs 
    WHERE user_prompt_preview LIKE %s OR user_prompt_full LIKE %s
    ORDER BY created_at ASC;
""", (f"%{test_run_tag}%", f"%{test_run_tag}%"))
rows = cur.fetchall()

print(f"Logged {len(rows)} entries for run {test_run_tag}:")
for r in rows:
    print(f"  [LOGGED] Plat: {r[0]:<12} | Action: {r[3]:<8} | Domain: {r[2]:<24} | Preview: {r[1]}")

cur.execute("""
    SELECT count(*) FROM browser_ai_logs 
    WHERE user_prompt_preview LIKE '89721075%' AND created_at > NOW() - INTERVAL '2 minutes';
""")
hex_count = cur.fetchone()[0]
print(f"\n[SECURITY NOISE CHECK] Hex noise count in last 2 minutes: {hex_count} (Expected: 0)")

conn.close()
print("\n=== TEST COMPLETE ===")
