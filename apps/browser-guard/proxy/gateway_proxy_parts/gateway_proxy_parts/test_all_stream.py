import sys
sys.path.insert(0, r'd:\unifai_project\apps\browser-guard\proxy')
import browser_ai_proxy as p
from unittest.mock import MagicMock

test_cases = [
    # 1. ChatGPT CDN upload
    ("files.oaiusercontent.com", "/file-AbCdEf123456", "POST", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"),
    # 2. ChatGPT direct file API
    ("chatgpt.com", "/backend-api/files", "POST", "application/json"),
    # 3. Claude web upload
    ("claude.ai", "/api/organizations/123-abc/upload", "POST", "multipart/form-data; boundary=----WebKitFormBoundary"),
    # 4. Claude doc convert
    ("claude.ai", "/api/convert_document", "POST", "multipart/form-data"),
    # 5. Claude CDN upload
    ("files.claudeusercontent.com", "/upload/abc123", "POST", "application/pdf"),
    # 6. DeepSeek upload
    ("chat.deepseek.com", "/api/v0/file/upload", "POST", "multipart/form-data"),
    # 7. Perplexity upload
    ("www.perplexity.ai", "/api/upload", "POST", "multipart/form-data"),
    # 8. Mistral upload
    ("chat.mistral.ai", "/api/trpc/file.uploadFile", "POST", "application/json"),
    # 9. Gemini upload
    ("gemini.google.com", "/upload/fife", "POST", "application/octet-stream"),
]

print("=== Testing _should_stream_without_inspection ===")
for host, path, method, ctype in test_cases:
    f = MagicMock()
    f.request.pretty_host = host
    f.request.path = path
    f.request.method = method
    f.request.headers = {"content-type": ctype, "content-length": "1048576"}
    f.request.stream = False
    
    stream_val = p._should_stream_without_inspection(f)
    print(f"{host}{path} -> should_stream={stream_val}")
