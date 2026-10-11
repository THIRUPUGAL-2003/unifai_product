import urllib.request
import ssl
import json
import time
import psycopg2

print("==========================================================")
print("   GATEWAY GUARD LOCAL COMPREHENSIVE LIVE TEST SUITE      ")
print("==========================================================")

PROXY_ADDR = "http://127.0.0.1:18103"
proxy_handler = urllib.request.ProxyHandler({'https': PROXY_ADDR, 'http': PROXY_ADDR})
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
opener = urllib.request.build_opener(proxy_handler, urllib.request.HTTPSHandler(context=ctx))

# Connect to DB to check rows before/after
conn = psycopg2.connect(
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    host='76.13.243.253',
    port=32768
)
cur = conn.cursor()

def get_latest_log_for(query_str):
    cur.execute(
        "SELECT id, domain, user_prompt_preview, attachment_name, action, rule_triggered "
        "FROM browser_ai_logs WHERE user_prompt_preview ILIKE %s OR attachment_name ILIKE %s "
        "ORDER BY created_at DESC LIMIT 5",
        (f"%{query_str}%", f"%{query_str}%")
    )
    return cur.fetchall()

# -------------------------------------------------------------
# TEST 1: ChatGPT Prompt Block (Post Code rule: 613002)
# -------------------------------------------------------------
print("\n[TEST 1] Testing ChatGPT Blocked Prompt (Post code 613002)...")
test1_prompt = "my office address post code is 613002"
req1 = urllib.request.Request(
    "https://chatgpt.com/backend-api/conversation",
    data=json.dumps({
        "model": "gpt-4o",
        "messages": [{"role": "user", "content": {"content_type": "text", "parts": [test1_prompt]}}]
    }).encode("utf-8"),
    headers={"Content-Type": "application/json", "Accept": "text/event-stream", "Origin": "https://chatgpt.com"}
)

try:
    with opener.open(req1, timeout=5) as resp:
        body = resp.read().decode("utf-8", errors="replace")
        print(f" -> HTTP Status: {resp.status} (Expected: 200)")
        print(f" -> Content-Type: {resp.headers.get('Content-Type')}")
        has_block_text = "post code not alloweed" in body or "post code" in body
        print(f" -> Block message rendered in SSE body: {has_block_text}")
        print(f" -> Raw SSE snippet: {body[:180]}...")
except Exception as e:
    print(f" -> Error: {e}")

time.sleep(1)
rows1 = get_latest_log_for("613002")
print(f" -> DB Log Rows Found: {len(rows1)}")
for r in rows1[:1]:
    print(f"    DB Entry: action={r[4]}, rule={r[5]}, domain={r[1]}, prompt={r[2]}")

# -------------------------------------------------------------
# TEST 2: ChatGPT Prompt Block (India PAN Card rule: ABCDE1234F)
# -------------------------------------------------------------
print("\n[TEST 2] Testing ChatGPT Blocked Prompt (PAN Card: ABCDE1234F)...")
test2_prompt = "My PAN number is ABCDE1234F"
req2 = urllib.request.Request(
    "https://chatgpt.com/backend-api/conversation",
    data=json.dumps({
        "model": "gpt-4o",
        "messages": [{"role": "user", "content": {"content_type": "text", "parts": [test2_prompt]}}]
    }).encode("utf-8"),
    headers={"Content-Type": "application/json", "Accept": "text/event-stream", "Origin": "https://chatgpt.com"}
)

try:
    with opener.open(req2, timeout=5) as resp:
        body = resp.read().decode("utf-8", errors="replace")
        print(f" -> HTTP Status: {resp.status}")
        print(f" -> Block message in SSE: {'ABCDE1234F' in body or 'PAN' in body or 'not alloweed' in body or 'Blocked' in body}")
        print(f" -> Raw snippet: {body[:180]}...")
except Exception as e:
    print(f" -> Error: {e}")

time.sleep(1)
rows2 = get_latest_log_for("ABCDE1234F")
print(f" -> DB Log Rows Found: {len(rows2)}")
for r in rows2[:1]:
    print(f"    DB Entry: action={r[4]}, rule={r[5]}, domain={r[1]}, prompt={r[2]}")

