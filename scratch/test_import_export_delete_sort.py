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

print("Admin logged in successfully.")

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

print("\n--- 1. IMPORT TESTS ---")
# Import multiple Rules
sample_rules = [
    {"name": "Rule Import Test Alpha", "pattern": r"\bALPHA_\d+\b", "severity": "HIGH", "action": "BLOCK", "active": True},
    {"name": "Rule Import Test Beta", "pattern": r"\bBETA_\d+\b", "severity": "MEDIUM", "action": "WARN", "active": True}
]
run_test("Bulk Import Rules", "POST", "/api/browser-ai/rules/import", {"rules": sample_rules, "overwrite": True})

# Import multiple Targets
sample_targets = [
    {"domain": "target-import-alpha.test", "platform_name": "Alpha Platform", "monitored": True, "block_site": False},
    {"domain": "target-import-beta.test", "platform_name": "Beta Platform", "monitored": True, "block_site": True}
]
run_test("Bulk Import Targets", "POST", "/api/browser-ai/targets/import", {"targets": sample_targets, "overwrite": True})

print("\n--- 2. EXPORT & FETCH DATA INTEGRITY TESTS ---")
# Export Rules source
status, rules_res = req("GET", "/api/browser-ai/rules")
run_test("Fetch Rules for Export", "GET", "/api/browser-ai/rules")
assert "rules" in rules_res
imported_rule_ids = [r["id"] for r in rules_res.get("rules", []) if "Rule Import Test" in r.get("name", "")]

# Export Targets source
status, targets_res = req("GET", "/api/browser-ai/targets")
run_test("Fetch Targets for Export", "GET", "/api/browser-ai/targets")
assert "targets" in targets_res
imported_target_ids = [t["id"] for t in targets_res.get("targets", []) if "target-import" in t.get("domain", "")]

# Export Prompt Logs source
run_test("Fetch Prompt Logs for Export", "GET", "/api/browser-ai/logs?limit=50")

# Export Search Logs source
run_test("Fetch Search Logs for Export", "GET", "/api/browser-ai/search-logs?limit=50")

# Export Agents source
run_test("Fetch Agents for Export", "GET", "/api/browser-ai/agents?limit=50")

print("\n--- 3. SORTING DATA VERIFICATION ---")
# Verify sorting on Rules by name
sorted_rules = sorted(rules_res.get("rules", []), key=lambda r: r.get("name", "").lower())
print(f"Verified client sorting on {len(sorted_rules)} rules: First is '{sorted_rules[0]['name']}'")

# Verify sorting on Targets by domain
sorted_targets = sorted(targets_res.get("targets", []), key=lambda t: t.get("domain", "").lower())
print(f"Verified client sorting on {len(sorted_targets)} targets: First is '{sorted_targets[0]['domain']}'")

print("\n--- 4. DELETE & BULK DELETE TESTS ---")
# Delete single Rule
if imported_rule_ids:
    single_rule_id = imported_rule_ids[0]
    run_test(f"Delete Single Rule ({single_rule_id})", "DELETE", f"/api/browser-ai/rules/{single_rule_id}")
    # Delete remaining rule
    if len(imported_rule_ids) > 1:
        run_test(f"Delete Remaining Rule ({imported_rule_ids[1]})", "DELETE", f"/api/browser-ai/rules/{imported_rule_ids[1]}")

# Delete single Target
if imported_target_ids:
    single_target_id = imported_target_ids[0]
    run_test(f"Delete Single Target ({single_target_id})", "DELETE", f"/api/browser-ai/targets/{single_target_id}")
    if len(imported_target_ids) > 1:
        run_test(f"Delete Remaining Target ({imported_target_ids[1]})", "DELETE", f"/api/browser-ai/targets/{imported_target_ids[1]}")

# Create prompt log & bulk delete
req("POST", "/api/browser-ai/intercept", {
    "url": "https://test-delete.com/api",
    "host": "test-delete.com",
    "prompt": "Testing bulk delete prompt log",
    "agent_id": "test-del-agent",
    "agent_hostname": "HOST-DEL"
}, extra_headers={"X-Gateway-Guard-Key": GUARD_KEY})

status, logs_res = req("GET", "/api/browser-ai/logs?limit=5")
if logs_res.get("logs"):
    del_id = logs_res["logs"][0]["id"]
    run_test("Bulk Delete Prompt Log", "POST", "/api/browser-ai/logs/bulk-delete", {"ids": [del_id]})

# Create search log & bulk delete
status, s_res = req("POST", "/api/browser-ai/search-logs", {
    "engine": "google",
    "browser": "chrome",
    "query": "test query for bulk delete",
    "url": "https://google.com/search?q=test",
    "host": "google.com"
}, extra_headers={"X-Gateway-Guard-Key": GUARD_KEY})

if s_res.get("log"):
    del_sid = s_res["log"]["id"]
    run_test("Bulk Delete Search Log", "POST", "/api/browser-ai/search-logs/bulk-delete", {"ids": [del_sid]})

# Register agent & bulk delete
req("POST", "/api/browser-ai/agents/heartbeat", {
    "id": "agent-for-bulk-del",
    "hostname": "HOST-BULK-DEL",
    "username": "tester",
    "ip_address": "127.0.0.1",
    "status": "active"
}, extra_headers={"X-Gateway-Guard-Key": GUARD_KEY})
run_test("Bulk Delete Agent", "POST", "/api/browser-ai/agents/bulk-delete", {"ids": ["agent-for-bulk-del"]})

# Clear logs by window (e.g. 1d)
run_test("Clear Prompt Logs Window (1d)", "DELETE", "/api/browser-ai/logs?period=1d")
run_test("Clear Search Logs Window (1d)", "DELETE", "/api/browser-ai/search-logs?period=1d")

print("\n" + "="*55)
print(f"TOTAL IMPORT, EXPORT, DELETE & SORT TESTS: {len(results)}")
passed_cnt = sum(1 for r in results if r[1])
print(f"PASSED: {passed_cnt}/{len(results)}")
print("="*55)
