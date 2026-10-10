#!/usr/bin/env python3
"""Comprehensive Perplexity Live Verification Suite:
1. Target Domains & Proxy Cache Inspection:
   - Verifies all Perplexity domains loaded in backend and proxy cache:
     perplexity.ai, www.perplexity.ai, labs.perplexity.ai, api.perplexity.ai
   - Verifies host roles and upload family bindings.
2. Prompts Testing (inch-by-inch across REST & Socket.IO):
   - Clean text -> ALLOWED
   - Sensitive digits / Pincode (600028) -> BLOCKED
   - Pure numbers -> BLOCKED
   - Symbols & Punctuation -> ALLOWED
   - Emojis + Secret -> BLOCKED
   - Tamil Unicode + Secret -> BLOCKED
   - Code snippet violation -> BLOCKED
   - Socket.IO with ack ID -> BLOCKED with Security Message
3. Perplexity Security Bubble Message Verification (All 4 Protocols):
   - HTTP SSE stream (/rest/sse/perplexity_ask):
     data: {"text": "<warning>", "answer": "<warning>", "status": "completed", "final": true, "blocks": [...]}
     data: [DONE]
   - HTTP JSON endpoint (/rest/thread with Accept: application/json):
     {"text": "<warning>", "answer": "<warning>", "status": "completed", "final": true, ...}
   - Socket.IO HTTP Polling (/socket.io/?transport=polling):
     42["query_progress", {"text": "<warning>", ...}]
     42["query_answered", {"text": "<warning>", ...}]
   - Socket.IO WebSocket stream (wss://www.perplexity.ai/socket.io/...):
     42["query_progress", ...], 42["query_answered", ...], 43<id>[...]
     so Perplexity chat UI displays the security warning bubble without crashing or spinning!
4. All 15 File Categories (Upload + Send Binding):
   - PDF, Word (DOCX), Excel (XLSX), PowerPoint (PPTX), CSV, Python, JS, SQL,
     JSON, .env, ZIP archive, Binary, Clean Text, Clean Code, Image Screenshot OCR.
5. Perplexity Multi-File Simultaneous Upload (3 Files at once):
   - Clean notes + CSV with violation + Clean Python code
   - Truthful filenames preserved, violation pinned, turn blocked with security bubble.
"""

import io
import json
import os
import re
import sys
import time
import uuid
import zipfile
from pathlib import Path
from PIL import Image, ImageDraw
from mitmproxy import http
from mitmproxy.test import tflow

# Load gateway proxy parts
PARTS = Path(r"d:\unifai_project\apps\browser-guard\proxy\gateway_proxy_parts")
ns = {"__name__": "perplexity_suite_test"}
names = [
    ln.strip().lstrip("\ufeff")
    for ln in (PARTS / "MANIFEST.txt").read_text(encoding="utf-8-sig").splitlines()
    if ln.strip() and not ln.strip().startswith("#")
]
for name in names:
    p = PARTS / name
    exec(compile(p.read_text(encoding="utf-8"), str(p), "exec"), ns)

# Setup test rule:
# Rule: Pincode \b[1-9][0-9]{5}\b (e.g. 600028) -> BLOCK
ns["_cached_rules"] = [
    {
        "id": "rule-pincode",
        "name": "Pincode Rule",
        "pattern": r"\b[1-9][0-9]{5}\b",
        "action": "BLOCK",
        "warning_message": "Sensitive pincode detected"
    }
]

addon = ns["BrowserAIInterceptor"]()

# Helper builders for files
def make_docx(text: str) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("[Content_Types].xml", '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>')
        z.writestr("word/document.xml", f'<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>{text}</w:t></w:r></w:p></w:body></w:document>')
    return buf.getvalue()

def make_xlsx(text: str) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("[Content_Types].xml", '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>')
        z.writestr("xl/sharedStrings.xml", f'<?xml version="1.0"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>{text}</t></si></sst>')
        z.writestr("xl/worksheets/sheet1.xml", '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row><c t="s"><v>0</v></c></row></sheetData></worksheet>')
    return buf.getvalue()

def make_pptx(text: str) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("[Content_Types].xml", '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>')
        z.writestr("ppt/slides/slide1.xml", f'<?xml version="1.0"?><p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>{text}</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>')
    return buf.getvalue()

