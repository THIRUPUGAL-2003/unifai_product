#!/usr/bin/env python3
"""
Comprehensive Live Verification for Skill Repository
Tests both Authenticated Admin CRUD & Public Marketplace Serving Endpoints.
"""

import sys
import os
import json
import urllib.request
import urllib.error

if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

sys.path.append(os.path.join(os.path.dirname(__file__)))
from db_config import get_base_url, _CONFIG

BASE_URL = get_base_url()
ADMIN_USER = _CONFIG.get("ADMIN_EMAIL", "")
ADMIN_PASSWORD = _CONFIG.get("ADMIN_PASSWORD", "")

passed = 0
failed = 0

def record(test_id: str, desc: str, ok: bool, detail: str = ""):
    global passed, failed
    if ok:
        passed += 1
        print(f"[PASS] ✅ {test_id:<14}: {desc}")
    else:
        failed += 1
        print(f"[FAIL] ❌ {test_id:<14}: {desc}")
    if detail:
        print(f"               Detail: {detail}")

print("=" * 80)
print(f"🛠️ SKILL REPOSITORY - COMPREHENSIVE LIVE VERIFICATION")
print(f"   Target Gateway: {BASE_URL}")
print("=" * 80)

# Step 1: Admin Login
login_payload = json.dumps({"username": ADMIN_USER, "password": ADMIN_PASSWORD}).encode()
login_req = urllib.request.Request(
    f"{BASE_URL}/api/session/login",
    data=login_payload,
    headers={"Content-Type": "application/json"}
)

cookie = None
try:
    with urllib.request.urlopen(login_req) as resp:
        cookie = resp.headers.get("Set-Cookie")
        record("AUTH-01", "Admin Login for Skills API", resp.status == 200)
except Exception as e:
    record("AUTH-01", "Admin Login for Skills API", False, str(e))
    sys.exit(1)

def req(method, path, body=None, is_json=True):
    url = f"{BASE_URL}{path}"
    headers = {}
    if cookie:
        headers["Cookie"] = cookie
    data = None
    if body is not None:
        if is_json:
            headers["Content-Type"] = "application/json"
            data = json.dumps(body).encode()
        else:
            data = body
    r = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(r, timeout=10) as resp:
            content = resp.read()
            return resp.status, content, resp.headers
    except urllib.error.HTTPError as e:
        return e.code, e.read(), e.headers
    except Exception as e:
        return 0, str(e).encode(), {}

# 1. Check All Skills Version
status, body, _ = req("GET", "/api/skills/all/version")
record("SKILL-VER", "GET /api/skills/all/version", status == 200, body.decode()[:100])

# 2. List Skills
status, body, _ = req("GET", "/api/skills?limit=10&offset=0")
record("SKILL-LIST", "GET /api/skills (list skills)", status == 200, body.decode()[:100])

# 3. Create a Test Skill
test_skill_name = "test-agent-skill-live"
create_payload = {
    "name": test_skill_name,
    "description": "Live test skill for full repository verification",
    "version": "1.0.0",
    "skill_md_body": "# Live Test Skill\nThis skill instructions are verified live.",
    "files": [
        {
            "path": "scripts/run.py",
            "content": "print('hello from live test skill')",
            "source_type": "text",
            "mime_type": "text/x-python"
        }
    ]
}
status, body, _ = req("POST", "/api/skills", create_payload)
created_data = json.loads(body.decode()) if status in [200, 201] else {}
skill_id = created_data.get("id")
record("SKILL-CREATE", f"POST /api/skills (create skill: {test_skill_name})", status in [200, 201] and bool(skill_id), f"Status: {status}, Body: {body.decode()[:200]}")

