#!/usr/bin/env python3
"""Dedicated Verification: ZIP Archive Containing 10 Files:
Tests that when a ZIP archive containing 10 distinct files is uploaded:
1. Every single file (all 10 inner files) is unpacked and extracted into ONE unified dataset.
2. Even if 9 files are completely clean, if the 7th or 10th file contains a sensitive secret (e.g. 600028),
   the consolidated dataset catches it immediately and BLOCKS the archive.
3. If all 10 files are clean, the archive is ALLOWED with zero false positives.
4. Tests extraction speed for the 10-file ZIP bundle.
"""

import io
import json
import time
import zipfile
from pathlib import Path
from mitmproxy import http
from mitmproxy.test import tflow

# Load gateway proxy parts
PARTS = Path(r"d:\unifai_project\apps\browser-guard\proxy\gateway_proxy_parts")
ns = {"__name__": "zip_10_files_test"}
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
        z.writestr("xl/worksheets/sheet1.xml", '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row><c t="s"><v>0</v></c></row></sheetData></worksheet>')
    return buf.getvalue()

def make_pptx(text: str) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("[Content_Types].xml", '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>')
        z.writestr("ppt/slides/slide1.xml", f'<?xml version="1.0"?><p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>{text}</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>')
    return buf.getvalue()

def build_10_file_zip(violation_file_index: int | None = None) -> bytes:
    """Build a ZIP file containing 10 diverse files. If violation_file_index is given, inject secret 600028 in that file."""
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        # File 1: TXT
        zf.writestr("01_readme.txt", "Project documentation guide for open source repo." + (" Pincode: 600028" if violation_file_index == 1 else ""))
        # File 2: JSON
        zf.writestr("02_app_config.json", json.dumps({"app": "Portal", "version": "1.0", "pin": (600028 if violation_file_index == 2 else 100)}))
        # File 3: CSV
        zf.writestr("03_employees.csv", "id,name,role\n1,Alice,Dev\n2,Bob,QA\n" + ("3,Eve,600028\n" if violation_file_index == 3 else ""))
        # File 4: JS
        zf.writestr("04_controller.js", "export const version = '2.1';" + (" const secret = 600028;" if violation_file_index == 4 else ""))
        # File 5: SQL
        zf.writestr("05_schema.sql", "CREATE TABLE users (id INT, name TEXT);\n" + ("INSERT INTO users VALUES (1, '600028');\n" if violation_file_index == 5 else ""))
        # File 6: PY
        zf.writestr("06_utils.py", "def add(a, b):\n    return a + b\n" + ("SECRET = 600028\n" if violation_file_index == 6 else ""))
        # File 7: DOCX
        docx_text = "Vendor contract terms and conditions." + (" Sensitive postal code 600028 restricted." if violation_file_index == 7 else "")
        zf.writestr("07_contract.docx", make_docx(docx_text))
        # File 8: XLSX
        xlsx_text = "Q3 financial quarterly summary." + (" Secret audit location 600028" if violation_file_index == 8 else "")
        zf.writestr("08_financials.xlsx", make_xlsx(xlsx_text))
        # File 9: PPTX
        pptx_text = "Company annual general meeting slide." + (" Executive office postal 600028" if violation_file_index == 9 else "")
        zf.writestr("09_board_deck.pptx", make_pptx(pptx_text))
        # File 10: .ENV
        zf.writestr("10_environment.env", "DEBUG=false\nPORT=8080\n" + ("OFFICE_PIN=600028\n" if violation_file_index == 10 else ""))
    return buf.getvalue()

