import os
import sys
from pathlib import Path

parts_dir = Path("apps/browser-guard/proxy/gateway_proxy_parts").resolve()
manifest_path = parts_dir / "MANIFEST.txt"
names = [
    ln.strip().lstrip("\ufeff")
    for ln in manifest_path.read_text(encoding="utf-8-sig").splitlines()
    if ln.strip().lstrip("\ufeff") and not ln.strip().startswith("#")
]

ns = {"__file__": str(parts_dir / "browser_ai_proxy.py"), "__name__": "__main__"}
for name in names:
    code = (parts_dir / name).read_text(encoding="utf-8")
    exec(compile(code, name, "exec"), ns)

print("Proxy parts loaded successfully!")

chat_carries_attachment = ns["chat_carries_attachment"]
_extract_file_ids_from_chat = ns["_extract_file_ids_from_chat"]
_file_policy_applies_on_send = ns["_file_policy_applies_on_send"]
_domain_has_pending_upload_cache = ns["_domain_has_pending_upload_cache"]

# Test 1: pure prompt in ChatGPT
p1 = '{"action": "next", "messages": [{"author": {"role": "user"}, "content": {"content_type": "text", "parts": ["how are u"]}}]}'
print("Test 1 chat_carries_attachment:", chat_carries_attachment(p1))
print("Test 1 _extract_file_ids_from_chat:", _extract_file_ids_from_chat(p1))
print("Test 1 _file_policy_applies_on_send (no cache):", _file_policy_applies_on_send("/backend-api/conversation", p1, p1.encode(), domain="chatgpt.com", host="chatgpt.com"))

# Test 2: prompt with attachment in ChatGPT
p2 = '{"action": "next", "messages": [{"author": {"role": "user"}, "content": {"content_type": "multimodal_text", "parts": [{"asset_pointer": "file-service://file-abc123xyz", "size_bytes": 1024}, "analyze this"]}}]}'
print("Test 2 chat_carries_attachment:", chat_carries_attachment(p2))
print("Test 2 _extract_file_ids_from_chat:", _extract_file_ids_from_chat(p2))
print("Test 2 _file_policy_applies_on_send:", _file_policy_applies_on_send("/backend-api/conversation", p2, p2.encode(), domain="chatgpt.com", host="chatgpt.com"))

# Test 3: What if there IS a pending upload in cache for chatgpt.com?
import time
ns["_UPLOAD_FILE_CACHE"]["chatgpt.com|latest"] = {
    "file_name": "xlsc.aspx",
    "ts": time.time(),
    "raw_bytes": b"fake content",
    "cache_uid": "test-1",
}

# Define fixed version of _file_policy_applies_on_send:
def fixed_file_policy_applies_on_send(path, raw_text, raw_bytes=b"", *, domain="", host=""):
    path_l = (path or "").lower().split("?", 1)[0]
    if ns["_path_has_ignore_pattern"](path_l) or ns["is_noise"](path_l, raw_text or ""):
        return False
    if "/realtime" in path_l or ns["_is_gateway_inject_frame"](raw_text or "") or ns["_is_typing_or_draft_path"](path_l, raw_text or ""):
        return False
    if ns["_path_looks_like_upload"](path_l):
        return False

    body = raw_text or ""
    has_file = (
        ns["_send_carries_attachment"](body)
        or ns["messages_parts_carries_file"](body)
        or ns["chat_carries_attachment"](body)
        or ns["event_send_carries_binary_attach"](body)
    )
    confident = ns["_is_confident_chat_send"](path, raw_text, raw_bytes) or ns["_copilot_frame_is_user_send"](body)
    chatish = confident or ns["is_chat_path"](path, host, body) or ns["_path_has_chat_marker"](path)
    if ns["_is_persistent_chat_websocket"](path) and ("\x1e" in body or '"target":' in body.lower() or '"invocationid"' in body.lower()) and not confident:
        chatish = False

    if ns["_path_looks_like_upload"](path) and not (confident or (has_file and chatish)):
        return False

    finished = ns["_is_finished_user_file_send"](path, raw_text, raw_bytes, host)
    if has_file and chatish:
        return finished
    return False

print("Test 1 (PURE PROMPT) with FIX:", fixed_file_policy_applies_on_send("/backend-api/conversation", p1, p1.encode(), domain="chatgpt.com", host="chatgpt.com"))
print("Test 2 (WITH ATTACHMENT) with FIX:", fixed_file_policy_applies_on_send("/backend-api/conversation", p2, p2.encode(), domain="chatgpt.com", host="chatgpt.com"))

