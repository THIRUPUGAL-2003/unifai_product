import sys
from pathlib import Path
ROOT = Path(r"d:\unifai_project\apps\browser-guard")
sys.path.insert(0, str(ROOT / "proxy"))
sys.path.insert(0, str(ROOT / "proxy" / "gateway_proxy_parts" / "gateway_proxy_parts"))

import browser_ai_proxy
browser_ai_proxy._load_parts()
ns = browser_ai_proxy.__dict__
import json

# Simulate Claude PDF upload lifecycle:
# Step 1: User uploads PDF "secrets.pdf" containing pin code 600001
pdf_bytes = b"%PDF-1.4 simulated pdf with pin code 600001 %%EOF"
domain = "claude.ai"

# At upload time, no file_id exists yet:
ns['cache_upload_file'](
    domain,
    file_name="secrets.pdf",
    raw_bytes=pdf_bytes,
    content_type="application/pdf",
    upload_reason="upload_endpoint",
    file_id=""
)

# Step 2: Server responds with file_uuid "file-uuid-claude-888"
# rename_recent_nameless_caches is called:
changed = ns['rename_recent_nameless_caches'](domain, "secrets.pdf", "file-uuid-claude-888")
print(f"rename_recent_nameless_caches changed count: {changed}")

# Step 3: User sends prompt referencing file-uuid-claude-888
send_payload = json.dumps({
    "prompt": "Here is the PDF",
    "files": ["file-uuid-claude-888"],
    "attachments": [{"id": "file-uuid-claude-888", "file_name": "secrets.pdf"}]
})

# Let's see if take_all_cached_uploads_for_send finds it with allow_latest=False:
cached = ns['take_all_cached_uploads_for_send'](domain, send_payload, allow_latest=False)
print(f"take_all_cached_uploads_for_send (allow_latest=False) found count: {len(cached)}")
if cached:
    print(f"Found file: {cached[0].get('file_name')} with {len(cached[0].get('raw_bytes', b''))} bytes")
else:
    print("BUG CONFIRMED: Cache miss! File bytes were LOST and NOT evaluated!")
