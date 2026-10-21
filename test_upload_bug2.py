import sys
from pathlib import Path
ROOT = Path(r"d:\unifai_project\apps\browser-guard")
sys.path.insert(0, str(ROOT / "proxy"))
sys.path.insert(0, str(ROOT / "proxy" / "gateway_proxy_parts" / "gateway_proxy_parts"))

import browser_ai_proxy
browser_ai_proxy._load_parts()
ns = browser_ai_proxy.__dict__
import json

# At upload time:
pdf_bytes = b"%PDF-1.4 simulated pdf with pin code 600001 %%EOF" * 10
domain = "claude.ai"
ns['cache_upload_file'](
    domain,
    file_name="secrets.pdf",
    raw_bytes=pdf_bytes,
    content_type="application/pdf",
    upload_reason="upload_endpoint",
    file_id=""
)

# Server responds with file_uuid "file-uuid-claude-888":
ns['rename_recent_nameless_caches'](domain, "secrets.pdf", "file-uuid-claude-888")

# Now user submits ONLY the file UUID (as Claude often does in messages/files):
send_payload = json.dumps({
    "prompt": "Here is the PDF",
    "files": ["file-uuid-claude-888"]
})

cached = ns['take_all_cached_uploads_for_send'](domain, send_payload, allow_latest=False)
print(f"take_all_cached_uploads_for_send with ONLY file UUID: found={len(cached)}")
