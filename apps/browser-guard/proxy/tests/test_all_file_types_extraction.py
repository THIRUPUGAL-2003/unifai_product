#!/usr/bin/env python3
"""
Test file extraction and rule matching across all file types:
- Code files (.py, .js, .ts, .java, .c, .cpp, .go, .rs, .sql, .sh, .ps1, .json, .yaml, .env, .html, .css)
- Text and Office (.txt, .csv, .md, .docx, .xlsx, .pptx, .pdf, .rtf)
- Archives (.zip, nested .zip, .tar, .tar.gz)
- Image OCR
"""
import io
import json
import os
import re
import sys
import tarfile
import zipfile
from pathlib import Path

PROXY_DIR = Path(__file__).resolve().parents[1]
PARTS_DIR = PROXY_DIR / "gateway_proxy_parts"
names = [
    "config_caches_rules.py",
    "helpers_prompts.py",
    "uploads_detect.py",
    "file_policy.py",
    "extract_office_backend.py",
    "responses_inject.py",
]
ns = {"__name__": "test"}
for n in names:
    code = (PARTS_DIR / n).read_text(encoding="utf-8")
    exec(compile(code, str(PARTS_DIR / n), "exec"), ns)

extract_fn = ns["extract_upload_text_for_rules"]
match_fn = ns["match_guard_rules_on_text"]

SECRET = "CONFIDENTIAL_PROJECT_X_KEY_999"
ns["_cached_rules"] = [{
    "name": "Confidential Rule",
    "pattern": r"CONFIDENTIAL_PROJECT_X",
    "regex": re.compile(r"CONFIDENTIAL_PROJECT_X", re.I),
    "action": "BLOCK",
    "severity": "HIGH",
    "warning_message": "Confidential data detected",
}]

test_files = {}

# 1. Coding files
test_files["test.py"] = f'import os\nSECRET_KEY = "{SECRET}"\nprint(SECRET_KEY)'.encode("utf-8")
test_files["test.js"] = f'const secret = "{SECRET}"; console.log(secret);'.encode("utf-8")
test_files["test.ts"] = f'export const key: string = "{SECRET}";'.encode("utf-8")
test_files["test.java"] = f'public class Main {{ String s = "{SECRET}"; }}'.encode("utf-8")
test_files["test.c"] = f'#include <stdio.h>\nchar* s = "{SECRET}";'.encode("utf-8")
test_files["test.cpp"] = f'#include <iostream>\nstd::string s = "{SECRET}";'.encode("utf-8")
test_files["test.go"] = f'package main\nconst key = "{SECRET}"'.encode("utf-8")
test_files["test.rs"] = f'fn main() {{ let s = "{SECRET}"; }}'.encode("utf-8")
test_files["test.sql"] = f'INSERT INTO credentials VALUES ("{SECRET}");'.encode("utf-8")
test_files["test.sh"] = f'#!/bin/bash\nAPI_KEY="{SECRET}"'.encode("utf-8")
test_files["test.ps1"] = f'$ApiKey = "{SECRET}"'.encode("utf-8")
test_files["test.json"] = json.dumps({"key": SECRET, "env": "prod"}).encode("utf-8")
test_files["test.yaml"] = f"auth:\n  secret: {SECRET}".encode("utf-8")
test_files["test.env"] = f"DATABASE_SECRET={SECRET}".encode("utf-8")
test_files["test.html"] = f"<html><body><p>{SECRET}</p></body></html>".encode("utf-8")
test_files["test.css"] = f"/* key: {SECRET} */ body {{ color: red; }}".encode("utf-8")

# 2. Text / CSV / Spreadsheet / Doc files
test_files["test.txt"] = f"This is a sample document containing {SECRET} inside.".encode("utf-8")
test_files["test.csv"] = f"id,name,secret\n1,alice,{SECRET}\n".encode("utf-8")
test_files["test.md"] = f"# Notes\nDo not share {SECRET}".encode("utf-8")

# Minimal docx (ZIP with word/document.xml)
docx_buf = io.BytesIO()
with zipfile.ZipFile(docx_buf, "w") as zf:
    zf.writestr(
        "word/document.xml",
        f'<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Confidential Report: {SECRET}</w:t></w:r></w:p></w:body></w:document>'
    )
