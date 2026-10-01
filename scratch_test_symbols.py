import sys
sys.path.insert(0, r"c:\Users\sakth\Downloads\unifai_tesing (1)\unifai_tesing\unifai_product\apps\browser-guard\proxy")

import browser_ai_proxy
browser_ai_proxy._load_parts()
ns = browser_ai_proxy.__dict__

looks_like_user_prompt = ns["looks_like_user_prompt"]
_should_intercept_extracted_prompt = ns["_should_intercept_extracted_prompt"]
_clean_prompt_text = ns["_clean_prompt_text"]
_is_ide_non_chat_noise = ns["_is_ide_non_chat_noise"]
_looks_like_binary_or_wire_garbage = ns["_looks_like_binary_or_wire_garbage"]
extract_prompt = ns["extract_prompt"]
extract_prompt_universal = ns["extract_prompt_universal"]

symbols_tests = [
    "@$$$%@#$%$%@$%",
    "!@#$%^%%#$%@#!^",
    "$$$",
    "???",
    "!@#$",
    "வணக்கம்",  # Tamil
    "வணக்கம் எப்படி இருக்கிறீர்கள்?", # Tamil greeting
    "你好",      # Chinese
    "مرحبا",     # Arabic
    "Привет",    # Russian
    "नमस्ते",     # Hindi
    "fgfuhc)",
    "$$",
]

print("--- Testing symbols and languages ---")
for s in symbols_tests:
    body = f'{{"parts": ["{s}"]}}'.encode("utf-8")
    ext = extract_prompt(body, "application/json", "chatgpt.com")
    ext_univ = extract_prompt_universal(body, "application/json", "chatgpt.com")
    res = looks_like_user_prompt(s)
    clean = _clean_prompt_text(s)
    wire = _looks_like_binary_or_wire_garbage(s)
    ide = _is_ide_non_chat_noise(s)
    intercept = _should_intercept_extracted_prompt(s, "/backend-api/f/conversation", f'{{"parts": ["{s}"]}}', "chatgpt.com", "chatgpt.com")
    safe_s = s.encode('ascii', errors='backslashreplace').decode()
    safe_ext = ext.encode('ascii', errors='backslashreplace').decode() if ext else "None"
    print(f"Text: {safe_s!r:32} | ext: {safe_ext!r:16} | looks_like: {res!s:5} | wire_garbage: {wire!s:5} | ide: {ide!s:5} | intercept: {intercept}")
