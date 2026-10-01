import sys
sys.path.insert(0, r"c:\Users\sakth\Downloads\unifai_tesing (1)\unifai_tesing\unifai_product\apps\browser-guard\proxy")

import browser_ai_proxy
browser_ai_proxy._load_parts()
ns = browser_ai_proxy.__dict__

path = "/backend-api/f/conversation"
host = "chatgpt.com"
url = f"https://{host}{path}"

prompts = [
    "@$$$%@#$%$%@$%",
    "!@#$%^%%#$%@#!^",
    "$$",
    "fgfuhc)"
]

for p in prompts:
    body_str = f'{{"action":"next","messages":[{{"id":"aaa","author":{{"role":"user"}},"content":{{"content_type":"text","parts":["{p}"]}}}}]}}'
    raw_bytes = body_str.encode("utf-8")
    raw_text = body_str
    
    is_target, domain, platform = ns["detect_target"](host)
    peek_prompt = ns["extract_prompt_universal"](raw_bytes, "application/json", host=host, url=url)
    has_prompt = ns["_should_intercept_extracted_prompt"](peek_prompt, path, raw_text, domain, host=host, raw_bytes=raw_bytes)
    chat_path = ns["is_chat_path"](path, host, raw_text)
    confident = ns["_is_confident_chat_send"](path, raw_text, raw_bytes)
    
    print(f"Prompt: {p!r:20} -> peek: {peek_prompt!r:20} | has_prompt: {has_prompt} | is_chat: {chat_path} | confident: {confident}")