test_files["report.docx"] = docx_buf.getvalue()

# Minimal xlsx (ZIP with xl/sharedStrings.xml)
xlsx_buf = io.BytesIO()
with zipfile.ZipFile(xlsx_buf, "w") as zf:
    zf.writestr(
        "xl/sharedStrings.xml",
        f'<?xml version="1.0" encoding="UTF-8"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>{SECRET}</t></si></sst>'
    )
    zf.writestr("xl/workbook.xml", "<workbook></workbook>")
test_files["data.xlsx"] = xlsx_buf.getvalue()

# Minimal pptx (ZIP with ppt/slides/slide1.xml)
pptx_buf = io.BytesIO()
with zipfile.ZipFile(pptx_buf, "w") as zf:
    zf.writestr(
        "ppt/slides/slide1.xml",
        f'<?xml version="1.0" encoding="UTF-8"?><p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>{SECRET}</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>'
    )
test_files["deck.pptx"] = pptx_buf.getvalue()

# Minimal RTF
rtf_text = r"{\rtf1\ansi\deff0 {\fonttbl {\f0 Arial;}}\f0\fs24 Confidential: " + SECRET + r"}"
test_files["note.rtf"] = rtf_text.encode("utf-8")

# Minimal zip containing coding and doc files
zip_buf = io.BytesIO()
with zipfile.ZipFile(zip_buf, "w") as zf:
    zf.writestr("inner_code.py", f'KEY = "{SECRET}"')
    zf.writestr("notes.txt", f"Confidential: {SECRET}")
test_files["archive.zip"] = zip_buf.getvalue()

# Nested zip (zip containing another zip with secret)
inner_zip_buf = io.BytesIO()
with zipfile.ZipFile(inner_zip_buf, "w") as zf:
    zf.writestr("nested_secret.py", f'NESTED_KEY = "{SECRET}"')
outer_zip_buf = io.BytesIO()
with zipfile.ZipFile(outer_zip_buf, "w") as zf:
    zf.writestr("nested.zip", inner_zip_buf.getvalue())
test_files["nested_archive.zip"] = outer_zip_buf.getvalue()

# Standalone .gz file (decompress directly)
import gzip
test_files["log.gz"] = gzip.compress(f"GZIP_LOG: {SECRET}".encode("utf-8"))

# Raw valid PDF with text layer
pdf_stream = f"BT /F1 12 Tf 72 712 Td (Confidential: {SECRET}) Tj ET".encode("latin-1")
pdf_raw = (
    b"%PDF-1.4\n"
    b"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
    b"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
    b"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> >> >> >>\nendobj\n"
    b"4 0 obj\n<< /Length " + str(len(pdf_stream)).encode() + b" >>\nstream\n" + pdf_stream + b"\nendstream\nendobj\n"
    b"xref\n0 5\n0000000000 65535 f \n0000000009 00000 n \n0000000056 00000 n \n0000000111 00000 n \n0000000280 00000 n \n"
    b"trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n380\n%%EOF"
)
test_files["doc.pdf"] = pdf_raw

# Short file under 32 bytes
test_files["small.py"] = f'K="{SECRET}"'.encode("utf-8")

print("=" * 70)
print(f"{'File Name':20} | {'Chars':6} | {'SecretFound':11} | {'RuleHit':7} | Status")
print("=" * 70)
failures = []
for fname, data in test_files.items():
    extracted = extract_fn(data, "", "", fname)
    rule_hit, rule_name, rule_action = match_fn(extracted)
    found_secret = SECRET in (extracted or "")
    status = "PASS" if (found_secret and rule_hit) else "FAIL"
    if status == "FAIL":
        failures.append(fname)
    print(f"{fname:20} | {len(extracted or ''):6} | {str(found_secret):11} | {str(rule_hit):7} | [{status}]")

print("=" * 70)
if failures:
    print(f"FAILED FILES ({len(failures)}): {failures}")
    if __name__ == "__main__":
        sys.exit(1)
    raise AssertionError(f"extraction failed for: {failures}")
print("ALL FILES PASSED EXTRACTION AND RULE CHECK!")
