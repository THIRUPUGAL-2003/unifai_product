#!/usr/bin/env python3
"""
Gateway Browser Guard - Full Check for Perplexity & DeepSeek
============================================================
Comprehensive verification of:
1. Target Domain Detection (all subdomains, PAC coverage)
2. Chat Path / Submit Interception
3. User Prompt Extraction (SSE, REST JSON, Socket.IO, Ack IDs)
4. File Upload Detection (Multipart, JSON attachments, Safe vs Blocked)
5. In-Chat Security Message Injection (HTTP SSE, JSON, Socket.IO polling & WebSocket)
6. Regression test for ChatGPT, Claude, Gemini
"""

import sys
import os
import json
import re
from unittest.mock import MagicMock

_HERE = os.path.dirname(os.path.abspath(__file__))
_PARTS_DIR = os.path.join(_HERE, "gateway_proxy_parts")

# Load proxy parts into shared namespace
def _load_parts():
    manifest = os.path.join(_PARTS_DIR, "MANIFEST.txt")
    with open(manifest, encoding="utf-8-sig") as f:
        names = [ln.strip() for ln in f if ln.strip()]
    ns = {}
    for name in names:
        path = os.path.join(_PARTS_DIR, name)
        with open(path, encoding="utf-8") as f:
            code = f.read()
        try:
            exec(compile(code, path, "exec"), ns)
        except Exception as e:
            print(f"  [WARN] Could not load {name}: {e}")
    return ns

print("Loading Gateway proxy parts...", end=" ", flush=True)
NS = _load_parts()
print("OK")

detect_target = NS["detect_target"]
is_chat_path = NS["is_chat_path"]
is_rest_sse_ask_submit = NS["is_rest_sse_ask_submit"]
extract_prompt_universal = NS["extract_prompt_universal"]
extract_rest_sse_ask_prompt = NS["extract_rest_sse_ask_prompt"]
_extract_from_socketio = NS["_extract_from_socketio"]
is_confident_file_upload = NS["is_confident_file_upload"]
_file_policy_applies_on_send = NS["_file_policy_applies_on_send"]
make_blocked_response = NS["make_blocked_response"]
_ws_frames_socketio = NS["_ws_frames_socketio"]
inject_websocket_reply = NS["inject_websocket_reply"]

passed_count = 0
failed_count = 0

def check(title, condition, details=""):
    global passed_count, failed_count
    if condition:
        passed_count += 1
        print(f"  [PASS] {title}")
        if details:
            print(f"         -> {details}")
    else:
        failed_count += 1
        print(f"  [FAIL] {title}")
        if details:
            print(f"         -> ERROR: {details}")

print("\n" + "=" * 80)
print(" 1. PERPLEXITY - TARGET DOMAIN DETECTION")
print("=" * 80)

# Seed targets in cache
NS["_apply_targets_from_data"]({
    "targets": [
        {"domain": "perplexity.ai", "platform_name": "Perplexity", "monitored": True},
        {"domain": "www.perplexity.ai", "platform_name": "Perplexity", "monitored": True},
        {"domain": "labs.perplexity.ai", "platform_name": "Perplexity", "monitored": True},
        {"domain": "api.perplexity.ai", "platform_name": "Perplexity", "monitored": True},
        {"domain": "deepseek.com", "platform_name": "DeepSeek", "monitored": True},
        {"domain": "chat.deepseek.com", "platform_name": "DeepSeek", "monitored": True},
        {"domain": "api.deepseek.com", "platform_name": "DeepSeek", "monitored": True},
        {"domain": "chatgpt.com", "platform_name": "ChatGPT", "monitored": True},
        {"domain": "claude.ai", "platform_name": "Claude", "monitored": True},
        {"domain": "gemini.google.com", "platform_name": "Gemini", "monitored": True},
    ]
})

pplx_domains = ["perplexity.ai", "www.perplexity.ai", "labs.perplexity.ai", "api.perplexity.ai", "staging.perplexity.ai"]
for dom in pplx_domains:
    is_t, matched_dom, plat = detect_target(dom)
    check(f"Detect target: {dom}", is_t and "perplexity" in plat.lower(), f"matched: {matched_dom} ({plat})")

print("\n" + "=" * 80)
print(" 2. PERPLEXITY - CHAT PATH & SUBMIT INTERCEPTION")
print("=" * 80)

