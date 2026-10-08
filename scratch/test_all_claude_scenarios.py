import os
import sys
import json
import base64
import time
import re
from pathlib import Path

PARTS_DIR = Path(r"d:\unifai_project\apps\browser-guard\proxy\gateway_proxy_parts")
names = [
    "config_caches_rules.py",
    "helpers_prompts.py",
    "uploads_detect.py",
    "file_policy.py",
    "extract_office_backend.py",
    "responses_inject.py",
]
NS: dict = {"__name__": "browser_ai_proxy_test"}
for name in names:
    path = PARTS_DIR / name
    with open(path, "r", encoding="utf-8") as f:
        code = compile(f.read(), str(path), "exec")
        exec(code, NS)

print("=== Testing Updated Claude Detection on All Scenarios ===")

# Test 1: Claude extracted_content in attachments
payload_extracted = {
    "prompt": "Summarize these documents",
    "attachments": [
        {
            "file_name": "q1_financials.csv",
            "file_type": "text/csv",
            "extracted_content": "Quarter,Revenue,Profit\nQ1,1000000,250000"
        },
        {
            "file_name": "executive_summary.txt",
            "file_type": "text/plain",
            "extracted_content": "The company met all Q1 targets successfully."
        },
        {
            "file_name": "employee_data.csv",
            "file_type": "text/csv",
            "extracted_content": "Name,SSN,Salary\nBob Smith,123-45-6789,85000"
        }
    ]
}
text_ext = json.dumps(payload_extracted)
print("Test 1 names:", NS["extract_all_attachment_filenames_from_send"](text_ext))
print("Test 1 count:", NS["_expected_send_attachment_count"](text_ext))
print("Test 1 carries:", NS["chat_carries_attachment"](text_ext))

# Test 2: Claude with ONLY `files: ["uuid1", "uuid2", "uuid3"]`
payload_files_only = {
    "prompt": "Analyze all 3 files",
    "files": [
        "11111111-2222-3333-4444-555555555555",
        "66666666-7777-8888-9999-000000000000",
        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
    ]
}
text_files = json.dumps(payload_files_only)
print("\nTest 2 IDs:", NS["_extract_file_ids_from_chat"](text_files))
print("Test 2 count:", NS["_expected_send_attachment_count"](text_files))
print("Test 2 carries:", NS["chat_carries_attachment"](text_files))

# Test 3: Claude Anthropic messages with content blocks (pdf + txt + image)
payload_messages = {
    "messages": [
        {
            "role": "user",
            "content": [
                {"type": "text", "text": "Review these:"},
                {
                    "type": "document",
                    "source": {
                        "type": "base64",
                        "media_type": "application/pdf",
                        "data": base64.b64encode(b"%PDF-1.4 test document 1").decode()
                    },
                    "title": "contract_draft.pdf"
                },
                {
                    "type": "document",
                    "source": {
                        "type": "base64",
                        "media_type": "text/plain",
                        "data": base64.b64encode(b"Terms and conditions text here").decode()
                    },
                    "title": "terms.txt"
                },
                {
                    "type": "image",
                    "source": {
                        "type": "base64",
                        "media_type": "image/png",
                        "data": base64.b64encode(b"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB`\x82").decode()
                    }
                }
            ]
        }
    ]
}
text_msg = json.dumps(payload_messages)
print("\nTest 3 names:", NS["extract_all_attachment_filenames_from_send"](text_msg))
print("Test 3 count:", NS["_expected_send_attachment_count"](text_msg))
print("Test 3 carries:", NS["chat_carries_attachment"](text_msg))
