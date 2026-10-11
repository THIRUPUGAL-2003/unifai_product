import sys
import os
from pathlib import Path

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")

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

extract_prompt_universal = ns["extract_prompt_universal"]
looks_like_user_prompt = ns["looks_like_user_prompt"]

test_cases = [
    # 1. Plain text
    ("Hello, what is the capital of France?", "Plain text"),
    # 2. Numbers
    ("1234567890", "Numbers"),
    ("98.6 + 42 = ?", "Math/numbers"),
    # 3. Symbols
    ("!@#$%^&*()_+ ~`{}|:<>?", "Symbols"),
    ("SELECT * FROM users WHERE id = 1;", "SQL/Code"),
    # 4. Emojis
    ("🚀🔥🎉🤖❤️", "Emojis"),
    ("Hello world 🌍👋", "Text + Emojis"),
    # 5. Tamil / Multilingual
    ("வணக்கம், நீங்கள் எப்படி இருக்கிறீர்கள்?", "Tamil script"),
    ("Bonjour le monde", "French"),
    # 6. Single character / short
    ("hi", "Short 2 chars"),
    ("a", "Single char"),
    ("?", "Single question mark"),
]

print("=== TESTING PROMPT TYPES ===")
for text, desc in test_cases:
    # Test through ChatGPT JSON format
    import json
    p_json = json.dumps({"action": "next", "messages": [{"author": {"role": "user"}, "content": {"content_type": "text", "parts": [text]}}]})
    extracted = extract_prompt_universal(p_json.encode(), "application/json", host="chatgpt.com", url="https://chatgpt.com/backend-api/conversation")
    ok = (extracted == text)
    print(f"[{'PASS' if ok else 'FAIL'}] {desc}: input={text!r} -> extracted={extracted!r}")
