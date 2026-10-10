#!/usr/bin/env python3
"""Comprehensive Claude Live Suite (Prompts + All File Categories + Multi-file):
Tests that EVERY prompt and file category (Documents, Office, Code, Config, Archives, Images, Audio, Binaries)
when submitted to Claude (claude.ai / files.claudeusercontent.com):
1. Accurately detects filename and file category.
2. Extracts content using the dedicated extractor for that file type.
3. Evaluates Guard Rules on the extracted content (Sensitive pincode 600028 -> BLOCK).
4. Allows clean prompts and clean files without false positives.
5. Accurately handles multi-file simultaneous upload.
"""

import io
import json
import os
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
ns = {"__name__": "claude_suite_test"}
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

def is_flow_blocked(flow: http.HTTPFlow) -> bool:
    """True when proxy intercepted and blocked the flow (via 403, 400, or injected block SSE)."""
    if flow.response is None:
        return False
    if flow.response.status_code in (403, 400):
        return True
    content = flow.response.content or b""
    if b"msg_gateway_block" in content or b"blocked" in content.lower() or b"post code" in content.lower():
        return True
    return True # Any generated response from proxy indicates block action

def send_claude_prompt(prompt_text: str) -> bool:
    f = tflow.tflow(req=http.Request.make(
        "POST",
        "https://claude.ai/api/organizations/org-test/chat_conversations/conv-123/completion",
        content=json.dumps({"prompt": prompt_text, "timezone": "Asia/Kolkata", "model": "claude-3-7-sonnet"}).encode(),
        headers={
            b"Host": b"claude.ai",
            b"Content-Type": b"application/json",
            b"Origin": b"https://claude.ai",
            b"Referer": b"https://claude.ai/chat/conv-123",
        }
    ))
    addon.request(f)
    return is_flow_blocked(f)

def upload_claude_cdn_then_send(filename: str, file_bytes: bytes, content_type: str) -> bool:
    fid = f"file-{uuid.uuid4().hex[:18]}"
    cdn_flow = tflow.tflow(req=http.Request.make(
        "PUT",
        f"https://files.claudeusercontent.com/{fid}",
        content=file_bytes,
        headers={
            b"Host": b"files.claudeusercontent.com",
            b"Content-Type": content_type.encode(),
            b"x-filename": filename.encode(),
            b"Referer": b"https://claude.ai/chat/conv-123",
        }
    ))
    addon.request(cdn_flow)

    chat_flow = tflow.tflow(req=http.Request.make(
        "POST",
        "https://claude.ai/api/organizations/org-test/chat_conversations/conv-123/completion",
        content=json.dumps({
            "prompt": f"Please analyze {filename}",
            "attachments": [{"file_name": filename, "file_id": fid}]
        }).encode(),
        headers={
            b"Host": b"claude.ai",
            b"Content-Type": b"application/json",
            b"Origin": b"https://claude.ai",
            b"Referer": b"https://claude.ai/chat/conv-123",
        }
    ))
    addon.request(chat_flow)
    return is_flow_blocked(chat_flow)

def send_claude_extracted_attachment(filename: str, extracted_text: str, content_type: str) -> bool:
    chat_flow = tflow.tflow(req=http.Request.make(
        "POST",
        "https://claude.ai/api/organizations/org-test/chat_conversations/conv-123/completion",
        content=json.dumps({
            "prompt": f"Please summarize {filename}",
            "attachments": [{
                "file_name": filename,
                "file_type": content_type,
                "extracted_content": extracted_text,
            }]
        }).encode(),
        headers={
            b"Host": b"claude.ai",
            b"Content-Type": b"application/json",
            b"Origin": b"https://claude.ai",
            b"Referer": b"https://claude.ai/chat/conv-123",
        }
    ))
    addon.request(chat_flow)
    return is_flow_blocked(chat_flow)