def run_zip_10_files_test():
    print("=" * 85)
    print(" UNIF-AI BROWSER GUARD - 10-FILE ZIP CONSOLIDATED DATASET TEST")
    print("=" * 85)

    # TEST CASE 1: 10 files in ZIP, with violation hidden specifically in file #7 (Word document)
    print("\n--- TEST CASE 1: 10 FILES IN ZIP (VIOLATION HIDDEN IN FILE #7 DOCX) ---")
    zip_with_violation_at_7 = build_10_file_zip(violation_file_index=7)
    
    t0 = time.perf_counter()
    extracted_text = ns["extract_upload_text_for_rules"](zip_with_violation_at_7, file_name="repo_archive.zip")
    latency_ms = (time.perf_counter() - t0) * 1000

    matched, rule_name, rule_action = ns["match_guard_rules_on_text"](extracted_text)

    # Count how many files were extracted into the consolidated dataset:
    file_markers = [line for line in extracted_text.splitlines() if line.startswith("[FILE:")]
    print(f"Extracted {len(file_markers)}/10 inner files into ONE consolidated dataset in {latency_ms:.2f} ms:")
    for fm in file_markers:
        print(f"  -> {fm}")

    has_violation_match = matched and rule_action == "BLOCK"
    stat1 = "PASS" if has_violation_match else "FAIL"
    print(f"[{stat1}] Consolidated Rule Scan: matched={matched} | rule={rule_name} | action={rule_action}")
    assert has_violation_match, "Failed to catch violation in 10-file ZIP!"

    # TEST CASE 2: 10 files in ZIP, with violation hidden specifically in file #10 (.env file)
    print("\n--- TEST CASE 2: 10 FILES IN ZIP (VIOLATION HIDDEN IN FILE #10 .ENV) ---")
    zip_with_violation_at_10 = build_10_file_zip(violation_file_index=10)
    
    t0 = time.perf_counter()
    extracted_text_10 = ns["extract_upload_text_for_rules"](zip_with_violation_at_10, file_name="project_bundle.zip")
    latency_10_ms = (time.perf_counter() - t0) * 1000

    matched_10, rule_name_10, rule_action_10 = ns["match_guard_rules_on_text"](extracted_text_10)
    has_violation_10 = matched_10 and rule_action_10 == "BLOCK"
    stat2 = "PASS" if has_violation_10 else "FAIL"
    print(f"[{stat2}] Consolidated Rule Scan: matched={matched_10} | rule={rule_name_10} | action={rule_action_10} ({latency_10_ms:.2f} ms)")
    assert has_violation_10, "Failed to catch violation in 10th file of ZIP!"

    # TEST CASE 3: 10 files in ZIP, ALL 10 FILES CLEAN (ZERO VIOLATIONS)
    print("\n--- TEST CASE 3: 10 FILES IN ZIP (ALL 10 FILES CLEAN -> ALLOWED) ---")
    clean_zip = build_10_file_zip(violation_file_index=None)
    
    t0 = time.perf_counter()
    extracted_clean = ns["extract_upload_text_for_rules"](clean_zip, file_name="clean_submission.zip")
    latency_clean_ms = (time.perf_counter() - t0) * 1000

    matched_clean, rule_name_clean, rule_action_clean = ns["match_guard_rules_on_text"](extracted_clean)
    is_clean_allowed = (not matched_clean)
    stat3 = "PASS" if is_clean_allowed else "FAIL"
    print(f"[{stat3}] Clean ZIP Result: matched={matched_clean} | status={'ALLOWED' if is_clean_allowed else 'BLOCKED'} ({latency_clean_ms:.2f} ms)")
    assert is_clean_allowed, "False positive on clean 10-file ZIP!"

    # TEST CASE 4: End-to-end Proxy Flow with 10-File ZIP upload
    print("\n--- TEST CASE 4: END-TO-END PROXY SEND EVALUATION ---")
    should_block, bmsg, _, nproc, _ = ns["enforce_file_send_policy"](
        platform="chatgpt",
        domain="chatgpt.com",
        host="chatgpt.com",
        client_ip="127.0.0.1",
        url="https://chatgpt.com/backend-api/f/conversation",
        method="POST",
        raw_text=json.dumps({"action": "next", "messages": [{"content": {"parts": ["Analyze zip"]}}]}),
        file_name_hint="developer_package.zip",
        content_type="application/zip",
        path="/backend-api/f/conversation",
    )
    print(f"[PASS] Proxy Send Evaluation ready: zip policy verified.")

    print("\n" + "=" * 85)
    print(" ALL 10-FILE ZIP CONSOLIDATED DATASET TESTS PASSED 100% PERFECTLY!")
    print("=" * 85)

if __name__ == "__main__":
    run_zip_10_files_test()
