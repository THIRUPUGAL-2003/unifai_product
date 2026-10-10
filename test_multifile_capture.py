#!/usr/bin/env python3
"""Verification test:
1. Single file real filename preservation
2. Multiple files (multipart & separate uploads)
3. Content extraction & prediction accuracy
"""
import io
import json
import os
import sys
import uuid
from pathlib import Path

# Load gateway proxy parts in the same manner as browser_ai_proxy
PARTS = Path(r"d:\unifai_project\apps\browser-guard\proxy\gateway_proxy_parts")
ns = {"__name__": "test_harness"}
names = [
    ln.strip().lstrip("\ufeff")
    for ln in (PARTS / "MANIFEST.txt").read_text(encoding="utf-8-sig").splitlines()
    if ln.strip() and not ln.strip().startswith("#")
]
for name in names:
    p = PARTS / name
    exec(compile(p.read_text(encoding="utf-8"), str(p), "exec"), ns)

print("[+] Gateway Proxy Parts loaded successfully.")

# Setup test rules:
# Rule 1: Pincode \b[1-9][0-9]{5}\b -> BLOCK
test_rules = [
    {
        "id": "rule-pincode",
        "name": "Pincode Rule",
        "pattern": r"\b[1-9][0-9]{5}\b",
        "action": "BLOCK",
        "warning_message": "Sensitive pincode detected"
    }
]
ns["_cached_rules"] = test_rules

# Test 1: Filename validation and quality checks
print("\n=== TEST 1: Filename Validation & Preservation ===")
test_names = [
    ("quarterly_earnings_2026.pdf", True),
    ("employees_list.csv", True),
    ("auth_service.py", True),
    ("system_architecture.png", True),
    ("database_dump.sql", True),
    ("docker-compose.yml", True),
    (".env.production", True),
    ("attachment", False),  # fake placeholder
    ("blob", False),        # fake placeholder
    ("attachment.txt", False), # fake placeholder
    ("unknown", False)      # fake placeholder
]

for fname, expect_real in test_names:
    is_real = ns["_is_real_user_upload_name"](fname)
    quality = ns["_upload_name_quality"](fname)
    status = "REAL" if is_real else "FAKE"
    pass_fail = "PASS" if is_real == expect_real else "FAIL"
    print(f"[{pass_fail}] Filename: {fname:<30} -> {status} (quality={quality})")
    assert is_real == expect_real, f"Failed for {fname}"

# Test 2: Content Extraction across file categories
print("\n=== TEST 2: Content Extraction Across File Categories ===")
files_to_test = [
    ("test_plain.txt", b"Confidential memo for department 600028 internal only.", "text/plain", "600028"),
    ("test_csv.csv", b"name,id,zip\nAlice,101,600028\nBob,102,500001\n", "text/csv", "600028"),
    ("test_code.py", b"def get_user():\n    secret_pin = 600028\n    return secret_pin\n", "text/x-python", "600028"),
    ("test_json.json", b'{"organization": "UnifAI", "postal_code": 600028}', "application/json", "600028"),
    ("audit_report.pdf", (
        b"%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
        b"3 0 obj<</Type/Page/Parent 2 0 R/Contents 4 0 R>>endobj\n4 0 obj<</Length 55>>stream\n"
        b"BT /F1 12 Tf (Confidential Audit Pincode 600028 restricted) Tj ET\nendstream endobj\n"
        b"trailer<</Root 1 0 R>>\n%%EOF\n"
    ), "application/pdf", "600028"),
    ("clean_file.txt", b"Public documentation for open source software.", "text/plain", None),
]

for fname, data, ct, expected_target in files_to_test:
    extracted = ns["extract_upload_text_for_rules"](data, ct, "", fname)
    matched, rname, ract = ns["match_guard_rules_on_text"](extracted)
    
    if expected_target:
        assert expected_target in extracted, f"Extraction failed for {fname}: got {extracted[:50]!r}"
        assert matched and ract == "BLOCK", f"Rule matching failed for {fname}"
        print(f"[PASS] {fname:<18} -> Extracted {len(extracted)} chars | Content: {extracted.strip()[:40]!r} | RULE: {rname} ({ract})")
    else:
        assert not matched, f"False positive for clean file {fname}"
        print(f"[PASS] {fname:<18} -> Extracted {len(extracted)} chars | Content: {extracted.strip()[:40]!r} | ALLOWED (Clean)")

# Test 3: Multiple File Uploads - Handling 3 files together in Send
print("\n=== TEST 3: Multiple File Uploads Handling (3 Files) ===")
# Simulate user uploading 3 distinct files
domain = "claude.ai"
f1_bytes = b"Employee 1 details: Ravi"
f2_bytes = b"Employee 2 details: Suresh postal 600028"
f3_bytes = b"Company policy document: safe content"

ns["cache_upload_file"](domain, file_name="report_one.txt", raw_bytes=f1_bytes, content_type="text/plain", upload_reason="test")
ns["cache_upload_file"](domain, file_name="financial_records.csv", raw_bytes=f2_bytes, content_type="text/plain", upload_reason="test")
ns["cache_upload_file"](domain, file_name="guidelines.txt", raw_bytes=f3_bytes, content_type="text/plain", upload_reason="test")

# When user clicks Send on Claude
send_json = json.dumps({
    "prompt": "Analyze all 3 uploaded documents",
    "attachments": [
        {"file_name": "report_one.txt"},
        {"file_name": "financial_records.csv"},
        {"file_name": "guidelines.txt"}
    ]
})

cached_list = ns["take_all_cached_uploads_for_send"](domain, send_json, allow_latest=True)
print(f"Retrieved {len(cached_list)} cached files for Send:")
assert len(cached_list) == 3, f"Expected 3 files, got {len(cached_list)}"

retrieved_names = [e.get("file_name") for e in cached_list]
print(f"Files retrieved: {retrieved_names}")
assert "report_one.txt" in retrieved_names
assert "financial_records.csv" in retrieved_names
assert "guidelines.txt" in retrieved_names

# Now test rule evaluation on all 3 files
hit_count = 0
for e in cached_list:
    text = ns["extract_upload_text_for_rules"](e["raw_bytes"], e["content_type"], "", e["file_name"])
    hit, rname, ract = ns["match_guard_rules_on_text"](text)
    print(f"  - File '{e['file_name']}': content='{text}' -> Hit={hit} ({ract})")
    if hit:
        hit_count += 1
        assert e["file_name"] == "financial_records.csv", "Wrong file triggered violation!"

assert hit_count == 1, f"Expected exactly 1 violation among the 3 files, got {hit_count}"
print(f"[PASS] Multi-file test passed: all 3 file names intact, violation correctly localized to financial_records.csv!")

print("\nALL VERIFICATION TESTS COMPLETED SUCCESSFULLY!")