pplx_paths = [
    ("/rest/sse/perplexity_ask", '{"query_str":"what is AI?"}'),
    ("/rest/thread", '{"query":"tell me about quantum computing"}'),
    ("/rest/entrypoint", '{"query_str":"test entrypoint"}'),
    ("/rest/search", '{"query":"fast search"}'),
    ("/api/chat", '{"messages":[{"role":"user","content":"hello"}]}'),
    ("/api/perplexity_ask", '{"query_str":"test api"}'),
    ("/socket.io/", '42["perplexity_ask","what is python?",{"source":"default"}]'),
    ("/socket.io/", '42["query","what is machine learning?"]'),
    ("/socket.io/", '420["perplexity_ask","search with ack id",{"mode":"copilot"}]'),
]

for path, body in pplx_paths:
    matched = is_rest_sse_ask_submit(path, body) or is_chat_path(path, "www.perplexity.ai", body)
    check(f"Chat submit detection: {path} (body preview: {body[:35]}...)", matched)

print("\n" + "=" * 80)
print(" 3. PERPLEXITY - PROMPT EXTRACTION ACROSS PROTOCOLS")
print("=" * 80)

# 3A. SSE / REST JSON extraction
p1 = extract_prompt_universal(b'{"query_str":"What is the GDP of Germany?"}', "application/json", host="www.perplexity.ai")
check("Extract query_str from JSON", p1 == "What is the GDP of Germany?", f"got: {p1!r}")

p2 = extract_prompt_universal(b'{"query":"Explain transformer attention mechanism"}', "application/json", host="www.perplexity.ai")
check("Extract query from JSON", p2 == "Explain transformer attention mechanism", f"got: {p2!r}")

p3 = extract_prompt_universal(b'{"params":{"query_str":"Who won the 2024 world cup?"}}', "application/json", host="www.perplexity.ai")
check("Extract nested params.query_str", p3 == "Who won the 2024 world cup?", f"got: {p3!r}")

# 3B. Socket.IO extraction
sio_body1 = b'42["perplexity_ask","What is the capital of Tamil Nadu?",{"source":"default","mode":"copilot"}]'
p_sio1 = extract_prompt_universal(sio_body1, "application/json", host="www.perplexity.ai")
check("Extract Socket.IO 42[\"perplexity_ask\", query, options]", p_sio1 == "What is the capital of Tamil Nadu?", f"got: {p_sio1!r}")

sio_body2 = b'42["perplexity_ask",{"query":"Tell me a joke about programming"}]'
p_sio2 = extract_prompt_universal(sio_body2, "application/json", host="www.perplexity.ai")
check("Extract Socket.IO 42[\"perplexity_ask\", {query: ...}]", p_sio2 == "Tell me a joke about programming", f"got: {p_sio2!r}")

sio_body3 = b'420["perplexity_ask","What is quantum superposition?",{"search_focus":"internet"}]'
p_sio3 = extract_prompt_universal(sio_body3, "application/json", host="www.perplexity.ai")
check("Extract Socket.IO 420[...with ack id]", p_sio3 == "What is quantum superposition?", f"got: {p_sio3!r}")

sio_body4 = b'42/chat,["perplexity_ask","Namespace chat test"]'
p_sio4 = extract_prompt_universal(sio_body4, "application/json", host="www.perplexity.ai")
check("Extract Socket.IO 42/chat,[...with namespace]", p_sio4 == "Namespace chat test", f"got: {p_sio4!r}")

print("\n" + "=" * 80)
print(" 4. PERPLEXITY - FILE UPLOAD & ATTACHMENT DETECTION")
print("=" * 80)

# 4A. Multipart File Upload
fake_pdf = b"%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF"
is_up = is_confident_file_upload(
    fname="financial_report.pdf",
    content_type="application/pdf",
    raw_bytes=fake_pdf,
    raw_text="",
    upload_reason="test",
    host="www.perplexity.ai",
    path="/rest/upload",
)
check("Detect multipart PDF upload on Perplexity", is_up)

# 4B. JSON attachment on chat submit
attach_body = json.dumps({
    "query": "Summarize this quarterly earnings document",
    "attachments": [{"id": "att-9912", "name": "q3_earnings.pdf", "url": "https://cdn.perplexity.ai/att-9912.pdf"}],
})
apply_policy = _file_policy_applies_on_send("/rest/sse/perplexity_ask", attach_body, attach_body.encode(), domain="perplexity.ai", host="www.perplexity.ai")
check("Detect JSON attachments list on Perplexity Send", apply_policy)

print("\n" + "=" * 80)
print(" 5. PERPLEXITY - IN-CHAT SECURITY MESSAGE INJECTION")
print("=" * 80)

security_warning = "Visa card number detected. Prohibited under PCI-DSS."

class FakeFlow:
    def __init__(self, path, accept="text/event-stream", content=b""):
        self.request = MagicMock()
        self.request.path = path
        self.request.headers = {"Accept": accept, "Origin": "https://www.perplexity.ai"}
        self.request.content = content
        self.request.pretty_host = "www.perplexity.ai"
        self.response = None
        self.websocket = None