# -------------------------------------------------------------
# TEST 3: File Upload Staging Endpoint (Should NOT create any DB row!)
# -------------------------------------------------------------
print("\n[TEST 3] Testing File Staging Upload Endpoint (Should NOT log standalone)...")
boundary = "----WebKitFormBoundaryXYZ987654"
stage_file_name = "Confidential_Quarterly_Report_2026.docx"
multipart_body = (
    f"--{boundary}\r\n"
    f"Content-Disposition: form-data; name=\"file\"; filename=\"{stage_file_name}\"\r\n"
    f"Content-Type: application/vnd.openxmlformats-officedocument.wordprocessingml.document\r\n\r\n"
    f"Quarterly Financial Statement and Audit Details\r\n"
    f"--{boundary}--\r\n"
).encode("utf-8")

req3 = urllib.request.Request(
    "https://chatgpt.com/backend-api/files",
    data=multipart_body,
    headers={"Content-Type": f"multipart/form-data; boundary={boundary}", "Origin": "https://chatgpt.com"}
)

cur.execute("SELECT COUNT(*) FROM browser_ai_logs WHERE attachment_name ILIKE %s", (f"%{stage_file_name}%",))
count_before = cur.fetchone()[0]

try:
    with opener.open(req3, timeout=5) as resp:
        pass
except urllib.error.HTTPError as e:
    # Staging endpoints handled or rejected upstream
    pass
except Exception:
    pass

time.sleep(1)
cur.execute("SELECT COUNT(*) FROM browser_ai_logs WHERE attachment_name ILIKE %s", (f"%{stage_file_name}%",))
count_after_stage = cur.fetchone()[0]
print(f" -> Staging upload row count: Before={count_before}, After={count_after_stage} (Diff: {count_after_stage - count_before})")
if count_after_stage == count_before:
    print(" -> SUCCESS: Zero trailing/duplicate rows created on file staging!")
else:
    print(" -> WARNING: Row created on staging endpoint!")

# -------------------------------------------------------------
# TEST 4: Chat Send with Attachment (Should create EXACTLY 1 clean row!)
# -------------------------------------------------------------
print("\n[TEST 4] Testing Chat Send with File Attachment (Should create EXACTLY 1 row)...")
test4_file = "Audit_Checklist_LocalTest.xlsx"
test4_prompt = "Please review this audit checklist"

# 1. Staging pass
mult4 = (
    f"--{boundary}\r\n"
    f"Content-Disposition: form-data; name=\"file\"; filename=\"{test4_file}\"\r\n"
    f"Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet\r\n\r\n"
    f"Financial audit data\r\n"
    f"--{boundary}--\r\n"
).encode("utf-8")
try:
    opener.open(urllib.request.Request("https://chatgpt.com/backend-api/files", data=mult4, headers={"Content-Type": f"multipart/form-data; boundary={boundary}"}), timeout=3)
except Exception:
    pass

# 2. Chat Send pass (with blocked post code rule inside prompt)
req4 = urllib.request.Request(
    "https://chatgpt.com/backend-api/conversation",
    data=json.dumps({
        "model": "gpt-4o",
        "messages": [{
            "role": "user",
            "content": {"content_type": "text", "parts": [f"{test4_prompt} - post code is 613002"]},
            "metadata": {"attachments": [{"name": test4_file, "id": "file-998877"}]}
        }]
    }).encode("utf-8"),
    headers={"Content-Type": "application/json", "Accept": "text/event-stream", "Origin": "https://chatgpt.com"}
)

try:
    with opener.open(req4, timeout=5) as resp:
        body = resp.read().decode("utf-8", errors="replace")
        print(f" -> HTTP Status: {resp.status}")
        print(f" -> SSE in-page bubble rendered: {'post code not alloweed' in body}")
except Exception as e:
    print(f" -> Send error: {e}")

time.sleep(1)
rows4 = get_latest_log_for("Audit_Checklist_LocalTest")
print(f" -> DB Log Rows for this transaction: {len(rows4)} (Expected: exactly 1)")
for r in rows4:
    print(f"    Row: id={r[0]}, file='{r[3]}', prompt='{r[2]}', action={r[4]}, rule={r[5]}")

print("\n==========================================================")
print("   LOCAL TEST SUITE COMPLETED                             ")
print("==========================================================")