def make_zip(filename: str, content: str) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr(filename, content)
    return buf.getvalue()

def make_ocr_png(text: str) -> bytes:
    img = Image.new("RGB", (650, 150), color=(255, 255, 255))
    draw = ImageDraw.Draw(img)
    draw.text((40, 50), text, fill=(0, 0, 0))
    buf = io.BytesIO()
    img.save(buf, format="PNG")
    return buf.getvalue()

PDF = b"%PDF-1.4\n1 0 obj\n<< /Length 50 >>\nstream\nBT /F1 12 Tf 72 712 Td (Account pincode is 600028 secret) Tj ET\nendstream\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF"

def send_pplx_prompt(prompt_text: str, is_socketio=False, ack_id="0") -> tuple[bool, str]:
    if is_socketio:
        body = f'42{ack_id}["perplexity_ask",{json.dumps(prompt_text)},{{"source":"default","mode":"copilot"}}]'.encode("utf-8")
        f = tflow.tflow(
            req=http.Request.make("POST", "https://www.perplexity.ai/socket.io/?EIO=4&transport=polling", body, {
                "Host": "www.perplexity.ai",
                "Content-Type": "text/plain;charset=UTF-8",
                "Origin": "https://www.perplexity.ai",
            })
        )
    else:
        body = json.dumps({"query_str": prompt_text}).encode("utf-8")
        f = tflow.tflow(
            req=http.Request.make("POST", "https://www.perplexity.ai/rest/sse/perplexity_ask", body, {
                "Host": "www.perplexity.ai",
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
                "Origin": "https://www.perplexity.ai",
            })
        )
    addon.request(f)
    blocked = f.response is not None and f.response.status_code == 200
    resp_text = f.response.text if f.response else ""
    return blocked, resp_text

def test_file_category(cat_name: str, filename: str, content: bytes, content_type: str, expect_block: bool) -> bool:
    # Perplexity file upload to /rest/upload
    boundary = "----WebKitFormBoundaryPerplexity" + uuid.uuid4().hex[:12]
    multipart = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'
        f"Content-Type: {content_type}\r\n\r\n"
    ).encode("utf-8") + content + f"\r\n--{boundary}--\r\n".encode("utf-8")

    f_up = tflow.tflow(
        req=http.Request.make("POST", "https://www.perplexity.ai/rest/upload", multipart, {
            "Host": "www.perplexity.ai",
            "Content-Type": f"multipart/form-data; boundary={boundary}",
            "Origin": "https://www.perplexity.ai",
        })
    )
    addon.request(f_up)

    # Perplexity Chat Send referencing uploaded file
    f_send = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://www.perplexity.ai/rest/sse/perplexity_ask",
            json.dumps({
                "query_str": "Analyze this document for me",
                "attachments": [{"name": filename, "id": "att-" + uuid.uuid4().hex[:8]}]
            }).encode("utf-8"),
            {
                "Host": "www.perplexity.ai",
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
                "Origin": "https://www.perplexity.ai",
            }
        )
    )
    addon.request(f_send)
    blocked = f_send.response is not None and f_send.response.status_code == 200
    status_str = "BLOCKED" if blocked else "ALLOWED"
    expected_str = "BLOCKED" if expect_block else "ALLOWED"
    passed = (blocked == expect_block)
    print(f"[{'PASS' if passed else 'FAIL'}] {cat_name:<30} | File: {filename:<25} | Status: {status_str} | Expected: {expected_str}")
    return passed


print("=" * 85)
print(" GATEWAY BROWSER GUARD — PERPLEXITY LIVE SUITE")
print("=" * 85)

# SECTION 1: Domains & Target cache
print("\n>>> [SECTION 1] PERPLEXITY TARGET DOMAINS & BINDINGS")
pplx_doms = ["perplexity.ai", "www.perplexity.ai", "labs.perplexity.ai", "api.perplexity.ai"]
s1_ok = True
for d in pplx_doms:
    is_t, matched_dom, plat = ns["detect_target"](d)
    role = ns["get_target_host_role"](d)
    ok = is_t and "perplexity" in plat.lower()
    s1_ok = s1_ok and ok
    print(f"[{'PASS' if ok else 'FAIL'}] Domain: {d:<24} | Matched: {matched_dom:<16} | Platform: {plat:<12} | Role: {role}")

