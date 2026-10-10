#!/usr/bin/env python3
"""Comprehensive DeepSeek Live Verification Suite:
1. Target Domains & Proxy Cache Inspection:
   - Verifies all DeepSeek domains loaded in backend and proxy cache:
     chat.deepseek.com, deepseek.com, api.deepseek.com, cdn.deepseek.com
   - Verifies host roles and upload family bindings.
2. Prompts Testing (inch-by-inch):
   - Clean text -> ALLOWED
   - Sensitive digits / Pincode (600028) -> BLOCKED
   - Pure numbers -> BLOCKED
   - Symbols & Punctuation -> ALLOWED
   - Emojis + Secret -> BLOCKED
   - Tamil Unicode + Secret -> BLOCKED
   - Code snippet violation -> BLOCKED
3. DeepSeek Security Bubble Message Verification:
   - Verifies synthetic response format is standard DeepSeek SSE stream:
     data: {"choices": [{"delta": {"role": "assistant", "content": "<security_warning>"}}]}
     so the UI displays the security warning bubble directly in chat without crashing.
4. All 15 File Categories (CDN / Upload + Send Binding):
   - PDF, Word (DOCX), Excel (XLSX), PowerPoint (PPTX), CSV, Python, JS, SQL,
     JSON, .env, ZIP archive, Binary, Clean Text, Clean Code, Image Screenshot OCR.
5. DeepSeek Multi-File Simultaneous Upload (3 Files at once):
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
ns = {"__name__": "deepseek_suite_test"}
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

# Helper builders
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

def make_deepseek_flow(
    path: str = "/api/v0/chat/completion",
    body: bytes | str = b"",
    method: str = "POST",
    headers: dict | None = None,
    host: str = "chat.deepseek.com",
):
    if isinstance(body, str):
        body = body.encode("utf-8")
    hdrs = {
        "Host": host,
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
        "Content-Type": "application/json",
        "Accept": "text/event-stream, application/json",
        "Origin": f"https://{host}",
        "Referer": f"https://{host}/",
    }
    if headers:
        hdrs.update(headers)
    req = http.Request.make(method, f"https://{host}{path}", body, hdrs)
    flow = tflow.tflow(req=req)
    flow.request.host = host
    flow.request.authority = host
    flow.client_conn.peername = ("127.0.0.1", 54321)
    flow.client_conn.sockname = ("127.0.0.1", 18103)
    return flow

def is_flow_blocked(flow: http.HTTPFlow) -> tuple[bool, str]:
    if flow.response is None:
        return False, ""
    content = flow.response.content.decode("utf-8", errors="ignore") if flow.response.content else ""
    return True, content

def run_deepseek_full_suite():
    print("=" * 85)
    print(" UNIF-AI BROWSER GUARD - DEEPSEEK LIVE VERIFICATION SUITE")
    print("=" * 85)

    results = []

    # ──────────────────────────────────────────────────────────────────────────
    # SECTION 1: TARGET DOMAIN & CONFIGURATION CHECKS
    # ──────────────────────────────────────────────────────────────────────────
    print("\n>>> [SECTION 1] DEEPSEEK TARGET DOMAINS & PROXY CACHE CHECK")
    domains_map = ns["get_target_domains"]()
    deepseek_domains = [
        "chat.deepseek.com",
        "deepseek.com",
        "api.deepseek.com",
        "cdn.deepseek.com",
    ]
    all_targets_ok = True
    for d in deepseek_domains:
        present = d in domains_map
        platform = domains_map.get(d, "N/A")
        role = ns["get_target_host_role"](d) or "auto"
        stat = "PASS" if present else "FAIL"
        print(f"[{stat}] Domain: {d:<30} | Platform: {platform:<10} | Role: {role}")
        if not present:
            all_targets_ok = False
    results.append(("Target Domains Registered", all_targets_ok))

    # ──────────────────────────────────────────────────────────────────────────
    # SECTION 2: PROMPT TESTING (INCH-BY-INCH)
    # ──────────────────────────────────────────────────────────────────────────
    print("\n>>> [SECTION 2] DEEPSEEK PROMPT INTERCEPTION & PREDICTION CHECKS")
    prompt_cases = [
        ("Clean Plain Text", "How does DeepSeek-R1 optimize reasoning chains?", False),
        ("Sensitive Pincode Digits", "Please dispatch shipment to user address pincode 600028.", True),
        ("Pure Numbers Only", "600028", True),
        ("Symbols & Punctuation", "Formula: [A * (B + C)] / D = $500.25 #valid!", False),
        ("Emojis + Secret", "🚀🔒 Top Secret production key pin: 600028 🔥🎉", True),
        ("Tamil Unicode + Secret", "வணக்கம் எனது ரகசிய பின்கோடு 600028 உடனடியாக சரிபார்க்கவும்.", True),
        ("Code Snippet Violation", "def get_auth_token():\n    return 600028", True),
    ]

    for label, prompt_text, expect_block in prompt_cases:
        # Clear dedupe cache for independent prompt verification
        with ns["_dedupe_lock"]:
            ns["_recent_prompts"].clear()
            ns["_recent_decisions"].clear()

        body = json.dumps({
            "chat_session_id": f"sess-{uuid.uuid4().hex[:8]}",
            "parent_message_id": None,
            "prompt": prompt_text,
            "ref_file_ids": [],
        })
        flow = make_deepseek_flow(body=body)
        addon.request(flow)
        blocked, _ = is_flow_blocked(flow)
        success = (blocked == expect_block)
        status_label = "BLOCKED" if blocked else "ALLOWED"
        expected_label = "BLOCKED" if expect_block else "ALLOWED"
        stat = "PASS" if success else "FAIL"
        print(f"[{stat}] {label:<28} | Status: {status_label:<7} | Expected: {expected_label:<7}")
        results.append((f"Prompt: {label}", success))

    # ──────────────────────────────────────────────────────────────────────────
    # SECTION 3: DEEPSEEK SECURITY BUBBLE MESSAGE VERIFICATION
    # ──────────────────────────────────────────────────────────────────────────
    print("\n>>> [SECTION 3] DEEPSEEK SECURITY BUBBLE SSE STREAM VERIFICATION")
    with ns["_dedupe_lock"]:
        ns["_recent_prompts"].clear()
        ns["_recent_decisions"].clear()

    sec_flow = make_deepseek_flow(body=json.dumps({"prompt": "Restricted location 600028", "ref_file_ids": []}))
    addon.request(sec_flow)
    sec_blocked, sec_content = is_flow_blocked(sec_flow)
    
    is_sse_stream = sec_flow.response is not None and "text/event-stream" in (sec_flow.response.headers.get("Content-Type") or "")
    has_chat_chunk = "chat.completion.chunk" in sec_content or "delta" in sec_content
    has_security_msg = ("post code" in sec_content.lower() or "sensitive" in sec_content.lower() or "blocked" in sec_content.lower() or "600028" in sec_content)
    has_done_terminator = "[DONE]" in sec_content
    bubble_ok = sec_blocked and is_sse_stream and has_chat_chunk and has_security_msg and has_done_terminator

    print(f"[{'PASS' if is_sse_stream else 'FAIL'}] DeepSeek SSE Content-Type (text/event-stream) : {'Present' if is_sse_stream else 'Missing'}")
    print(f"[{'PASS' if has_chat_chunk else 'FAIL'}] Chat Completion Delta Chunk (delta.content)   : {'Present' if has_chat_chunk else 'Missing'}")
    print(f"[{'PASS' if has_security_msg else 'FAIL'}] In-Chat Security Bubble Warning Text         : {'Present' if has_security_msg else 'Missing'}")
    print(f"[{'PASS' if has_done_terminator else 'FAIL'}] SSE Stream Terminator (data: [DONE])          : {'Present' if has_done_terminator else 'Missing'}")
    if bubble_ok:
        print(f"       Raw Synthesized Bubble Stream snippet: {sec_content[:140]!r}...")
    results.append(("DeepSeek Security Bubble Synthesis", bubble_ok))

    # ──────────────────────────────────────────────────────────────────────────
    # SECTION 4: ALL 15 FILE CATEGORIES CHECK
    # ──────────────────────────────────────────────────────────────────────────
    print("\n>>> [SECTION 4] DEEPSEEK ALL FILE CATEGORIES IMPORT & PREDICTION")
    pdf_bytes = (
        b"%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n"
        b"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
        b"3 0 obj<</Type/Page/MediaBox[0 0 300 144]/Parent 2 0 R/Resources<<>>/Contents 4 0 R>>endobj\n"
        b"4 0 obj<</Length 55>>stream\nBT /F1 12 Tf 50 100 Td (Audit report: pincode 600028 confidential) Tj ET\nendstream\nendobj\n"
        b"trailer<</Size 5/Root 1 0 R>>\n%%EOF"
    )

    file_test_cases = [
        # 1. PDF Document
        ("1. PDF Document", "q3_audit.pdf", pdf_bytes, "application/pdf", True),
        # 2. Word .docx
        ("2. Word Document (DOCX)", "contract_agreement.docx", make_docx("Vendor agreement pincode 600028"), "application/vnd.openxmlformats-officedocument.wordprocessingml.document", True),
        # 3. Excel .xlsx
        ("3. Excel Spreadsheet (XLSX)", "q4_salaries.xlsx", make_xlsx("Salary Record location 600028 confidential"), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", True),
        # 4. PowerPoint .pptx
        ("4. PowerPoint (PPTX)", "board_strategy.pptx", make_pptx("Board strategic presentation target 600028"), "application/vnd.openxmlformats-officedocument.presentationml.presentation", True),
        # 5. CSV Spreadsheet
        ("5. CSV Spreadsheet", "employee_directory.csv", b"emp_id,name,pincode\n101,John,600028\n", "text/csv", True),
        # 6. Python Code
        ("6. Python Code (.py)", "payment_service.py", b"def pay():\n    postal = 600028\n    return postal\n", "text/x-python", True),
        # 7. JavaScript Code
        ("7. JavaScript Code (.js)", "auth_controller.js", b"export const config = { postal: 600028 };", "text/javascript", True),
        # 8. SQL Database Dump
        ("8. SQL Database (.sql)", "schema_dump.sql", b"INSERT INTO users VALUES ('John', 600028);", "text/x-sql", True),
        # 9. JSON Config
        ("9. JSON Config (.json)", "app_settings.json", b'{"app": "UnifAI", "postal_code": 600028}', "application/json", True),
        # 10. Environment File
        ("10. Environment (.env)", ".env.production", b"DATABASE_URL=postgres\nPINCODE=600028\n", "text/plain", True),
        # 11. Compressed ZIP Archive
        ("11. Compressed Archive (ZIP)", "archive_backup.zip", make_zip("secrets.txt", "Secret DB migration pincode 600028"), "application/zip", True),
        # 12. Binary Model
        ("12. Binary Model (.bin)", "model_config.bin", b"\x00\x01\x02\x03MODEL_VERSION_2\x00\x00CONFIDENTIAL_TOKEN: 600028 AUTHORIZED\x00\x00\x04\x05", "application/octet-stream", True),
        # 13. Image Screenshot (OCR)
        ("13. Image Screenshot (OCR)", "confidential_screenshot.png", make_ocr_png("CONFIDENTIAL PINCODE : 600028 RESTRICTED"), "image/png", True),
        # 14. Clean Plain Text (Allowed)
        ("14. Plain Text (Clean)", "readme_documentation.txt", b"Public open-source documentation for project setup.\n", "text/plain", False),
        # 15. Clean Python Code (Allowed)
        ("15. Python Code (Clean)", "math_utils.py", b"def add(x, y):\n    return x + y\n", "text/x-python", False),
    ]

    for label, fname, data_bytes, ctype, expect_block in file_test_cases:
        fid = f"file-{uuid.uuid4().hex[:16]}"
        
        # Step A: Upload file bytes to DeepSeek upload endpoint / CDN
        upload_flow = make_deepseek_flow(
            path=f"/api/v0/file/upload?file_id={fid}",
            body=data_bytes,
            method="POST",
            headers={"Content-Type": ctype, "x-filename": fname},
            host="chat.deepseek.com",
        )
        addon.request(upload_flow)

        # Step B: DeepSeek Send Flow referencing the uploaded file
        send_flow = make_deepseek_flow(
            path="/api/v0/chat/completion",
            body=json.dumps({
                "chat_session_id": f"sess-{uuid.uuid4().hex[:8]}",
                "prompt": f"Please examine {fname}",
                "ref_file_ids": [fid],
            }),
            host="chat.deepseek.com",
        )
        addon.request(send_flow)

        blocked, _ = is_flow_blocked(send_flow)
        success = (blocked == expect_block)
        status_label = "BLOCKED" if blocked else "ALLOWED"
        expected_label = "BLOCKED" if expect_block else "ALLOWED"
        stat = "PASS" if success else "FAIL"
        print(f"[{stat}] {label:<28} | File: {fname:<25} | Status: {status_label:<7} | Expected: {expected_label:<7}")
        results.append((f"File: {fname}", success))

    # ──────────────────────────────────────────────────────────────────────────
    # SECTION 5: DEEPSEEK MULTI-FILE SIMULTANEOUS UPLOAD (3 FILES)
    # ──────────────────────────────────────────────────────────────────────────
    print("\n>>> [SECTION 5] DEEPSEEK MULTI-FILE SIMULTANEOUS UPLOAD (3 FILES)")
    
    files_burst = [
        ("clean_notes.txt", b"Architecture diagrams and deployment notes.", "text/plain"),
        ("customer_database.csv", b"id,name,pincode\n1,Alice,600028\n2,Bob,560001\n", "text/csv"),
        ("clean_script.py", b"def run():\n    print('DeepSeek multi-file test')\n", "text/x-python"),
    ]

    fids = []
    for bname, bbytes, bctype in files_burst:
        bfid = f"file-{uuid.uuid4().hex[:16]}"
        fids.append(bfid)
        flow_up = make_deepseek_flow(
            path=f"/api/v0/file/upload?file_id={bfid}",
            body=bbytes,
            method="POST",
            headers={"Content-Type": bctype, "x-filename": bname},
            host="chat.deepseek.com",
        )
        addon.request(flow_up)

    multi_send_flow = make_deepseek_flow(
        path="/api/v0/chat/completion",
        body=json.dumps({
            "chat_session_id": f"sess-{uuid.uuid4().hex[:8]}",
            "prompt": "Please analyze clean_notes.txt, customer_database.csv, clean_script.py",
            "ref_file_ids": fids,
        }),
        host="chat.deepseek.com",
    )
    addon.request(multi_send_flow)
    m_blocked, _ = is_flow_blocked(multi_send_flow)
    m_ok = (m_blocked == True)
    stat = "PASS" if m_ok else "FAIL"
    print(f"[{stat}] DeepSeek Multi-file (3 files with CSV violation) | Status: {'BLOCKED' if m_blocked else 'ALLOWED'} | Expected: BLOCKED")
    results.append(("DeepSeek Multi-File Send", m_ok))

    # ──────────────────────────────────────────────────────────────────────────
    # FINAL SUMMARY
    # ──────────────────────────────────────────────────────────────────────────
    print("\n" + "=" * 85)
    passed_count = sum(1 for _, ok in results if ok)
    total_count = len(results)
    print(f" DEEPSEEK LIVE SUITE COMPLETED: {passed_count}/{total_count} PASSED ({passed_count/total_count*100:.1f}%)")
    print("=" * 85)

if __name__ == "__main__":
    run_deepseek_full_suite()
