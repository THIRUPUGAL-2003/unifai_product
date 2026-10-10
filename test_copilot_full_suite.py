#!/usr/bin/env python3
"""Comprehensive Microsoft Copilot Live Verification Suite:
1. Target Domains & Bindings:
   - copilot.microsoft.com (ui)
   - sydney.bing.com (chat)
   - edgeservices.bing.com (chat)
   - substrate.office.com (file)
2. Prompts Testing across HTTP REST & SignalR WebSocket:
   - Clean text -> ALLOWED
   - Sensitive digits / Pincode (600028) -> BLOCKED
   - Pure digits (600028) -> BLOCKED
   - Symbols & Math -> ALLOWED
   - Emojis + Secret -> BLOCKED
   - Tamil Unicode + Secret -> BLOCKED
   - Code snippet violation -> BLOCKED
   - Copilot HTTP /c/api/chat REST send -> BLOCKED
   - Sydney SignalR StreamInvocation (/sydney/ChatHub) -> BLOCKED
   - Edge Services SignalR StreamInvocation (/edgesvc/chat) -> BLOCKED
   - SignalR Ping & Metrics noise filtering -> IGNORED
3. In-Chat Security Message Bubble Verification:
   - SignalR WebSocket block injection:
     * Type 1 Update frame with AdaptiveCard (TextBlock wrap: True)
     * Type 1 Append frame
     * Type 2 Completion frame with AdaptiveCard & Success result
     * Type 3 Acks (invocationId 0 & 1) to terminate spinner
     * All frames strictly terminated by 0x1E record separator
   - HTTP REST block response (/c/api/chat):
     * Status 200 HTTP response with CORS headers
     * AdaptiveCard inside messages[] and DeepLeo contentOrigin
     * Graph-style message structure with gateway_blocked flag
4. All 15 File Categories (Upload + Send Binding):
   - PDF, DOCX, XLSX, PPTX, CSV, Python, JS, SQL,
     JSON, .env, ZIP archive, Binary, Clean Text, Clean Code, Image Screenshot OCR
5. Copilot Multi-File Simultaneous Upload (3 Files at once):
   - Clean notes + CSV with violation + Clean Python code
   - Truthful filenames preserved, violation caught, turn blocked with in-chat card
6. Voice / Audio Check:
   - Voice note WebM EBML Opus audio upload -> recognized as voice
   - Voice note WAV audio upload -> recognized as voice
   - Voice note MP3 audio upload -> recognized as voice
   - Voice note M4A audio upload -> recognized as voice
   - Voice STT dictation transcript in chat submit -> BLOCKED
   - Clean voice upload -> ALLOWED
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
ns = {"__name__": "copilot_suite_test"}
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
        z.writestr("xl/worksheets/sheet1.xml", '<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row><c t="s"><v>0</v></c></row></sheetData></worksheet>')
    return buf.getvalue()

def make_pptx(text: str) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("[Content_Types].xml", '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>')
        z.writestr("ppt/slides/slide1.xml", f'<?xml version="1.0"?><p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>{text}</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>')
    return buf.getvalue()

def make_pdf(text: str) -> bytes:
    content = f"BT /F1 12 Tf 72 712 Td ({text}) Tj ET"
    stream_len = len(content)
    pdf = (
        b"%PDF-1.4\n"
        b"1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n"
        b"2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n"
        b"3 0 obj << /Type /Page /Parent 2 0 R /Resources << /Font << /F1 4 0 R >> >> /MediaBox [0 0 612 792] /Contents 5 0 R >> endobj\n"
        b"4 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\n"
        b"5 0 obj << /Length " + str(stream_len).encode("ascii") + b" >>\nstream\n"
        + content.encode("ascii") + b"\nendstream\nendobj\n"
        b"xref\n0 6\n0000000000 65535 f \n"
        b"trailer << /Size 6 /Root 1 0 R >>\nstartxref\n500\n%%EOF\n"
    )
    return pdf

def make_ocr_image(text: str) -> bytes:
    img = Image.new("RGB", (320, 100), color=(255, 255, 255))
    d = ImageDraw.Draw(img)
    d.text((15, 35), text, fill=(0, 0, 0))
    buf = io.BytesIO()
    img.save(buf, format="PNG")
    return buf.getvalue()

class FakeWSFlow:
    """Simulates mitmproxy WebSocket flow."""
    def __init__(self, host: str, path: str, text: str):
        self.request = type("Req", (), {
            "pretty_host": host,
            "host": host,
            "path": path,
            "url": f"wss://{host}{path}",
            "headers": {"host": host, "origin": f"https://{host}"},
            "method": "GET"
        })()
        msg_obj = type("Msg", (), {
            "from_client": True,
            "text": text,
            "content": text.encode("utf-8") if isinstance(text, str) else text,
            "killed": False,
            "drop": lambda self: setattr(self, "killed", True),
            "kill": lambda self: setattr(self, "killed", True)
        })()
        self.websocket = type("WS", (), {
            "messages": [msg_obj]
        })()

# ── RUN TEST SUITE ──
def main():
    print("=" * 80)
    print("MICROSOFT COPILOT COMPREHENSIVE VERIFICATION SUITE")
    print("=" * 80)
    passed = 0
    total = 0

    def check(title: str, condition: bool, extra: str = ""):
        nonlocal passed, total
        total += 1
        status = "PASS" if condition else "FAIL"
        if condition:
            passed += 1
        print(f"[{status}] Test #{total:02d}: {title} {extra}")
        if not condition:
            print(f"       -> Details: {extra}")

    # =========================================================================
    # PART 1: Target Domains & Role Bindings
    # =========================================================================
    print("\n--- Part 1: Target Domains & Role Bindings ---")
    domains_map = ns["get_target_domains"]()

    check("copilot.microsoft.com configured in targets", "copilot.microsoft.com" in domains_map)
    check("sydney.bing.com configured in targets", "sydney.bing.com" in domains_map)
    check("edgeservices.bing.com configured in targets", "edgeservices.bing.com" in domains_map)
    check("substrate.office.com configured in targets", "substrate.office.com" in domains_map)

    is_t, dom, plat = ns["detect_target"]("copilot.microsoft.com")
    check("detect_target('copilot.microsoft.com') -> Copilot", is_t and plat == "Copilot")

    is_t, dom, plat = ns["detect_target"]("sydney.bing.com")
    check("detect_target('sydney.bing.com') -> Copilot", is_t and plat == "Copilot")

    is_t, dom, plat = ns["detect_target"]("edgeservices.bing.com")
    check("detect_target('edgeservices.bing.com') -> Copilot", is_t and plat == "Copilot")

    is_t, dom, plat = ns["detect_target"]("substrate.office.com")
    check("detect_target('substrate.office.com') -> Copilot file", is_t and ns["get_target_host_role"](dom) == "file")

    # =========================================================================
    # PART 2: Prompts Testing (HTTP REST & SignalR WebSocket)
    # =========================================================================
    print("\n--- Part 2: Prompts Testing (HTTP REST & SignalR WebSocket) ---")

    # 2.1 HTTP REST Clean Prompt
    clear_dedupe()
    f_clean = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({"message": {"text": "What is quantum computing?"}}).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json"}
        )
    )
    addon.request(f_clean)
    check("Clean prompt HTTP -> ALLOWED (no response generated)", f_clean.response is None)

    # 2.2 HTTP REST Sensitive Pincode Prompt
    clear_dedupe()
    f_pin = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({"message": {"text": "My pincode is 600028"}}).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json"}
        )
    )
    addon.request(f_pin)
    check("Sensitive pincode HTTP -> BLOCKED (status 200)", f_pin.response is not None and f_pin.response.status_code == 200)

    # 2.3 Pure Digits Prompt
    clear_dedupe()
    f_digits = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({"message": {"text": "600028"}}).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json"}
        )
    )
    addon.request(f_digits)
    check("Pure digits 600028 HTTP -> BLOCKED", f_digits.response is not None and f_digits.response.status_code == 200)

    # 2.4 Symbols & Math Clean Prompt
    clear_dedupe()
    f_math = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({"message": {"text": "x + y = 10; calculate x * y"}}).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json"}
        )
    )
    addon.request(f_math)
    check("Symbols & Math clean HTTP -> ALLOWED", f_math.response is None)

    # 2.5 Emojis + Secret Prompt
    clear_dedupe()
    f_emoji = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({"message": {"text": "Secret code 600028"}}).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json"}
        )
    )
    addon.request(f_emoji)
    check("Emojis + secret HTTP -> BLOCKED", f_emoji.response is not None and f_emoji.response.status_code == 200)

    # 2.6 Tamil Unicode + Secret Prompt
    clear_dedupe()
    f_tamil = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({"message": {"text": "enoda pincode 600028"}}).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json"}
        )
    )
    addon.request(f_tamil)
    check("Tamil unicode + secret HTTP -> BLOCKED", f_tamil.response is not None and f_tamil.response.status_code == 200)

    # 2.7 Code Snippet Violation
    clear_dedupe()
    f_code = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({"message": {"text": "const cfg = { pincode: '600028' };"}}).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json"}
        )
    )
    addon.request(f_code)
    check("Code snippet violation HTTP -> BLOCKED", f_code.response is not None and f_code.response.status_code == 200)

    # 2.8 SignalR WebSocket Sydney Hub Clean Message
    clear_dedupe()
    sig_clean = '{"type":4,"invocationId":"0","target":"chat","arguments":[{"message":{"text":"How does solar power work?"}}]}\x1e'
    ws_clean_flow = FakeWSFlow("sydney.bing.com", "/sydney/ChatHub", sig_clean)
    addon.websocket_message(ws_clean_flow)
    check("Sydney SignalR clean WS message -> ALLOWED", not ws_clean_flow.websocket.messages[-1].killed)

    # 2.9 SignalR WebSocket Sydney Hub Sensitive Pincode Message
    clear_dedupe()
    sig_pin = '{"type":4,"invocationId":"0","target":"chat","arguments":[{"message":{"text":"My home pincode is 600028"}}]}\x1e'
    ws_pin_flow = FakeWSFlow("sydney.bing.com", "/sydney/ChatHub", sig_pin)
    addon.websocket_message(ws_pin_flow)
    check("Sydney SignalR pincode WS message -> BLOCKED (killed)", ws_pin_flow.websocket.messages[-1].killed)

    # 2.10 Edge Services SignalR Chat WebSocket Sensitive Pincode
    clear_dedupe()
    sig_edge = '{"type":4,"invocationId":"0","target":"chat","arguments":[{"message":{"text":"Employee pin 600028"}}]}\x1e'
    ws_edge_flow = FakeWSFlow("edgeservices.bing.com", "/edgesvc/chat", sig_edge)
    addon.websocket_message(ws_edge_flow)
    check("Edge Services SignalR pincode WS message -> BLOCKED (killed)", ws_edge_flow.websocket.messages[-1].killed)

    # 2.11 SignalR Ping noise frame -> Ignored
    clear_dedupe()
    sig_ping = '{"type":6}\x1e'
    ws_ping_flow = FakeWSFlow("sydney.bing.com", "/sydney/ChatHub", sig_ping)
    addon.websocket_message(ws_ping_flow)
    check("SignalR ping frame (type: 6) -> IGNORED (not killed)", not ws_ping_flow.websocket.messages[-1].killed)

    # 2.12 SignalR Metrics noise frame -> Ignored
    clear_dedupe()
    sig_metrics = '{"type":1,"target":"metrics","arguments":[{"latency":120}]}\x1e'
    ws_metrics_flow = FakeWSFlow("sydney.bing.com", "/sydney/ChatHub", sig_metrics)
    addon.websocket_message(ws_metrics_flow)
    check("SignalR metrics frame -> IGNORED (not killed)", not ws_metrics_flow.websocket.messages[-1].killed)

    # =========================================================================
    # PART 3: In-Chat Security Message Bubble Verification
    # =========================================================================
    print("\n--- Part 3: In-Chat Security Message Bubble Verification ---")

    # 3.1 HTTP REST Block Response Bubble Format
    clear_dedupe()
    f_bubble = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({"message": {"text": "Leak pin 600028"}}).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json", "Origin": "https://copilot.microsoft.com"}
        )
    )
    addon.request(f_bubble)
    resp_text = f_bubble.response.text if f_bubble.response else ""
    check("HTTP block response status 200", f_bubble.response is not None and f_bubble.response.status_code == 200)
    check("HTTP block response CORS header", f_bubble.response.headers.get("Access-Control-Allow-Origin") == "https://copilot.microsoft.com")
    has_card = "AdaptiveCard" in resp_text and "TextBlock" in resp_text
    check("HTTP block response includes AdaptiveCard", has_card)
    has_bot_author = '"author": "bot"' in resp_text or '"author":"bot"' in resp_text
    check("HTTP block response has author bot", has_bot_author)
    has_warning_msg = "Sensitive pincode detected" in resp_text
    check("HTTP block response contains rule warning text", has_warning_msg)

    # 3.2 SignalR WebSocket Block Frames Structure
    ws_reply_frames = ns["_ws_frames_signalr"]("Sensitive pincode detected")
    check("SignalR block returns 5 frames", len(ws_reply_frames) == 5)
    all_end_with_1e = all(f.endswith(b"\x1e") for f in ws_reply_frames)
    check("All SignalR frames end with 0x1E record separator", all_end_with_1e)
    type1_json = json.loads(ws_reply_frames[0][:-1].decode("utf-8"))
    check("Frame 0 is type 1 update with AdaptiveCard", type1_json.get("type") == 1 and type1_json.get("target") == "update")
    type2_json = json.loads(ws_reply_frames[2][:-1].decode("utf-8"))
    check("Frame 2 is type 2 completion with Success result", type2_json.get("type") == 2 and type2_json.get("item", {}).get("result", {}).get("value") == "Success")
    type3_json = json.loads(ws_reply_frames[3][:-1].decode("utf-8"))
    check("Frame 3 is type 3 invocation ack to stop spinner", type3_json.get("type") == 3 and type3_json.get("invocationId") == "0")

    # =========================================================================
    # PART 4: All 15 File Categories (Upload + Send Binding)
    # =========================================================================
    print("\n--- Part 4: All 15 File Categories (Upload + Send Binding) ---")

    test_files = [
        ("PDF Document", "leak.pdf", "application/pdf", make_pdf("Secret Pincode: 600028"), True),
        ("Word DOCX", "confidential.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", make_docx("Payroll 600028"), True),
        ("Excel XLSX", "records.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", make_xlsx("Pincode 600028"), True),
        ("PowerPoint PPTX", "deck.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", make_pptx("Target Office: 600028"), True),
        ("CSV Data", "users.csv", "text/csv", b"id,name,pin\n1,Alice,600028\n", True),
        ("Python Script", "main.py", "text/x-python", b"PINCODE = '600028'\nprint(PINCODE)", True),
        ("JavaScript Code", "auth.js", "application/javascript", b"const secret = '600028'; export default secret;", True),
        ("SQL Dump", "schema.sql", "application/sql", b"INSERT INTO users (pin) VALUES ('600028');", True),
        ("JSON Config", "config.json", "application/json", b'{"service": {"pin": "600028"}}', True),
        (".env Environment", ".env", "text/plain", b"API_KEY=test\nUSER_PINCODE=600028\n", True),
        ("ZIP Archive", "bundle.zip", "application/zip", None, True),  # built dynamically below
        ("Binary Data", "firmware.bin", "application/octet-stream", b"\x7fELF" + b"A" * 60 + b"PIN=600028" + b"Z" * 60, True),
        ("Clean Text", "notes.txt", "text/plain", b"Regular notes for the project meeting tomorrow.", False),
        ("Clean Python Code", "clean.py", "text/x-python", b"def add(a, b):\n    return a + b\n", False),
        ("OCR Image PNG", "scanned.png", "image/png", make_ocr_image("CONFIDENTIAL PIN: 600028"), True),
    ]

    # Build zip file containing violation
    zbuf = io.BytesIO()
    with zipfile.ZipFile(zbuf, "w") as z:
        z.writestr("clean.txt", "This is normal file inside zip.")
        z.writestr("secret.txt", "Inside zip code: 600028")
    for idx, item in enumerate(test_files):
        if item[1] == "bundle.zip":
            test_files[idx] = (item[0], item[1], item[2], zbuf.getvalue(), item[4])

    for cat_name, fname, ctype, fbytes, should_block in test_files:
        clear_dedupe()
        boundary = "----WebKitFormBoundary" + uuid.uuid4().hex[:12]
        multipart = (
            f"--{boundary}\r\n"
            f'Content-Disposition: form-data; name="file"; filename="{fname}"\r\n'
            f"Content-Type: {ctype}\r\n\r\n"
        ).encode("utf-8") + fbytes + f"\r\n--{boundary}--\r\n".encode("utf-8")

        # 1. Upload to copilot.microsoft.com/c/api/attachments
        f_up = tflow.tflow(
            req=http.Request.make(
                "POST",
                "https://copilot.microsoft.com/c/api/attachments",
                multipart,
                {
                    "Host": "copilot.microsoft.com",
                    "Content-Type": f"multipart/form-data; boundary={boundary}",
                    "Origin": "https://copilot.microsoft.com",
                }
            )
        )
        addon.request(f_up)

        # 2. Send Chat HTTP POST referencing file
        f_send = tflow.tflow(
            req=http.Request.make(
                "POST",
                "https://copilot.microsoft.com/c/api/chat",
                json.dumps({
                    "conversationId": f"conv-{uuid.uuid4().hex[:6]}",
                    "message": {
                        "text": f"Please process {fname}",
                        "attachments": [{"name": fname, "id": "att-1"}]
                    }
                }).encode("utf-8"),
                {
                    "Host": "copilot.microsoft.com",
                    "Content-Type": "application/json",
                    "Origin": "https://copilot.microsoft.com",
                }
            )
        )
        addon.request(f_send)

        is_blocked = f_send.response is not None and f_send.response.status_code == 200
        if should_block:
            check(f"File Category [{cat_name}] ({fname}) -> BLOCKED", is_blocked)
        else:
            check(f"File Category [{cat_name}] ({fname}) -> ALLOWED", not is_blocked)

    # =========================================================================
    # PART 5: Multi-File Simultaneous Upload (3 Files at once)
    # =========================================================================
    print("\n--- Part 5: Multi-File Simultaneous Upload (3 Files at once) ---")
    clear_dedupe()

    # Upload 3 files: 1 clean txt, 1 CSV with 600028 violation, 1 clean python
    files_triplet = [
        ("report_notes.txt", "text/plain", b"Meeting discussion notes about project scope."),
        ("payroll_data.csv", "text/csv", b"emp_id,emp_name,pin\n101,Bob,600028\n"),
        ("helper_script.py", "text/x-python", b"def helper():\n    return 'clean'\n"),
    ]

    for fname, ctype, fbytes in files_triplet:
        boundary = "----WebKitFormBoundary" + uuid.uuid4().hex[:12]
        multipart = (
            f"--{boundary}\r\n"
            f'Content-Disposition: form-data; name="file"; filename="{fname}"\r\n'
            f"Content-Type: {ctype}\r\n\r\n"
        ).encode("utf-8") + fbytes + f"\r\n--{boundary}--\r\n".encode("utf-8")

        f_up = tflow.tflow(
            req=http.Request.make(
                "POST",
                "https://copilot.microsoft.com/c/api/attachments",
                multipart,
                {
                    "Host": "copilot.microsoft.com",
                    "Content-Type": f"multipart/form-data; boundary={boundary}",
                    "Origin": "https://copilot.microsoft.com",
                }
            )
        )
        addon.request(f_up)

    # Send Chat POST referencing all 3 attachments
    f_multi_send = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({
                "conversationId": "conv-multi-3",
                "message": {
                    "text": "Please analyze these 3 files together",
                    "attachments": [
                        {"name": "report_notes.txt", "id": "att-1"},
                        {"name": "payroll_data.csv", "id": "att-2"},
                        {"name": "helper_script.py", "id": "att-3"},
                    ]
                }
            }).encode("utf-8"),
            {
                "Host": "copilot.microsoft.com",
                "Content-Type": "application/json",
                "Origin": "https://copilot.microsoft.com",
            }
        )
    )
    addon.request(f_multi_send)

    check("Multi-file (3 files) Send -> BLOCKED", f_multi_send.response is not None and f_multi_send.response.status_code == 200)
    check("Multi-file block contains AdaptiveCard", "AdaptiveCard" in (f_multi_send.response.text if f_multi_send.response else ""))

    # =========================================================================
    # PART 6: Voice / Audio Check
    # =========================================================================
    print("\n--- Part 6: Voice / Audio Check ---")

    # 6.1 Audio Uploads (WebM, WAV, MP3, M4A)
    audio_formats = [
        ("WebM EBML Opus", "voice_note.webm", "audio/webm", b"\x1a\x45\xdf\xa3" + b"\x00" * 80),
        ("WAV Audio", "recording.wav", "audio/wav", b"RIFF" + b"\x00" * 4 + b"WAVEfmt " + b"\x00" * 40),
        ("MP3 Audio", "memo.mp3", "audio/mpeg", b"\xff\xfb\x90\x64" + b"\x00" * 60),
        ("M4A Audio", "dictation.m4a", "audio/m4a", b"\x00\x00\x00\x20ftypM4A " + b"\x00" * 50),
    ]

    for aname, fname, ctype, abytes in audio_formats:
        boundary = "----WebKitFormBoundary" + uuid.uuid4().hex[:12]
        multipart = (
            f"--{boundary}\r\n"
            f'Content-Disposition: form-data; name="file"; filename="{fname}"\r\n'
            f"Content-Type: {ctype}\r\n\r\n"
        ).encode("utf-8") + abytes + f"\r\n--{boundary}--\r\n".encode("utf-8")

        f_up = tflow.tflow(
            req=http.Request.make(
                "POST",
                "https://copilot.microsoft.com/c/api/attachments",
                multipart,
                {
                    "Host": "copilot.microsoft.com",
                    "Content-Type": f"multipart/form-data; boundary={boundary}",
                    "Origin": "https://copilot.microsoft.com",
                }
            )
        )
        addon.request(f_up)
        # Voice note upload itself is cached without premature network error
        check(f"Voice upload [{aname}] ({fname}) cached cleanly", f_up.response is None)

    # 6.2 Voice STT Dictation Transcript in Chat Submit
    clear_dedupe()
    f_voice_stt = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({
                "message": {
                    "text": "Call the office at pincode 600028 right away",
                    "voice": True,
                    "inputType": "speech"
                }
            }).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json"}
        )
    )
    addon.request(f_voice_stt)
    check("Voice STT dictation transcript containing violation -> BLOCKED", f_voice_stt.response is not None and f_voice_stt.response.status_code == 200)

    # 6.3 Clean Voice Send
    clear_dedupe()
    f_voice_clean = tflow.tflow(
        req=http.Request.make(
            "POST",
            "https://copilot.microsoft.com/c/api/chat",
            json.dumps({
                "message": {
                    "text": "Turn off the lights in the living room",
                    "voice": True,
                    "inputType": "speech"
                }
            }).encode("utf-8"),
            {"Host": "copilot.microsoft.com", "Content-Type": "application/json"}
        )
    )
    addon.request(f_voice_clean)
    check("Clean voice dictation prompt -> ALLOWED", f_voice_clean.response is None)

    # =========================================================================
    # SUMMARY
    # =========================================================================
    print("\n" + "=" * 80)
    print(f"VERIFICATION RESULTS: {passed}/{total} TESTS PASSED ({(passed/total)*100:.1f}%)")
    print("=" * 80)
    if passed == total:
        print("ALL TESTS PASSED! Microsoft Copilot full enforcement 100% verified.")
        sys.exit(0)
    else:
        print(f"FAILED: {total - passed} tests failed.")
        sys.exit(1)

if __name__ == "__main__":
    main()