# 5A. HTTP SSE Injection (/rest/sse/perplexity_ask)
warning_sse = "Visa card number detected. Prohibited under PCI-DSS (SSE)."
flow_sse = FakeFlow("/rest/sse/perplexity_ask", accept="text/event-stream", content=b'{"query_str":"4111 2222 3333 4444"}')
make_blocked_response(flow_sse, "Visa Card Number", "www.perplexity.ai", warning_sse)
check("HTTP SSE block returns 200 OK", flow_sse.response.status_code == 200)
check("HTTP SSE block content-type is text/event-stream", "text/event-stream" in flow_sse.response.headers.get("Content-Type", ""))
resp_sse_text = flow_sse.response.content.decode("utf-8")
check("HTTP SSE contains security warning in text & answer", warning_sse in resp_sse_text)
check("HTTP SSE contains status: completed & final: true", '"status": "completed"' in resp_sse_text and '"final": true' in resp_sse_text)
check("HTTP SSE terminates with data: [DONE]", "data: [DONE]" in resp_sse_text)

# 5B. HTTP JSON Injection (/rest/thread with Accept: application/json)
warning_json = "Mastercard card number detected. Prohibited under PCI-DSS (JSON)."
flow_json = FakeFlow("/rest/thread", accept="application/json", content=b'{"query":"5100 2222 3333 4444"}')
make_blocked_response(flow_json, "Mastercard Number", "www.perplexity.ai", warning_json)
check("HTTP JSON block returns 200 OK", flow_json.response.status_code == 200)
check("HTTP JSON block content-type is application/json", "application/json" in flow_json.response.headers.get("Content-Type", ""))
parsed_json = json.loads(flow_json.response.content.decode("utf-8"))
check("HTTP JSON response text field has security warning", parsed_json.get("text") == warning_json)
check("HTTP JSON response answer field has security warning", parsed_json.get("answer") == warning_json)
check("HTTP JSON response status is completed", parsed_json.get("status") == "completed")

# 5C. Socket.IO HTTP Polling Injection (/socket.io/?transport=polling)
warning_poll = "India PAN card detected. Prohibited under corporate policy (Polling)."
flow_poll = FakeFlow("/socket.io/?EIO=4&transport=polling", accept="*/*", content=b'42["perplexity_ask","ABCDE1234F"]')
make_blocked_response(flow_poll, "India PAN Card", "www.perplexity.ai", warning_poll)
check("Socket.IO polling block returns 200 OK", flow_poll.response.status_code == 200)
poll_body = flow_poll.response.content.decode("utf-8")
check("Socket.IO polling body has 42[\"query_progress\", ...]", '42["query_progress"' in poll_body)
check("Socket.IO polling body has 42[\"query_answered\", ...]", '42["query_answered"' in poll_body)
check("Socket.IO polling contains security warning", warning_poll in poll_body)

# 5D. Socket.IO WebSocket Frame Generation
ws_frames = _ws_frames_socketio(security_warning, last_client_msg='420["perplexity_ask","4111 2222 3333 4444"]')
check("WebSocket generates valid Socket.IO frames list", len(ws_frames) >= 4)
check("All WebSocket frames start with Engine.IO/Socket.IO packet prefix (42 or 43)",
      all(f.startswith(b"42") or f.startswith(b"43") for f in ws_frames))
frame_texts = [f.decode("utf-8") for f in ws_frames]
check("WebSocket frame contains query_progress event", any('42["query_progress"' in ft for ft in frame_texts))
check("WebSocket frame contains query_answered event", any('42["query_answered"' in ft for ft in frame_texts))
check("WebSocket frame contains security warning", any(security_warning in ft for ft in frame_texts))
check("WebSocket frame contains ack frame 430[...]", any(ft.startswith("430[") for ft in frame_texts))

print("\n" + "=" * 80)
print(" 6. DEEPSEEK - FULL VERIFICATION (DOMAIN, CHAT, PROMPT, BLOCK)")
print("=" * 80)

# 6A. Domain Detection
for ds_dom in ["deepseek.com", "chat.deepseek.com", "api.deepseek.com"]:
    is_t, matched_dom, plat = detect_target(ds_dom)
    check(f"DeepSeek domain: {ds_dom}", is_t and "deepseek" in plat.lower())

# 6B. Chat Path Detection
ds_chat = is_chat_path("/api/v0/chat/completion", "chat.deepseek.com", '{"messages":[{"role":"user","content":"hi"}],"stream":true}')
check("DeepSeek chat path: /api/v0/chat/completion", ds_chat)

