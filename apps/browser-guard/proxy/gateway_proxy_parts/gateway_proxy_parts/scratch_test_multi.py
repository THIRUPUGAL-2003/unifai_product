import sys
import os
import json
import time

sys.path.insert(0, r"d:\unifai_project\apps\browser-guard\proxy\gateway_proxy_parts\gateway_proxy_parts")

import config_caches_rules
import extract_office_backend
import uploads_detect
import helpers_prompts
import file_policy
import responses_inject
import responses_addon

modules = [config_caches_rules, extract_office_backend, uploads_detect, helpers_prompts, file_policy, responses_inject, responses_addon]

# Simulate mitmproxy execution where all files are loaded into a single namespace
combined_ns = {}
for m in modules:
    for k, v in m.__dict__.items():
        if not k.startswith("__"):
            combined_ns[k] = v

for m in modules:
    for k, v in combined_ns.items():
        m.__dict__.setdefault(k, v)

from uploads_detect import (
    _isolate_active_send_message,
    _extract_file_ids_from_chat,
    extract_all_attachment_filenames_from_send,
    _expected_send_attachment_count,
    take_all_cached_uploads_for_send,
    cache_upload_file,
    clear_domain_upload_caches,
    _bind_real_filenames_to_cached_uploads,
    remember_file_create_handshake,
)
from file_policy import enforce_file_send_policy

chatgpt_payload = {
    "action": "next",
    "messages": [
        {
            "id": "aaa-111",
            "author": {"role": "user"},
            "content": {
                "content_type": "multimodal_text",
                "parts": [
                    "Can you summarize these files for me?",
                    {
                        "asset_pointer": "file-service://file-AAA111",
                        "size_bytes": 1024,
                        "width": None,
                        "height": None
                    },
                    {
                        "asset_pointer": "file-service://file-BBB222",
                        "size_bytes": 2048,
                        "width": None,
                        "height": None
                    },
                    {
                        "asset_pointer": "file-service://file-CCC333",
                        "size_bytes": 4096,
                        "width": None,
                        "height": None
                    }
                ]
            },
            "metadata": {
                "attachments": [
                    {"id": "file-AAA111", "name": "financial_report.xlsx", "size": 1024, "mime_type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
                    {"id": "file-BBB222", "name": "passwords_list.docx", "size": 2048, "mime_type": "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
                    {"id": "file-CCC333", "name": "project_notes.txt", "size": 4096, "mime_type": "text/plain"}
                ]
            }
        }
    ],
    "parent_message_id": "root",
    "model": "gpt-4o"
}

raw_json = json.dumps(chatgpt_payload)

print("=== Case C: Realistic size bytes with create handshake + nameless CDN uploads ===")
clear_domain_upload_caches("chatgpt.com")

# 1. First ChatGPT sends 3 metadata create handshakes:
remember_file_create_handshake(json.dumps({"file_name": "financial_report.xlsx", "file_size": 1024, "use_case": "multimodal"}), "chatgpt.com")
remember_file_create_handshake(json.dumps({"file_name": "passwords_list.docx", "file_size": 2048, "use_case": "multimodal"}), "chatgpt.com")
remember_file_create_handshake(json.dumps({"file_name": "project_notes.txt", "file_size": 4096, "use_case": "multimodal"}), "chatgpt.com")

# 2. Then 3 files are uploaded to files.oaiusercontent.com (> 100 bytes):
b_xlsx = b"PK\x03\x04" + b"A" * 500
b_docx = b"PK\x03\x04" + b"B" * 500
b_txt = b"Confidential password: admin123\n" * 10

cache_upload_file("chatgpt.com", file_name="", raw_bytes=b_xlsx, content_type="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", upload_reason="CDN upload", file_id="")
cache_upload_file("chatgpt.com", file_name="", raw_bytes=b_docx, content_type="application/vnd.openxmlformats-officedocument.wordprocessingml.document", upload_reason="CDN upload", file_id="")
cache_upload_file("chatgpt.com", file_name="", raw_bytes=b_txt, content_type="text/plain", upload_reason="CDN upload", file_id="")

# 3. User sends message:
print("\n--- Running enforce_file_send_policy ---")
res = enforce_file_send_policy(
    platform="OpenAI",
    domain="chatgpt.com",
    host="chatgpt.com",
    client_ip="127.0.0.1",
    url="https://chatgpt.com/backend-api/conversation",
    method="POST",
    raw_text=raw_json,
    raw_bytes=raw_json.encode("utf-8"),
    user_prompt_hint="Can you summarize these files for me?",
    content_type="application/json",
    file_name_hint="",
    path="/backend-api/conversation"
)
print("enforce_file_send_policy result:", res)