# SECTION 2: Prompts Testing
print("\n>>> [SECTION 2] PERPLEXITY INCH-BY-INCH PROMPT INTERCEPTION")
prompt_cases = [
    ("1. Clean Question", "What is quantum mechanics in simple terms?", False),
    ("2. Sensitive Pincode", "My delivery address is Chennai 600028 near temple", True),
    ("3. Pure Digits (Sensitive)", "600028", True),
    ("4. Symbols & Math", "Calculate: 10 + 20 * (30 / 5) = 130", False),
    ("5. Emojis + Secret", "Delivery at 600028 🚚📦 urgent please!", True),
    ("6. Tamil Unicode + Secret", "என் முகவரி சென்னை 600028 தமிழ்நாடு", True),
    ("7. Code Snippet Violation", 'pincode = "600028"\nprint(f"Shipping to {pincode}")', True),
    ("8. Socket.IO Polling Violation", "Secret pin code: 600028", True),
    ("9. Socket.IO Clean Prompt", "Tell me the story of the Indus Valley civilization", False),
]

s2_ok = True
for label, p_text, expect_block in prompt_cases:
    is_sio = "Socket.IO" in label
    blocked, resp = send_pplx_prompt(p_text, is_socketio=is_sio)
    status_str = "BLOCKED" if blocked else "ALLOWED"
    expected_str = "BLOCKED" if expect_block else "ALLOWED"
    passed = (blocked == expect_block)
    s2_ok = s2_ok and passed
    print(f"[{'PASS' if passed else 'FAIL'}] {label:<32} | Status: {status_str} | Expected: {expected_str}")

# SECTION 3: Perplexity Security Bubble Message Format
print("\n>>> [SECTION 3] PERPLEXITY IN-CHAT SECURITY MESSAGE BUBBLES")
ns["_recent_prompts"].clear()

# 3A. SSE Stream
f_sse = tflow.tflow(
    req=http.Request.make("POST", "https://www.perplexity.ai/rest/sse/perplexity_ask", b'{"query_str":"Pincode 600028"}', {
        "Host": "www.perplexity.ai",
        "Content-Type": "application/json",
        "Accept": "text/event-stream",
        "Origin": "https://www.perplexity.ai",
    })
)
addon.request(f_sse)
print("f_sse.response:", f_sse.response)
if f_sse.response:
    print("f_sse text:", repr(f_sse.response.text[:200]))
sse_text = f_sse.response.text if f_sse.response else ""
has_sse_bubble = (
    f_sse.response is not None
    and f_sse.response.status_code == 200
    and any(w in sse_text for w in ("Sensitive pincode detected", "post code not alloweed", "Blocked by Gateway Guard"))
    and '"status": "completed"' in sse_text
    and '"final": true' in sse_text
    and "data: [DONE]" in sse_text
)
print(f"[{'PASS' if has_sse_bubble else 'FAIL'}] 1. Perplexity HTTP SSE In-Chat Bubble       | 200 OK | text/event-stream | [DONE]")

# 3B. HTTP JSON
f_json = tflow.tflow(
    req=http.Request.make("POST", "https://www.perplexity.ai/rest/thread", b'{"query":"Pincode 600028 (JSON)"}', {
        "Host": "www.perplexity.ai",
        "Content-Type": "application/json",
        "Accept": "application/json",
        "Origin": "https://www.perplexity.ai",
    })
)
# Force unique message to bypass dedupe in same process
ns["make_blocked_response"](f_json, "Pincode Rule", "www.perplexity.ai", "Sensitive pincode detected (JSON)")
json_text = f_json.response.text if f_json.response else ""
has_json_bubble = (
    f_json.response.status_code == 200
    and "Sensitive pincode detected (JSON)" in json_text
    and '"final": true' in json_text
    and "application/json" in f_json.response.headers.get("Content-Type", "")
)
print(f"[{'PASS' if has_json_bubble else 'FAIL'}] 2. Perplexity HTTP JSON REST In-Chat Bubble   | 200 OK | application/json | text+answer")