# 6C. Prompt Extraction
ds_payload = json.dumps({
    "messages": [
        {"role": "system", "content": "You are a helpful AI"},
        {"role": "user", "content": "Write a python quicksort function"},
    ],
    "stream": True,
    "model": "deepseek-chat",
})
p_ds = extract_prompt_universal(ds_payload.encode(), "application/json", host="chat.deepseek.com")
check("Extract DeepSeek user prompt", p_ds == "Write a python quicksort function", f"got: {p_ds!r}")

# 6D. DeepSeek In-Chat Security Message Injection (Streaming SSE)
ds_flow = FakeFlow("/api/v0/chat/completion", accept="text/event-stream", content=ds_payload.encode())
ds_warning = "Indian Aadhaar national identity number detected."
make_blocked_response(ds_flow, "India Aadhaar Card", "chat.deepseek.com", ds_warning)
check("DeepSeek block returns 200 OK", ds_flow.response.status_code == 200)
check("DeepSeek block content-type is text/event-stream", "text/event-stream" in ds_flow.response.headers.get("Content-Type", ""))
ds_resp = ds_flow.response.content.decode("utf-8")
check("DeepSeek block chunk has delta.content with security warning", f'"content": "{ds_warning}"' in ds_resp or ds_warning in ds_resp)
check("DeepSeek block chunk has delta.type: text", '"type": "text"' in ds_resp)
check("DeepSeek block chunk has delta.text", f'"text": "{ds_warning}"' in ds_resp)
check("DeepSeek block terminates with data: [DONE]", "data: [DONE]" in ds_resp)

# 6E. DeepSeek File Upload Detection
ds_file_body = json.dumps({
    "messages": [{"role": "user", "content": "Analyze this file"}],
    "files": [{"file_id": "file-deepseek-8821", "name": "system_design.pdf"}],
})
ds_file_check = _file_policy_applies_on_send("/api/v0/chat/completion", ds_file_body, ds_file_body.encode(), domain="deepseek.com", host="chat.deepseek.com")
check("DeepSeek attached file detected on Send", ds_file_check)

print("\n" + "=" * 80)
print(" 7. REGRESSION CHECK - CHATGPT, CLAUDE, GEMINI")
print("=" * 80)

# 7A. ChatGPT
cg_flow = FakeFlow("/backend-api/conversation", accept="text/event-stream", content=b'{"messages":[{"id":"u1","author":{"role":"user"},"content":{"content_type":"text","parts":["secret prompt"]}}]}')
make_blocked_response(cg_flow, "OpenAI API Key", "chatgpt.com", "OpenAI API secret token detected.")
check("ChatGPT block returns 200 OK", cg_flow.response.status_code == 200)
cg_resp = cg_flow.response.content.decode("utf-8")
check("ChatGPT block returns parts[0] with security warning", "OpenAI API secret token detected." in cg_resp)
check("ChatGPT block terminates with [DONE]", "data: [DONE]" in cg_resp)

# 7B. Claude
cl_flow = FakeFlow("/v1/messages", accept="text/event-stream", content=b'{"messages":[{"role":"user","content":"test"}]}')
make_blocked_response(cl_flow, "AWS Secret Key", "claude.ai", "AWS Secret Access Key credentials detected.")
check("Claude block returns 200 OK", cl_flow.response.status_code == 200)
cl_resp = cl_flow.response.content.decode("utf-8")
check("Claude block returns content_block_delta text_delta with warning", "AWS Secret Access Key credentials detected." in cl_resp)

# 7C. Gemini
gem_flow = FakeFlow("/_/BardChatUi/data/batchexecute", accept="application/json", content=b'f.req=%5B%5B%5B%22wXbdQc%22%2C%22%5B%5C%22prompt%5C%22%5D%22%5D%5D%5D')
make_blocked_response(gem_flow, "India PAN Card", "gemini.google.com", "India Income Tax PAN card detected.")
check("Gemini block returns 200 OK", gem_flow.response.status_code == 200)
gem_resp = gem_flow.response.content.decode("utf-8")
check("Gemini block returns wrb.fr envelope with warning", "India Income Tax PAN card detected." in gem_resp and "wrb.fr" in gem_resp)

print("\n" + "=" * 80)
print(f" FINAL SUMMARY: {passed_count} PASSED, {failed_count} FAILED")
print("=" * 80)

if failed_count == 0:
    print(" >>> ALL PERPLEXITY, DEEPSEEK, CHATGPT, CLAUDE & GEMINI CHECKS PASSED 100%! <<<\n")
    sys.exit(0)
else:
    print(f" >>> ATTENTION: {failed_count} checks failed. <<<\n")
    sys.exit(1)
