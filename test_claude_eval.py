import sys
from pathlib import Path
ROOT = Path(r"d:\unifai_project\apps\browser-guard")
sys.path.insert(0, str(ROOT / "proxy"))
sys.path.insert(0, str(ROOT / "proxy" / "gateway_proxy_parts" / "gateway_proxy_parts"))

import browser_ai_proxy
browser_ai_proxy._load_parts()
ns = browser_ai_proxy.__dict__
import json

# Test Case A: Empty prompt / caption with attached file in Claude
pa = json.dumps({
    "prompt": "",
    "attachments": [
        {
            "id": "att-1",
            "file_name": "pincode.txt",
            "file_size": 20,
            "file_type": "text/plain",
            "extracted_content": "Secret pin code: 600001"
        }
    ]
})
print("\n--- Test Case A (Empty prompt with attachment) ---")
print("Case A _is_anthropic_messages_api_shape:", ns['_is_anthropic_messages_api_shape']("/api/organizations/org-123/chat_conversations/conv-456/completion", pa))
print("Case A chat_carries_attachment:", ns['chat_carries_attachment'](pa))
print("Case A _file_policy_applies_on_send:", ns['_file_policy_applies_on_send']("/api/organizations/org-123/chat_conversations/conv-456/completion", pa, domain="claude.ai", host="claude.ai"))

# Test Case B: Claude with files array carrying extracted_content or objects
pb = json.dumps({
    "prompt": "Here",
    "files": [
        {
            "file_name": "pin.csv",
            "extracted_content": "name,pin\nJohn,600001"
        }
    ]
})
print("\n--- Test Case B (files array with extracted_content) ---")
print("Case B chat_carries_attachment:", ns['chat_carries_attachment'](pb))
inlines_b = ns['extract_all_inline_attachment_bytes'](pb)
print("Case B inlines extracted count:", len(inlines_b))

# Test Case C: Claude with content array of document blocks (type: document, source: text / base64)
pc = json.dumps({
    "prompt": "Here",
    "content": [
        {
            "type": "document",
            "title": "confidential.txt",
            "source": {
                "type": "text",
                "media_type": "text/plain",
                "data": "Customer pin code 600001"
            }
        }
    ]
})
print("\n--- Test Case C (content array with type: document, source: text) ---")
print("Case C chat_carries_attachment:", ns['chat_carries_attachment'](pc))
inlines_c = ns['extract_all_inline_attachment_bytes'](pc)
print("Case C inlines extracted count:", len(inlines_c))

# Test Case D: Claude PDF upload then Send
# Step 1: Upload PDF to /api/organizations/org-123/upload
# Claude client receives response: {"file_uuid": "doc-uuid-999", "file_name": "secrets.pdf"}
# Step 2: Send completion with "files": ["doc-uuid-999"] or "attachments": [{"id": "doc-uuid-999", "file_name": "secrets.pdf"}]
pd = json.dumps({
    "prompt": "Please read this PDF",
    "attachments": [
        {
            "id": "doc-uuid-999",
            "file_name": "secrets.pdf",
            "file_size": 50000,
            "file_type": "application/pdf"
        }
    ],
    "files": ["doc-uuid-999"]
})
print("\n--- Test Case D (Claude PDF reference by id) ---")
print("Case D chat_carries_attachment:", ns['chat_carries_attachment'](pd))
print("Case D extracted file ids:", ns['_extract_file_ids_from_chat'](pd))
print("Case D inlines extracted count:", len(ns['extract_all_inline_attachment_bytes'](pd)))

# Test Case E: What if Claude is using Connect-RPC / protobuf or different endpoint?
# E.g. Claude desktop app or new web UI
pe = json.dumps({
    "prompt": "Analyze",
    "attachments": [{
        "file_name": "pin.txt",
        "extracted_content": "600001"
    }]
})
print("\n--- Test Case E (different endpoint) ---")
for test_path in [
    "/api/organizations/org-123/chat_conversations/conv-456/completion",
    "/api/organizations/org-123/chat_conversations/conv-456/retry",
    "/api/organizations/org-123/chat_conversations/conv-456/message",
    "/api/organizations/org-123/chat_conversations/conv-456/messages",
    "/api/organizations/org-123/chat_conversations/conv-456/stream",
    "/api/append_message",
    "/v1/messages",
]:
    applies = ns['_file_policy_applies_on_send'](test_path, pe, domain="claude.ai", host="claude.ai")
    print(f"Path: {test_path} -> applies: {applies}")
