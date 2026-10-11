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

tag = f"filecheck-{int(time.time())}"
print(f"=== TESTING FILE UPLOAD + CAPTION BINDING ({tag}) ===")

# 1. Simulate Claude staging upload /wiggle/upload-file (multipart without user prompt)
boundary = "----WebKitFormBoundary7MA4YWxkTrZu0gW"
body_upload = (
    f"--{boundary}\r\n"
    f"Content-Disposition: form-data; name=\"file\"; filename=\"audit_sample.xlsx\"\r\n"
    f"Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet\r\n\r\n"
    f"PK\x03\x04mock_excel_bytes_content_here\r\n"
    f"--{boundary}--\r\n"
).encode("latin-1")

req1 = urllib.request.Request(
    "https://claude.ai/api/organizations/org_test/conversations/conv_test/wiggle/upload-file",
    data=body_upload,
    headers={
        "Content-Type": f"multipart/form-data; boundary={boundary}",
        "User-Agent": "Mozilla/5.0"
    },
    method="POST"
)
try:
    with OPENER.open(req1, timeout=4) as r:
        print(f"Stage 1 (Upload File) Response: {r.getcode()}")
except Exception as e:
    print(f"Stage 1 note: {e}")

time.sleep(1)

# 2. Simulate Claude finished Send /completion with prompt caption
body_completion = json.dumps({
    "prompt": f"{tag} caption text",
    "attachments": [{"file_name": "audit_sample.xlsx", "extracted_content": "mock_excel"}]
}).encode("utf-8")

req2 = urllib.request.Request(
    "https://claude.ai/api/organizations/org_test/chat_conversations/conv_test/completion",
    data=body_completion,
    headers={
        "Content-Type": "application/json",
        "User-Agent": "Mozilla/5.0"
    },
    method="POST"
)
try:
    with OPENER.open(req2, timeout=4) as r:
        print(f"Stage 2 (Send Message) Response: {r.getcode()}")
except Exception as e:
    print(f"Stage 2 note: {e}")

time.sleep(2)

cur.execute("""
    SELECT platform, user_prompt_preview, attachment_name, action, created_at
    FROM browser_ai_logs 
    WHERE user_prompt_preview LIKE %s OR user_prompt_full LIKE %s OR attachment_name LIKE 'audit_sample%'
    ORDER BY created_at DESC LIMIT 5;
""", (f"%{tag}%", f"%{tag}%"))
rows = cur.fetchall()
print(f"\nLogged entries for file test {tag}:")
for r in rows:
    print(f"  Plat: {r[0]} | Preview: {repr(r[1])} | Attach: {repr(r[2])} | Action: {r[3]}")

conn.close()
print("=== FILE TEST FINISHED ===")
