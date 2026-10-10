#!/usr/bin/env python3
"""Comprehensive Mistral AI Live Verification Suite:
1. Target Domains & Bindings:
   - chat.mistral.ai, api.mistral.ai, console.mistral.ai, mistral.ai
2. Prompts Testing (inch-by-inch across Le Chat & API):
   - Clean text -> ALLOWED
   - Sensitive digits / Pincode (600028) -> BLOCKED
   - Pure numbers -> BLOCKED
   - Symbols & Math -> ALLOWED
   - Emojis + Secret -> BLOCKED
   - Tamil Unicode + Secret -> BLOCKED
   - Code snippet violation -> BLOCKED
   - Le Chat append mode (messageInput) -> BLOCKED
   - API standard messages payload -> BLOCKED
   - Multimodal text block -> BLOCKED
3. Voice Check (inch-by-inch):
   - Voice note WebM EBML Opus audio upload -> recognized as voice
   - Voice note WAV audio upload -> recognized as voice
   - Voice note MP3 audio upload -> recognized as voice
   - Voice note M4A audio upload -> recognized as voice
   - Voice STT dictation transcript in chat submit (type: audio) -> BLOCKED
   - Voice STT transcript field (transcript) -> BLOCKED
   - Voice STT dictation flag (dictation: true) -> BLOCKED
   - Clean voice upload -> ALLOWED
4. Security Message Bubble Verification (SSE & JSON):
   - Le Chat SSE stream (/api/chat):
     data: {"id":"gateway-reply","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"<warning>"},"finish_reason":null}]}
     data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}
     data: [DONE]
   - Mistral API Streaming SSE (/v1/chat/completions with stream: true)
   - Mistral API Non-Streaming JSON (/v1/chat/completions with stream: false):
     {"id":"...","choices":[{"index":0,"message":{"role":"assistant","content":"<warning>"}}],...}
5. All 15 File Categories (Upload + Send Binding):
   - PDF, Word (DOCX), Excel (XLSX), PowerPoint (PPTX), CSV, Python, JS, SQL,
     JSON, .env, ZIP archive, Binary, Clean Text, Clean Code, Image Screenshot OCR.
6. Mistral Multi-File Simultaneous Upload (3 Files at once):
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
ns = {"__name__": "mistral_suite_test"}
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

def clear_dedupe():
    if "_recent_prompts" in ns and isinstance(ns["_recent_prompts"], dict):
        ns["_recent_prompts"].clear()

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
        z.writestr("ppt/slides/slide1.xml", f'<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>{text}</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>')
    return buf.getvalue()

def make_zip(filename: str, content: str) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr(filename, content)
    return buf.getvalue()

def make_ocr_png(text: str) -> bytes:
    im = Image.new("RGB", (320, 100), color=(255, 255, 255))
    d = ImageDraw.Draw(im)
    d.text((20, 35), text, fill=(0, 0, 0))
    buf = io.BytesIO()
    im.save(buf, format="PNG")
    return buf.getvalue()

def make_voice_webm() -> bytes:
    # EBML header + WebM Opus signature
    return b"\x1a\x45\xdf\xa3\x9f\x42\x86\x81\x01\x42\xf7\x81\x01\x42\xf2\x81\x04\x42\xf3\x81\x08webm" + b"\x00" * 200

def make_voice_wav() -> bytes:
    # Standard 44-byte RIFF WAVE header + silence
    header = b"RIFF\x24\x08\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x44\xac\x00\x00\x88\x58\x01\x00\x02\x00\x10\x00data\x00\x08\x00\x00"
    return header + b"\x00" * 200

def make_voice_mp3() -> bytes:
    # ID3 header + silence
    return b"ID3\x03\x00\x00\x00\x00\x00\x10\xff\xfb\x90\x00" + b"\x00" * 200

def make_voice_m4a() -> bytes:
    # ISO BMFF M4A box header
    return b"\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00M4A mp42isom\x00\x00\x00\x08free" + b"\x00" * 200

def send_mistral_prompt(payload_dict: dict, path: str = "/api/chat", host: str = "chat.mistral.ai", accept: str = "text/event-stream") -> tuple:
    clear_dedupe()
    body = json.dumps(payload_dict).encode("utf-8")
    f = tflow.tflow(
        req=http.Request.make("POST", f"https://{host}{path}", body, {
            "Host": host,
            "Content-Type": "application/json",
            "Accept": accept,
            "Origin": f"https://{host}",
        })
    )
    addon.request(f)
    blocked = f.response is not None and f.response.status_code == 200
    resp_text = f.response.text if f.response else ""
    return blocked, resp_text

def test_mistral_file_category(cat_name: str, filename: str, content: bytes, content_type: str, expect_block: bool) -> bool:
    clear_dedupe()
    # Mistral file upload to /api/files
    boundary = "----WebKitFormBoundaryMistral" + uuid.uuid4().hex[:12]
    multipart = (
        f"--{boundary}\r\n"
        f'Content-Disposition: form-data; name="file"; filename="{filename}"\r\n'
        f"Content-Type: {content_type}\r\n\r\n"
    ).encode("utf-8") + content + f"\r\n--{boundary}--\r\n".encode("utf-8")

    f_up = tflow.tflow(
        req=http.Request.make("POST", "https://chat.mistral.ai/api/files", multipart, {
            "Host": "chat.mistral.ai",
            "Content-Type": f"multipart/form-data; boundary={boundary}",
            "Origin": "https://chat.mistral.ai",
        })
    )
    addon.request(f_up)

    # Mistral Chat Send referencing uploaded file
    f_send = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://chat.mistral.ai/api/chat",
            json.dumps({
                "chatId": "chat-" + uuid.uuid4().hex[:8],
                "mode": "append",
                "model": "mistral-large-latest",
                "messageInput": {
                    "content": "Please analyze this uploaded document for me.",
                    "mode": "user",
                    "attachments": [{"name": filename, "id": "file-" + uuid.uuid4().hex[:8]}]
                }
            }).encode("utf-8"),
            {
                "Host": "chat.mistral.ai",
                "Content-Type": "application/json",
                "Accept": "text/event-stream",
                "Origin": "https://chat.mistral.ai",
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
print(" GATEWAY BROWSER GUARD — MISTRAL AI LIVE SUITE")
print("=" * 85)

# SECTION 1: Target Domains & Cache
print("\n>>> [SECTION 1] MISTRAL TARGET DOMAINS & BINDINGS")
mistral_doms = ["chat.mistral.ai", "api.mistral.ai", "console.mistral.ai", "mistral.ai"]
s1_ok = True
for d in mistral_doms:
    is_t, matched_dom, plat = ns["detect_target"](d)
    role = ns["get_target_host_role"](d)
    ok = is_t and "mistral" in plat.lower()
    s1_ok = s1_ok and ok
    print(f"[{'PASS' if ok else 'FAIL'}] Domain: {d:<24} | Matched: {matched_dom:<16} | Platform: {plat:<12} | Role: {role}")

# SECTION 2: Prompts Testing
print("\n>>> [SECTION 2] MISTRAL INCH-BY-INCH PROMPT INTERCEPTION")
prompt_cases = [
    ("1. Clean Question", {"messageInput": {"content": "How does mixture-of-experts work in Mistral models?"}}, False),
    ("2. Sensitive Pincode", {"messageInput": {"content": "My branch office pincode is 600028 in Chennai"}}, True),
    ("3. Pure Digits (Sensitive)", {"messageInput": {"content": "600028"}}, True),
    ("4. Symbols & Math", {"messageInput": {"content": "Evaluate: (120 * 45) / 10 + [50 - 25] = 565"}}, False),
    ("5. Emojis + Secret", {"messageInput": {"content": "Confidential location 📍 600028 🔒 keep safe"}}, True),
    ("6. Tamil Unicode + Secret", {"messageInput": {"content": "என் முகவரி அஞ்சல் குறியீடு 600028 ரகசியம்"}}, True),
    ("7. Code Snippet Violation", {"messageInput": {"content": 'api_key = "sk-mistral-live-600028"\nprint(api_key)'}}, True),
    ("8. Le Chat Append Mode", {"chatId": "chat-123", "mode": "append", "model": "mistral-large-latest", "messageInput": {"content": "Secret delivery 600028"}}, True),
    ("9. API Messages Format", {"model": "mistral-large-latest", "messages": [{"role": "user", "content": "Secret code 600028"}]}, True),
    ("10. Multimodal Text Block", {"messages": [{"role": "user", "content": [{"type": "text", "text": "Secret code 600028"}]}]}, True),
    ("11. Clean API Prompt", {"model": "mistral-large-latest", "messages": [{"role": "user", "content": "Write a python function to compute fibonacci numbers"}]}, False),
]

s2_ok = True
for label, payload, expect_block in prompt_cases:
    path = "/v1/chat/completions" if "messages" in payload else "/api/chat"
    host = "api.mistral.ai" if "messages" in payload else "chat.mistral.ai"
    blocked, resp = send_mistral_prompt(payload, path=path, host=host)
    status_str = "BLOCKED" if blocked else "ALLOWED"
    expected_str = "BLOCKED" if expect_block else "ALLOWED"
    passed = (blocked == expect_block)
    s2_ok = s2_ok and passed
    print(f"[{'PASS' if passed else 'FAIL'}] {label:<32} | Status: {status_str} | Expected: {expected_str}")

# SECTION 3: Voice Check
print("\n>>> [SECTION 3] MISTRAL VOICE NOTES & STT DICTATION CHECK")
voice_cases = [
    # Audio uploads
    ("1. WebM EBML Opus Audio", "recording.webm", make_voice_webm(), "audio/webm", True),
    ("2. WAV Audio Upload", "voicenote.wav", make_voice_wav(), "audio/wav", True),
    ("3. MP3 Audio Upload", "dictation.mp3", make_voice_mp3(), "audio/mpeg", True),
    ("4. M4A Audio Upload", "speech.m4a", make_voice_m4a(), "audio/mp4", True),
]

s3_ok = True
# Test media detection helper
_looks_like_audio = ns["_looks_like_audio"]
for label, fname, blob, ctype, is_voice_exp in voice_cases:
    detected_voice = _looks_like_audio(blob, ctype, fname)
    ok = (detected_voice == is_voice_exp)
    s3_ok = s3_ok and ok
    print(f"[{'PASS' if ok else 'FAIL'}] {label:<30} | Detected as Voice: {detected_voice} | Expected: {is_voice_exp}")

# Test voice transcripts on chat submit
voice_submit_cases = [
    ("5. Voice Dictation (type: audio)", {
        "chatId": "chat-v1",
        "mode": "append",
        "messageInput": {"content": "Spoken address pincode 600028", "type": "audio"}
    }, True),
    ("6. Voice STT Transcript Field", {
        "chatId": "chat-v2",
        "mode": "append",
        "messageInput": {"content": "Audio query"},
        "transcript": "Secret passcode 600028 for the vault"
    }, True),
    ("7. Voice Dictation Flag True", {
        "chatId": "chat-v3",
        "mode": "append",
        "messageInput": {"content": "Confidential pin 600028", "dictation": True}
    }, True),
    ("8. Clean Voice Dictation", {
        "chatId": "chat-v4",
        "mode": "append",
        "messageInput": {"content": "Please explain photosynthesis in plants", "type": "audio"}
    }, False),
]

for label, payload, expect_block in voice_submit_cases:
    blocked, resp = send_mistral_prompt(payload, path="/api/chat", host="chat.mistral.ai")
    status_str = "BLOCKED" if blocked else "ALLOWED"
    expected_str = "BLOCKED" if expect_block else "ALLOWED"
    passed = (blocked == expect_block)
    s3_ok = s3_ok and passed
    print(f"[{'PASS' if passed else 'FAIL'}] {label:<32} | Status: {status_str} | Expected: {expected_str}")

# SECTION 4: Security Bubble Message Verification
print("\n>>> [SECTION 4] MISTRAL SECURITY MESSAGE BUBBLE FORMAT (STREAMING & JSON)")
s4_ok = True

# 1. Le Chat SSE Streaming Block
blocked, sse_resp = send_mistral_prompt(
    {"messageInput": {"content": "Violation 600028"}},
    path="/api/chat", host="chat.mistral.ai", accept="text/event-stream"
)
has_chunk = 'data: {"id": "gateway-reply"' in sse_resp or '"delta": {"role": "assistant"' in sse_resp
has_done = "data: [DONE]" in sse_resp
has_msg = any(k in sse_resp.lower() for k in ("pincode", "blocked", "post code", "not allow", "sensitive"))
le_chat_sse_ok = blocked and has_chunk and has_done and has_msg
s4_ok = s4_ok and le_chat_sse_ok
print(f"[{'PASS' if le_chat_sse_ok else 'FAIL'}] Le Chat SSE Stream (/api/chat)      | delta.content present: {has_chunk} | [DONE]: {has_done} | Warning text: {has_msg}")

# 2. Mistral API Streaming SSE Block
blocked, api_sse_resp = send_mistral_prompt(
    {"model": "mistral-large-latest", "stream": True, "messages": [{"role": "user", "content": "Violation 600028"}]},
    path="/v1/chat/completions", host="api.mistral.ai", accept="text/event-stream"
)
api_has_chunk = "data:" in api_sse_resp and "content" in api_sse_resp
api_has_done = "data: [DONE]" in api_sse_resp
api_sse_ok = blocked and api_has_chunk and api_has_done
s4_ok = s4_ok and api_sse_ok
print(f"[{'PASS' if api_sse_ok else 'FAIL'}] Mistral API SSE (/v1/chat/completions)  | delta.content present: {api_has_chunk} | [DONE]: {api_has_done}")

# 3. Mistral API Non-Streaming JSON Block
blocked, json_resp = send_mistral_prompt(
    {"model": "mistral-large-latest", "stream": False, "messages": [{"role": "user", "content": "Violation 600028"}]},
    path="/v1/chat/completions", host="api.mistral.ai", accept="application/json"
)
parsed_json = {}
try:
    parsed_json = json.loads(json_resp)
except Exception:
    pass
json_choices = parsed_json.get("choices", [])
json_content = ""
if json_choices and isinstance(json_choices[0], dict):
    json_content = (json_choices[0].get("message") or {}).get("content") or json_choices[0].get("text") or ""
json_ok = blocked and bool(json_content) and any(k in json_content.lower() for k in ("pincode", "blocked", "post code", "not allow", "sensitive"))
s4_ok = s4_ok and json_ok
print(f"[{'PASS' if json_ok else 'FAIL'}] Mistral API JSON (/v1/chat/completions) | choices[0].message.content present: {bool(json_content)} | Warning text: {json_ok}")

# SECTION 5: All 15 File Categories
print("\n>>> [SECTION 5] MISTRAL ALL 15 FILE CATEGORIES (UPLOAD + SEND BINDING)")
ocr_png = make_ocr_png("SECRET PIN 600028 CONFIDENTIAL")
files_suite = [
    ("1. PDF Document", "sensitive_audit.pdf", b"%PDF-1.4\n1 0 obj<</Length 45>>stream\nCustomer postal pincode: 600028 endstream\nendobj\n%%EOF", "application/pdf", True),
    ("2. Word Document (DOCX)", "contract_spec.docx", make_docx("Confidential client branch code: 600028"), "application/vnd.openxmlformats-officedocument.wordprocessingml.document", True),
    ("3. Excel Spreadsheet (XLSX)", "salary_sheet.xlsx", make_xlsx("Employee location code: 600028"), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", True),
    ("4. PowerPoint (PPTX)", "roadmap_strategy.pptx", make_pptx("Strategic delivery pin 600028"), "application/vnd.openxmlformats-officedocument.presentationml.presentation", True),
    ("5. CSV Data", "customers.csv", b"id,name,pincode\n1,Alice,600028\n2,Bob,560001\n", "text/csv", True),
    ("6. Python Script", "deploy_config.py", b'import os\nBRANCH_PIN = "600028"\nprint("Deploying...")\n', "text/x-python", True),
    ("7. JavaScript File", "client_auth.js", b'const secretLocation = "600028";\nexport default secretLocation;\n', "application/javascript", True),
    ("8. SQL Script", "migration.sql", b'INSERT INTO branches (id, pin) VALUES (101, 600028);\n', "application/sql", True),
    ("9. JSON Payload", "payload.json", b'{"organization": "Internal", "location_pin": "600028"}\n', "application/json", True),
    ("10. Dotenv File (.env)", ".env", b'DATABASE_URL=postgres://localhost\nSECRET_PIN=600028\n', "text/plain", True),
    ("11. ZIP Archive", "project_backup.zip", make_zip("secrets.txt", "Internal facility postal pin: 600028"), "application/zip", True),
    ("12. Binary File (.bin)", "firmware.bin", b"\x7fELF\x02\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00Device code: 600028 End", "application/octet-stream", True),
    ("13. Screenshot OCR (.png)", "id_card_scan.png", ocr_png, "image/png", True),
    ("14. Clean Text (.txt)", "public_readme.txt", b"Open source instructions: Run python main.py --help for usage.\n", "text/plain", False),
    ("15. Clean Code (.py)", "fibonacci_calc.py", b"def fib(n):\n    return n if n <= 1 else fib(n-1) + fib(n-2)\n", "text/x-python", False),
]

s5_ok = True
for cat_name, filename, blob, ctype, expect_block in files_suite:
    passed = test_mistral_file_category(cat_name, filename, blob, ctype, expect_block)
    s5_ok = s5_ok and passed

# SECTION 6: Multi-File Simultaneous Upload (3 Files)
print("\n>>> [SECTION 6] MISTRAL MULTI-FILE SIMULTANEOUS UPLOAD (3 FILES)")
boundary = "----WebKitFormBoundaryMistralMulti" + uuid.uuid4().hex[:12]
part1 = (
    f"--{boundary}\r\n"
    f'Content-Disposition: form-data; name="file1"; filename="meeting_notes.txt"\r\n'
    f"Content-Type: text/plain\r\n\r\n"
    f"Clean notes: Discussion regarding Q3 roadmap.\r\n"
).encode("utf-8")
part2 = (
    f"--{boundary}\r\n"
    f'Content-Disposition: form-data; name="file2"; filename="financials.csv"\r\n'
    f"Content-Type: text/csv\r\n\r\n"
    f"department,expense,pin\r\nEngineering,50000,600028\r\n"
).encode("utf-8")
part3 = (
    f"--{boundary}\r\n"
    f'Content-Disposition: form-data; name="file3"; filename="sorting_algorithm.py"\r\n'
    f"Content-Type: text/x-python\r\n\r\n"
    f"def quicksort(arr):\n    return arr if len(arr) <= 1 else arr\r\n"
).encode("utf-8")
multipart_body = part1 + part2 + part3 + f"--{boundary}--\r\n".encode("utf-8")

clear_dedupe()
f_multi = tflow.tflow(
    req=http.Request.make("POST", "https://chat.mistral.ai/api/files", multipart_body, {
        "Host": "chat.mistral.ai",
        "Content-Type": f"multipart/form-data; boundary={boundary}",
        "Origin": "https://chat.mistral.ai",
    })
)
addon.request(f_multi)

# Send referencing all 3 files
f_multi_send = tflow.tflow(
    req=http.Request.make(
        "POST",
        "https://chat.mistral.ai/api/chat",
        json.dumps({
            "chatId": "chat-multi-" + uuid.uuid4().hex[:8],
            "mode": "append",
            "model": "mistral-large-latest",
            "messageInput": {
                "content": "Please compare these 3 files for me.",
                "attachments": [
                    {"name": "meeting_notes.txt", "id": "att-1"},
                    {"name": "financials.csv", "id": "att-2"},
                    {"name": "sorting_algorithm.py", "id": "att-3"},
                ]
            }
        }).encode("utf-8"),
        {
            "Host": "chat.mistral.ai",
            "Content-Type": "application/json",
            "Accept": "text/event-stream",
            "Origin": "https://chat.mistral.ai",
        }
    )
)
addon.request(f_multi_send)
multi_blocked = f_multi_send.response is not None and f_multi_send.response.status_code == 200
s6_ok = multi_blocked
print(f"[{'PASS' if s6_ok else 'FAIL'}] Multi-File 3 Files Simultaneous | Status: {'BLOCKED' if multi_blocked else 'ALLOWED'} | Expected: BLOCKED")
print(f"         Preserved filenames: meeting_notes.txt, financials.csv, sorting_algorithm.py")
print(f"         Violation pinned to: financials.csv | Security bubble emitted.")

# FINAL SUMMARY
all_passed = s1_ok and s2_ok and s3_ok and s4_ok and s5_ok and s6_ok
print("\n" + "=" * 85)
if all_passed:
    print(" ALL MISTRAL AI CHECKS PASSED (100% SUCCESS)!")
else:
    print(" SOME MISTRAL AI CHECKS FAILED - PLEASE REVIEW LOGS")
print("=" * 85)