# 3C. Socket.IO Polling
f_sio_poll = tflow.tflow(
    req=http.Request.make("POST", "https://www.perplexity.ai/socket.io/?EIO=4&transport=polling", b'42["perplexity_ask","600028"]', {
        "Host": "www.perplexity.ai",
        "Content-Type": "text/plain;charset=UTF-8",
        "Origin": "https://www.perplexity.ai",
    })
)
ns["make_blocked_response"](f_sio_poll, "Pincode Rule", "www.perplexity.ai", "Sensitive pincode detected (Polling)")
poll_text = f_sio_poll.response.text if f_sio_poll.response else ""
has_poll_bubble = (
    f_sio_poll.response.status_code == 200
    and '42["query_progress"' in poll_text
    and '42["query_answered"' in poll_text
    and "Sensitive pincode detected (Polling)" in poll_text
)
print(f"[{'PASS' if has_poll_bubble else 'FAIL'}] 3. Perplexity Socket.IO Polling Packet       | 200 OK | 42[\"query_progress\"], 42[\"query_answered\"]")

# 3D. Socket.IO WebSocket Frames
ws_frames = ns["_ws_frames_socketio"]("Sensitive pincode detected (WS)", last_client_msg='420["perplexity_ask","600028"]')
has_ws_frames = (
    len(ws_frames) >= 4
    and all(f.startswith(b"42") or f.startswith(b"43") for f in ws_frames)
    and any(b"Sensitive pincode detected (WS)" in f for f in ws_frames)
    and any(f.startswith(b"430[") for f in ws_frames)
)
print(f"[{'PASS' if has_ws_frames else 'FAIL'}] 4. Perplexity Socket.IO WebSocket Stream      | Multi-frame 42 event burst + 430 ack")

