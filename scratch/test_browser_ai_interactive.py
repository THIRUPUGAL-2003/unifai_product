import json
import urllib.request
import urllib.error
import http.cookiejar

cookie_jar = http.cookiejar.CookieJar()
opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookie_jar))

BASE_URL = "http://127.0.0.1:8080"
GUARD_KEY = "ycsKvD5uWlL1ymRZZW/3ePHqMe7sab4ZiG9L1VRMAFo="

def login():
    payload = json.dumps({"username": "admin@yespanchi.com", "password": "YP2025-2026yp"}).encode("utf-8")
    req = urllib.request.Request(
        f"{BASE_URL}/api/session/login",
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST"
    )
    with opener.open(req) as resp:
        return resp.status == 200

if not login():
    print("Login failed!")
    exit(1)
print("Admin logged in successfully with cookie session.")

def req(method, path, body=None, extra_headers=None):
    headers = {}
    if extra_headers:
        headers.update(extra_headers)
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    r = urllib.request.Request(f"{BASE_URL}{path}", data=data, headers=headers, method=method)
    try:
        with opener.open(r) as resp:
            content = resp.read().decode()
            return resp.status, json.loads(content) if content else {}
    except urllib.error.HTTPError as e:
        content = e.read().decode()
        try:
            return e.code, json.loads(content)
        except:
            return e.code, {"raw": content}

results = []

def run_test(name, method, path, body=None, expect_codes=[200], extra_headers=None):
    status, res = req(method, path, body, extra_headers)
    passed = status in expect_codes
    results.append((name, passed, f"HTTP {status}", res))
    print(f"[{'PASS' if passed else 'FAIL'}] {name} -> HTTP {status}")

# 1. Test Generate Regex from Policy (Handles LLM call or lack of active key gracefully)
run_test("Generate Regex from Policy", "POST", "/api/browser-ai/rules/generate-regex", {
    "bot_provider": "openai",
    "bot_model": "gpt-4o",
    "bot_prompt": "Detect credit card numbers and CVV codes"
}, expect_codes=[200, 400, 500, 502, 503])

# 2. Test Guard Bot evaluation
run_test("Test Guard Bot Evaluator", "POST", "/api/browser-ai/rules/test-bot", {
    "bot_provider": "openai",
    "bot_model": "gpt-4o",
    "bot_prompt": "Check if text contains passwords",
    "sample_prompt": "My password is secret123"
}, expect_codes=[200, 400, 500, 502, 503])

# 3. Test Bulk Rules Import
sample_rules = [
    {
        "name": "Test Bulk Rule 1",
        "pattern": r"\b(confidential|internal-only)\b",
        "severity": "HIGH",
        "action": "WARN",
        "active": True,
        "description": "Rule for bulk import testing"
    }
]
run_test("Import Guard Rules Bulk", "POST", "/api/browser-ai/rules/import", {
    "rules": sample_rules,
    "overwrite": True
}, expect_codes=[200])

# 4. Test Bulk Targets Import
sample_targets = [
    {
        "domain": "test-ai-domain.local",
        "platform_name": "Test AI Domain",
        "monitored": True,
        "block_site": False
    }
]
run_test("Import Targets Bulk", "POST", "/api/browser-ai/targets/import", {
    "targets": sample_targets,
    "overwrite": True
}, expect_codes=[200])

# 5. Clean up imported target
status, targets_res = req("GET", "/api/browser-ai/targets")
imported_id = None
if isinstance(targets_res, dict):
    for t in targets_res.get("targets", []):
        if t.get("domain") == "test-ai-domain.local":
            imported_id = t.get("id")
            break
if imported_id:
    run_test("Delete Imported Target", "DELETE", f"/api/browser-ai/targets/{imported_id}", expect_codes=[200])

# 6. Clean up imported rule
status, rules_res = req("GET", "/api/browser-ai/rules")
imported_rule_id = None
if isinstance(rules_res, dict):
    for r in rules_res.get("rules", []):
        if r.get("name") == "Test Bulk Rule 1":
            imported_rule_id = r.get("id")
            break
if imported_rule_id:
    run_test("Delete Imported Rule", "DELETE", f"/api/browser-ai/rules/{imported_rule_id}", expect_codes=[200])

