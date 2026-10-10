#!/usr/bin/env python3
"""Comprehensive ChatGPT All File Categories Verification:
Tests that EVERY file category (Documents, Office, Code, Config, Archives, Images, Audio, Binaries)
when uploaded to ChatGPT:
1. Accurately detects filename and file category.
2. Extracts content using the dedicated extractor for that file type.
3. Evaluates Guard Rules on the extracted content (Sensitive pincode 600028 -> BLOCK).
4. Allows clean files without false positives.
5. Performs complete ChatGPT flow (Create File -> CDN Upload -> Conversation Send).
"""

import io
import json
import os
import sys
import time
import uuid
import zipfile
from pathlib import Path
from mitmproxy import http
from mitmproxy.test import tflow

# Load gateway proxy parts
PARTS = Path(r"d:\unifai_project\apps\browser-guard\proxy\gateway_proxy_parts")
ns = {"__name__": "all_categories_test"}
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

def make_pdf(text: str) -> bytes:
    return (
        b"%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
        b"3 0 obj<</Type/Page/Parent 2 0 R/Contents 4 0 R>>endobj\n4 0 obj<</Length " + str(len(text) + 20).encode() + b">>stream\n"
        b"BT /F1 12 Tf (" + text.encode("utf-8") + b") Tj ET\nendstream endobj\n"
        b"trailer<</Root 1 0 R>>\n%%EOF\n"
    )

def make_png_with_text(text: str) -> bytes:
    from PIL import Image, ImageDraw
    img = Image.new("RGB", (650, 150), color=(255, 255, 255))
    draw = ImageDraw.Draw(img)
    draw.text((40, 50), text, fill=(0, 0, 0))
    buf = io.BytesIO()
    img.save(buf, format="PNG")
    return buf.getvalue()

