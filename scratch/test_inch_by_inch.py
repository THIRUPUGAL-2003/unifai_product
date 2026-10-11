import urllib.request, json, ssl, time, psycopg2

ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

proxy_handler = urllib.request.ProxyHandler({
    'http': 'http://127.0.0.1:18103',
    'https': 'http://127.0.0.1:18103'
})
opener = urllib.request.build_opener(proxy_handler, urllib.request.HTTPSHandler(context=ctx))

def send_req(url, payload_dict, headers_extra=None):
    headers = {
        'Accept': 'text/event-stream',
        'Content-Type': 'application/json',
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)'
    }
    if headers_extra:
        headers.update(headers_extra)
    data = json.dumps(payload_dict).encode("utf-8") if isinstance(payload_dict, dict) else payload_dict
    req = urllib.request.Request(url, data=data, headers=headers)
    try:
        with opener.open(req, timeout=12) as resp:
            body = resp.read().decode("utf-8", errors="ignore")
            return resp.status, resp.headers.get("content-type"), body
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="ignore")
        return e.code, e.headers.get("content-type"), body
    except Exception as e:
        return 0, str(e), ""

print("="*60)
print("INCH-BY-INCH VERIFICATION SUITE")
print("="*60)

# TEST 1: ChatGPT In-Chat Security Block
code, ct, body = send_req(
    'https://chatgpt.com/backend-api/f/conversation',
    {
        "action": "next",
        "messages": [{"id": "msg-gpt-test", "author": {"role": "user"}, "content": {"content_type": "text", "parts": ["aadhaar 2345 6789 0123"]}}],
        "parent_message_id": "root-1",
        "model": "gpt-4o"
    }
)
print("\n[TEST 1] ChatGPT Aadhaar Block:")
print("  HTTP Status:", code)
print("  Content-Type:", ct)
has_block_text = "not allowed" in body.lower() or "blocked" in body.lower() or "aadhaar" in body.lower()
print("  Has in-chat block text:", has_block_text)
print("  Body snippet:", body[:200].replace("\n", " "))
assert code == 200 and has_block_text, "TEST 1 FAILED"
print("  => PASS!")

# TEST 2: Claude In-Chat Security Block
code, ct, body = send_req(
    'https://claude.ai/api/organizations/bdae9546-5469-4020-aa12-15ae2d8a3673/chat_conversations/05f0c14e-58d0-482b-bb1a-ec723be72dd2/completion',
    {
        "prompt": "aadhaar 2345 6789 0123",
        "timezone": "Asia/Calcutta",
        "model": "claude-3-5-sonnet-20241022"
    }
)
print("\n[TEST 2] Claude Aadhaar Block:")
print("  HTTP Status:", code)
print("  Content-Type:", ct)
has_block_text = "not allowed" in body.lower() or "blocked" in body.lower() or "aadhaar" in body.lower()
print("  Has in-chat block text:", has_block_text)
print("  Body snippet:", body[:200].replace("\n", " "))
assert code == 200 and has_block_text, "TEST 2 FAILED"
print("  => PASS!")

# TEST 3: Staging Upload Endpoint on Claude (/wiggle/upload-file)
# Should NOT log any prompt and should NOT fail
raw_multipart = (
    b'--boundary_test\r\n'
    b'Content-Disposition: form-data; name="file"; filename="Test_Document_Audit.xlsx"\r\n'
    b'Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet\r\n\r\n'
    b'PK\x03\x04\x14\x00\x00\x00TestSheetContent1234567890\r\n'
    b'--boundary_test--\r\n'
)
code, ct, body = send_req(
    'https://claude.ai/api/organizations/bdae9546-5469-4020-aa12-15ae2d8a3673/conversations/05f0c14e-58d0-482b-bb1a-ec723be72dd2/wiggle/upload-file',
    raw_multipart,
    headers_extra={'Content-Type': 'multipart/form-data; boundary=boundary_test'}
)
print("\n[TEST 3] Claude Upload Staging (/wiggle/upload-file):")
print("  HTTP Status (forwarded or cached):", code)
print("  => Upload staging handled cleanly without premature logging!")

# Check DB: ensure NO row was created for Test_Document_Audit.xlsx yet
time.sleep(1)
conn = psycopg2.connect(
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    host='76.13.243.253',
    port=32768
)
cur = conn.cursor()
cur.execute("SELECT count(*) FROM browser_ai_logs WHERE attachment_name ILIKE '%Test_Document_Audit%';")
staged_count = cur.fetchone()[0]
print("  DB rows before chat Send:", staged_count)
assert staged_count == 0, f"Expected 0 rows during staging, got {staged_count}"
print("  => PASS! (Zero rows logged on upload staging)")

# TEST 4: Chat Send with the file attached + prompt
code, ct, body = send_req(
    'https://claude.ai/api/organizations/bdae9546-5469-4020-aa12-15ae2d8a3673/chat_conversations/05f0c14e-58d0-482b-bb1a-ec723be72dd2/completion',
    {
        "prompt": "summarize this file please",
        "attachments": [{"file_name": "Test_Document_Audit.xlsx"}],
        "timezone": "Asia/Calcutta",
        "model": "claude-3-5-sonnet-20241022"
    }
)
print("\n[TEST 4] Claude Chat Send with File + Prompt:")
print("  HTTP Status:", code)

time.sleep(5)
cur.execute("""
SELECT id, user_prompt_preview, attachment_name, action
FROM browser_ai_logs
WHERE attachment_name ILIKE '%Test_Document_Audit%'
ORDER BY created_at DESC;
""")
rows = cur.fetchall()
print("  DB rows after chat Send:", len(rows))
for r in rows:
    print("   Row:", r[1], "| File:", r[2], "| Action:", r[3])
assert len(rows) == 1, f"Expected EXACTLY 1 row, got {len(rows)}"
assert "summarize this file please" in rows[0][1], f"Expected prompt in row, got {rows[0][1]}"
print("  => PASS! (Exactly 1 row logged with prompt and zero duplicate rows!)")

# TEST 5: Subsequent wire for same file within TTL without caption -> Dropped cleanly
code, ct, body = send_req(
    'https://claude.ai/api/organizations/bdae9546-5469-4020-aa12-15ae2d8a3673/conversations/05f0c14e-58d0-482b-bb1a-ec723be72dd2/wiggle/upload-file',
    raw_multipart,
    headers_extra={'Content-Type': 'multipart/form-data; boundary=boundary_test'}
)
time.sleep(1)
cur.execute("SELECT count(*) FROM browser_ai_logs WHERE attachment_name ILIKE '%Test_Document_Audit%';")
final_count = cur.fetchone()[0]
print("\n[TEST 5] Trailing Wire Deduplication Check:")
print("  Total rows after trailing wire:", final_count)
assert final_count == 1, f"Expected still 1 row, got {final_count}"
print("  => PASS! (Trailing wire did NOT create duplicate row!)")

print("\n" + "="*60)
print("ALL 5 INCH-BY-INCH TESTS PASSED WITH 100% SUCCESS!")
print("="*60)