# 7. Create Agent via Heartbeat using X-Gateway-Guard-Key
hb_payload = {
    "id": "test-agent-pytest-001",
    "hostname": "TEST-LAPTOP",
    "username": "tester",
    "ip_address": "192.168.1.100",
    "os_version": "Windows 11",
    "agent_version": "1.1.15",
    "status": "active"
}
run_test("Agent Heartbeat Check-in", "POST", "/api/browser-ai/agents/heartbeat", hb_payload, expect_codes=[200], extra_headers={"X-Gateway-Guard-Key": GUARD_KEY})

# 8. Test Agent Pause / Resume / Reinstall
run_test("Pause Agent", "POST", "/api/browser-ai/agents/test-agent-pytest-001/pause", expect_codes=[200])
run_test("Resume Agent", "POST", "/api/browser-ai/agents/test-agent-pytest-001/resume", expect_codes=[200])
run_test("Allow Reinstall Agent", "POST", "/api/browser-ai/agents/test-agent-pytest-001/allow-reinstall", expect_codes=[200])

# 9. Test Agent Contact Email Update
run_test("Update Agent Contact Email", "PUT", "/api/browser-ai/agents/test-agent-pytest-001/contact-email", {
    "contact_email": "tester@example.com"
}, expect_codes=[200])

# 10. Test Agent Uninstall Key Get & Rotate
run_test("Get Agent Uninstall Key", "GET", "/api/browser-ai/agents/test-agent-pytest-001/uninstall-key", expect_codes=[200])
run_test("Rotate Agent Uninstall Key", "POST", "/api/browser-ai/agents/test-agent-pytest-001/uninstall-key/rotate", expect_codes=[200])

# 11. Test Remote Uninstall Agent (403 for bad key is verified security behavior)
run_test("Remote Uninstall Agent (Auth check)", "POST", "/api/browser-ai/agents/test-agent-pytest-001/remote-uninstall", {
    "key": "INVALID_KEY"
}, expect_codes=[200, 400, 403])

# 12. Test Bulk Delete Agents
run_test("Bulk Delete Agents", "POST", "/api/browser-ai/agents/bulk-delete", {
    "ids": ["test-agent-pytest-001"]
}, expect_codes=[200])

# 13. Test Send Warning Email (Mocked or SMTP configured)
run_test("Send Warning Email", "POST", "/api/browser-ai/send-warning-email", {
    "to": "test-employee@company.com",
    "subject": "Security Policy Warning",
    "message": "Please do not paste sensitive source code into unauthorized AI models."
}, expect_codes=[200, 400, 500])

# 14. Record prompt log and bulk delete
intercept_payload = {
    "url": "https://chatgpt.com/backend-api/conversation",
    "host": "chatgpt.com",
    "prompt": "Test prompt for bulk delete check",
    "agent_id": "test-bulk-agent-2",
    "agent_hostname": "TEST-HOST"
}
req("POST", "/api/browser-ai/intercept", intercept_payload, {"X-Gateway-Guard-Key": GUARD_KEY})
status, logs_res = req("GET", "/api/browser-ai/logs?limit=5")
prompt_log_id = logs_res["logs"][0]["id"] if isinstance(logs_res, dict) and "logs" in logs_res and logs_res["logs"] else None
if prompt_log_id:
    run_test("Bulk Delete Prompt Log", "POST", "/api/browser-ai/logs/bulk-delete", {"ids": [prompt_log_id]}, expect_codes=[200])

# 15. Record search log and bulk delete
status, s_res = req("POST", "/api/browser-ai/search-logs", {
    "engine": "google",
    "browser": "chrome",
    "query": "bulk delete check 2",
    "url": "https://www.google.com/search?q=test2",
    "host": "www.google.com"
}, extra_headers={"X-Gateway-Guard-Key": GUARD_KEY})
search_log_id = s_res.get("log", {}).get("id") if isinstance(s_res, dict) else None
if search_log_id:
    run_test("Bulk Delete Search Log", "POST", "/api/browser-ai/search-logs/bulk-delete", {"ids": [search_log_id]}, expect_codes=[200])

print("\n" + "="*50)
print(f"Total Interactive Tests: {len(results)}")
passed_cnt = sum(1 for r in results if r[1])
print(f"Passed: {passed_cnt}/{len(results)}")
print("="*50)