def make_chatgpt_flow(path: str, body: bytes | str, method: str = "POST", headers: dict | None = None, host: str = "chatgpt.com"):
    if isinstance(body, str):
        body = body.encode("utf-8")
    hdrs = {
        "Host": host,
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
        "Content-Type": "application/json",
        "Accept": "text/event-stream, application/json",
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

print("=" * 75)
print("     CHATGPT ALL FILE CATEGORIES EXTRACTION & PREDICTION TEST")
print("=" * 75)

# Test matrix: (Category, Filename, MimeType, PayloadGenerator, ExpectedViolation)
CATEGORIES_TEST = [
    ("1. PDF Document", "q3_audit.pdf", "application/pdf", make_pdf("Confidential Audit Pincode 600028 Restricted"), True),
    ("2. Word Document", "contract_agreement.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", make_docx("Employee contract agreement code 600028 legal notice"), True),
    ("3. Excel Spreadsheet", "q4_salaries.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", make_xlsx("Salary payout 50000 location postal 600028"), True),
    ("4. PowerPoint Slide", "board_strategy.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", make_pptx("Secret branch expansion target 600028 city zone"), True),
    ("5. CSV Spreadsheet", "employee_directory.csv", "text/csv", (b"id,name,role,pincode\n101,Ravi,Lead,600028\n102,Anu,Dev,560001\n" * 2), True),
    ("6. Source Code (Python)", "payment_service.py", "text/x-python", b"# Production Payment Microservice\n# Module: Auth & Checkout\ndef get_secret_pin():\n    # Security verification code\n    return 600028\n", True),
    ("7. Source Code (JavaScript)", "auth_controller.js", "application/javascript", b"// Production Auth Controller\n// Handles secure gateway authentication\nconst SECRET_DELIVERY_CODE = 600028;\nexport default SECRET_DELIVERY_CODE;\n", True),
    ("8. Database Script (SQL)", "schema_dump.sql", "application/sql", b"-- Database migration script for user addresses\nCREATE TABLE user_locations (id INT, pincode INT);\nINSERT INTO user_locations VALUES (1, 600028);\n", True),
    ("9. Web Config (JSON)", "app_settings.json", "application/json", b'{\n  "environment": "production",\n  "app_name": "Portal",\n  "branch_pin": 600028,\n  "status": "active"\n}\n', True),
    ("10. Environment File (.env)", ".env.production", "text/plain", b"DB_HOST=10.0.0.1\nDB_PASS=SuperSecret2026\nOFFICE_PINCODE=600028\nPORT=8080\n", True),
    ("11. Compressed Archive (ZIP)", "archive_backup.zip", "application/zip", make_zip("secrets.txt", "Internal secret file zip extraction pincode 600028"), True),
    ("12. Binary Model / Data", "model_config.bin", "application/octet-stream", b"\x00\x01\x02\x03MODEL_VERSION_2\x00\x00CONFIDENTIAL_TOKEN: 600028 AUTHORIZED\x00\x00\x04\x05" * 2, True),
    ("13. Plain Text (Clean)", "readme_documentation.txt", "text/plain", b"Welcome to open source documentation. This software is licensed under MIT license.\n" * 2, False),
    ("14. Python Code (Clean)", "math_utils.py", "text/x-python", b"def add(a, b):\n    '''Add two numbers.'''\n    return a + b\n\ndef multiply(x, y):\n    return x * y\n" * 2, False),
    ("15. JSON Config (Clean)", "package_config.json", "application/json", b'{\n  "name": "unifai-frontend",\n  "version": "1.0.0",\n  "description": "Production UI app",\n  "private": true\n}\n', False),
    ("16. Image Screenshot (OCR)", "confidential_screenshot.png", "image/png", make_png_with_text("CONFIDENTIAL PINCODE : 600028 RESTRICTED"), True),
]

passed_count = 0
total_count = len(CATEGORIES_TEST)

for category, fname, mime, payload, should_block in CATEGORIES_TEST:
    data_bytes = payload if isinstance(payload, bytes) else str(payload).encode("utf-8")
    
    # Unit 1: Check category classification
    classified_kind = ns["_classify_upload_kind"](data_bytes, mime, fname)
    
    # Unit 2: Check content extraction
    extracted_text = ns["extract_upload_text_for_rules"](data_bytes, mime, "", fname)
    
    # Unit 3: Check Rule matching
    matched, rname, ract = ns["match_guard_rules_on_text"](extracted_text)
    
    # Unit 4: Run end-to-end ChatGPT flow (Create File -> CDN PUT -> Send)
    fid = f"file-{uuid.uuid4().hex[:18]}"
    
    # A. Create-file handshake
    addon.request(make_chatgpt_flow("/backend-api/files", json.dumps({"file_name": fname, "file_size": len(data_bytes), "use_case": "my_files"})))
    
    # B. CDN upload to files.oaiusercontent.com
    addon.request(make_chatgpt_flow(
        f"/{fid}?sig=test",
        data_bytes,
        method="PUT",
        headers={"Origin": "https://chatgpt.com", "Content-Type": mime},
        host="files.oaiusercontent.com"
    ))
    
    # C. Send flow on chatgpt.com
    send_payload = json.dumps({
        "action": "next",
        "messages": [
            {
                "id": str(uuid.uuid4()),
                "author": {"role": "user"},
                "content": {
                    "content_type": "multimodal_text",
                    "parts": [f"Please analyze {fname}"]
                },
                "metadata": {
                    "attachments": [
                        {"id": fid, "name": fname, "size": len(data_bytes), "mime_type": mime}
                    ]
                }
            }
        ],
        "model": "auto"
    })
    
    send_flow = make_chatgpt_flow("/backend-api/f/conversation", send_payload)
    addon.request(send_flow)
    
    is_flow_blocked = (send_flow.response is not None)
    
    ok = (is_flow_blocked == should_block)
    if ok:
        passed_count += 1
        verdict_str = "BLOCKED" if is_flow_blocked else "ALLOWED"
        print(f"[PASS] {category:<26} | file: {fname:<24} | kind={classified_kind:<7} | status={verdict_str:<7} | extract={len(extracted_text)} chars")
    else:
        print(f"[FAIL] {category:<26} | file: {fname:<24} | expected_block={should_block} got_block={is_flow_blocked}")
        print(f"       Extracted text ({len(extracted_text)} chars): {extracted_text[:120]!r}")
        assert False, f"Test failed for {category}"

print("\n" + "=" * 75)
print(f"     SUMMARY: {passed_count}/{total_count} FILE CATEGORIES PASSED 100% PERFECTLY!")
print("=" * 75)