if skill_id:
    # 4. Get Skill by ID
    status, body, _ = req("GET", f"/api/skills/{skill_id}")
    record("SKILL-GET", f"GET /api/skills/{skill_id}", status == 200, f"Retrieved name: {json.loads(body.decode()).get('name') if status==200 else ''}")

    # 5. List Skill Versions
    status, body, _ = req("GET", f"/api/skills/{skill_id}/versions")
    record("SKILL-VERSIONS", f"GET /api/skills/{skill_id}/versions", status == 200, body.decode()[:100])

    # 6. Update Skill
    update_payload = {
        "description": "Updated live test skill description",
        "version": "1.0.1",
        "skill_md_body": "# Live Test Skill Updated\nUpdated instructions.",
        "files": []
    }
    status, body, _ = req("PUT", f"/api/skills/{skill_id}", update_payload)
    record("SKILL-UPDATE", f"PUT /api/skills/{skill_id}", status == 200, body.decode()[:100])

    # 7. Public Marketplace Endpoints (No Auth Needed!)
    # Claude Code Marketplace
    status, body, _ = req("GET", "/api/skills/serve/claude-code/.claude-plugin/marketplace.json")
    record("MKT-CLAUDE", "GET /api/skills/serve/claude-code marketplace.json", status == 200 and test_skill_name in body.decode(), f"Status: {status}")

    # Codex Marketplace
    status, body, _ = req("GET", "/api/skills/serve/codex/.agents/plugins/marketplace.json")
    record("MKT-CODEX", "GET /api/skills/serve/codex marketplace.json", status == 200 and test_skill_name in body.decode(), f"Status: {status}")

    # Cursor Marketplace
    status, body, _ = req("GET", "/api/skills/serve/cursor/.cursor-plugin/marketplace.json")
    record("MKT-CURSOR", "GET /api/skills/serve/cursor marketplace.json", status == 200 and test_skill_name in body.decode(), f"Status: {status}")

    # Codex Git smart HTTP info/refs
    status, body, _ = req("GET", "/api/skills/serve/codex/info/refs?service=git-upload-pack")
    record("GIT-CODEX", "GET /api/skills/serve/codex/info/refs (Git Smart HTTP)", status == 200, f"Length: {len(body)} bytes")

    # Cursor Git smart HTTP info/refs
    status, body, _ = req("GET", "/api/skills/serve/cursor/info/refs?service=git-upload-pack")
    record("GIT-CURSOR", "GET /api/skills/serve/cursor/info/refs (Git Smart HTTP)", status == 200, f"Length: {len(body)} bytes")

    # Generic Zip Download for Test Skill
    status, body, _ = req("GET", f"/api/skills/serve/{test_skill_name}/download.zip")
    record("ZIP-SKILL", f"GET /api/skills/serve/{test_skill_name}/download.zip", status == 200 and body[:2] == b"PK", f"Zip size: {len(body)} bytes")

    # Generic Single File Download
    status, body, _ = req("GET", f"/api/skills/serve/{test_skill_name}/files/scripts/run.py")
    record("FILE-SERVE", f"GET /api/skills/serve/{test_skill_name}/files/scripts/run.py", status == 200 and b"hello from live test skill" in body, body.decode()[:100])

    # All Skills Zip Download
    status, body, _ = req("GET", "/api/skills/serve/all/download.zip")
    record("ZIP-ALL", "GET /api/skills/serve/all/download.zip", status == 200 and body[:2] == b"PK", f"Zip size: {len(body)} bytes")

    # 8. Clean up Orphan Files
    status, body, _ = req("DELETE", "/api/skills/files/orphans")
    record("SKILL-ORPHAN", "DELETE /api/skills/files/orphans", status == 200, body.decode()[:100])

    # 9. Delete Test Skill
    status, body, _ = req("DELETE", f"/api/skills/{skill_id}")
    record("SKILL-DELETE", f"DELETE /api/skills/{skill_id}", status == 200, f"Status: {status}")

print("=" * 80)
print(f"📊 SUMMARY: {passed} PASSED | {failed} FAILED (Total: {passed + failed})")
print("=" * 80)