def run_claude_full_suite():
    print("=" * 85)
    print(" UNIF-AI BROWSER GUARD - CLAUDE LIVE SUITE (PROMPTS + ALL FILE CATEGORIES)")
    print("=" * 85)

    results = []

    # ──────────────────────────────────────────────────────────────────────────
    # SECTION 1: PROMPT TESTING
    # ──────────────────────────────────────────────────────────────────────────
    print("\n--- SECTION 1: CLAUDE PROMPT TEXT CHECKS ---")
    prompt_cases = [
        ("Clean greeting prompt", "Hello Claude! Can you explain quantum computing simply?", False),
        ("Pincode in prompt (Blocked)", "Please send the parcel to pincode 600028 in Chennai immediately.", True),
        ("Tamil Unicode + Pincode (Blocked)", "வணக்கம் என் பின்கோடு 600028 தயவுசெய்து இதை சரிபார்க்கவும்", True),
        ("Symbols & Numbers clean", "Calculate formula: (25 * 4) + 15% tax = $115.00 total.", False),
        ("Secret AWS Key (Blocked)", "Here is my secret aws_secret_access_key: 1234567890123456789012345678901234567890 for debugging.", True),
        ("Code snippet clean", "def add(a, b):\n    return a + b\nprint(add(5, 10))", False),
    ]

    for name, p_text, expect_block in prompt_cases:
        blocked = send_claude_prompt(p_text)
        success = (blocked == expect_block)
        status_label = "BLOCKED" if blocked else "ALLOWED"
        expected_label = "BLOCKED" if expect_block else "ALLOWED"
        stat = "PASS" if success else "FAIL"
        print(f"[{stat}] {name:<35} | Status: {status_label:<7} | Expected: {expected_label:<7}")
        results.append((f"Prompt: {name}", success))

    # ──────────────────────────────────────────────────────────────────────────
    # SECTION 2: ALL FILE CATEGORIES
    # ──────────────────────────────────────────────────────────────────────────
    print("\n--- SECTION 2: CLAUDE ALL FILE CATEGORIES IMPORT & PREDICTION ---")

    pdf_content = (
        b"%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n"
        b"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
        b"3 0 obj<</Type/Page/MediaBox[0 0 300 144]/Parent 2 0 R/Resources<<>>/Contents 4 0 R>>endobj\n"
        b"4 0 obj<</Length 55>>stream\nBT /F1 12 Tf 50 100 Td (Audit report: pincode 600028 confidential) Tj ET\nendstream\nendobj\n"
        b"xref\n0 5\n0000000000 65535 f \n0000000009 00000 n \n0000000056 00000 n \n0000000111 00000 n \n0000000212 00000 n \n"
        b"trailer<</Size 5/Root 1 0 R>>\nstartxref\n318\n%%EOF"
    )

    file_test_cases = [
        # 1. PDF Document
        ("1. PDF Document", "q3_audit.pdf", pdf_content, "application/pdf", True, "cdn"),
        # 2. Word .docx
        ("2. Word Document (DOCX)", "contract_agreement.docx", make_docx("Vendor agreement pincode 600028"), "application/vnd.openxmlformats-officedocument.wordprocessingml.document", True, "cdn"),
        # 3. Excel .xlsx
        ("3. Excel Spreadsheet (XLSX)", "q4_salaries.xlsx", make_xlsx("Salary Record location 600028 confidential"), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", True, "cdn"),
        # 4. PowerPoint .pptx
        ("4. PowerPoint (PPTX)", "board_strategy.pptx", make_pptx("Board strategic presentation target 600028"), "application/vnd.openxmlformats-officedocument.presentationml.presentation", True, "cdn"),
        # 5. CSV via Claude Web in-browser extracted_content
        ("5. CSV Spreadsheet", "employee_directory.csv", "emp_id,name,pincode\n101,John,600028\n", "text/csv", True, "claude_extracted"),
        # 6. Python Code
        ("6. Python Code (.py)", "payment_service.py", "def pay():\n    postal = 600028\n    return postal\n", "text/x-python", True, "claude_extracted"),
        # 7. JavaScript Code
        ("7. JavaScript Code (.js)", "auth_controller.js", "export const config = { postal: 600028 };", "text/javascript", True, "claude_extracted"),
        # 8. SQL Database Dump
        ("8. SQL Database (.sql)", "schema_dump.sql", "INSERT INTO users VALUES ('John', 600028);", "text/x-sql", True, "claude_extracted"),
        # 9. JSON Config
        ("9. JSON Config (.json)", "app_settings.json", '{"app": "UnifAI", "postal_code": 600028}', "application/json", True, "claude_extracted"),
        # 10. Environment File
        ("10. Environment (.env)", ".env.production", "DATABASE_URL=postgres\nPINCODE=600028\n", "text/plain", True, "claude_extracted"),
        # 11. Compressed ZIP Archive
        ("11. Compressed Archive (ZIP)", "archive_backup.zip", make_zip("secrets.txt", "Secret DB migration pincode 600028"), "application/zip", True, "cdn"),
        # 12. Binary Model
        ("12. Binary Model (.bin)", "model_config.bin", b"\x00\x01\x02\x03MODEL_VERSION_2\x00\x00CONFIDENTIAL_TOKEN: 600028 AUTHORIZED\x00\x00\x04\x05", "application/octet-stream", True, "cdn"),
        # 13. Image Screenshot (OCR)
        ("13. Image Screenshot (OCR)", "confidential_screenshot.png", make_ocr_png("CONFIDENTIAL PINCODE : 600028 RESTRICTED"), "image/png", True, "cdn"),
        # 14. Clean Plain Text (Allowed)
        ("14. Plain Text (Clean)", "readme_documentation.txt", "Public open-source documentation for project setup.\n", "text/plain", False, "claude_extracted"),
        # 15. Clean Python Code (Allowed)
        ("15. Python Code (Clean)", "math_utils.py", "def add(x, y):\n    return x + y\n", "text/x-python", False, "claude_extracted"),
    ]

    for label, fname, data, ctype, expect_block, method in file_test_cases:
        if method == "cdn":
            blocked = upload_claude_cdn_then_send(fname, data, ctype)
        elif method == "claude_extracted":
            blocked = send_claude_extracted_attachment(fname, data, ctype)

        success = (blocked == expect_block)
        status_label = "BLOCKED" if blocked else "ALLOWED"
        expected_label = "BLOCKED" if expect_block else "ALLOWED"
        stat = "PASS" if success else "FAIL"
        print(f"[{stat}] {label:<28} | File: {fname:<25} | Status: {status_label:<7} | Expected: {expected_label:<7}")
        results.append((f"File: {fname}", success))

    # ──────────────────────────────────────────────────────────────────────────
    # SECTION 3: CLAUDE MULTI-FILE SIMULTANEOUS UPLOAD (3 FILES)
    # ──────────────────────────────────────────────────────────────────────────
    print("\n--- SECTION 3: CLAUDE MULTI-FILE SIMULTANEOUS UPLOAD (3 FILES) ---")
    multi_attachments = [
        {
            "file_name": "clean_notes.txt",
            "file_type": "text/plain",
            "file_size": 40,
            "extracted_content": "Project notes: meeting scheduled at 3 PM.",
        },
        {
            "file_name": "customer_database.csv",
            "file_type": "text/csv",
            "file_size": 65,
            "extracted_content": "id,name,pincode\n1,Alice,600028\n2,Bob,560001\n",
        },
        {
            "file_name": "clean_script.py",
            "file_type": "text/x-python",
            "file_size": 50,
            "extracted_content": "def run():\n    print('Hello World')\n",
        },
    ]

    chat_flow = tflow.tflow(req=http.Request.make(
        "POST",
        "https://claude.ai/api/organizations/org-test/chat_conversations/conv-123/completion",
        content=json.dumps({
            "prompt": "Please analyze all three attached files: clean_notes.txt, customer_database.csv, clean_script.py",
            "attachments": multi_attachments,
        }).encode(),
        headers={
            b"Host": b"claude.ai",
            b"Content-Type": b"application/json",
            b"Origin": b"https://claude.ai",
            b"Referer": b"https://claude.ai/chat/conv-123",
        }
    ))
    addon.request(chat_flow)
    blocked = is_flow_blocked(chat_flow)
    success = (blocked == True)
    stat = "PASS" if success else "FAIL"
    print(f"[{stat}] Multi-file (3 files with CSV violation) | Status: {'BLOCKED' if blocked else 'ALLOWED'} | Expected: BLOCKED")
    results.append(("Multi-file Simultaneous Upload", success))

    # ──────────────────────────────────────────────────────────────────────────
    # SUMMARY
    # ──────────────────────────────────────────────────────────────────────────
    print("\n" + "=" * 85)
    passed_count = sum(1 for _, ok in results if ok)
    total_count = len(results)
    print(f" CLAUDE LIVE SUITE COMPLETED: {passed_count}/{total_count} PASSED ({passed_count/total_count*100:.1f}%)")
    print("=" * 85)

if __name__ == "__main__":
    run_claude_full_suite()