# SECTION 4: All 15 File Categories
print("\n>>> [SECTION 4] PERPLEXITY 15 FILE CATEGORIES (ATTACH & SEND)")
files_tested = [
    ("1. PDF Document (.pdf)", "statement.pdf", PDF, "application/pdf", True),
    ("2. Microsoft Word (.docx)", "letter.docx", make_docx("Confidential postal pincode 600028"), "application/vnd.openxmlformats-officedocument.wordprocessingml.document", True),
    ("3. Microsoft Excel (.xlsx)", "salaries.xlsx", make_xlsx("Employee office 600028"), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", True),
    ("4. PowerPoint (.pptx)", "pitch.pptx", make_pptx("HQ Branch pincode 600028"), "application/vnd.openxmlformats-officedocument.presentationml.presentation", True),
    ("5. CSV Spreadsheet (.csv)", "customers.csv", b"id,name,pincode\n1,Ramesh,600028\n", "text/csv", True),
    ("6. Python Script (.py)", "data_loader.py", b'# script\nloc = "Chennai 600028"\n', "text/x-python", True),
    ("7. JavaScript Code (.js)", "auth_controller.js", b'const pin = "600028";\n', "application/javascript", True),
    ("8. SQL Database (.sql)", "schema_dump.sql", b"INSERT INTO branch VALUES ('Chennai', 600028);", "text/x-sql", True),
    ("9. JSON Config (.json)", "app_settings.json", b'{"app": "unifai", "pincode": "600028"}', "application/json", True),
    ("10. Environment (.env)", ".env.production", b"DATABASE_URL=postgres\nPINCODE=600028\n", "text/plain", True),
    ("11. Compressed Archive (ZIP)", "archive_backup.zip", make_zip("secrets.txt", "Secret pincode: 600028"), "application/zip", True),
    ("12. Binary Model (.bin)", "model_config.bin", b"\x00\x01\x02\x03MODEL_VERSION_2\x00\x00CONFIDENTIAL_TOKEN: 600028 AUTHORIZED\x00\x00\x04\x05", "application/octet-stream", True),
    ("13. Image Screenshot (OCR)", "confidential_screenshot.png", make_ocr_png("CONFIDENTIAL PINCODE: 600028"), "image/png", True),
    ("14. Plain Text (Clean)", "readme_documentation.txt", b"Public API Documentation - Welcome to Perplexity AI!", "text/plain", False),
    ("15. Python Code (Clean)", "math_utils.py", b"def add(a, b):\n    return a + b\n", "text/x-python", False),
]

s4_ok = True
for cat_name, fname, fcontent, fctype, exp_b in files_tested:
    ok = test_file_category(cat_name, fname, fcontent, fctype, exp_b)
    s4_ok = s4_ok and ok

# SECTION 5: Multi-File Simultaneous Upload
print("\n>>> [SECTION 5] PERPLEXITY MULTI-FILE SIMULTANEOUS UPLOAD (3 FILES)")
# Upload File 1: Clean text
f1_bytes = b"Architecture overview: Perplexity utilizes fast RAG pipelines"
b1 = "----WebKitFormBoundary" + uuid.uuid4().hex[:12]
mp1 = f"--{b1}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"clean_notes.txt\"\r\nContent-Type: text/plain\r\n\r\n".encode() + f1_bytes + f"\r\n--{b1}--\r\n".encode()
f_up1 = tflow.tflow(req=http.Request.make("POST", "https://www.perplexity.ai/rest/upload", mp1, {"Host": "www.perplexity.ai", "Content-Type": f"multipart/form-data; boundary={b1}"}))
addon.request(f_up1)

# Upload File 2: CSV with Pincode violation
f2_bytes = b"id,name,pin\n101,Sakthi,600028\n102,Kumar,600001\n"
b2 = "----WebKitFormBoundary" + uuid.uuid4().hex[:12]
mp2 = f"--{b2}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"customer_database.csv\"\r\nContent-Type: text/csv\r\n\r\n".encode() + f2_bytes + f"\r\n--{b2}--\r\n".encode()
f_up2 = tflow.tflow(req=http.Request.make("POST", "https://www.perplexity.ai/rest/upload", mp2, {"Host": "www.perplexity.ai", "Content-Type": f"multipart/form-data; boundary={b2}"}))
addon.request(f_up2)

# Upload File 3: Clean Python code
f3_bytes = b"def run_pipeline():\n    print('Executing RAG pipeline')\n"
b3 = "----WebKitFormBoundary" + uuid.uuid4().hex[:12]
mp3 = f"--{b3}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"clean_script.py\"\r\nContent-Type: text/x-python\r\n\r\n".encode() + f3_bytes + f"\r\n--{b3}--\r\n".encode()
f_up3 = tflow.tflow(req=http.Request.make("POST", "https://www.perplexity.ai/rest/upload", mp3, {"Host": "www.perplexity.ai", "Content-Type": f"multipart/form-data; boundary={b3}"}))
addon.request(f_up3)

# Chat submit referencing all 3 files
f_multi_send = tflow.tflow(
    req=http.Request.make(
        "POST",
        "https://www.perplexity.ai/rest/sse/perplexity_ask",
        json.dumps({
            "query_str": "Analyze these 3 files together",
            "attachments": [
                {"name": "clean_notes.txt", "id": "att-1"},
                {"name": "customer_database.csv", "id": "att-2"},
                {"name": "clean_script.py", "id": "att-3"},
            ]
        }).encode("utf-8"),
        {
            "Host": "www.perplexity.ai",
            "Content-Type": "application/json",
            "Accept": "text/event-stream",
            "Origin": "https://www.perplexity.ai",
        }
    )
)
addon.request(f_multi_send)
multi_blocked = f_multi_send.response is not None and f_multi_send.response.status_code == 200
print(f"[{'PASS' if multi_blocked else 'FAIL'}] Perplexity Multi-file (3 files with CSV violation) | Status: {'BLOCKED' if multi_blocked else 'ALLOWED'} | Expected: BLOCKED")

print("\n" + "=" * 85)
print(f"Sections: s1={s1_ok}, s2={s2_ok}, sse={has_sse_bubble}, json={has_json_bubble}, poll={has_poll_bubble}, ws={has_ws_frames}, s4={s4_ok}, multi={multi_blocked}")
all_ok = s1_ok and s2_ok and has_sse_bubble and has_json_bubble and has_poll_bubble and has_ws_frames and s4_ok and multi_blocked
total_passed = sum([s1_ok, s2_ok, has_sse_bubble, has_json_bubble, has_poll_bubble, has_ws_frames, s4_ok, multi_blocked])
if all_ok:
    print(" PERPLEXITY LIVE SUITE COMPLETED: 100% PASSED!")
    print("=" * 85)
    sys.exit(0)
else:
    print(f" PERPLEXITY LIVE SUITE HAD FAILURES ({total_passed}/8 sections passed)")
    print("=" * 85)
    sys.exit(1)
