import sys
import os
import json

sys.path.insert(0, r"d:\unifai_project\apps\browser-guard\proxy")
import browser_ai_proxy

browser_ai_proxy._load_parts()
ns = browser_ai_proxy.__dict__

_extract_file_send_user_caption = ns["_extract_file_send_user_caption"]
extract_prompt_universal = ns["extract_prompt_universal"]
looks_like_user_prompt = ns["looks_like_user_prompt"]
enforce_file_send_policy = ns["enforce_file_send_policy"]

# Test 1: Standard Claude Web completion payload
payload1 = json.dumps({
    "prompt": "review this code and tell me bugs",
    "timezone": "Asia/Kolkata",
    "model": "claude-3-5-sonnet-20241022",
    "attachments": [
        {
            "id": "att-123",
            "file_name": "home.html",
            "file_type": "text/html",
            "file_size": 50,
            "extracted_content": "<html><body><h1>Hello World</h1></body></html>"
        },
        {
            "id": "att-456",
            "file_name": "script.js",
            "file_type": "text/javascript",
            "file_size": 30,
            "extracted_content": "console.log('hi');"
        }
    ],
    "files": []
})

print("--- Test 1: Standard Claude Web completion ---")
cap1 = _extract_file_send_user_caption(payload1, "application/json", "claude.ai", "https://claude.ai/api/organizations/123/chat_conversations/456/completion")
print("Caption 1:", repr(cap1))
univ1 = extract_prompt_universal(payload1.encode('utf-8'), "application/json", "claude.ai", "https://claude.ai/api/organizations/123/chat_conversations/456/completion")
print("Universal 1:", repr(univ1))

# Test 2: Claude with parent_message_uuid
payload2 = json.dumps({
    "parent_message_uuid": "11111111-2222-3333-4444-555555555555",
    "prompt": "check this styles",
    "timezone": "Asia/Kolkata",
    "model": "claude-3-5-sonnet-20241022",
    "attachments": [
        {
            "id": "att-789",
            "file_name": "styles.css",
            "file_type": "text/css",
            "file_size": 100,
            "extracted_content": "body { color: red; }"
        }
    ]
})

print("\n--- Test 2: Claude with parent_message_uuid ---")
cap2 = _extract_file_send_user_caption(payload2, "application/json", "claude.ai", "https://claude.ai/api/organizations/123/chat_conversations/456/completion")
print("Caption 2:", repr(cap2))

# Test 3: What if Claude upload happens in Claude API format (messages array)?
payload3 = json.dumps({
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1024,
    "messages": [
        {
            "role": "user",
            "content": [
                {
                    "type": "text",
                    "text": "please review my website"
                },
                {
                    "type": "document",
                    "source": {
                        "type": "base64",
                        "media_type": "text/html",
                        "data": "PGh0bWw+"
                    },
                    "title": "home.html"
                }
            ]
        }
    ]
})
print("\n--- Test 3: Claude API messages format ---")
cap3 = _extract_file_send_user_caption(payload3, "application/json", "api.anthropic.com", "https://api.anthropic.com/v1/messages")
print("Caption 3:", repr(cap3))
univ3 = extract_prompt_universal(payload3.encode('utf-8'), "application/json", "api.anthropic.com", "https://api.anthropic.com/v1/messages")
print("Universal 3:", repr(univ3))
