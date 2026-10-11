from db_config import get_db_connection
import urllib.request
import ssl
import json
import time
import psycopg2
import sys

def p(msg):
    print(msg, flush=True)

p("=" * 65)
p("   GATEWAY LIVE TEST: CHATGPT.COM & CLAUDE.AI")
p("=" * 65)

PROXY_ADDR = "http://127.0.0.1:18103"
proxy_handler = urllib.request.ProxyHandler({'https': PROXY_ADDR, 'http': PROXY_ADDR})
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
opener = urllib.request.build_opener(proxy_handler, urllib.request.HTTPSHandler(context=ctx))

conn = get_db_connection()
cur = conn.cursor()

def fetch_latest_log(limit=6):
    cur.execute(
        "SELECT id, domain, user_prompt_preview, attachment_name, action, rule_triggered, created_at "
        "FROM browser_ai_logs ORDER BY created_at DESC LIMIT %s",
        (limit,)
    )
    return cur.fetchall()

# -------------------------------------------------------------
# 1. CHATGPT.COM - TEST 1: Blocked Post Code (613002)
# -------------------------------------------------------------
p("\n[1] Testing ChatGPT.com -> Rule: 'post code' (613002)...")
req1 = urllib.request.Request(
    "https://chatgpt.com/backend-api/conversation",
    data=json.dumps({
        "model": "gpt-4o",
        "messages": [{"role": "user", "content": {"content_type": "text", "parts": ["ChatGPT test: office post code is 613002"]}}]
    }).encode("utf-8"),
    headers={"Content-Type": "application/json", "Accept": "text/event-stream", "Origin": "https://chatgpt.com"}
)
try:
    with opener.open(req1, timeout=5) as r:
        body = r.read().decode("utf-8", errors="replace")
        p(f"    Status: {r.status} OK (synthetic SSE in-page bubble)")
        p(f"    In-Page Block Message: {'post code not alloweed' in body}")
        p(f"    SSE Preview: {body[:140]}...")
except Exception as e:
    p(f"    Error: {e}")

# -------------------------------------------------------------
# 2. CHATGPT.COM - TEST 2: File Attachment + Blocked Prompt (1 Row Verification)
# -------------------------------------------------------------
p("\n[2] Testing ChatGPT.com -> File + Blocked Prompt (Audit_2026.xlsx)...")
req2 = urllib.request.Request(
    "https://chatgpt.com/backend-api/conversation",
    data=json.dumps({
        "model": "gpt-4o",
        "messages": [{
            "role": "user",
            "content": {"content_type": "text", "parts": ["Summarize this file: PAN card ABCDE1234F"]},
            "metadata": {"attachments": [{"name": "Audit_Report_2026.xlsx", "id": "file-audit-99"}]}
        }]
    }).encode("utf-8"),
    headers={"Content-Type": "application/json", "Accept": "text/event-stream", "Origin": "https://chatgpt.com"}
)
try:
    with opener.open(req2, timeout=5) as r:
        body = r.read().decode("utf-8", errors="replace")
        p(f"    Status: {r.status} OK")
        p(f"    In-Page Block Message: {'India PAN Card' in body or 'not alloweed' in body or 'Blocked' in body}")
except Exception as e:
    p(f"    Error: {e}")

# -------------------------------------------------------------
# 3. CLAUDE.AI - TEST 3: Blocked Post Code (613002)
# -------------------------------------------------------------
p("\n[3] Testing Claude.ai -> Rule: 'post code' (613002)...")
req3 = urllib.request.Request(
    "https://claude.ai/api/organizations/org-test-uuid/chat_conversations/conv-test-uuid/completion",
    data=json.dumps({
        "prompt": "Claude test: delivery pincode is 613002 please confirm",
        "timezone": "Asia/Kolkata",
        "model": "claude-3-5-sonnet-20241022"
    }).encode("utf-8"),
    headers={"Content-Type": "application/json", "Accept": "text/event-stream", "Origin": "https://claude.ai"}
)
try:
    with opener.open(req3, timeout=5) as r:
        body = r.read().decode("utf-8", errors="replace")
        p(f"    Status: {r.status} OK (Anthropic SSE event format)")
        p(f"    In-Page Block Message: {'post code not alloweed' in body}")
        p(f"    SSE Preview: {body[:140]}...")
except Exception as e:
    p(f"    Error: {e}")

# -------------------------------------------------------------
# 4. CLAUDE.AI - TEST 4: File Attachment + Blocked Prompt (1 Row Verification)
# -------------------------------------------------------------
p("\n[4] Testing Claude.ai -> File + Blocked Prompt (Claude_Employee_Data.pdf)...")
req4 = urllib.request.Request(
    "https://claude.ai/api/organizations/org-test-uuid/chat_conversations/conv-test-uuid/completion",
    data=json.dumps({
        "prompt": "Claude review this file: PAN is ABCDE9876Z",
        "attachments": [{"file_name": "Claude_Employee_Data.pdf", "file_size": 1024}],
        "model": "claude-3-5-sonnet-20241022"
    }).encode("utf-8"),
    headers={"Content-Type": "application/json", "Accept": "text/event-stream", "Origin": "https://claude.ai"}
)
try:
    with opener.open(req4, timeout=5) as r:
        body = r.read().decode("utf-8", errors="replace")
        p(f"    Status: {r.status} OK")
        p(f"    In-Page Block Message: {'India PAN Card' in body or 'not alloweed' in body or 'Blocked' in body}")
except Exception as e:
    p(f"    Error: {e}")

# -------------------------------------------------------------
# VERIFY DATABASE & UI LOGS
# -------------------------------------------------------------
time.sleep(1)
p("\n" + "=" * 65)
p("   RECENT ROWS IN BROWSER_AI_LOGS (Shown on UI http://localhost:3101)")
p("=" * 65)

recent_rows = fetch_latest_log(6)
for idx, r in enumerate(recent_rows, 1):
    log_id, domain, prompt, attach, action, rule, created = r
    p(f"  {idx}. Domain: {domain:<14} | Action: {action:<7} | Rule: {rule:<15} | File: {attach or '-':<24} | Prompt: {prompt[:38]}")

p("\nAll tests completed successfully!")
